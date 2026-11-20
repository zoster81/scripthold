package sourceintelligence

import (
	"regexp"
	"strings"
)

var (
	hdlSVCovergroupHeader    = regexp.MustCompile(`(?i)^[ \t]*covergroup[ \t]+([A-Za-z_][A-Za-z0-9_$]*)`)
	hdlSVCovergroupEnd       = regexp.MustCompile(`(?i)^[ \t]*endgroup(?:[ \t]*:[ \t]*([A-Za-z_][A-Za-z0-9_$]*))?[ \t]*$`)
	hdlSVCovergroupCandidate = regexp.MustCompile(`(?im)^[ \t]*covergroup(?:[ \t;]|$)`)
)

// hdlSVModuleCovergroups indexes only complete, directly authored named
// covergroup blocks in modules. Coverage sampling semantics are not interpreted.
func hdlSVModuleCovergroups(builder *SymbolBuilder, document *SourceDocument, masked string, start, end int, parent *SymbolParent) {
	if start < 0 || end > len(masked) || end <= start || len(masked) != len(document.Text) {
		return
	}
	if !hdlSVCovergroupCandidate.MatchString(masked[start:end]) {
		return // Preserve the allocation-free path for ordinary modules.
	}
	lines := sourceTextLines(masked[start:end])
	nested, directive, candidates := 0, false, 0
	seen := make(map[string]struct{})
	diagnose := func(begin int, detail string) {
		addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-covergroup",
			detail, begin, begin+1)
	}
	isCovergroup := func(s string) bool {
		return len(s) >= len("covergroup") &&
			strings.EqualFold(s[:len("covergroup")], "covergroup") &&
			(len(s) == len("covergroup") || s[len("covergroup")] == ' ' || s[len("covergroup")] == '\t' || s[len("covergroup")] == ';')
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
		if nested > 0 || !isCovergroup(trimmed) {
			continue
		}
		begin := start + line.start + strings.Index(line.text, trimmed)
		candidates++
		if candidates > 128 {
			diagnose(begin, "Module exceeds the bounded covergroup candidate count")
			return
		}
		header := hdlSVCovergroupHeader.FindStringSubmatchIndex(line.text)
		if len(header) < 4 || line.end-line.start > 4096 || !hdlSVCovergroupHeaderValid(line.text[header[1]:]) {
			diagnose(begin, "Unsupported or oversized module covergroup header")
			continue
		}
		nameStart, nameEnd := start+line.start+header[2], start+line.start+header[3]
		name := document.Text[nameStart:nameEnd]
		if _, exists := seen[name]; exists {
			diagnose(begin, "Duplicate directly authored module covergroup")
			continue
		}
		headerEnd := start + line.end
		closing, unsafe := -1, false
		for j := i + 1; j < len(lines); j++ {
			if start+lines[j].end-begin > 65536 {
				break
			}
			body := strings.TrimSpace(lines[j].text)
			if strings.HasPrefix(body, "`") || isCovergroup(body) {
				unsafe = true
				if isCovergroup(body) {
					i = j // Do not reconsider an unsupported nested declaration.
				}
				break
			}
			if strings.HasPrefix(strings.ToLower(body), "endmodule") {
				break
			}
			if closeMatch := hdlSVCovergroupEnd.FindStringSubmatch(body); len(closeMatch) > 0 {
				if len(closeMatch) > 1 && closeMatch[1] != "" && closeMatch[1] != name {
					unsafe = true
				} else {
					closing = j
				}
				break
			}
		}
		if unsafe || closing < 0 {
			diagnose(begin, "Module covergroup is unsupported, unclosed or exceeds its bounded span")
			continue
		}
		declaration := OffsetRange{Start: begin, End: start + lines[closing].end}
		signature := OffsetRange{Start: begin, End: headerEnd}
		body := OffsetRange{Start: headerEnd, End: declaration.End}
		addStructuralSymbol(builder, SymbolSpec{
			Kind: SymbolKindType, NativeKind: "covergroup", Name: name, Parent: parent,
			Declaration: declaration, NameRange: OffsetRange{Start: nameStart, End: nameEnd},
			Signature: &signature, Body: &body, Evidence: SymbolEvidenceStructural,
		})
		seen[name] = struct{}{}
		i = closing
	}
}

// hdlSVCovergroupHeaderValid accepts only a semicolon or one complete
// parenthesized event control followed by a semicolon.
func hdlSVCovergroupHeaderValid(tail string) bool {
	tail = strings.TrimSpace(tail)
	if tail == ";" {
		return true
	}
	if len(tail) < 4 || tail[0] != '@' {
		return false
	}
	tail = strings.TrimSpace(tail[1:])
	if len(tail) < 3 || tail[0] != '(' {
		return false
	}
	depth, close := 0, -1
	for i := 0; i < len(tail); i++ {
		switch tail[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				close = i
			}
		case ';', '`':
			if depth > 0 || tail[i] == '`' {
				return false
			}
		}
		if depth < 0 || close >= 0 {
			break
		}
	}
	return close > 1 && strings.TrimSpace(tail[close+1:]) == ";"
}
