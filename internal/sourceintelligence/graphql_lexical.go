package sourceintelligence

import "strings"

// graphqlSimpleNamedOperationEnd accepts only a complete, directly braced
// selection after a named operation. Variable definitions, directives, nested
// operations and unbounded bodies retain the existing header-only navigation.
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
	for cursor < len(masked) && cursor-headerEnd <= maxHeaderGap {
		switch masked[cursor] {
		case ' ', '\t', '\r', '\n':
			cursor++
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
