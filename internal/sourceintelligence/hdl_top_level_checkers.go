package sourceintelligence

import (
	"regexp"
	"strings"
)

var (
	hdlSVCheckerCandidate = regexp.MustCompile(`(?im)^[ \t]*checker(?:[ \t;]|$)`)
	hdlSVCheckerStart     = regexp.MustCompile(`(?i)^[ \t]*checker[ \t]+([A-Za-z_][A-Za-z0-9_$]*)`)
	hdlSVCheckerEnd       = regexp.MustCompile(`(?i)^[ \t]*endchecker(?:[ \t]*:[ \t]*([A-Za-z_][A-Za-z0-9_$]*))?[ \t]*$`)
	hdlSVCheckerEnclosing = regexp.MustCompile(`(?i)^[ \t]*(module|interface|package)[ \t]+[A-Za-z_][A-Za-z0-9_$]*\b`)
)

// hdlSVTopLevelCheckers indexes directly authored top-level checker
// declarations. It does not elaborate bindings or evaluate assertions.
func hdlSVTopLevelCheckers(builder *SymbolBuilder, document *SourceDocument, masked string) {
	if len(masked) != len(document.Text) || !hdlSVCheckerCandidate.MatchString(masked) {
		return
	}
	lines := sourceTextLines(masked)
	nested, directive, candidates := 0, false, 0
	seen := make(map[string]struct{})
	diagnose := func(at int, detail string) {
		addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-checker", detail, at, at+1)
	}
	isChecker := func(s string) bool {
		return len(s) >= len("checker") &&
			strings.EqualFold(s[:len("checker")], "checker") &&
			(len(s) == len("checker") || s[len("checker")] == ' ' ||
				s[len("checker")] == '\t' || s[len("checker")] == ';')
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
		if match := hdlSVCheckerEnclosing.FindStringSubmatch(line.text); len(match) > 1 {
			kind := strings.ToLower(match[1])
			end := hdlEnd(masked, line.end, "end"+kind)
			if end < 0 {
				// An unclosed container cannot establish top-level ownership.
				return
			}
			for i+1 < len(lines) && lines[i+1].start < end {
				i++
			}
			continue
		}
		if match := hdlPackageBlockBoundary.FindStringSubmatch(line.text); len(match) > 1 {
			switch strings.ToLower(match[1]) {
			case "class", "task", "function":
				nested++
			case "endclass", "endtask", "endfunction":
				if nested > 0 {
					nested--
				}
			}
			continue
		}
		if nested > 0 || !isChecker(trimmed) {
			continue
		}
		begin := line.start + strings.Index(line.text, trimmed)
		candidates++
		if candidates > 128 {
			diagnose(begin, "File exceeds the bounded top-level checker candidate count")
			return
		}
		m := hdlSVCheckerStart.FindStringSubmatchIndex(line.text)
		if len(m) < 4 {
			diagnose(begin, "Unsupported or malformed top-level checker header")
			continue
		}
		nameStart, nameEnd := line.start+m[2], line.start+m[3]
		name := document.Text[nameStart:nameEnd]
		if _, exists := seen[name]; exists {
			diagnose(begin, "Duplicate directly authored top-level checker")
			continue
		}
		// Recognize a bare semicolon or a single balanced ANSI argument list.
		limit := begin + 4096
		if limit > len(masked) {
			limit = len(masked)
		}
		cursor := nameEnd
		for cursor < limit && (masked[cursor] == ' ' || masked[cursor] == '\t' ||
			masked[cursor] == '\r' || masked[cursor] == '\n') {
			cursor++
		}
		headerEnd := -1
		if cursor < limit && masked[cursor] == ';' {
			headerEnd = cursor + 1
		} else if cursor < limit && masked[cursor] == '(' {
			depth := 0
			closed := false
			for ; cursor < limit; cursor++ {
				switch masked[cursor] {
				case '(':
					depth++
				case ')':
					depth--
					if depth == 0 {
						closed = true
					}
				case ';', '`':
					if depth > 0 || masked[cursor] == '`' {
						closed = false
						cursor = limit
					}
				}
				if cursor >= limit || depth < 0 || closed {
					break
				}
			}
			if closed && depth == 0 {
				cursor++
				for cursor < limit && (masked[cursor] == ' ' || masked[cursor] == '\t' ||
					masked[cursor] == '\r' || masked[cursor] == '\n') {
					cursor++
				}
				if cursor < limit && masked[cursor] == ';' {
					headerEnd = cursor + 1
				}
			}
		}
		if headerEnd < 0 {
			diagnose(begin, "Top-level checker header is malformed or oversized")
			continue
		}
		startBodyLine := i
		for startBodyLine < len(lines) && lines[startBodyLine].end < headerEnd {
			startBodyLine++
		}
		// Other source code on the terminator's physical line is unsupported.
		if startBodyLine < len(lines) &&
			strings.TrimSpace(masked[headerEnd:lines[startBodyLine].end]) != "" {
			diagnose(begin, "Checker header shares its physical line with body text")
			continue
		}
		closeLine, unsafe := -1, false
		for j := startBodyLine + 1; j < len(lines); j++ {
			if lines[j].end-begin > 65536 {
				break
			}
			bodyLine := strings.TrimSpace(lines[j].text)
			if strings.HasPrefix(bodyLine, "`") || isChecker(bodyLine) {
				unsafe = true
				if isChecker(bodyLine) {
					i = j
				}
				break
			}
			if strings.HasPrefix(strings.ToLower(bodyLine), "module ") ||
				strings.HasPrefix(strings.ToLower(bodyLine), "interface ") ||
				strings.HasPrefix(strings.ToLower(bodyLine), "package ") {
				break
			}
			if m := hdlSVCheckerEnd.FindStringSubmatch(bodyLine); len(m) > 0 {
				if len(m) > 1 && m[1] != "" && m[1] != name {
					unsafe = true
				} else {
					closeLine = j
				}
				break
			}
		}
		if unsafe || closeLine < 0 {
			diagnose(begin, "Checker is unsupported, unclosed or exceeds its bounded span")
			continue
		}
		declaration := OffsetRange{Start: begin, End: lines[closeLine].end}
		signature := OffsetRange{Start: begin, End: headerEnd}
		body := OffsetRange{Start: headerEnd, End: declaration.End}
		addStructuralSymbol(builder, SymbolSpec{
			Kind: SymbolKindType, NativeKind: "checker", Name: name,
			Declaration: declaration, NameRange: OffsetRange{Start: nameStart, End: nameEnd},
			Signature: &signature, Body: &body, Evidence: SymbolEvidenceStructural,
		})
		seen[name] = struct{}{}
		i = closeLine
	}
}
