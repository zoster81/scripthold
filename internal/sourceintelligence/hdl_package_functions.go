package sourceintelligence

import "strings"

// hdlSVPackageFunctions indexes directly authored package functions only.
// It does not elaborate macros, resolve types, or interpret function bodies.
func hdlSVPackageFunctions(builder *SymbolBuilder, document *SourceDocument, masked string, start, end int, parent *SymbolParent) {
	if start < 0 || end > len(masked) || end <= start || len(document.Text) != len(masked) {
		return
	}
	lines := sourceTextLines(masked[start:end])
	nested, directive := 0, false
	functionCandidates := 0
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
		match := hdlPackageBlockBoundary.FindStringSubmatch(line.text)
		if len(match) < 2 {
			continue
		}
		kind := strings.ToLower(match[1])
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
		case "class", "task":
			nested++
		case "function":
			keyword := strings.Index(line.text, "function")
			if keyword < 0 {
				continue
			}
			functionStart := start + line.start + keyword
			functionCandidates++
			if functionCandidates > 128 {
				addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-package-function", "Package exceeds the bounded function scan count", functionStart, functionStart+1)
				return
			}
			name, nameRange, signatureEnd, ok := hdlSVFunctionSignature(masked, functionStart, end)
			if !ok {
				addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-package-function", "Unsupported or malformed package function signature", functionStart, functionStart+1)
				continue
			}
			if signatureEnd-functionStart > 4096 {
				addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-package-function", "Package function header exceeds the supported size", functionStart, functionStart+1)
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
				case "endfunction":
					closingLine = j
				case "function", "class", "task", "endclass", "endtask":
					// A nested or unclosed function is not a safe source range.
					j = len(lines)
				}
				if closingLine >= 0 {
					break
				}
			}
			if closingLine < 0 {
				addDocumentDataHardwareDiagnostic(builder, "systemverilog-unclosed-package-function", "Package function body is unclosed or exceeds its supported span", functionStart, functionStart+1)
				continue
			}
			declarationEnd := start + lines[closingLine].end
			if declarationEnd-functionStart > 65536 {
				addDocumentDataHardwareDiagnostic(builder, "systemverilog-unclosed-package-function", "Package function span exceeds the supported limit", functionStart, functionStart+1)
				continue
			}
			signature := OffsetRange{Start: functionStart, End: signatureEnd}
			body := OffsetRange{Start: signatureEnd, End: declarationEnd}
			addStructuralSymbol(builder, SymbolSpec{
				Kind: SymbolKindFunction, NativeKind: "package-function", Name: name, Parent: parent,
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
func hdlSVFunctionSignature(masked string, begin, end int) (string, OffsetRange, int, bool) {
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
	if name == "function" || name == "automatic" || name == "static" {
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
