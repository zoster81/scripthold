package sourceintelligence

import (
	"regexp"
	"strings"
)

var (
	hdlSVSequenceHeader    = regexp.MustCompile(`(?i)^[ \t]*sequence[ \t]+([A-Za-z_][A-Za-z0-9_$]*)[ \t]*;[ \t]*$`)
	hdlSVSequenceEnd       = regexp.MustCompile(`(?i)^[ \t]*endsequence(?:[ \t]*:[ \t]*([A-Za-z_][A-Za-z0-9_$]*))?[ \t]*$`)
	hdlSVSequenceCandidate = regexp.MustCompile(`(?im)^[ \t]*sequence(?:[ \t;]|$)`)
)

// hdlSVModuleSequences indexes only complete, directly authored named
// sequence blocks in modules. Temporal expressions are never interpreted.
func hdlSVModuleSequences(builder *SymbolBuilder, document *SourceDocument, masked string, start, end int, parent *SymbolParent) {
	if start < 0 || end > len(masked) || end <= start || len(masked) != len(document.Text) {
		return
	}
	if !hdlSVSequenceCandidate.MatchString(masked[start:end]) {
		return // Preserve the allocation-free path for ordinary modules.
	}
	lines := sourceTextLines(masked[start:end])
	nested, directive, candidates := 0, false, 0
	seen := make(map[string]struct{})
	diagnose := func(begin int, detail string) {
		addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-sequence",
			detail, begin, begin+1)
	}
	isSequence := func(s string) bool {
		return len(s) >= len("sequence") &&
			strings.EqualFold(s[:len("sequence")], "sequence") &&
			(len(s) == len("sequence") || s[len("sequence")] == ' ' || s[len("sequence")] == '\t' || s[len("sequence")] == ';')
	}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line.text)
		if directive {
			directive = strings.HasSuffix(trimmed, "\\")
			continue
		}
		if strings.HasPrefix(trimmed, "`") {
			directive = strings.HasSuffix(trimmed, "\\")
			continue
		}
		boundary := hdlPackageBlockBoundary.FindStringSubmatch(line.text)
		if len(boundary) > 1 {
			switch strings.ToLower(boundary[1]) {
			case "class", "task", "function":
				nested++
			case "endclass", "endtask", "endfunction":
				if nested > 0 {
					nested--
				}
			}
			continue
		}
		if nested > 0 || !isSequence(trimmed) {
			continue
		}
		begin := start + line.start + strings.Index(line.text, trimmed)
		candidates++
		if candidates > 128 {
			diagnose(begin, "Module exceeds the bounded sequence candidate count")
			return
		}
		header := hdlSVSequenceHeader.FindStringSubmatchIndex(line.text)
		if len(header) < 4 || line.end-line.start > 4096 {
			diagnose(begin, "Unsupported or oversized module sequence header")
			continue
		}
		nameStart, nameEnd := start+line.start+header[2], start+line.start+header[3]
		name := document.Text[nameStart:nameEnd]
		if _, exists := seen[name]; exists {
			diagnose(begin, "Duplicate directly authored module sequence")
			continue
		}
		headerEnd := start + line.end
		closing, unsafe := -1, false
		for j := i + 1; j < len(lines); j++ {
			if start+lines[j].end-begin > 65536 {
				break
			}
			body := strings.TrimSpace(lines[j].text)
			if strings.HasPrefix(body, "`") || isSequence(body) {
				unsafe = true
				if isSequence(body) {
					i = j // Do not reconsider an unsupported nested declaration.
				}
				break
			}
			if strings.HasPrefix(strings.ToLower(body), "endmodule") {
				break
			}
			if closeMatch := hdlSVSequenceEnd.FindStringSubmatch(body); len(closeMatch) > 0 {
				if len(closeMatch) > 1 && closeMatch[1] != "" && closeMatch[1] != name {
					unsafe = true
				} else {
					closing = j
				}
				break
			}
		}
		if unsafe || closing < 0 {
			diagnose(begin, "Module sequence is unsupported, unclosed or exceeds its bounded span")
			continue
		}
		declaration := OffsetRange{Start: begin, End: start + lines[closing].end}
		signature := OffsetRange{Start: begin, End: headerEnd}
		body := OffsetRange{Start: headerEnd, End: declaration.End}
		addStructuralSymbol(builder, SymbolSpec{
			Kind: SymbolKindEntity, NativeKind: "sequence", Name: name, Parent: parent,
			Declaration: declaration, NameRange: OffsetRange{Start: nameStart, End: nameEnd},
			Signature: &signature, Body: &body, Evidence: SymbolEvidenceStructural,
		})
		seen[name] = struct{}{}
		i = closing
	}
}
