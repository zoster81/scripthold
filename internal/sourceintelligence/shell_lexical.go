package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

func maskPOSIXShellLexicalHazards(ctx context.Context, document *SourceDocument, maxNesting int) (string, error) {
	masked, err := maskShellArithmeticExpansions(ctx, document.Text, maxNesting)
	if err != nil {
		return "", err
	}
	if masked == document.Text {
		return maskBashCasePatternClosers(ctx, document, maxNesting)
	}
	clone := *document
	clone.Text = masked
	clone.lineStarts = buildLineStarts(masked)
	return maskBashCasePatternClosers(ctx, &clone, maxNesting)
}

func maskShellArithmeticExpansions(ctx context.Context, text string, maxNesting int) (string, error) {
	if !strings.Contains(text, "$((") {
		return text, nil
	}
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	masked := []byte(text)
	changed := false
	for at := 0; at+2 < len(text); {
		if at&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		if !strings.HasPrefix(text[at:], "$((") {
			at++
			continue
		}
		end, ok, err := shellArithmeticExpansionEnd(ctx, text, at, maxNesting)
		if err != nil {
			return "", err
		}
		if !ok {
			at += 3
			continue
		}
		maskBashRange(masked, at, end)
		changed = true
		at = end
	}
	if !changed {
		return text, nil
	}
	return string(masked), nil
}

func shellArithmeticExpansionEnd(ctx context.Context, text string, start, maxNesting int) (int, bool, error) {
	if start < 0 || start+2 >= len(text) || !strings.HasPrefix(text[start:], "$((") {
		return 0, false, nil
	}
	depth := 0
	for at := start + 3; at < len(text); {
		if at&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, false, err
			}
		}
		switch text[at] {
		case '\\':
			at = bashSkipEscapedByte(text, at)
		case '\'':
			next, ok := bashSkipSingleQuoted(text, at)
			if !ok {
				return 0, false, nil
			}
			at = next
		case '"':
			next, ok, err := bashSkipDoubleQuotedCommandText(ctx, text, at, maxNesting, 1)
			if err != nil {
				return 0, false, err
			}
			if !ok {
				return 0, false, nil
			}
			at = next
		case '(':
			depth++
			if depth > maxNesting {
				return 0, false, operation.New(operation.KindLimit, "shell arithmetic expansion nesting exceeds limit")
			}
			at++
		case ')':
			if depth > 0 {
				depth--
				at++
				continue
			}
			if at+1 < len(text) && text[at+1] == ')' {
				return at + 2, true, nil
			}
			at++
		default:
			at++
		}
	}
	return 0, false, nil
}

func maskBashLexicalHazards(ctx context.Context, document *SourceDocument, maxNesting int) (string, error) {
	masked, err := maskBashDoubleQuotedExpansions(ctx, document.Text, maxNesting)
	if err != nil {
		return "", err
	}
	if masked == document.Text {
		return maskBashCasePatternClosers(ctx, document, maxNesting)
	}
	clone := *document
	clone.Text = masked
	clone.lineStarts = buildLineStarts(masked)
	return maskBashCasePatternClosers(ctx, &clone, maxNesting)
}

func maskBashDoubleQuotedExpansions(ctx context.Context, text string, maxNesting int) (string, error) {
	if !strings.Contains(text, "$(") && !strings.Contains(text, "${") && !strings.ContainsRune(text, '`') {
		return text, nil
	}
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	masked := []byte(text)
	changed := false
	for at := 0; at < len(text); {
		if at&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		switch text[at] {
		case '\\':
			at = bashSkipEscapedByte(text, at)
		case '\'':
			next, ok := bashSkipSingleQuoted(text, at)
			if !ok {
				return string(masked), nil
			}
			at = next
		case '"':
			next, ranges, ok, err := bashDoubleQuotedExpansionRanges(ctx, text, at, maxNesting)
			if err != nil {
				return "", err
			}
			if !ok {
				return string(masked), nil
			}
			for _, value := range ranges {
				maskBashRange(masked, value.Start, value.End)
				changed = true
			}
			at = next
		case '`':
			next, ok := bashSkipBacktick(text, at)
			if !ok {
				return string(masked), nil
			}
			at = next
		case '#':
			if bashCommentStartsAt(text, at) {
				at = bashSkipLineComment(text, at)
			} else {
				at++
			}
		case '$':
			if strings.HasPrefix(text[at:], "${") {
				next, ok, err := bashSkipBracedExpansion(ctx, text, at, maxNesting, 1)
				if err != nil {
					return "", err
				}
				if ok {
					maskBashRange(masked, at, next)
					changed = true
					at = next
					continue
				}
			}
			if strings.HasPrefix(text[at:], "$(") {
				end, ok, err := findBashCommandSubstitutionEnd(ctx, text, at, maxNesting, 1)
				if err != nil {
					return "", err
				}
				if ok {
					at = end + 1
					continue
				}
			}
			at++
		default:
			at++
		}
	}
	if !changed {
		return text, nil
	}
	return string(masked), nil
}

func bashDoubleQuotedExpansionRanges(ctx context.Context, text string, start, maxNesting int) (int, []OffsetRange, bool, error) {
	ranges := make([]OffsetRange, 0, 2)
	for at := start + 1; at < len(text); {
		if at&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, nil, false, err
			}
		}
		switch text[at] {
		case '\\':
			at = bashSkipEscapedByte(text, at)
		case '"':
			return at + 1, ranges, true, nil
		case '`':
			next, ok := bashSkipBacktick(text, at)
			if !ok {
				return 0, nil, false, nil
			}
			ranges = append(ranges, OffsetRange{Start: at, End: next})
			at = next
		case '$':
			if strings.HasPrefix(text[at:], "${") {
				next, ok, err := bashSkipBracedExpansion(ctx, text, at, maxNesting, 1)
				if err != nil {
					return 0, nil, false, err
				}
				if !ok {
					return 0, nil, false, nil
				}
				ranges = append(ranges, OffsetRange{Start: at, End: next})
				at = next
				continue
			}
			if strings.HasPrefix(text[at:], "$(") {
				end, ok, err := findBashCommandSubstitutionEnd(ctx, text, at, maxNesting, 1)
				if err != nil {
					return 0, nil, false, err
				}
				if !ok {
					return 0, nil, false, nil
				}
				ranges = append(ranges, OffsetRange{Start: at, End: end + 1})
				at = end + 1
				continue
			}
			at++
		default:
			at++
		}
	}
	return 0, nil, false, nil
}

func maskBashRange(masked []byte, start, end int) {
	for offset := max(start, 0); offset < min(end, len(masked)); offset++ {
		if masked[offset] != '\r' && masked[offset] != '\n' {
			masked[offset] = ' '
		}
	}
}

func findBashCommandSubstitutionEnd(ctx context.Context, text string, start, maxNesting, nesting int) (int, bool, error) {
	if start < 0 || start+1 >= len(text) || text[start] != '$' || text[start+1] != '(' {
		return 0, false, nil
	}
	if nesting > maxNesting {
		return 0, false, operation.New(operation.KindLimit, "Bash command substitution nesting exceeds limit")
	}
	parenDepth := 0
	for at := start + 2; at < len(text); {
		if at&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, false, err
			}
		}
		switch text[at] {
		case '\\':
			at = bashSkipEscapedByte(text, at)
		case '\'':
			next, ok := bashSkipSingleQuoted(text, at)
			if !ok {
				return 0, false, nil
			}
			at = next
		case '"':
			next, ok, err := bashSkipDoubleQuotedCommandText(ctx, text, at, maxNesting, nesting)
			if err != nil {
				return 0, false, err
			}
			if !ok {
				return 0, false, nil
			}
			at = next
		case '`':
			next, ok := bashSkipBacktick(text, at)
			if !ok {
				return 0, false, nil
			}
			at = next
		case '#':
			if bashCommentStartsAt(text, at) {
				at = bashSkipLineComment(text, at)
			} else {
				at++
			}
		case '$':
			if strings.HasPrefix(text[at:], "${") {
				next, ok, err := bashSkipBracedExpansion(ctx, text, at, maxNesting, nesting)
				if err != nil {
					return 0, false, err
				}
				if !ok {
					return 0, false, nil
				}
				at = next
				continue
			}
			if strings.HasPrefix(text[at:], "$(") && (at+2 >= len(text) || text[at+2] != '(') {
				end, ok, err := findBashCommandSubstitutionEnd(ctx, text, at, maxNesting, nesting+1)
				if err != nil {
					return 0, false, err
				}
				if !ok {
					return 0, false, nil
				}
				at = end + 1
				continue
			}
			at++
		case '(':
			parenDepth++
			if nesting+parenDepth > maxNesting {
				return 0, false, operation.New(operation.KindLimit, "Bash parenthesis nesting exceeds limit")
			}
			at++
		case ')':
			if parenDepth == 0 {
				return at, true, nil
			}
			parenDepth--
			at++
		default:
			at++
		}
	}
	return 0, false, nil
}

func bashSkipDoubleQuotedCommandText(ctx context.Context, text string, start, maxNesting, nesting int) (int, bool, error) {
	for at := start + 1; at < len(text); {
		if at&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, false, err
			}
		}
		switch text[at] {
		case '\\':
			at = bashSkipEscapedByte(text, at)
		case '"':
			return at + 1, true, nil
		case '`':
			next, ok := bashSkipBacktick(text, at)
			if !ok {
				return 0, false, nil
			}
			at = next
		case '$':
			if strings.HasPrefix(text[at:], "${") {
				next, ok, err := bashSkipBracedExpansion(ctx, text, at, maxNesting, nesting)
				if err != nil {
					return 0, false, err
				}
				if !ok {
					return 0, false, nil
				}
				at = next
				continue
			}
			if strings.HasPrefix(text[at:], "$(") && (at+2 >= len(text) || text[at+2] != '(') {
				end, ok, err := findBashCommandSubstitutionEnd(ctx, text, at, maxNesting, nesting+1)
				if err != nil {
					return 0, false, err
				}
				if !ok {
					return 0, false, nil
				}
				at = end + 1
				continue
			}
			at++
		default:
			at++
		}
	}
	return 0, false, nil
}

func bashSkipBracedExpansion(ctx context.Context, text string, start, maxNesting, nesting int) (int, bool, error) {
	if nesting > maxNesting {
		return 0, false, operation.New(operation.KindLimit, "Bash parameter expansion nesting exceeds limit")
	}
	depth := 1
	for at := start + 2; at < len(text); {
		if at&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, false, err
			}
		}
		switch text[at] {
		case '\\':
			at = bashSkipEscapedByte(text, at)
		case '\'':
			next, ok := bashSkipSingleQuoted(text, at)
			if !ok {
				return 0, false, nil
			}
			at = next
		case '"':
			next, ok, err := bashSkipDoubleQuotedCommandText(ctx, text, at, maxNesting, nesting)
			if err != nil {
				return 0, false, err
			}
			if !ok {
				return 0, false, nil
			}
			at = next
		case '`':
			next, ok := bashSkipBacktick(text, at)
			if !ok {
				return 0, false, nil
			}
			at = next
		case '$':
			if strings.HasPrefix(text[at:], "${") {
				depth++
				if nesting+depth > maxNesting {
					return 0, false, operation.New(operation.KindLimit, "Bash parameter expansion nesting exceeds limit")
				}
				at += 2
				continue
			}
			if strings.HasPrefix(text[at:], "$(") && (at+2 >= len(text) || text[at+2] != '(') {
				end, ok, err := findBashCommandSubstitutionEnd(ctx, text, at, maxNesting, nesting+1)
				if err != nil {
					return 0, false, err
				}
				if !ok {
					return 0, false, nil
				}
				at = end + 1
				continue
			}
			at++
		case '}':
			depth--
			at++
			if depth == 0 {
				return at, true, nil
			}
		default:
			at++
		}
	}
	return 0, false, nil
}

func bashSkipSingleQuoted(text string, start int) (int, bool) {
	if end := strings.IndexByte(text[start+1:], '\''); end >= 0 {
		return start + 1 + end + 1, true
	}
	return 0, false
}

func bashSkipBacktick(text string, start int) (int, bool) {
	for at := start + 1; at < len(text); {
		if text[at] == '\\' {
			at = bashSkipEscapedByte(text, at)
			continue
		}
		if text[at] == '`' {
			return at + 1, true
		}
		at++
	}
	return 0, false
}

func bashSkipEscapedByte(text string, at int) int {
	if at+1 >= len(text) {
		return len(text)
	}
	if text[at+1] == '\r' && at+2 < len(text) && text[at+2] == '\n' {
		return at + 3
	}
	return at + 2
}

func bashCommentStartsAt(text string, at int) bool {
	if at <= 0 {
		return true
	}
	previous := text[at-1]
	return previous == ' ' || previous == '\t' || previous == '\r' || previous == '\n' || strings.ContainsRune(";|&(){}", rune(previous))
}

func bashSkipLineComment(text string, at int) int {
	if end := strings.IndexByte(text[at:], '\n'); end >= 0 {
		return at + end
	}
	return len(text)
}

type bashCaseFrame struct {
	waitingIn    bool
	pattern      bool
	patternStart bool
	body         bool
	bodyStart    bool
}

func maskBashCasePatternClosers(ctx context.Context, document *SourceDocument, maxNesting int) (string, error) {
	text := document.Text
	if strings.IndexByte(text, ')') < 0 || !strings.Contains(strings.ToLower(text), "case") {
		return text, nil
	}
	profile := ShellScannerProfile("bash")
	profile.DisableDelimiterTracking = true
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, document, profile, ScannerLimits{MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return "", err
	}
	masked := []byte(text)
	stack := make([]bashCaseFrame, 0, 4)
	for index := 0; index < len(scan.Tokens); index++ {
		token := scan.Tokens[index]
		lower := strings.ToLower(token.Text)
		caseCommandStart := bashCommandWordPosition(scan.Tokens, index)
		if len(stack) > 0 && stack[len(stack)-1].body && stack[len(stack)-1].bodyStart {
			caseCommandStart = true
		}
		if (len(stack) == 0 || stack[len(stack)-1].body) && lower == "case" && caseCommandStart {
			if len(stack) > 0 {
				stack[len(stack)-1].bodyStart = false
			}
			stack = append(stack, bashCaseFrame{waitingIn: true})
			continue
		}
		if len(stack) == 0 {
			continue
		}
		frame := &stack[len(stack)-1]
		if frame.waitingIn {
			if lower == "in" {
				frame.waitingIn = false
				frame.pattern = true
				frame.patternStart = true
			}
			continue
		}
		if frame.pattern {
			if lower == "esac" && bashCommandWordPosition(scan.Tokens, index) {
				stack = stack[:len(stack)-1]
				continue
			}
			if frame.patternStart {
				if token.Kind == TokenNewline {
					continue
				}
				frame.patternStart = false
				if token.Text == "(" {
					for offset := token.StartOffset; offset < token.EndOffset; offset++ {
						if masked[offset] != '\r' && masked[offset] != '\n' {
							masked[offset] = ' '
						}
					}
					continue
				}
			}
			if token.Text == ")" {
				for offset := token.StartOffset; offset < token.EndOffset; offset++ {
					if masked[offset] != '\r' && masked[offset] != '\n' {
						masked[offset] = ' '
					}
				}
				frame.pattern = false
				frame.body = true
				frame.bodyStart = true
			}
			continue
		}
		if frame.body {
			if frame.bodyStart && token.Kind != TokenNewline {
				frame.bodyStart = false
			}
			if lower == "esac" && bashCommandWordPosition(scan.Tokens, index) {
				stack = stack[:len(stack)-1]
				continue
			}
			if end, ok := bashCaseArmTerminator(scan.Tokens, index); ok {
				frame.body = false
				frame.pattern = true
				frame.patternStart = true
				index = end
			}
		}
	}
	return string(masked), nil
}

func bashCommandWordPosition(tokens []Token, index int) bool {
	if index <= 0 {
		return true
	}
	previous := tokens[index-1]
	if previous.Kind == TokenNewline {
		return true
	}
	switch strings.ToLower(previous.Text) {
	case ";", "|", "&", "&&", "||", "!", "(", "{", "then", "do", "else", "if", "elif", "while", "until":
		return true
	default:
		return false
	}
}

func bashCaseArmTerminator(tokens []Token, index int) (int, bool) {
	if index < 0 || index >= len(tokens) || tokens[index].Text != ";" || index+1 >= len(tokens) {
		return index, false
	}
	if tokens[index+1].Text == "&" {
		return index + 1, true
	}
	if tokens[index+1].Text != ";" {
		return index, false
	}
	if index+2 < len(tokens) && tokens[index+2].Text == "&" {
		return index + 2, true
	}
	return index + 1, true
}
