package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

type ShellAnalyzer struct{}
type BashAnalyzer struct{}
type TclAnalyzer struct{}
type AutoHotkeyAnalyzer struct{}

func (ShellAnalyzer) ID() AnalyzerID        { return AnalyzerShell }
func (ShellAnalyzer) Language() string      { return "shell" }
func (BashAnalyzer) ID() AnalyzerID         { return AnalyzerBash }
func (BashAnalyzer) Language() string       { return "bash" }
func (TclAnalyzer) ID() AnalyzerID          { return AnalyzerTcl }
func (TclAnalyzer) Language() string        { return "tcl" }
func (AutoHotkeyAnalyzer) ID() AnalyzerID   { return AnalyzerAutoHotkey }
func (AutoHotkeyAnalyzer) Language() string { return "autohotkey" }

func (ShellAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeShellFamily(ctx, document, options, "shell", AnalyzerShell, false)
}

func (BashAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeShellFamily(ctx, document, options, "bash", AnalyzerBash, true)
}

func analyzeShellFamily(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID, bash bool) (AnalyzerResult, error) {
	state, err := newPhase8State(ctx, document, options, language, analyzer)
	if err != nil {
		return AnalyzerResult{}, err
	}
	masked := document.Text
	if bash {
		masked, err = maskBashLexicalHazards(ctx, document, options.MaxNesting)
		if err != nil {
			return AnalyzerResult{}, err
		}
	}
	scan, err := state.scan(options, ShellScannerProfile(language), masked)
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, line := range BuildLogicalLines(scan.Tokens, LogicalLineProfile{Separators: []string{";"}}) {
		if len(line.Tokens) < 2 {
			continue
		}
		first := strings.ToLower(line.Tokens[0].Text)
		if first != "." && !(bash && first == "source") {
			continue
		}
		if value, start, end, ok := phase8StaticDependencyTarget(document.Text, line.Tokens[1:]); ok {
			state.addDependency(StructuralDependencyImport, value, start, end)
		}
	}
	pairs := PairDelimiterTokens(scan.Tokens, nil)
	for i := 0; i < len(scan.Tokens) && !state.stopped; {
		token := scan.Tokens[i]
		if token.Kind == TokenEOF {
			break
		}
		if token.Nesting != 0 || token.Kind == TokenNewline || token.Kind == TokenHereDoc {
			i++
			continue
		}
		start := i
		var nameIndex int
		paren := -1
		if bash && strings.EqualFold(token.Text, "function") {
			nameIndex = phase8NextIdentifier(scan.Tokens, i+1, len(scan.Tokens))
			if nameIndex < 0 {
				i++
				continue
			}
			next := nextStructuralToken(scan.Tokens, nameIndex+1, len(scan.Tokens))
			if next < len(scan.Tokens) && scan.Tokens[next].Text == "(" {
				paren = next
			}
		} else if token.Kind == TokenIdentifier {
			nameIndex = i
			next := nextStructuralToken(scan.Tokens, i+1, len(scan.Tokens))
			if next >= len(scan.Tokens) || scan.Tokens[next].Text != "(" {
				i++
				continue
			}
			paren = next
		} else {
			i++
			continue
		}
		search := nameIndex + 1
		if paren >= 0 {
			closeParen := pairs[paren]
			if closeParen <= paren {
				i++
				continue
			}
			search = closeParen + 1
		}
		open := nextStructuralToken(scan.Tokens, search, len(scan.Tokens))
		if open >= len(scan.Tokens) || scan.Tokens[open].Text != "{" {
			i++
			continue
		}
		close := pairs[open]
		if close <= open {
			state.builder.MarkIncomplete()
			break
		}
		name := scan.Tokens[nameIndex]
		state.add(SymbolSpec{Kind: SymbolKindFunction, NativeKind: "function", Name: name.Text, Declaration: OffsetRange{Start: scan.Tokens[start].StartOffset, End: scan.Tokens[close].EndOffset}, NameRange: OffsetRange{Start: name.StartOffset, End: name.EndOffset}, Signature: &OffsetRange{Start: scan.Tokens[start].StartOffset, End: scan.Tokens[open].StartOffset}, Body: &OffsetRange{Start: scan.Tokens[open].StartOffset, End: scan.Tokens[close].EndOffset}, Evidence: SymbolEvidenceStructural})
		i = close + 1
	}
	return state.result()
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
	waitingIn bool
	pattern   bool
	body      bool
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
		if (len(stack) == 0 || stack[len(stack)-1].body) && lower == "case" && bashCommandWordPosition(scan.Tokens, index) {
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
			}
			continue
		}
		if frame.pattern {
			if lower == "esac" && bashCommandWordPosition(scan.Tokens, index) {
				stack = stack[:len(stack)-1]
				continue
			}
			if token.Text == ")" {
				for offset := token.StartOffset; offset < token.EndOffset; offset++ {
					if masked[offset] != '\r' && masked[offset] != '\n' {
						masked[offset] = ' '
					}
				}
				frame.pattern = false
				frame.body = true
			}
			continue
		}
		if frame.body {
			if lower == "esac" && bashCommandWordPosition(scan.Tokens, index) {
				stack = stack[:len(stack)-1]
				continue
			}
			if end, ok := bashCaseArmTerminator(scan.Tokens, index); ok {
				frame.body = false
				frame.pattern = true
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
	case ";", "|", "&", "&&", "||", "(", "{", "then", "do", "else", "if", "elif", "while", "until":
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

func (TclAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	state, err := newPhase8State(ctx, document, options, "tcl", AnalyzerTcl)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, err := state.scan(options, TclScannerProfile(), document.Text)
	if err != nil {
		return AnalyzerResult{}, err
	}
	parser := &tclPhase8Parser{state: state, tokens: scan.Tokens, pairs: PairDelimiterTokens(scan.Tokens, nil)}
	parser.parseRange(0, len(scan.Tokens), 0, nil)
	return state.result()
}

type tclPhase8Parser struct {
	state  *phase8State
	tokens []Token
	pairs  map[int]int
}

func (p *tclPhase8Parser) parseRange(start, end, nesting int, parent *SymbolParent) {
	for i := start; i < end && !p.state.stopped; {
		if p.state.ctx.Err() != nil {
			return
		}
		i = nextStructuralToken(p.tokens, i, end)
		if i >= end || p.tokens[i].Kind == TokenEOF {
			return
		}
		if p.tokens[i].Nesting != nesting {
			i++
			continue
		}
		first := strings.ToLower(p.tokens[i].Text)
		switch first {
		case "proc":
			nameIndex := phase8NextIdentifier(p.tokens, i+1, end)
			if nameIndex < 0 {
				i++
				continue
			}
			argsOpen := nextStructuralToken(p.tokens, nameIndex+1, end)
			if argsOpen >= end || p.tokens[argsOpen].Text != "{" {
				i++
				continue
			}
			argsClose := p.pairs[argsOpen]
			bodyOpen := nextStructuralToken(p.tokens, argsClose+1, end)
			if argsClose <= argsOpen || bodyOpen >= end || p.tokens[bodyOpen].Text != "{" {
				i++
				continue
			}
			bodyClose := p.pairs[bodyOpen]
			if bodyClose <= bodyOpen {
				p.state.builder.MarkIncomplete()
				return
			}
			name := p.tokens[nameIndex]
			p.state.add(SymbolSpec{Kind: SymbolKindFunction, NativeKind: "proc", Name: name.Text, Parent: parent, Declaration: OffsetRange{Start: p.tokens[i].StartOffset, End: p.tokens[bodyClose].EndOffset}, NameRange: OffsetRange{Start: name.StartOffset, End: name.EndOffset}, Signature: &OffsetRange{Start: p.tokens[i].StartOffset, End: p.tokens[bodyOpen].StartOffset}, Body: &OffsetRange{Start: p.tokens[bodyOpen].StartOffset, End: p.tokens[bodyClose].EndOffset}, Evidence: SymbolEvidenceStructural})
			i = bodyClose + 1
		case "namespace":
			eval := nextStructuralToken(p.tokens, i+1, end)
			if eval >= end || !strings.EqualFold(p.tokens[eval].Text, "eval") {
				i++
				continue
			}
			nameIndex := phase8NextIdentifier(p.tokens, eval+1, end)
			if nameIndex < 0 {
				i++
				continue
			}
			bodyOpen := nextStructuralToken(p.tokens, nameIndex+1, end)
			if bodyOpen >= end || p.tokens[bodyOpen].Text != "{" {
				i++
				continue
			}
			bodyClose := p.pairs[bodyOpen]
			if bodyClose <= bodyOpen {
				p.state.builder.MarkIncomplete()
				return
			}
			name := p.tokens[nameIndex]
			symbol, ok := p.state.add(SymbolSpec{Kind: SymbolKindNamespace, NativeKind: "namespace-eval", Name: name.Text, Parent: parent, Declaration: OffsetRange{Start: p.tokens[i].StartOffset, End: p.tokens[bodyClose].EndOffset}, NameRange: OffsetRange{Start: name.StartOffset, End: name.EndOffset}, Signature: &OffsetRange{Start: p.tokens[i].StartOffset, End: p.tokens[bodyOpen].StartOffset}, Body: &OffsetRange{Start: p.tokens[bodyOpen].StartOffset, End: p.tokens[bodyClose].EndOffset}, Evidence: SymbolEvidenceStructural})
			if ok {
				child := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
				p.parseRange(bodyOpen+1, bodyClose, nesting+1, child)
			}
			i = bodyClose + 1
		case "source":
			lineEnd := phase8TokenLineEnd(p.tokens, i+1)
			if value, start, end, ok := phase8StaticDependencyTarget(p.state.document.Text, p.tokens[i+1:lineEnd]); ok {
				p.state.addDependency(StructuralDependencyImport, value, start, end)
			}
			i = max(lineEnd, i+1)
		case "package":
			lineEnd := phase8TokenLineEnd(p.tokens, i+1)
			require := nextStructuralToken(p.tokens, i+1, lineEnd)
			if require < lineEnd && strings.EqualFold(p.tokens[require].Text, "require") {
				idx := phase8NextIdentifier(p.tokens, require+1, lineEnd)
				if idx >= 0 {
					p.state.addDependency(StructuralDependencyImport, p.tokens[idx].Text, p.tokens[idx].StartOffset, p.tokens[idx].EndOffset)
				}
			}
			i = max(lineEnd, i+1)
		default:
			i++
		}
	}
}

func (AutoHotkeyAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	state, err := newPhase8State(ctx, document, options, "autohotkey", AnalyzerAutoHotkey)
	if err != nil {
		return AnalyzerResult{}, err
	}
	masked, maskComplete := maskAutoHotkeyCodeStrings(document.Text)
	scan, err := state.scan(options, AutoHotkeyScannerProfile(), masked)
	if err != nil {
		return AnalyzerResult{}, err
	}
	if !maskComplete {
		state.builder.MarkIncomplete()
		_ = state.builder.AddDiagnostic(DiagnosticSpec{Code: "autohotkey-unterminated-string", Message: "AutoHotkey source contains an unterminated string", Severity: DiagnosticWarning, AffectsCoverage: true})
	}
	for _, token := range scan.Tokens {
		if token.Kind != TokenDirective {
			continue
		}
		trimmed := strings.TrimSpace(token.Text)
		lower := strings.ToLower(trimmed)
		prefix := ""
		if strings.HasPrefix(lower, "#includeagain") {
			prefix = "#includeagain"
		} else if strings.HasPrefix(lower, "#include") {
			prefix = "#include"
		}
		if prefix == "" {
			continue
		}
		rest := strings.TrimSpace(trimmed[len(prefix):])
		value := phase7QuotedOrAngleValue(rest)
		if value == "" && rest != "" && !strings.ContainsAny(rest, "%`") {
			value = strings.Fields(rest)[0]
		}
		if value != "" {
			state.addDependency(StructuralDependencyImport, value, token.StartOffset, token.EndOffset)
		}
	}
	parser := &autoHotkeyPhase8Parser{state: state, tokens: scan.Tokens, pairs: PairDelimiterTokens(scan.Tokens, nil)}
	parser.parseRange(0, len(scan.Tokens), 0, nil)
	return state.result()
}

type autoHotkeyPhase8Parser struct {
	state  *phase8State
	tokens []Token
	pairs  map[int]int
}

func (p *autoHotkeyPhase8Parser) parseRange(start, end, nesting int, parent *SymbolParent) {
	for i := start; i < end && !p.state.stopped; {
		i = nextStructuralToken(p.tokens, i, end)
		if i >= end || p.tokens[i].Kind == TokenEOF {
			return
		}
		if p.tokens[i].Kind == TokenDirective || p.tokens[i].Nesting != nesting {
			i++
			continue
		}
		if strings.EqualFold(p.tokens[i].Text, "class") {
			nameIndex := phase8NextIdentifier(p.tokens, i+1, end)
			if nameIndex < 0 {
				i++
				continue
			}
			open := -1
			for j := nameIndex + 1; j < end; j++ {
				if p.tokens[j].Text == "{" && p.tokens[j].Nesting == nesting+1 {
					open = j
					break
				}
				if p.tokens[j].Kind == TokenNewline {
					break
				}
			}
			if open < 0 {
				i++
				continue
			}
			close := p.pairs[open]
			if close <= open {
				p.state.builder.MarkIncomplete()
				return
			}
			name := p.tokens[nameIndex]
			symbol, ok := p.state.add(SymbolSpec{Kind: SymbolKindClass, NativeKind: "class", Name: name.Text, Parent: parent, Declaration: OffsetRange{Start: p.tokens[i].StartOffset, End: p.tokens[close].EndOffset}, NameRange: OffsetRange{Start: name.StartOffset, End: name.EndOffset}, Signature: &OffsetRange{Start: p.tokens[i].StartOffset, End: p.tokens[open].StartOffset}, Body: &OffsetRange{Start: p.tokens[open].StartOffset, End: p.tokens[close].EndOffset}, Evidence: SymbolEvidenceStructural})
			if ok {
				for j := nameIndex + 1; j < open; j++ {
					if strings.EqualFold(p.tokens[j].Text, "extends") {
						target := phase8NextIdentifier(p.tokens, j+1, open)
						if target >= 0 {
							p.state.addRelation("extends", symbol.QualifiedName, p.tokens[target].Text, p.tokens[target].StartOffset, p.tokens[target].EndOffset)
						}
						break
					}
				}
				child := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
				p.parseRange(open+1, close, nesting+1, child)
			}
			i = close + 1
			continue
		}
		if p.tokens[i].Kind == TokenIdentifier {
			paren := nextStructuralToken(p.tokens, i+1, end)
			if paren < end && p.tokens[paren].Text == "(" {
				closeParen := p.pairs[paren]
				open := nextStructuralToken(p.tokens, closeParen+1, end)
				if closeParen > paren && open < end && p.tokens[open].Text == "{" {
					close := p.pairs[open]
					if close > open {
						kind := SymbolKindFunction
						if parent != nil {
							kind = SymbolKindMethod
						}
						name := p.tokens[i]
						p.state.add(SymbolSpec{Kind: kind, NativeKind: "function", Name: name.Text, Parent: parent, Declaration: OffsetRange{Start: name.StartOffset, End: p.tokens[close].EndOffset}, NameRange: OffsetRange{Start: name.StartOffset, End: name.EndOffset}, Signature: &OffsetRange{Start: name.StartOffset, End: p.tokens[open].StartOffset}, Body: &OffsetRange{Start: p.tokens[open].StartOffset, End: p.tokens[close].EndOffset}, Evidence: SymbolEvidenceStructural})
						i = close + 1
						continue
					}
				}
			}
		}
		i++
	}
}
