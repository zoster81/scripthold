package sourceintelligence

import (
	"context"
	"strings"
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
	scan, err := state.scan(options, ShellScannerProfile(language), document.Text)
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
		nameIndex := -1
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
