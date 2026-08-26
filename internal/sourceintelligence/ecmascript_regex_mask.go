package sourceintelligence

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/operation"
)

// maskECMAScriptRegexLiterals hides complete regular-expression literals before
// the shared delimiter scanner runs. Recognition is deliberately conservative:
// a slash is treated as a regex opener only where an expression may begin.
// Division and /= therefore remain visible to the normal scanner/parser.
func maskECMAScriptRegexLiterals(ctx context.Context, text string, maxNesting int) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	masker := ecmaRegexMasker{ctx: ctx, text: text, masked: []byte(text), maxNesting: maxNesting}
	if err := masker.maskTopLevel(); err != nil {
		return "", err
	}
	if !masker.changed {
		return text, nil
	}
	return string(masker.masked), nil
}

type ecmaRegexMasker struct {
	ctx        context.Context
	text       string
	masked     []byte
	maxNesting int
	changed    bool
}

func (masker *ecmaRegexMasker) maskTopLevel() error {
	canStartRegex := true
	for at := 0; at < len(masker.text); {
		end, nextCanStartRegex, handled, err := masker.skipCodeToken(at, canStartRegex, 0)
		if err != nil {
			return err
		}
		if handled {
			at = end
			canStartRegex = nextCanStartRegex
			continue
		}
		_, size := utf8.DecodeRuneInString(masker.text[at:])
		canStartRegex = ecmaCanStartRegexAfterPunctuation(masker.text[at])
		at += size
	}
	return nil
}

func (masker *ecmaRegexMasker) skipCodeToken(at int, canStartRegex bool, nesting int) (int, bool, bool, error) {
	if at&4095 == 0 {
		if err := masker.ctx.Err(); err != nil {
			return 0, false, false, err
		}
	}
	if end, nextCanStartRegex, handled, err := masker.skipTriviaOrLiteral(at, canStartRegex, nesting); handled || err != nil {
		return end, nextCanStartRegex, handled, err
	}
	if end, nextCanStartRegex, handled := ecmaWordOrNumberEnd(masker.text, at); handled {
		return end, nextCanStartRegex, true, nil
	}
	if masker.text[at] == '/' {
		return masker.skipSlash(at, canStartRegex)
	}
	return at, canStartRegex, false, nil
}

func (masker *ecmaRegexMasker) skipTriviaOrLiteral(at int, canStartRegex bool, nesting int) (int, bool, bool, error) {
	switch {
	case isHorizontalSpace(masker.text[at]) || isNewlineStart(masker.text[at]):
		return at + 1, canStartRegex, true, nil
	case strings.HasPrefix(masker.text[at:], "//"):
		return ecmaLineCommentEnd(masker.text, at), canStartRegex, true, nil
	case strings.HasPrefix(masker.text[at:], "/*"):
		return ecmaBlockCommentEnd(masker.text, at), canStartRegex, true, nil
	case masker.text[at] == '\'' || masker.text[at] == '"':
		return ecmaQuotedLiteralEnd(masker.text, at), false, true, nil
	case masker.text[at] == '`':
		end, _, err := masker.templateLiteralEnd(at, nesting+1)
		return end, false, true, err
	default:
		return at, canStartRegex, false, nil
	}
}

func ecmaWordOrNumberEnd(text string, at int) (int, bool, bool) {
	r, size := utf8.DecodeRuneInString(text[at:])
	if ecmaMaskIdentifierStart(r) {
		end := ecmaIdentifierEnd(text, at+size)
		return end, ecmaKeywordAllowsExpression(text[at:end]), true
	}
	if unicode.IsDigit(r) {
		return ecmaNumberEnd(text, at+size), false, true
	}
	return at, false, false
}

func ecmaIdentifierEnd(text string, at int) int {
	for at < len(text) {
		next, size := utf8.DecodeRuneInString(text[at:])
		if !ecmaMaskIdentifierContinue(next) {
			break
		}
		at += size
	}
	return at
}

func ecmaNumberEnd(text string, at int) int {
	for at < len(text) {
		next, size := utf8.DecodeRuneInString(text[at:])
		if !(unicode.IsDigit(next) || unicode.IsLetter(next) || next == '.' || next == '_') {
			break
		}
		at += size
	}
	return at
}

func (masker *ecmaRegexMasker) skipSlash(at int, canStartRegex bool) (int, bool, bool, error) {
	if canStartRegex && (at+1 >= len(masker.text) || masker.text[at+1] != '=') {
		if end, ok := completeECMAScriptRegexLiteral(masker.text, at); ok {
			masker.maskRange(at, end)
			return end, false, true, nil
		}
	}
	if at+1 < len(masker.text) && masker.text[at+1] == '=' {
		return at + 2, true, true, nil
	}
	return at + 1, true, true, nil
}

func (masker *ecmaRegexMasker) templateLiteralEnd(start, nesting int) (int, bool, error) {
	if err := masker.checkNesting(nesting); err != nil {
		return 0, false, err
	}
	for at := start + 1; at < len(masker.text); {
		if at&4095 == 0 {
			if err := masker.ctx.Err(); err != nil {
				return 0, false, err
			}
		}
		switch {
		case masker.text[at] == '\\':
			at = ecmaEscapedRuneEnd(masker.text, at)
		case masker.text[at] == '`':
			return at + 1, true, nil
		case strings.HasPrefix(masker.text[at:], "${"):
			end, ok, err := masker.templateExpressionEnd(at+2, nesting+1)
			if err != nil {
				return 0, false, err
			}
			if !ok {
				return len(masker.text), false, nil
			}
			at = end
		default:
			_, size := utf8.DecodeRuneInString(masker.text[at:])
			at += size
		}
	}
	return len(masker.text), false, nil
}

func (masker *ecmaRegexMasker) templateExpressionEnd(start, nesting int) (int, bool, error) {
	if err := masker.checkNesting(nesting); err != nil {
		return 0, false, err
	}
	braceDepth := 1
	canStartRegex := true
	for at := start; at < len(masker.text); {
		currentNesting := nesting + braceDepth - 1
		end, nextCanStartRegex, handled, err := masker.skipCodeToken(at, canStartRegex, currentNesting)
		if err != nil {
			return 0, false, err
		}
		if handled {
			at = end
			canStartRegex = nextCanStartRegex
			continue
		}

		_, size := utf8.DecodeRuneInString(masker.text[at:])
		switch masker.text[at] {
		case '{':
			braceDepth++
			if err := masker.checkNesting(nesting + braceDepth - 1); err != nil {
				return 0, false, err
			}
			canStartRegex = true
			at += size
		case '}':
			braceDepth--
			at += size
			if braceDepth == 0 {
				return at, true, nil
			}
			canStartRegex = false
		default:
			canStartRegex = ecmaCanStartRegexAfterPunctuation(masker.text[at])
			at += size
		}
	}
	return len(masker.text), false, nil
}

func (masker *ecmaRegexMasker) checkNesting(nesting int) error {
	if nesting <= masker.maxNesting {
		return nil
	}
	return operation.Wrap(operation.KindLimit, "mask_ecmascript_regex_literals", "", fmt.Errorf("lexical nesting %d exceeds limit %d", nesting, masker.maxNesting))
}

func (masker *ecmaRegexMasker) maskRange(start, end int) {
	for at := start; at < end; at++ {
		if masker.masked[at] != '\r' && masker.masked[at] != '\n' {
			masker.masked[at] = ' '
		}
	}
	masker.changed = true
}

func ecmaLineCommentEnd(text string, start int) int {
	at := start + 2
	for at < len(text) && !isNewlineStart(text[at]) {
		at++
	}
	return at
}

func ecmaBlockCommentEnd(text string, start int) int {
	if end := strings.Index(text[start+2:], "*/"); end >= 0 {
		return start + end + 4
	}
	return len(text)
}

func ecmaQuotedLiteralEnd(text string, start int) int {
	quote := text[start]
	for at := start + 1; at < len(text); {
		if text[at] == '\\' {
			at = ecmaEscapedRuneEnd(text, at)
			continue
		}
		if text[at] == quote {
			return at + 1
		}
		if isNewlineStart(text[at]) {
			return at
		}
		_, size := utf8.DecodeRuneInString(text[at:])
		at += size
	}
	return len(text)
}

func ecmaEscapedRuneEnd(text string, start int) int {
	at := start + 1
	if at >= len(text) {
		return at
	}
	if text[at] == '\r' && at+1 < len(text) && text[at+1] == '\n' {
		return at + 2
	}
	_, size := utf8.DecodeRuneInString(text[at:])
	return at + size
}

func ecmaCanStartRegexAfterPunctuation(value byte) bool {
	switch value {
	case ')', ']', '}', '.', '#':
		return false
	default:
		return true
	}
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
