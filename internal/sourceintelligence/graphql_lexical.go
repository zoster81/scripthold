package sourceintelligence

import (
	"regexp"
	"strings"
)

var graphqlSimpleVariableDefinition = regexp.MustCompile(`^\s*\$[A-Za-z_][A-Za-z0-9_]*\s*:\s*(?:[A-Za-z_][A-Za-z0-9_]*|\[\s*[A-Za-z_][A-Za-z0-9_]*!?\s*\])!?\s*(?:=\s*(?:[A-Za-z_][A-Za-z0-9_]*|-?[0-9]+(?:\.[0-9]+)?))?\s*$`)

// graphqlSimpleVariablesEnd accepts a small lexical signature consisting of
// directly authored $name: Type references with optional scalar/enum defaults.
// Nested defaults, directive calls, strings and malformed variable definitions
// remain outside the supported navigation subset.
func graphqlSimpleVariablesEnd(masked string, open, maxEnd int) (int, bool) {
	if open >= len(masked) || masked[open] != '(' {
		return 0, false
	}
	limit := min(len(masked), maxEnd)
	for end := open + 1; end < limit; end++ {
		switch masked[end] {
		case ')':
			items := strings.Split(masked[open+1:end], ",")
			if len(items) == 0 || len(items) > 16 {
				return 0, false
			}
			for _, item := range items {
				if !graphqlSimpleVariableDefinition.MatchString(item) {
					return 0, false
				}
			}
			return end + 1, true
		case '(', '{', '}', '"', '\'', '@':
			return 0, false
		}
	}
	return 0, false
}

// graphqlSimpleNamedOperationEnd accepts only a complete, directly braced
// selection after a named operation, with an optional proven small variable
// signature. Directives, nested defaults and unbounded bodies retain the
// existing header-only navigation.
func graphqlSimpleNamedOperationEnd(masked string, headerStart, headerEnd int) (int, bool) {
	if headerStart < 0 || headerEnd <= headerStart || headerEnd > len(masked) {
		return 0, false
	}
	const maxHeaderGap, maxDeclarationBytes, maxDeclarationLines = 256, 16384, 128
	lineStart := headerStart
	for lineStart > 0 && masked[lineStart-1] != '\n' {
		lineStart--
	}
	for _, ch := range masked[lineStart:headerStart] {
		if ch != ' ' && ch != '\t' {
			return 0, false
		}
	}
	cursor := headerEnd
	variablesParsed := false
	for cursor < len(masked) && cursor-headerEnd <= maxHeaderGap {
		switch masked[cursor] {
		case ' ', '\t', '\r', '\n':
			cursor++
		case '(':
			if variablesParsed {
				return 0, false
			}
			end, ok := graphqlSimpleVariablesEnd(masked, cursor, min(len(masked), headerEnd+512))
			if !ok {
				return 0, false
			}
			variablesParsed = true
			cursor = end
		case '{':
			limit := min(len(masked), headerStart+maxDeclarationBytes)
			if cursor >= limit {
				return 0, false
			}
			close := matchingBraceOffset(masked[:limit], cursor)
			if close < 0 || strings.Count(masked[headerStart:close+1], "\n") > maxDeclarationLines {
				return 0, false
			}
			return close + 1, true
		default:
			return 0, false
		}
	}
	return 0, false
}

func maskGraphQLSource(text string) string {
	masked := maskSourceStrings(text, false, true, true)
	return maskSourceComments(masked, []string{"#"}, "", "")
}
