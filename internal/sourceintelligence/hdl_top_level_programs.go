package sourceintelligence

import (
	"regexp"
	"strings"
)

var (
	hdlSVProgramCandidate = regexp.MustCompile(`(?im)^[ \t]*program(?:[ \t;]|$)`)
	hdlSVProgramStart     = regexp.MustCompile(`(?i)^[ \t]*program[ \t]+(?:(?:automatic|static)[ \t]+)?([A-Za-z_][A-Za-z0-9_$]*)`)
	hdlSVProgramEnd       = regexp.MustCompile(`(?i)^[ \t]*endprogram(?:[ \t]*:[ \t]*([A-Za-z_][A-Za-z0-9_$]*))?[ \t]*$`)
	hdlSVProgramEnclosing = regexp.MustCompile(`(?i)^[ \t]*(module|interface|package|checker)[ \t]+[A-Za-z_][A-Za-z0-9_$]*\b`)
)

// hdlSVTopLevelPrograms indexes directly authored top-level program
// declarations without elaborating the testbench or executing its statements.
func hdlSVTopLevelPrograms(builder *SymbolBuilder, document *SourceDocument, masked string) {
	if len(masked) != len(document.Text) || !hdlSVProgramCandidate.MatchString(masked) {
		return
	}
	lines := sourceTextLines(masked)
	nested, directive, candidates := 0, false, 0
	seen := make(map[string]struct{})
	diagnose := func(at int, detail string) {
		addDocumentDataHardwareDiagnostic(builder, "systemverilog-unsupported-program", detail, at, at+1)
	}
	isProgram := func(s string) bool {
		return len(s) >= len("program") &&
			strings.EqualFold(s[:len("program")], "program") &&
			(len(s) == len("program") || s[len("program")] == ' ' ||
				s[len("program")] == '\t' || s[len("program")] == ';')
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
		if match := hdlSVProgramEnclosing.FindStringSubmatch(line.text); len(match) > 1 {
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
		if nested > 0 || !isProgram(trimmed) {
			continue
		}
		begin := line.start + strings.Index(line.text, trimmed)
		candidates++
		if candidates > 128 {
			diagnose(begin, "File exceeds the bounded top-level program candidate count")
			return
		}
		m := hdlSVProgramStart.FindStringSubmatchIndex(line.text)
		if len(m) < 4 {
			diagnose(begin, "Unsupported or malformed top-level program header")
			continue
		}
		nameStart, nameEnd := line.start+m[2], line.start+m[3]
		name := document.Text[nameStart:nameEnd]
		if _, exists := seen[name]; exists {
			diagnose(begin, "Duplicate directly authored top-level program")
			continue
		}
		// A bounded optional parameter list and optional port list precede ';'.
		limit := begin + 4096
		if limit > len(masked) {
			limit = len(masked)
		}
		headerEnd := hdlSVProgramHeaderEnd(masked, nameEnd, limit)
		if headerEnd < 0 {
			diagnose(begin, "Top-level program header is malformed or oversized")
			continue
		}
		startBodyLine := i
		for startBodyLine < len(lines) && lines[startBodyLine].end < headerEnd {
			startBodyLine++
		}
		// Other source code on the terminator's physical line is unsupported.
		if startBodyLine < len(lines) &&
			strings.TrimSpace(masked[headerEnd:lines[startBodyLine].end]) != "" {
			diagnose(begin, "Program header shares its physical line with body text")
			continue
		}
		closeLine, unsafe := -1, false
		for j := startBodyLine + 1; j < len(lines); j++ {
			if lines[j].end-begin > 65536 {
				break
			}
			bodyLine := strings.TrimSpace(lines[j].text)
			if strings.HasPrefix(bodyLine, "`") || isProgram(bodyLine) {
				unsafe = true
				if isProgram(bodyLine) {
					i = j
				}
				break
			}
			if strings.HasPrefix(strings.ToLower(bodyLine), "module ") ||
				strings.HasPrefix(strings.ToLower(bodyLine), "interface ") ||
				strings.HasPrefix(strings.ToLower(bodyLine), "package ") {
				break
			}
			if m := hdlSVProgramEnd.FindStringSubmatch(bodyLine); len(m) > 0 {
				if len(m) > 1 && m[1] != "" && m[1] != name {
					unsafe = true
				} else {
					closeLine = j
				}
				break
			}
		}
		if unsafe || closeLine < 0 {
			diagnose(begin, "Program is unsupported, unclosed or exceeds its bounded span")
			continue
		}
		declaration := OffsetRange{Start: begin, End: lines[closeLine].end}
		signature := OffsetRange{Start: begin, End: headerEnd}
		body := OffsetRange{Start: headerEnd, End: declaration.End}
		addStructuralSymbol(builder, SymbolSpec{
			Kind: SymbolKindModule, NativeKind: "program", Name: name,
			Declaration: declaration, NameRange: OffsetRange{Start: nameStart, End: nameEnd},
			Signature: &signature, Body: &body, Evidence: SymbolEvidenceStructural,
		})
		seen[name] = struct{}{}
		i = closeLine
	}
}

func hdlSVProgramHeaderEnd(source string, cursor, limit int) int {
	skipSpace := func() {
		for cursor < limit && (source[cursor] == ' ' || source[cursor] == '\t' ||
			source[cursor] == '\r' || source[cursor] == '\n') {
			cursor++
		}
	}
	balanced := func() bool {
		if cursor >= limit || source[cursor] != '(' {
			return false
		}
		depth := 0
		for cursor < limit {
			switch source[cursor] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					cursor++
					return true
				}
			case ';', '`':
				return false
			}
			if depth < 0 {
				return false
			}
			cursor++
		}
		return false
	}
	skipSpace()
	if cursor < limit && source[cursor] == '#' {
		cursor++
		skipSpace()
		if !balanced() {
			return -1
		}
		skipSpace()
	}
	if cursor < limit && source[cursor] == '(' {
		if !balanced() {
			return -1
		}
		skipSpace()
	}
	if cursor < limit && source[cursor] == ';' {
		return cursor + 1
	}
	return -1
}
