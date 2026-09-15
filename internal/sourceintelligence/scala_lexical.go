package sourceintelligence

import "strings"

func maskScalaTripleQuotedStrings(text string) string {
	masked := []byte(text)
	for at := 0; at < len(text); {
		switch {
		case strings.HasPrefix(text[at:], "//"):
			if end := strings.IndexByte(text[at:], '\n'); end >= 0 {
				at += end + 1
			} else {
				at = len(text)
			}
			continue
		case strings.HasPrefix(text[at:], "/*"):
			end, ok := scalaNestedBlockCommentEnd(text, at)
			if !ok {
				return string(masked)
			}
			at = end
			continue
		case strings.HasPrefix(text[at:], `"""`) && (at == 0 || text[at-1] != '"'):
			cursor := at + 3
			closed := false
			for cursor+2 < len(text) {
				if text[cursor] != '"' {
					cursor++
					continue
				}
				runEnd := cursor
				for runEnd < len(text) && text[runEnd] == '"' {
					runEnd++
				}
				if runEnd-cursor >= 3 {
					maskRangePreservingLines(masked, at, runEnd)
					at = runEnd
					closed = true
					break
				}
				cursor = runEnd
			}
			if !closed {
				at += 3
			}
			continue
		case text[at] == '"':
			if end, ok := scalaQuotedLiteralEnd(text, at, '"'); ok {
				at = end
				continue
			}
		case text[at] == '\'':
			if end, ok := scalaQuotedLiteralEnd(text, at, '\''); ok {
				at = end
				continue
			}
		}
		at++
	}
	return string(masked)
}

func maskScalaInterpolatedStrings(text string) (string, []OffsetRange) {
	masked := []byte(text)
	var interpolationExpressions []OffsetRange
	for at := 0; at < len(text); {
		switch {
		case strings.HasPrefix(text[at:], "//"):
			if end := strings.IndexByte(text[at:], '\n'); end >= 0 {
				at += end + 1
			} else {
				at = len(text)
			}
			continue
		case strings.HasPrefix(text[at:], "/*"):
			end, ok := scalaNestedBlockCommentEnd(text, at)
			if !ok {
				return string(masked), interpolationExpressions
			}
			at = end
			continue
		case text[at] == '"':
			end, ok := scalaQuotedLiteralEnd(text, at, '"')
			if !ok {
				at++
				continue
			}
			at = end
			continue
		case text[at] == '\'':
			if end, ok := scalaQuotedLiteralEnd(text, at, '\''); ok {
				at = end
				continue
			}
			at++
			continue
		}

		prefixLen := scalaInterpolatorPrefixLength(text, at)
		if prefixLen == 0 {
			at++
			continue
		}
		quote := at + prefixLen
		end, expressions, ok := scalaInterpolatedStringLayout(text, quote)
		if !ok {
			at = quote + 1
			continue
		}
		cursor := at
		for _, expression := range expressions {
			maskRangePreservingLines(masked, cursor, expression.Start)
			interpolationExpressions = append(interpolationExpressions, expression)
			cursor = expression.End
		}
		maskRangePreservingLines(masked, cursor, end)
		at = end
	}
	return string(masked), interpolationExpressions
}
func scalaOffsetInRanges(offset int, ranges []OffsetRange) bool {
	left, right := 0, len(ranges)
	for left < right {
		middle := left + (right-left)/2
		value := ranges[middle]
		if offset < value.Start {
			right = middle
		} else if offset >= value.End {
			left = middle + 1
		} else {
			return true
		}
	}
	return false
}

func scalaInterpolatorPrefixLength(text string, at int) int {
	if at < 0 || at >= len(text) || !isScalaIdentifierStartByte(text[at]) || at > 0 && isScalaSymbolIdentifierByte(text[at-1]) {
		return 0
	}
	end := at + 1
	for end < len(text) && isScalaSymbolIdentifierByte(text[end]) {
		end++
	}
	if end >= len(text) || text[end] != '"' {
		return 0
	}
	return end - at
}

func isScalaIdentifierStartByte(value byte) bool {
	return value == '_' || value == '$' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}
func scalaInterpolatedStringLayout(text string, quote int) (int, []OffsetRange, bool) {
	if quote < 0 || quote >= len(text) || text[quote] != '"' || quote+2 < len(text) && text[quote:quote+3] == `"""` {
		return 0, nil, false
	}
	var expressions []OffsetRange
	for cursor := quote + 1; cursor < len(text); {
		switch text[cursor] {
		case '\\':
			if cursor+1 < len(text) {
				cursor += 2
			} else {
				cursor++
			}
		case '"':
			return cursor + 1, expressions, true
		case '$':
			if cursor+1 < len(text) && text[cursor+1] == '{' {
				end, ok := scalaInterpolationExpressionEnd(text, cursor+1)
				if !ok {
					return 0, nil, false
				}
				expressions = append(expressions, OffsetRange{Start: cursor + 1, End: end})
				cursor = end
				continue
			}
			cursor++
		default:
			cursor++
		}
	}
	return 0, nil, false
}

func scalaInterpolationExpressionEnd(text string, open int) (int, bool) {
	if open < 0 || open >= len(text) || text[open] != '{' {
		return 0, false
	}
	depth := 1
	for cursor := open + 1; cursor < len(text); {
		if strings.HasPrefix(text[cursor:], `"""`) {
			end := strings.Index(text[cursor+3:], `"""`)
			if end < 0 {
				return 0, false
			}
			cursor += 3 + end + 3
			continue
		}
		if strings.HasPrefix(text[cursor:], "//") {
			if end := strings.IndexByte(text[cursor+2:], '\n'); end >= 0 {
				cursor += 2 + end + 1
			} else {
				return 0, false
			}
			continue
		}
		if strings.HasPrefix(text[cursor:], "/*") {
			end, ok := scalaNestedBlockCommentEnd(text, cursor)
			if !ok {
				return 0, false
			}
			cursor = end
			continue
		}
		switch text[cursor] {
		case '"':
			end, ok := scalaQuotedLiteralEnd(text, cursor, '"')
			if !ok {
				return 0, false
			}
			cursor = end
		case '\'':
			if end, ok := scalaQuotedLiteralEnd(text, cursor, '\''); ok {
				cursor = end
			} else {
				cursor++
			}
		case '{':
			depth++
			cursor++
		case '}':
			depth--
			cursor++
			if depth == 0 {
				return cursor, true
			}
		default:
			cursor++
		}
	}
	return 0, false
}

func scalaQuotedLiteralEnd(text string, quote int, delimiter byte) (int, bool) {
	for cursor := quote + 1; cursor < len(text); cursor++ {
		if text[cursor] == '\\' && cursor+1 < len(text) {
			cursor++
			continue
		}
		if text[cursor] == delimiter {
			return cursor + 1, true
		}
		if text[cursor] == '\r' || text[cursor] == '\n' {
			return 0, false
		}
	}
	return 0, false
}

func scalaNestedBlockCommentEnd(text string, start int) (int, bool) {
	depth := 1
	for cursor := start + 2; cursor < len(text); {
		switch {
		case strings.HasPrefix(text[cursor:], "/*"):
			depth++
			cursor += 2
		case strings.HasPrefix(text[cursor:], "*/"):
			depth--
			cursor += 2
			if depth == 0 {
				return cursor, true
			}
		default:
			cursor++
		}
	}
	return 0, false
}

func scalaPhysicalIndent(text string, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	lineStart := offset
	for lineStart > 0 && text[lineStart-1] != '\n' && text[lineStart-1] != '\r' {
		lineStart--
	}
	columns := 0
	for lineStart < offset {
		switch text[lineStart] {
		case ' ':
			columns++
		case '\t':
			columns += 8 - columns%8
		case '\f':
			columns = 0
		default:
			return columns
		}
		lineStart++
	}
	return columns
}

func maskScalaSymbolLiterals(text string) string {
	masked := []byte(text)
	for at := 0; at < len(text); at++ {
		if text[at] != '\'' || at+1 >= len(text) || !isScalaSymbolIdentifierByte(text[at+1]) {
			continue
		}
		end := at + 2
		for end < len(text) && isScalaSymbolIdentifierByte(text[end]) {
			end++
		}
		if end < len(text) && text[end] == '\'' {
			continue
		}
		maskRangePreservingLines(masked, at, end)
		at = end - 1
	}
	return string(masked)
}

func isScalaSymbolIdentifierByte(value byte) bool {
	return value == '_' || value == '$' || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func scalaTokenEqual(token Token, value string) bool { return token.Text == value }

func scalaLineHasTokenAtNesting(tokens []Token, value string, nesting int) bool {
	for _, token := range tokens {
		if token.Text == value && token.Nesting == nesting {
			return true
		}
	}
	return false
}

func scalaHeaderEnd(tokens []Token, start int) int {
	end := len(tokens)
	for index := start; index < len(tokens); index++ {
		switch tokens[index].Text {
		case ":", "{", "=":
			return index
		}
	}
	return end
}

// ScalaScannerProfile covers the declaration-oriented Scala 2/3 lexical subset
// used by the Scala recognizer. Newlines remain visible even inside braces so
// brace-owned and indentation-owned declarations can share one logical-line pass.
func ScalaScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "scala",
		Keywords: []string{
			"abstract", "case", "class", "def", "derives", "enum", "export", "extends", "final", "given", "implicit", "import", "inline", "lazy", "object", "opaque", "open", "override", "package", "private", "protected", "sealed", "trait", "transparent", "type", "val", "var", "with",
		},
		Identifier:   DefaultIdentifierPolicy(),
		LineComments: []string{"//"},
		BlockComments: []BlockCommentRule{
			{Start: "/*", End: "*/", Nestable: true},
		},
		Strings: []StringRule{
			{Prefixes: []string{"s", "f", "raw", ""}, Delimiter: "\"\"\"", Multiline: true, BackslashEscapes: true},
			{Prefixes: []string{"s", "f", "raw", ""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
		Indentation: true,
	}
}
