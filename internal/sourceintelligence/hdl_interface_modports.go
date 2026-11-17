package sourceintelligence

import (
	"regexp"
	"strings"
)

var hdlSVModportStart = regexp.MustCompile(`(?i)^[ \t]*modport[ \t]+([A-Za-z_][A-Za-z0-9_$]*)[ \t]*\(`)

// hdlSVInterfaceModports indexes only directly authored, bounded interface
// modport declarations. Directions and referenced signals are not interpreted.
func hdlSVInterfaceModports(builder *SymbolBuilder, document *SourceDocument, masked string, start, end int, parent *SymbolParent) {
	if start < 0 || end > len(masked) || end <= start || len(masked) != len(document.Text) {
		return
	}
	lines := sourceTextLines(masked[start:end])
	nested, directive, candidates := 0, false, 0
	seen := make(map[string]struct{})
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
			case "class", "function", "task":
				nested++
			case "endclass", "endfunction", "endtask":
				if nested > 0 {
					nested--
				}
			}
			continue
		}
		if nested > 0 {
			continue
		}
		if len(trimmed) < len("modport") || !strings.EqualFold(trimmed[:len("modport")], "modport") {
			continue
		}
		if len(trimmed) > len("modport") && trimmed[len("modport")] != ' ' && trimmed[len("modport")] != '\t' {
			continue
		}
		begin := start + line.start + strings.Index(line.text, trimmed)
		candidates++
		if candidates > 128 {
			addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-modport",
				"Interface exceeds the bounded modport candidate count", begin, begin+1)
			return
		}
		match := hdlSVModportStart.FindStringSubmatchIndex(line.text)
		if len(match) < 4 {
			addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-modport",
				"Unsupported or malformed interface modport declaration", begin, begin+1)
			continue
		}
		name := document.Text[start+line.start+match[2] : start+line.start+match[3]]
		if _, exists := seen[name]; exists {
			addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-modport",
				"Duplicate directly authored interface modport", begin, begin+1)
			continue
		}
		// Parse only a complete, parenthesized declaration within 4 KiB.
		limit := begin + 4096
		if limit > end {
			limit = end
		}
		if limit < begin {
			return
		}
		span := masked[begin:limit]
		open := strings.IndexByte(span, '(')
		if open < 0 {
			addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-modport",
				"Interface modport has no argument list", begin, begin+1)
			continue
		}
		depth, close := 0, -1
		valid := true
		for j := open; j < len(span); j++ {
			switch span[j] {
			case '`':
				valid = false
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					close = j
				}
			case ';':
				if depth > 0 {
					valid = false
				}
			}
			if depth < 0 || !valid || close >= 0 {
				break
			}
		}
		finish := -1
		if valid && close >= 0 {
			j := close + 1
			for j < len(span) && (span[j] == ' ' || span[j] == '\t' || span[j] == '\r' || span[j] == '\n') {
				j++
			}
			if j < len(span) && span[j] == ';' {
				finish = begin + j + 1
			}
		}
		if finish < 0 {
			addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-modport",
				"Unclosed, oversized or unsupported interface modport declaration", begin, begin+1)
			continue
		}
		nameRange := OffsetRange{Start: start + line.start + match[2], End: start + line.start + match[3]}
		declaration := OffsetRange{Start: begin, End: finish}
		addStructuralSymbol(builder, SymbolSpec{
			Kind: SymbolKindType, NativeKind: "modport", Name: name, Parent: parent,
			Declaration: declaration, NameRange: nameRange,
			Signature: &declaration, Evidence: SymbolEvidenceStructural,
		})
		seen[name] = struct{}{}
		for i+1 < len(lines) && start+lines[i].end < finish {
			i++
		}
	}
}
