package sourceintelligence

import "strings"

func nextStructuralToken(tokens []Token, index, end int) int {
	for index < end {
		switch tokens[index].Kind {
		case TokenNewline, TokenDirective, TokenString:
			index++
		default:
			return index
		}
	}
	return end
}

func previousStructuralToken(tokens []Token, index, start int) int {
	for index >= start {
		switch tokens[index].Kind {
		case TokenNewline, TokenDirective, TokenString:
			index--
		default:
			return index
		}
	}
	return -1
}

func nextIdentifierToken(tokens []Token, index, end int) int {
	for index < end {
		if tokens[index].Kind == TokenIdentifier {
			return index
		}
		index++
	}
	return -1
}

func previousIdentifierToken(tokens []Token, index, start int) int {
	for index >= start {
		if tokens[index].Kind == TokenIdentifier {
			return index
		}
		index--
	}
	return -1
}

func tokenRangeText(tokens []Token, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(tokens) {
		end = len(tokens)
	}
	if start >= end {
		return ""
	}
	var builder strings.Builder
	for index := start; index < end; index++ {
		token := tokens[index]
		if token.Kind == TokenNewline || token.Kind == TokenDirective || token.Kind == TokenString || token.Kind == TokenEOF {
			continue
		}
		builder.WriteString(token.Text)
	}
	return builder.String()
}

func splitTokenRangeAt(tokens []Token, start, end int, separator string, nesting int) [][2]int {
	var result [][2]int
	partStart := start
	for index := start; index < end; index++ {
		if tokens[index].Text == separator && tokens[index].Nesting == nesting {
			if partStart < index {
				result = append(result, [2]int{partStart, index})
			}
			partStart = index + 1
		}
	}
	if partStart < end {
		result = append(result, [2]int{partStart, end})
	}
	return result
}

func normalizedTypeSpelling(tokens []Token, start, end int, drop map[string]struct{}) string {
	for start < end {
		if _, ok := drop[strings.ToLower(tokens[start].Text)]; ok {
			start++
			continue
		}
		break
	}
	for end > start {
		if _, ok := drop[strings.ToLower(tokens[end-1].Text)]; ok {
			end--
			continue
		}
		break
	}
	return strings.TrimSpace(tokenRangeText(tokens, start, end))
}

func visibilityFromModifiers(modifiers []string) Visibility {
	for _, modifier := range modifiers {
		switch strings.ToLower(modifier) {
		case "public":
			return VisibilityPublic
		case "private":
			return VisibilityPrivate
		case "protected":
			return VisibilityProtected
		case "internal":
			return VisibilityInternal
		}
	}
	return ""
}

func collectKnownModifiers(tokens []Token, start, end int, known map[string]struct{}) []string {
	var result []string
	for index := start; index < end; index++ {
		value := strings.ToLower(tokens[index].Text)
		if _, ok := known[value]; ok {
			result = append(result, value)
		}
	}
	return result
}
