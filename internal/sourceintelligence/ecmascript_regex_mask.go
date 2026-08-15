package sourceintelligence

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maskECMAScriptRegexLiterals hides complete regular-expression literals before
// the shared delimiter scanner runs. Recognition is deliberately conservative:
// a slash is treated as a regex opener only where an expression may begin.
// Division and /= therefore remain visible to the normal scanner/parser.
func maskECMAScriptRegexLiterals(ctx context.Context, text string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	masked := []byte(text)
	changed := false
	canStartRegex := true

	for at := 0; at < len(text); {
		if at&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		if isHorizontalSpace(text[at]) || isNewlineStart(text[at]) {
			at++
			continue
		}
		if strings.HasPrefix(text[at:], "//") {
			at += 2
			for at < len(text) && !isNewlineStart(text[at]) {
				at++
			}
			continue
		}
		if strings.HasPrefix(text[at:], "/*") {
			end := strings.Index(text[at+2:], "*/")
			if end < 0 {
				return text, nil
			}
			at += end + 4
			continue
		}
		if text[at] == '\'' || text[at] == '"' || text[at] == '`' {
			quote := text[at]
			at++
			for at < len(text) {
				if text[at] == '\\' {
					at++
					if at < len(text) {
						_, size := utf8.DecodeRuneInString(text[at:])
						at += size
					}
					continue
				}
				if text[at] == quote {
					at++
					break
				}
				if quote != '`' && isNewlineStart(text[at]) {
					break
				}
				_, size := utf8.DecodeRuneInString(text[at:])
				at += size
			}
			canStartRegex = false
			continue
		}

		r, size := utf8.DecodeRuneInString(text[at:])
		if ecmaMaskIdentifierStart(r) {
			start := at
			at += size
			for at < len(text) {
				next, nextSize := utf8.DecodeRuneInString(text[at:])
				if !ecmaMaskIdentifierContinue(next) {
					break
				}
				at += nextSize
			}
			word := text[start:at]
			canStartRegex = ecmaKeywordAllowsExpression(word)
			continue
		}
		if unicode.IsDigit(r) {
			at += size
			for at < len(text) {
				next, nextSize := utf8.DecodeRuneInString(text[at:])
				if !(unicode.IsDigit(next) || unicode.IsLetter(next) || next == '.' || next == '_') {
					break
				}
				at += nextSize
			}
			canStartRegex = false
			continue
		}

		if text[at] == '/' {
			if canStartRegex && (at+1 >= len(text) || text[at+1] != '=') {
				if end, ok := completeECMAScriptRegexLiteral(text, at); ok {
					for cursor := at; cursor < end; cursor++ {
						if masked[cursor] != '\r' && masked[cursor] != '\n' {
							masked[cursor] = ' '
						}
					}
					changed = true
					at = end
					canStartRegex = false
					continue
				}
			}
			if at+1 < len(text) && text[at+1] == '=' {
				at += 2
			} else {
				at++
			}
			canStartRegex = true
			continue
		}

		switch text[at] {
		case ')', ']', '}':
			canStartRegex = false
		case '.', '#':
			canStartRegex = false
		case '(', '[', '{', ',', ':', ';', '=', '!', '?', '~':
			canStartRegex = true
		case '+', '-', '*', '%', '&', '|', '^', '<', '>':
			canStartRegex = true
		default:
			canStartRegex = true
		}
		at += size
	}

	if !changed {
		return text, nil
	}
	return string(masked), nil
}

func completeECMAScriptRegexLiteral(text string, start int) (int, bool) {
	if start < 0 || start >= len(text) || text[start] != '/' {
		return 0, false
	}
	inClass := false
	for at := start + 1; at < len(text); at++ {
		switch text[at] {
		case '\r', '\n':
			return 0, false
		case '\\':
			at++
			if at >= len(text) || isNewlineStart(text[at]) {
				return 0, false
			}
		case '[':
			if !inClass {
				inClass = true
			}
		case ']':
			if inClass {
				inClass = false
			}
		case '/':
			if inClass {
				continue
			}
			at++
			for at < len(text) {
				r, size := utf8.DecodeRuneInString(text[at:])
				if !ecmaMaskIdentifierContinue(r) {
					break
				}
				at += size
			}
			return at, true
		}
	}
	return 0, false
}

func ecmaMaskIdentifierStart(value rune) bool {
	return value == '_' || value == '$' || unicode.IsLetter(value)
}

func ecmaMaskIdentifierContinue(value rune) bool {
	return ecmaMaskIdentifierStart(value) || unicode.IsDigit(value) || unicode.IsMark(value) || unicode.In(value, unicode.Pc)
}

func ecmaKeywordAllowsExpression(value string) bool {
	switch value {
	case "await", "case", "delete", "do", "else", "in", "instanceof", "new", "of", "return", "throw", "typeof", "void", "yield":
		return true
	default:
		return false
	}
}
