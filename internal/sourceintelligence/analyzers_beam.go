package sourceintelligence

import (
	"context"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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
	state, err := newStructuralAnalyzerState(ctx, document, options, "elixir", AnalyzerElixir)
	if err != nil {
		return AnalyzerResult{}, err
	}
	masked := maskElixirCharacterLiterals(maskElixirSigils(document.Text))
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
	lines := BuildLogicalLines(scan.Tokens, LogicalLineProfile{})
	for lineIndex, line := range lines {
		if state.stopped || len(line.Tokens) == 0 {
			continue
		}
		first := strings.ToLower(line.Tokens[0].Text)
		var declarationScope *elixirScope
		switch first {
		case "defmodule":
			start := nextIdentifierOrKeywordToken(line.Tokens, 1, len(line.Tokens))
			if start >= 0 {
				end := start + 1
				for end+1 < len(line.Tokens) && line.Tokens[end].Text == "." && (line.Tokens[end+1].Kind == TokenIdentifier || line.Tokens[end+1].Kind == TokenKeyword) {
					end += 2
				}
				name := tokenRangeText(line.Tokens, start, end)
				if name != "" {
					symbol, ok := state.add(SymbolSpec{Kind: SymbolKindModule, NativeKind: "defmodule", Name: name, QualifiedName: name, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[start].StartOffset, End: line.Tokens[end-1].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
					if ok {
						parent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
						scope := elixirScope{kind: "module", parent: parent}
						declarationScope = &scope
					}
				}
			}
		case "def", "defp", "defmacro", "defmacrop", "defguard", "defguardp":
			idx := nextIdentifierOrKeywordToken(line.Tokens, 1, len(line.Tokens))
			if idx >= 0 {
				tok := line.Tokens[idx]
				if _, ok := state.add(SymbolSpec{Kind: SymbolKindFunction, NativeKind: first, Name: tok.Text, Parent: currentModule(), Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural}); ok {
					scope := elixirScope{kind: "callable"}
					declarationScope = &scope
				}
			}
		case "alias", "import", "require", "use":
			start := nextIdentifierOrKeywordToken(line.Tokens, 1, len(line.Tokens))
			if start >= 0 {
				end := start + 1
				for end+1 < len(line.Tokens) && line.Tokens[end].Text == "." && (line.Tokens[end+1].Kind == TokenIdentifier || line.Tokens[end+1].Kind == TokenKeyword) {
					end += 2
				}
				value := tokenRangeText(line.Tokens, start, end)
				if value != "" {
					state.addImportDependency(value, line.Tokens[start].StartOffset, line.Tokens[end-1].EndOffset)
				}
			}
		}

		consumedDeclaration := elixirApplyStructuralTokens(line.Tokens, declarationScope, &pending, &scopes, state.builder)
		if declarationScope != nil && !consumedDeclaration && (elixirLineHasUnclosedDelimiter(line.Tokens) || elixirNextLineOpensGuard(lines, lineIndex)) {
			scope := *declarationScope
			pending = &scope
		}
	}
	if (len(scopes) > 0 || pending != nil) && !state.stopped {
		_ = state.builder.AddDiagnostic(DiagnosticSpec{Code: "elixir-unterminated-block", Message: "Elixir source contains one or more blocks without matching end", Severity: DiagnosticWarning, AffectsCoverage: true})
	}
	return state.result()
}

func elixirNextLineOpensGuard(lines []LogicalLine, lineIndex int) bool {
	for next := lineIndex + 1; next < len(lines); next++ {
		if len(lines[next].Tokens) == 0 {
			continue
		}
		return strings.EqualFold(lines[next].Tokens[0].Text, "when") && elixirOpensBlock(lines[next].Tokens)
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
	for index := range tokens {
		if elixirStructuralKeywordToken(tokens, index, "do") {
			return true
		}
	}
	return false
}

func elixirStructuralKeywordToken(tokens []Token, index int, value string) bool {
	if index < 0 || index >= len(tokens) || !strings.EqualFold(tokens[index].Text, value) {
		return false
	}
	if index > 0 && tokens[index-1].Text == ":" && tokens[index-1].EndOffset == tokens[index].StartOffset {
		return false
	}
	if index+1 < len(tokens) && tokens[index+1].Text == ":" && tokens[index].EndOffset == tokens[index+1].StartOffset {
		return false
	}
	return true
}

func elixirApplyStructuralTokens(tokens []Token, declarationScope *elixirScope, pending **elixirScope, scopes *[]elixirScope, builder *SymbolBuilder) bool {
	consumedDeclaration := false
	for index := range tokens {
		switch strings.ToLower(tokens[index].Text) {
		case "fn":
			if elixirStructuralKeywordToken(tokens, index, "fn") {
				*scopes = append(*scopes, elixirScope{kind: "block"})
			}
		case "do":
			if !elixirStructuralKeywordToken(tokens, index, "do") {
				if pending != nil && *pending != nil && index+1 < len(tokens) && tokens[index+1].Text == ":" && tokens[index].EndOffset == tokens[index+1].StartOffset {
					*pending = nil
				}
				continue
			}
			if declarationScope != nil && !consumedDeclaration {
				*scopes = append(*scopes, *declarationScope)
				consumedDeclaration = true
				continue
			}
			if pending != nil && *pending != nil {
				*scopes = append(*scopes, **pending)
				*pending = nil
				continue
			}
			*scopes = append(*scopes, elixirScope{kind: "block"})
		case "end":
			if !elixirStructuralKeywordToken(tokens, index, "end") {
				continue
			}
			if len(*scopes) == 0 {
				value := OffsetRange{Start: tokens[index].StartOffset, End: tokens[index].EndOffset}
				_ = builder.AddDiagnostic(DiagnosticSpec{Code: "elixir-unmatched-end", Message: "Elixir end has no matching structural block", Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
				continue
			}
			*scopes = (*scopes)[:len(*scopes)-1]
		}
	}
	return consumedDeclaration
}

func maskElixirCharacterLiterals(text string) string {
	masked := []byte(text)
	for at := 0; at < len(text); {
		switch text[at] {
		case '#':
			_, next := physicalLineBounds(text, at)
			at = next
			continue
		case '\'', '"':
			end, ok := erlangQuotedLiteralEnd(text, at)
			if !ok {
				return string(masked)
			}
			at = end
			continue
		case '?':
			if elixirQuestionMarkStartsCharacter(text, at) {
				if end, ok := elixirCharacterLiteralEnd(text, at); ok {
					maskRangePreservingLines(masked, at, end)
					at = end
					continue
				}
			}
		}
		_, size := utf8.DecodeRuneInString(text[at:])
		if size <= 0 {
			size = 1
		}
		at += size
	}
	return string(masked)
}

func elixirQuestionMarkStartsCharacter(text string, at int) bool {
	if at <= 0 {
		return true
	}
	previous, _ := utf8.DecodeLastRuneInString(text[:at])
	return !(previous == '_' || previous == '!' || previous == '?' || previous == '@' || unicode.IsLetter(previous) || unicode.IsDigit(previous) || unicode.IsMark(previous))
}

func elixirCharacterLiteralEnd(text string, start int) (int, bool) {
	at := start + 1
	if at >= len(text) || isNewlineStart(text[at]) {
		return 0, false
	}
	if text[at] == '\\' {
		at++
		if at >= len(text) || isNewlineStart(text[at]) {
			return 0, false
		}
		if (text[at] == 'x' || text[at] == 'u') && at+1 < len(text) && text[at+1] == '{' {
			if close := strings.IndexByte(text[at+2:], '}'); close >= 0 {
				return at + 2 + close + 1, true
			}
			return 0, false
		}
	}
	_, size := utf8.DecodeRuneInString(text[at:])
	if size <= 0 {
		return 0, false
	}
	return at + size, true
}

func maskElixirSigils(text string) string {
	masked := []byte(text)
	for at := 0; at+2 < len(text); at++ {
		if text[at] != '~' || !elixirSigilLetter(text[at+1]) {
			continue
		}
		delimiterAt := at + 2
		interpolated := text[at+1] >= 'a' && text[at+1] <= 'z'
		end, ok := elixirSigilEnd(text, delimiterAt, interpolated)
		if !ok {
			continue
		}
		maskRangePreservingLines(masked, at, end)
		at = end - 1
	}
	return string(masked)
}

func elixirSigilLetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func elixirSigilEnd(text string, delimiterAt int, interpolated bool) (int, bool) {
	if delimiterAt >= len(text) {
		return 0, false
	}
	if delimiterAt+2 < len(text) && (text[delimiterAt] == '\'' || text[delimiterAt] == '"') && text[delimiterAt+1] == text[delimiterAt] && text[delimiterAt+2] == text[delimiterAt] {
		delimiter := text[delimiterAt : delimiterAt+3]
		for cursor := delimiterAt + 3; cursor < len(text); {
			if interpolated && strings.HasPrefix(text[cursor:], "#{") {
				end, ok := elixirInterpolationEnd(text, cursor+2)
				if !ok {
					return 0, false
				}
				cursor = end
				continue
			}
			if strings.HasPrefix(text[cursor:], delimiter) {
				return cursor + len(delimiter), true
			}
			if text[cursor] == '\\' && cursor+1 < len(text) {
				cursor += 2
				continue
			}
			_, size := utf8.DecodeRuneInString(text[cursor:])
			if size <= 0 {
				size = 1
			}
			cursor += size
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
		if interpolated && strings.HasPrefix(text[cursor:], "#{") {
			end, ok := elixirInterpolationEnd(text, cursor+2)
			if !ok {
				return 0, false
			}
			cursor = end - 1
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

func elixirInterpolationEnd(text string, start int) (int, bool) {
	depth := 1
	for at := start; at < len(text); {
		switch text[at] {
		case '#':
			if strings.HasPrefix(text[at:], "#{") {
				depth++
				at += 2
				continue
			}
			_, next := physicalLineBounds(text, at)
			at = next
			continue
		case '\'', '"':
			end, ok := erlangQuotedLiteralEnd(text, at)
			if !ok {
				return 0, false
			}
			at = end
			continue
		case '?':
			if elixirQuestionMarkStartsCharacter(text, at) {
				if end, ok := elixirCharacterLiteralEnd(text, at); ok {
					at = end
					continue
				}
			}
		case '~':
			if at+2 < len(text) && elixirSigilLetter(text[at+1]) {
				end, ok := elixirSigilEnd(text, at+2, text[at+1] >= 'a' && text[at+1] <= 'z')
				if ok {
					at = end
					continue
				}
			}
		case '{':
			depth++
			at++
			continue
		case '}':
			depth--
			at++
			if depth == 0 {
				return at, true
			}
			continue
		}
		_, size := utf8.DecodeRuneInString(text[at:])
		if size <= 0 {
			size = 1
		}
		at += size
	}
	return 0, false
}

func maskErlangCharacterLiterals(text string) string {
	masked := []byte(text)
	for at := 0; at < len(text); {
		switch text[at] {
		case '%':
			_, next := physicalLineBounds(text, at)
			at = next
			continue
		case '\'', '"':
			end, ok := erlangQuotedLiteralEnd(text, at)
			if !ok {
				return string(masked)
			}
			at = end
			continue
		case '$':
			if end, ok := erlangCharacterLiteralEnd(text, at); ok {
				maskRangePreservingLines(masked, at, end)
				at = end
				continue
			}
		}
		_, size := utf8.DecodeRuneInString(text[at:])
		if size <= 0 {
			size = 1
		}
		at += size
	}
	return string(masked)
}

func erlangQuotedLiteralEnd(text string, start int) (int, bool) {
	quote := text[start]
	if quote == '"' {
		run := 1
		for start+run < len(text) && text[start+run] == quote {
			run++
		}
		if run >= 3 {
			delimiter := text[start : start+run]
			if relative := strings.Index(text[start+run:], delimiter); relative >= 0 {
				return start + run + relative + run, true
			}
			return len(text), false
		}
	}
	for at := start + 1; at < len(text); {
		if text[at] == '\\' {
			at++
			if at >= len(text) {
				break
			}
			if text[at] == '\r' && at+1 < len(text) && text[at+1] == '\n' {
				at += 2
				continue
			}
			_, size := utf8.DecodeRuneInString(text[at:])
			at += max(size, 1)
			continue
		}
		if text[at] == quote {
			return at + 1, true
		}
		_, size := utf8.DecodeRuneInString(text[at:])
		at += max(size, 1)
	}
	return len(text), false
}

func erlangCharacterLiteralEnd(text string, start int) (int, bool) {
	at := start + 1
	if at >= len(text) || text[at] == '\r' || text[at] == '\n' {
		return 0, false
	}
	if text[at] == '\\' {
		at++
		if at >= len(text) || text[at] == '\r' || text[at] == '\n' {
			return 0, false
		}
	}
	_, size := utf8.DecodeRuneInString(text[at:])
	if size <= 0 {
		return 0, false
	}
	return at + size, true
}

func (ErlangAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	state, err := newStructuralAnalyzerState(ctx, document, options, "erlang", AnalyzerErlang)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, err := state.scan(options, ErlangScannerProfile(), maskErlangCharacterLiterals(document.Text))
	if err != nil {
		return AnalyzerResult{}, err
	}
	segments := beamTopLevelSegments(scan.Tokens, ".")
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
			open := beamFindToken(tokens, 2, "(")
			close := -1
			if open >= 0 {
				close = beamMatchingLocal(tokens, open, "(", ")")
			}
			if open < 0 || close <= open+1 {
				continue
			}
			switch attribute {
			case "module":
				idx := nextIdentifierOrKeywordToken(tokens, open+1, close)
				if idx >= 0 {
					tok := tokens[idx]
					symbol, ok := state.add(SymbolSpec{Kind: SymbolKindModule, NativeKind: "module", Name: tok.Text, QualifiedName: tok.Text, Declaration: OffsetRange{Start: tokens[0].StartOffset, End: tokens[len(tokens)-1].EndOffset}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: tokens[0].StartOffset, End: tokens[len(tokens)-1].EndOffset}, Evidence: SymbolEvidenceStructural})
					if ok {
						module = &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
					}
				}
			case "include", "include_lib":
				for _, tok := range tokens[open+1 : close] {
					if value := quotedTokenStringValue(tok); value != "" {
						state.addImportDependency(value, tok.StartOffset, tok.EndOffset)
						break
					}
				}
			case "import", "behaviour", "behavior":
				idx := nextIdentifierOrKeywordToken(tokens, open+1, close)
				if idx >= 0 {
					state.addImportDependency(tokens[idx].Text, tokens[idx].StartOffset, tokens[idx].EndOffset)
				}
			case "record":
				idx := nextIdentifierOrKeywordToken(tokens, open+1, close)
				if idx >= 0 {
					tok := tokens[idx]
					state.add(SymbolSpec{Kind: SymbolKindStruct, NativeKind: "record", Name: tok.Text, Parent: module, Declaration: OffsetRange{Start: tokens[0].StartOffset, End: tokens[len(tokens)-1].EndOffset}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Evidence: SymbolEvidenceStructural})
				}
			}
			continue
		}
		nameIndex := nextIdentifierOrKeywordToken(tokens, 0, len(tokens))
		if nameIndex != 0 || beamFindArrow(tokens) < 0 {
			continue
		}
		open := beamFindToken(tokens, nameIndex+1, "(")
		if open < 0 {
			continue
		}
		close := beamMatchingLocal(tokens, open, "(", ")")
		if close <= open {
			continue
		}
		arity := beamArity(tokens, open+1, close)
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
	state, err := newStructuralAnalyzerState(ctx, document, options, "gleam", AnalyzerGleam)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, err := state.scan(options, GleamScannerProfile(), document.Text)
	if err != nil {
		return AnalyzerResult{}, err
	}
	pairs := PairDelimiterTokens(scan.Tokens, nil)
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
			lineEnd := beamTokenLineEnd(scan.Tokens, i+1)
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
				state.addImportDependency(value, scan.Tokens[i+1].StartOffset, scan.Tokens[targetEnd-1].EndOffset)
			}
			i = max(lineEnd, i+1)
		case "type":
			idx := nextIdentifierOrKeywordToken(scan.Tokens, i+1, len(scan.Tokens))
			if idx >= 0 {
				tok := scan.Tokens[idx]
				symbol, added := state.add(SymbolSpec{Kind: SymbolKindType, NativeKind: "type", Name: tok.Text, Declaration: OffsetRange{Start: scan.Tokens[start].StartOffset, End: beamDeclarationLineEnd(scan.Tokens, start)}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: scan.Tokens[start].StartOffset, End: beamDeclarationLineEnd(scan.Tokens, start)}, Evidence: SymbolEvidenceStructural})
				if added {
					if open, close, ok := gleamTypeBody(scan.Tokens, pairs, idx); ok {
						parent := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
						addGleamConstructors(state, scan.Tokens, pairs, open, close, &parent)
					}
				}
			}
			i++
		case "fn":
			idx := nextIdentifierOrKeywordToken(scan.Tokens, i+1, len(scan.Tokens))
			if idx >= 0 {
				tok := scan.Tokens[idx]
				state.add(SymbolSpec{Kind: SymbolKindFunction, NativeKind: "fn", Name: tok.Text, Declaration: OffsetRange{Start: scan.Tokens[start].StartOffset, End: beamDeclarationLineEnd(scan.Tokens, start)}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: scan.Tokens[start].StartOffset, End: beamDeclarationLineEnd(scan.Tokens, start)}, Evidence: SymbolEvidenceStructural})
			}
			i++
		case "const":
			idx := nextIdentifierOrKeywordToken(scan.Tokens, i+1, len(scan.Tokens))
			if idx >= 0 {
				tok := scan.Tokens[idx]
				state.add(SymbolSpec{Kind: SymbolKindConstant, NativeKind: "const", Name: tok.Text, Declaration: OffsetRange{Start: scan.Tokens[start].StartOffset, End: beamDeclarationLineEnd(scan.Tokens, start)}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Evidence: SymbolEvidenceStructural})
			}
			i++
		default:
			i++
		}
	}
	return state.result()
}

func gleamTypeBody(tokens []Token, pairs map[int]int, nameIndex int) (int, int, bool) {
	for index := nameIndex + 1; index < len(tokens); index++ {
		token := tokens[index]
		if token.Kind == TokenEOF {
			return 0, 0, false
		}
		if token.Text == "{" && token.Nesting == 1 {
			close, ok := pairs[index]
			return index, close, ok && close > index
		}
		if token.Kind != TokenNewline || token.Nesting != 0 {
			continue
		}
		next := index + 1
		for next < len(tokens) && tokens[next].Kind == TokenNewline {
			next++
		}
		if next < len(tokens) && tokens[next].Text == "{" && tokens[next].Nesting == 1 {
			close, ok := pairs[next]
			return next, close, ok && close > next
		}
		return 0, 0, false
	}
	return 0, 0, false
}

func addGleamConstructors(state *structuralAnalyzerState, tokens []Token, pairs map[int]int, open, close int, parent *SymbolParent) {
	if state == nil || parent == nil || open < 0 || close <= open || close >= len(tokens) {
		return
	}
	bodyNesting := tokens[open].Nesting
	for index := open + 1; index < close && !state.stopped; index++ {
		token := tokens[index]
		if token.Nesting != bodyNesting || token.Kind != TokenIdentifier || !gleamConstructorName(token.Text) {
			continue
		}
		declarationEnd := token.EndOffset
		if next := index + 1; next < close && tokens[next].Text == "(" && tokens[next].Nesting == bodyNesting+1 {
			if pair, ok := pairs[next]; ok && pair > next && pair < close {
				declarationEnd = tokens[pair].EndOffset
				index = pair
			}
		}
		declaration := OffsetRange{Start: token.StartOffset, End: declarationEnd}
		state.add(SymbolSpec{
			Kind: SymbolKindConstructor, NativeKind: "constructor", Name: token.Text, Parent: parent,
			Declaration: declaration, NameRange: OffsetRange{Start: token.StartOffset, End: token.EndOffset},
			Signature: &declaration, Evidence: SymbolEvidenceStructural,
		})
	}
}

func gleamConstructorName(name string) bool {
	first, _ := utf8.DecodeRuneInString(name)
	return first != utf8.RuneError && unicode.IsUpper(first)
}

func beamTopLevelSegments(tokens []Token, separator string) [][2]int {
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

func beamFindToken(tokens []Token, start int, text string) int {
	for i := start; i < len(tokens); i++ {
		if tokens[i].Text == text {
			return i
		}
	}
	return -1
}

func beamMatchingLocal(tokens []Token, open int, left, right string) int {
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

func beamFindArrow(tokens []Token) int {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Text == "->" || (tokens[i].Text == "-" && i+1 < len(tokens) && tokens[i+1].Text == ">") {
			return i
		}
	}
	return -1
}

func beamArity(tokens []Token, start, end int) int {
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

func beamTokenLineEnd(tokens []Token, start int) int {
	for i := start; i < len(tokens); i++ {
		if tokens[i].Kind == TokenNewline || tokens[i].Kind == TokenEOF {
			return i
		}
	}
	return len(tokens)
}

func beamDeclarationLineEnd(tokens []Token, start int) int {
	end := beamTokenLineEnd(tokens, start)
	if end <= start {
		return tokens[start].EndOffset
	}
	return tokens[end-1].EndOffset
}
