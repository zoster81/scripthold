package sourceintelligence

import (
	"regexp"
	"strings"
)

var (
	hdlSVClockingStart = regexp.MustCompile(`(?i)^[ \t]*clocking[ \t]+([A-Za-z_][A-Za-z0-9_$]*)[ \t]*@`)
	hdlSVClockingEnd   = regexp.MustCompile(`(?i)^[ \t]*endclocking(?:[ \t]*:[ \t]*([A-Za-z_][A-Za-z0-9_$]*))?[ \t]*$`)
)

// hdlSVInterfaceClocking indexes directly authored interface clocking blocks
// with complete, bounded source spans. It does not interpret timing semantics.
func hdlSVInterfaceClocking(builder *SymbolBuilder, document *SourceDocument, masked string, start, end int, parent *SymbolParent) {
	if start < 0 || end > len(masked) || end <= start || len(masked) != len(document.Text) {
		return
	}
	lines := sourceTextLines(masked[start:end])
	nested, directive, candidates := 0, false, 0
	seen := make(map[string]struct{})
	diagnose := func(begin int, detail string) {
		addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-clocking",
			detail, begin, begin+1)
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
		if nested > 0 || len(trimmed) < len("clocking") ||
			!strings.EqualFold(trimmed[:len("clocking")], "clocking") {
			continue
		}
		if len(trimmed) > len("clocking") && trimmed[len("clocking")] != ' ' &&
			trimmed[len("clocking")] != '\t' {
			continue
		}
		begin := start + line.start + strings.Index(line.text, trimmed)
		candidates++
		if candidates > 128 {
			diagnose(begin, "Interface exceeds the bounded clocking block candidate count")
			return
		}
		match := hdlSVClockingStart.FindStringSubmatchIndex(line.text)
		if len(match) < 4 {
			diagnose(begin, "Unsupported or malformed interface clocking header")
			continue
		}
		nameStart, nameEnd := start+line.start+match[2], start+line.start+match[3]
		name := document.Text[nameStart:nameEnd]
		if _, exists := seen[name]; exists {
			diagnose(begin, "Duplicate directly authored interface clocking block")
			continue
		}
		// The header must contain one bounded, balanced @(event) followed
		// by a semicolon; only the original masked source is examined.
		headerLimit := begin + 4096
		if headerLimit > end {
			headerLimit = end
		}
		header := masked[begin:headerLimit]
		at := strings.IndexByte(header, '@')
		if at < 0 {
			diagnose(begin, "Clocking header lacks an event control")
			continue
		}
		open := at + 1
		for open < len(header) && (header[open] == ' ' || header[open] == '\t' ||
			header[open] == '\r' || header[open] == '\n') {
			open++
		}
		if open >= len(header) || header[open] != '(' {
			diagnose(begin, "Clocking event control is not parenthesized")
			continue
		}
		depth, close, valid := 0, -1, true
		for j := open; j < len(header); j++ {
			switch header[j] {
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
			if !valid || depth < 0 || close >= 0 {
				break
			}
		}
		headerEnd := -1
		if valid && close > open+1 {
			j := close + 1
			for j < len(header) && (header[j] == ' ' || header[j] == '\t' ||
				header[j] == '\r' || header[j] == '\n') {
				j++
			}
			if j < len(header) && header[j] == ';' {
				headerEnd = begin + j + 1
			}
		}
		if headerEnd < 0 {
			diagnose(begin, "Unsupported, malformed or oversized clocking header")
			continue
		}
		closingLine := -1
		unsafe := false
		for j := i + 1; j < len(lines); j++ {
			if start+lines[j].end-begin > 65536 {
				break
			}
			bodyLine := strings.TrimSpace(lines[j].text)
			nestedClocking := len(bodyLine) >= len("clocking") &&
				strings.EqualFold(bodyLine[:len("clocking")], "clocking") &&
				(len(bodyLine) == len("clocking") || bodyLine[len("clocking")] == ' ' || bodyLine[len("clocking")] == '\t')
			if strings.HasPrefix(bodyLine, "`") || nestedClocking {
				unsafe = true
				if nestedClocking {
					i = j // Do not reconsider the rejected nested declaration.
				}
				break
			}
			if strings.HasPrefix(strings.ToLower(bodyLine), "endinterface") {
				break
			}
			if closeMatch := hdlSVClockingEnd.FindStringSubmatch(bodyLine); len(closeMatch) > 0 {
				if len(closeMatch) > 1 && closeMatch[1] != "" && closeMatch[1] != name {
					unsafe = true
					break
				}
				closingLine = j
				break
			}
		}
		if unsafe || closingLine < 0 {
			diagnose(begin, "Clocking block is unclosed, oversized or contains unsupported directives")
			continue
		}
		declaration := OffsetRange{Start: begin, End: start + lines[closingLine].end}
		signature := OffsetRange{Start: begin, End: headerEnd}
		body := OffsetRange{Start: headerEnd, End: declaration.End}
		addStructuralSymbol(builder, SymbolSpec{
			Kind: SymbolKindEntity, NativeKind: "clocking-block", Name: name, Parent: parent,
			Declaration: declaration, NameRange: OffsetRange{Start: nameStart, End: nameEnd},
			Signature: &signature, Body: &body, Evidence: SymbolEvidenceStructural,
		})
		seen[name] = struct{}{}
		i = closingLine
	}
}
