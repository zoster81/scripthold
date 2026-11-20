package sourceintelligence

import "strings"

// hdlSVPackageCallables indexes directly authored package functions and tasks.
// It does not elaborate macros, resolve types, or interpret callable bodies.
func hdlSVPackageCallables(builder *SymbolBuilder, document *SourceDocument, masked string, start, end int, parent *SymbolParent) {
	if start < 0 || end > len(masked) || end <= start || len(document.Text) != len(masked) {
		return
	}
	lines := sourceTextLines(masked[start:end])
	nested, directive := 0, false
	functionCandidates, taskCandidates := 0, 0
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
		match := hdlPackageBlockBoundary.FindStringSubmatchIndex(line.text)
		if len(match) < 4 {
			continue
		}
		kind := strings.ToLower(line.text[match[2]:match[3]])
		if nested > 0 {
			switch kind {
			case "class", "task":
				nested++
			case "endclass", "endtask":
				nested--
			}
			continue
		}
		switch kind {
		case "class":
			nested++
		case "function", "task":
			functionStart := start + line.start + match[2]
			if kind == "function" {
				functionCandidates++
			} else {
				taskCandidates++
			}
			if functionCandidates > 128 || taskCandidates > 128 {
				addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-package-"+kind, "Package exceeds the bounded callable scan count", functionStart, functionStart+1)
				return
			}
			name, nameRange, signatureEnd, ok := hdlSVCallableSignature(masked, functionStart, end)
			if !ok {
				addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-package-"+kind, "Unsupported or malformed package callable signature", functionStart, functionStart+1)
				if kind == "task" {
					nested = 1 // Keep an invalid task body opaque until endtask.
				}
				continue
			}
			if signatureEnd-functionStart > 4096 {
				addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-package-"+kind, "Package callable header exceeds the supported size", functionStart, functionStart+1)
				if kind == "task" {
					nested = 1
				}
				continue
			}
			signatureLine := i
			for signatureLine < len(lines) && start+lines[signatureLine].end < signatureEnd {
				signatureLine++
			}
			closingLine := -1
			for j := signatureLine + 1; j < len(lines); j++ {
				abs := start + lines[j].start
				if abs-functionStart > 65536 {
					break
				}
				boundary := hdlPackageBlockBoundary.FindStringSubmatch(lines[j].text)
				if len(boundary) < 2 {
					continue
				}
				switch strings.ToLower(boundary[1]) {
				case "endfunction", "endtask":
					if strings.EqualFold(boundary[1], "end"+kind) {
						closingLine = j
					} else {
						j = len(lines)
					}
				case "function", "class", "task", "endclass":
					// Nested or unclosed callable bodies are not safe source ranges.
					j = len(lines)
				}
				if closingLine >= 0 {
					break
				}
			}
			if closingLine < 0 {
				addDocumentDataHardwareDiagnostic(builder, "systemverilog-unclosed-package-"+kind, "Package callable body is unclosed or exceeds its supported span", functionStart, functionStart+1)
				if kind == "task" {
					nested = 1
				}
				continue
			}
			declarationEnd := start + lines[closingLine].end
			if declarationEnd-functionStart > 65536 {
				addDocumentDataHardwareDiagnostic(builder, "systemverilog-unclosed-package-"+kind, "Package callable span exceeds the supported limit", functionStart, functionStart+1)
				if kind == "task" {
					nested = 1
				}
				continue
			}
			signature := OffsetRange{Start: functionStart, End: signatureEnd}
			body := OffsetRange{Start: signatureEnd, End: declarationEnd}
			native := "package-" + kind
			addStructuralSymbol(builder, SymbolSpec{
				Kind: SymbolKindFunction, NativeKind: native, Name: name, Parent: parent,
				Declaration: OffsetRange{Start: functionStart, End: declarationEnd},
				NameRange:   nameRange, Signature: &signature, Body: &body,
				Evidence: SymbolEvidenceStructural,
			})
			i = closingLine
		}
	}
}

// The supported subset requires a direct named declaration with one balanced
// parenthesized argument list and a semicolon, within a bounded source span.
func hdlSVCallableSignature(masked string, begin, end int) (string, OffsetRange, int, bool) {
	const maxHeader = 4096
	if begin < 0 || begin >= end || end > len(masked) {
		return "", OffsetRange{}, 0, false
	}
	limit := begin + maxHeader
	if limit > end {
		limit = end
	}
	header := masked[begin:limit]
	open := strings.IndexByte(header, '(')
	if open < 0 || strings.IndexByte(header[:open], ';') >= 0 || strings.Contains(header[:open], "`") {
		return "", OffsetRange{}, 0, false
	}
	nameEnd := open
	for nameEnd > 0 && (header[nameEnd-1] == ' ' || header[nameEnd-1] == '\t' || header[nameEnd-1] == '\r' || header[nameEnd-1] == '\n') {
		nameEnd--
	}
	nameStart := nameEnd
	for nameStart > 0 {
		c := header[nameStart-1]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '$' {
			nameStart--
			continue
		}
		break
	}
	if nameStart == nameEnd || nameStart == 0 || header[nameStart-1] == '#' || (header[nameStart] >= '0' && header[nameStart] <= '9') {
		return "", OffsetRange{}, 0, false
	}
	name := header[nameStart:nameEnd]
	if name == "function" || name == "task" || name == "automatic" || name == "static" {
		return "", OffsetRange{}, 0, false
	}
	depth, close := 0, -1
	for i := open; i < len(header); i++ {
		switch header[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				close = i
			}
		}
		if depth < 0 || (header[i] == ';' && depth > 0) {
			return "", OffsetRange{}, 0, false
		}
		if close >= 0 {
			break
		}
	}
	if close < 0 {
		return "", OffsetRange{}, 0, false
	}
	semicolon := close + 1
	for semicolon < len(header) && (header[semicolon] == ' ' || header[semicolon] == '\t' || header[semicolon] == '\r' || header[semicolon] == '\n') {
		semicolon++
	}
	if semicolon >= len(header) || header[semicolon] != ';' {
		return "", OffsetRange{}, 0, false
	}
	return name, OffsetRange{Start: begin + nameStart, End: begin + nameEnd}, begin + semicolon + 1, true
}
