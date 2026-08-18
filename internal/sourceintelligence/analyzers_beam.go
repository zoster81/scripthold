package sourceintelligence

import (
	"context"
	"strconv"
	"strings"
)

type ElixirAnalyzer struct{}
type ErlangAnalyzer struct{}
type GleamAnalyzer struct{}

func (ElixirAnalyzer) ID() AnalyzerID   { return AnalyzerElixir }
func (ElixirAnalyzer) Language() string { return "elixir" }
func (ErlangAnalyzer) ID() AnalyzerID   { return AnalyzerErlang }
func (ErlangAnalyzer) Language() string { return "erlang" }
func (GleamAnalyzer) ID() AnalyzerID    { return AnalyzerGleam }
func (GleamAnalyzer) Language() string  { return "gleam" }

type elixirScope struct {
	kind   string
	parent *SymbolParent
}

func (ElixirAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	state, err := newPhase8State(ctx, document, options, "elixir", AnalyzerElixir)
	if err != nil {
		return AnalyzerResult{}, err
	}
	masked := maskElixirSigils(document.Text)
	scan, err := state.scan(options, ElixirScannerProfile(), masked)
	if err != nil {
		return AnalyzerResult{}, err
	}
	var scopes []elixirScope
	var pending *elixirScope
	currentModule := func() *SymbolParent {
		for i := len(scopes) - 1; i >= 0; i-- {
			if scopes[i].kind == "module" && scopes[i].parent != nil {
				value := *scopes[i].parent
				return &value
			}
		}
		return nil
	}
	for _, line := range BuildLogicalLines(scan.Tokens, LogicalLineProfile{}) {
		if state.stopped || len(line.Tokens) == 0 {
			continue
		}
		first := strings.ToLower(line.Tokens[0].Text)
		if pending != nil && elixirHasDoToken(line.Tokens) {
			if elixirOpensBlock(line.Tokens) {
				scopes = append(scopes, *pending)
			}
			pending = nil
		}
		if first == "end" {
			if len(scopes) == 0 {
				value := OffsetRange{Start: line.StartOffset, End: line.EndOffset}
				_ = state.builder.AddDiagnostic(DiagnosticSpec{Code: "elixir-unmatched-end", Message: "Elixir end has no matching structural block", Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
				continue
			}
			scopes = scopes[:len(scopes)-1]
			continue
		}
		switch first {
		case "defmodule":
			start := phase8NextIdentifier(line.Tokens, 1, len(line.Tokens))
			if start < 0 {
				continue
			}
			end := start + 1
			for end+1 < len(line.Tokens) && line.Tokens[end].Text == "." && (line.Tokens[end+1].Kind == TokenIdentifier || line.Tokens[end+1].Kind == TokenKeyword) {
				end += 2
			}
			name := tokenRangeText(line.Tokens, start, end)
			if name == "" {
				continue
			}
			symbol, ok := state.add(SymbolSpec{Kind: SymbolKindModule, NativeKind: "defmodule", Name: name, QualifiedName: name, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[start].StartOffset, End: line.Tokens[end-1].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			if ok {
				parent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
				scope := elixirScope{kind: "module", parent: parent}
				if elixirOpensBlock(line.Tokens) {
					scopes = append(scopes, scope)
				} else if elixirLineHasUnclosedDelimiter(line.Tokens) {
					pending = &scope
				}
			}
			continue
		case "def", "defp", "defmacro", "defmacrop", "defguard", "defguardp":
			idx := phase8NextIdentifier(line.Tokens, 1, len(line.Tokens))
			if idx < 0 {
				continue
			}
			tok := line.Tokens[idx]
			native := first
			kind := SymbolKindFunction
			state.add(SymbolSpec{Kind: kind, NativeKind: native, Name: tok.Text, Parent: currentModule(), Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			if elixirOpensBlock(line.Tokens) {
				scopes = append(scopes, elixirScope{kind: "callable"})
			} else if elixirLineHasUnclosedDelimiter(line.Tokens) {
				scope := elixirScope{kind: "callable"}
				pending = &scope
			}
			continue
		case "alias", "import", "require", "use":
			start := phase8NextIdentifier(line.Tokens, 1, len(line.Tokens))
			if start >= 0 {
				end := start + 1
				for end+1 < len(line.Tokens) && line.Tokens[end].Text == "." && (line.Tokens[end+1].Kind == TokenIdentifier || line.Tokens[end+1].Kind == TokenKeyword) {
					end += 2
				}
				value := tokenRangeText(line.Tokens, start, end)
				if value != "" {
					state.addDependency(StructuralDependencyImport, value, line.Tokens[start].StartOffset, line.Tokens[end-1].EndOffset)
				}
			}
			continue
		}
		if elixirAnonymousBlock(first, line.Tokens) {
			scopes = append(scopes, elixirScope{kind: "block"})
		}
	}
	if (len(scopes) > 0 || pending != nil) && !state.stopped {
		_ = state.builder.AddDiagnostic(DiagnosticSpec{Code: "elixir-unterminated-block", Message: "Elixir source contains one or more blocks without matching end", Severity: DiagnosticWarning, AffectsCoverage: true})
	}
	return state.result()
}

func elixirHasDoToken(tokens []Token) bool {
	for _, token := range tokens {
		if strings.EqualFold(token.Text, "do") {
			return true
		}
	}
	return false
}

func elixirLineHasUnclosedDelimiter(tokens []Token) bool {
	balance := 0
	for _, token := range tokens {
		switch token.Text {
		case "(", "[", "{":
			balance++
		case ")", "]", "}":
			balance--
		}
	}
	return balance > 0
}

func elixirOpensBlock(tokens []Token) bool {
	for i := 0; i < len(tokens); i++ {
		if !strings.EqualFold(tokens[i].Text, "do") {
			continue
		}
		if i+1 < len(tokens) && tokens[i+1].Text == ":" {
			return false
		}
		return true
	}
	return false
}

func elixirAnonymousBlock(first string, tokens []Token) bool {
	fnDepth := 0
	for _, token := range tokens {
		switch strings.ToLower(token.Text) {
		case "fn":
			fnDepth++
		case "end":
			if fnDepth > 0 {
				fnDepth--
			}
		}
	}
	if fnDepth > 0 {
		return true
	}
	if !elixirOpensBlock(tokens) {
		return false
	}
	switch first {
	case "case", "cond", "defimpl", "defprotocol", "for", "if", "receive", "try", "unless", "with", "quote":
		return true
	}
	return false
}

func maskElixirSigils(text string) string {
	masked := []byte(text)
	for at := 0; at+2 < len(text); at++ {
		if text[at] != '~' || !phase8ElixirSigilLetter(text[at+1]) {
			continue
		}
		delimiterAt := at + 2
		end, ok := phase8ElixirSigilEnd(text, delimiterAt)
		if !ok {
			continue
		}
		phase8MaskRange(masked, at, end)
		at = end - 1
	}
	return string(masked)
}

func phase8ElixirSigilLetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func phase8ElixirSigilEnd(text string, delimiterAt int) (int, bool) {
	if delimiterAt >= len(text) {
		return 0, false
	}
	if delimiterAt+2 < len(text) && (text[delimiterAt] == '\'' || text[delimiterAt] == '"') && text[delimiterAt+1] == text[delimiterAt] && text[delimiterAt+2] == text[delimiterAt] {
		delimiter := text[delimiterAt : delimiterAt+3]
		if relative := strings.Index(text[delimiterAt+3:], delimiter); relative >= 0 {
			return delimiterAt + 3 + relative + 3, true
		}
		return 0, false
	}
	left := text[delimiterAt]
	right := left
	paired := true
	switch left {
	case '(':
		right = ')'
	case '[':
		right = ']'
	case '{':
		right = '}'
	case '<':
		right = '>'
	case '/', '|', '\'', '"':
		paired = false
	default:
		return 0, false
	}
	depth := 1
	for cursor := delimiterAt + 1; cursor < len(text); cursor++ {
		if text[cursor] == '\\' && cursor+1 < len(text) {
			cursor++
			continue
		}
		if paired && text[cursor] == left {
			depth++
			continue
		}
		if text[cursor] != right {
			continue
		}
		depth--
		if depth == 0 {
			return cursor + 1, true
		}
	}
	return 0, false
}

func (ErlangAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	state, err := newPhase8State(ctx, document, options, "erlang", AnalyzerErlang)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, err := state.scan(options, ErlangScannerProfile(), document.Text)
	if err != nil {
		return AnalyzerResult{}, err
	}
	segments := phase8TopLevelSegments(scan.Tokens, ".")
	var module *SymbolParent
	seenFunctions := make(map[string]struct{})
	for _, segment := range segments {
		if state.stopped || segment[0] >= segment[1] {
			continue
		}
		start := segment[0]
		for start < segment[1] && (scan.Tokens[start].Kind == TokenNewline || scan.Tokens[start].Kind == TokenDirective) {
			start++
		}
		if start >= segment[1] {
			continue
		}
		tokens := scan.Tokens[start:segment[1]]
		if len(tokens) == 0 {
			continue
		}
		if tokens[0].Text == "-" && len(tokens) > 1 {
			attribute := strings.ToLower(tokens[1].Text)
			open := phase8FindToken(tokens, 2, "(")
			close := -1
			if open >= 0 {
				close = phase8MatchingLocal(tokens, open, "(", ")")
			}
			if open < 0 || close <= open+1 {
				continue
			}
			switch attribute {
			case "module":
				idx := phase8NextIdentifier(tokens, open+1, close)
				if idx >= 0 {
					tok := tokens[idx]
					symbol, ok := state.add(SymbolSpec{Kind: SymbolKindModule, NativeKind: "module", Name: tok.Text, QualifiedName: tok.Text, Declaration: OffsetRange{Start: tokens[0].StartOffset, End: tokens[len(tokens)-1].EndOffset}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: tokens[0].StartOffset, End: tokens[len(tokens)-1].EndOffset}, Evidence: SymbolEvidenceStructural})
					if ok {
						module = &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
					}
				}
			case "include", "include_lib":
				for _, tok := range tokens[open+1 : close] {
					if value := phase8StringValue(tok); value != "" {
						state.addDependency(StructuralDependencyImport, value, tok.StartOffset, tok.EndOffset)
						break
					}
				}
			case "import", "behaviour", "behavior":
				idx := phase8NextIdentifier(tokens, open+1, close)
				if idx >= 0 {
					state.addDependency(StructuralDependencyImport, tokens[idx].Text, tokens[idx].StartOffset, tokens[idx].EndOffset)
				}
			case "record":
				idx := phase8NextIdentifier(tokens, open+1, close)
				if idx >= 0 {
					tok := tokens[idx]
					state.add(SymbolSpec{Kind: SymbolKindStruct, NativeKind: "record", Name: tok.Text, Parent: module, Declaration: OffsetRange{Start: tokens[0].StartOffset, End: tokens[len(tokens)-1].EndOffset}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Evidence: SymbolEvidenceStructural})
				}
			}
			continue
		}
		nameIndex := phase8NextIdentifier(tokens, 0, len(tokens))
		if nameIndex != 0 || phase8FindArrow(tokens) < 0 {
			continue
		}
		open := phase8FindToken(tokens, nameIndex+1, "(")
		if open < 0 {
			continue
		}
		close := phase8MatchingLocal(tokens, open, "(", ")")
		if close <= open {
			continue
		}
		arity := phase8Arity(tokens, open+1, close)
		key := tokens[nameIndex].Text + "/" + strconv.Itoa(arity)
		if _, duplicate := seenFunctions[key]; duplicate {
			continue
		}
		seenFunctions[key] = struct{}{}
		tok := tokens[nameIndex]
		state.add(SymbolSpec{Kind: SymbolKindFunction, NativeKind: "function", Name: tok.Text, Parent: module, Declaration: OffsetRange{Start: tokens[0].StartOffset, End: tokens[len(tokens)-1].EndOffset}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: tokens[0].StartOffset, End: tokens[close].EndOffset}, Evidence: SymbolEvidenceStructural, Disambiguator: key})
	}
	return state.result()
}

func (GleamAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	state, err := newPhase8State(ctx, document, options, "gleam", AnalyzerGleam)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, err := state.scan(options, GleamScannerProfile(), document.Text)
	if err != nil {
		return AnalyzerResult{}, err
	}
	for i := 0; i < len(scan.Tokens) && !state.stopped; {
		if scan.Tokens[i].Kind == TokenNewline || scan.Tokens[i].Kind == TokenEOF || scan.Tokens[i].Nesting != 0 {
			i++
			continue
		}
		start := i
		if strings.EqualFold(scan.Tokens[i].Text, "pub") {
			i++
			if i >= len(scan.Tokens) {
				break
			}
		}
		keyword := strings.ToLower(scan.Tokens[i].Text)
		switch keyword {
		case "import":
			lineEnd := phase8TokenLineEnd(scan.Tokens, i+1)
			targetEnd := lineEnd
			for j := i + 1; j < lineEnd; j++ {
				if strings.EqualFold(scan.Tokens[j].Text, "as") && scan.Tokens[j].Nesting == 0 {
					targetEnd = j
					break
				}
				if scan.Tokens[j].Text == "{" {
					targetEnd = j
					if targetEnd > i+1 && scan.Tokens[targetEnd-1].Text == "." {
						targetEnd--
					}
					break
				}
			}
			if targetEnd > i+1 {
				value := tokenRangeText(scan.Tokens, i+1, targetEnd)
				state.addDependency(StructuralDependencyImport, value, scan.Tokens[i+1].StartOffset, scan.Tokens[targetEnd-1].EndOffset)
			}
			i = max(lineEnd, i+1)
		case "type":
			idx := phase8NextIdentifier(scan.Tokens, i+1, len(scan.Tokens))
			if idx >= 0 {
				tok := scan.Tokens[idx]
				state.add(SymbolSpec{Kind: SymbolKindType, NativeKind: "type", Name: tok.Text, Declaration: OffsetRange{Start: scan.Tokens[start].StartOffset, End: phase8DeclarationLineEnd(scan.Tokens, start)}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: scan.Tokens[start].StartOffset, End: phase8DeclarationLineEnd(scan.Tokens, start)}, Evidence: SymbolEvidenceStructural})
			}
			i++
		case "fn":
			idx := phase8NextIdentifier(scan.Tokens, i+1, len(scan.Tokens))
			if idx >= 0 {
				tok := scan.Tokens[idx]
				state.add(SymbolSpec{Kind: SymbolKindFunction, NativeKind: "fn", Name: tok.Text, Declaration: OffsetRange{Start: scan.Tokens[start].StartOffset, End: phase8DeclarationLineEnd(scan.Tokens, start)}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: scan.Tokens[start].StartOffset, End: phase8DeclarationLineEnd(scan.Tokens, start)}, Evidence: SymbolEvidenceStructural})
			}
			i++
		case "const":
			idx := phase8NextIdentifier(scan.Tokens, i+1, len(scan.Tokens))
			if idx >= 0 {
				tok := scan.Tokens[idx]
				state.add(SymbolSpec{Kind: SymbolKindConstant, NativeKind: "const", Name: tok.Text, Declaration: OffsetRange{Start: scan.Tokens[start].StartOffset, End: phase8DeclarationLineEnd(scan.Tokens, start)}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Evidence: SymbolEvidenceStructural})
			}
			i++
		default:
			i++
		}
	}
	return state.result()
}

func phase8TopLevelSegments(tokens []Token, separator string) [][2]int {
	var result [][2]int
	start := 0
	for i, token := range tokens {
		if token.Kind == TokenEOF {
			if start < i {
				result = append(result, [2]int{start, i})
			}
			break
		}
		if token.Text == separator && token.Nesting == 0 {
			if start < i {
				result = append(result, [2]int{start, i})
			}
			start = i + 1
		}
	}
	return result
}

func phase8FindToken(tokens []Token, start int, text string) int {
	for i := start; i < len(tokens); i++ {
		if tokens[i].Text == text {
			return i
		}
	}
	return -1
}

func phase8MatchingLocal(tokens []Token, open int, left, right string) int {
	depth := 0
	for i := open; i < len(tokens); i++ {
		switch tokens[i].Text {
		case left:
			depth++
		case right:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func phase8FindArrow(tokens []Token) int {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Text == "->" || (tokens[i].Text == "-" && i+1 < len(tokens) && tokens[i+1].Text == ">") {
			return i
		}
	}
	return -1
}

func phase8Arity(tokens []Token, start, end int) int {
	if start >= end {
		return 0
	}
	arity := 1
	base := tokens[start].Nesting
	for i := start; i < end; i++ {
		if tokens[i].Text == "," && tokens[i].Nesting == base {
			arity++
		}
	}
	return arity
}

func phase8TokenLineEnd(tokens []Token, start int) int {
	for i := start; i < len(tokens); i++ {
		if tokens[i].Kind == TokenNewline || tokens[i].Kind == TokenEOF {
			return i
		}
	}
	return len(tokens)
}

func phase8DeclarationLineEnd(tokens []Token, start int) int {
	end := phase8TokenLineEnd(tokens, start)
	if end <= start {
		return tokens[start].EndOffset
	}
	return tokens[end-1].EndOffset
}
