package sourceintelligence

import (
	"context"
	"strings"
)

type PerlAnalyzer struct{}
type LuaAnalyzer struct{}
type LuauAnalyzer struct{}
type GroovyAnalyzer struct{}

func (PerlAnalyzer) ID() AnalyzerID     { return AnalyzerPerl }
func (PerlAnalyzer) Language() string   { return "perl" }
func (LuaAnalyzer) ID() AnalyzerID      { return AnalyzerLua }
func (LuaAnalyzer) Language() string    { return "lua" }
func (LuauAnalyzer) ID() AnalyzerID     { return AnalyzerLuau }
func (LuauAnalyzer) Language() string   { return "luau" }
func (GroovyAnalyzer) ID() AnalyzerID   { return AnalyzerGroovy }
func (GroovyAnalyzer) Language() string { return "groovy" }

func (PerlAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	state, err := newPhase8State(ctx, document, options, "perl", AnalyzerPerl)
	if err != nil {
		return AnalyzerResult{}, err
	}
	masked, maskComplete := maskPerlNonCode(document.Text)
	scan, err := state.scan(options, PerlScannerProfile(), masked)
	if err != nil {
		return AnalyzerResult{}, err
	}
	if !maskComplete {
		state.builder.MarkIncomplete()
		_ = state.builder.AddDiagnostic(DiagnosticSpec{Code: "perl-unterminated-heredoc", Message: "Perl source contains an unterminated quoted heredoc", Severity: DiagnosticWarning, AffectsCoverage: true})
	}
	var currentPackage *SymbolParent
	lines := BuildLogicalLines(scan.Tokens, LogicalLineProfile{Separators: []string{";"}})
	for _, line := range lines {
		if state.stopped || len(line.Tokens) == 0 {
			continue
		}
		first := strings.ToLower(line.Tokens[0].Text)
		switch first {
		case "package":
			nameStart := phase8NextIdentifier(line.Tokens, 1, len(line.Tokens))
			if nameStart < 0 {
				continue
			}
			nameEnd := nameStart + 1
			for nameEnd < len(line.Tokens) {
				text := line.Tokens[nameEnd].Text
				if text == ":" || line.Tokens[nameEnd].Kind == TokenIdentifier || line.Tokens[nameEnd].Kind == TokenKeyword {
					nameEnd++
					continue
				}
				break
			}
			name := tokenRangeText(line.Tokens, nameStart, nameEnd)
			if name == "" {
				continue
			}
			symbol, ok := state.add(SymbolSpec{Kind: SymbolKindModule, NativeKind: "package", Name: name, QualifiedName: name, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameStart].StartOffset, End: line.Tokens[nameEnd-1].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			if ok {
				parent := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
				currentPackage = &parent
			}
		case "sub":
			nameIndex := phase8NextIdentifier(line.Tokens, 1, len(line.Tokens))
			if nameIndex < 0 {
				continue
			}
			tok := line.Tokens[nameIndex]
			state.add(SymbolSpec{Kind: SymbolKindFunction, NativeKind: "sub", Name: tok.Text, Parent: currentPackage, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
		case "use":
			idx := phase8NextIdentifier(line.Tokens, 1, len(line.Tokens))
			if idx >= 0 {
				end := idx + 1
				for end < len(line.Tokens) && (line.Tokens[end].Text == ":" || line.Tokens[end].Kind == TokenIdentifier || line.Tokens[end].Kind == TokenKeyword) {
					end++
				}
				value := tokenRangeText(line.Tokens, idx, end)
				lower := strings.ToLower(value)
				if value != "" && lower != "strict" && lower != "warnings" && lower != "feature" && lower != "lib" {
					state.addDependency(StructuralDependencyImport, value, line.Tokens[idx].StartOffset, line.Tokens[end-1].EndOffset)
				}
			}
		case "require":
			for _, token := range line.Tokens[1:] {
				if value := phase8StringValue(token); value != "" {
					state.addDependency(StructuralDependencyImport, value, token.StartOffset, token.EndOffset)
					break
				}
			}
		}
	}
	return state.result()
}

func (LuaAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeLuaFamily(ctx, document, options, "lua", AnalyzerLua, false)
}

func (LuauAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeLuaFamily(ctx, document, options, "luau", AnalyzerLuau, true)
}

func analyzeLuaFamily(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID, luau bool) (AnalyzerResult, error) {
	state, err := newPhase8State(ctx, document, options, language, analyzer)
	if err != nil {
		return AnalyzerResult{}, err
	}
	masked, maskComplete := maskLuaLongBrackets(document.Text)
	scan, err := state.scan(options, LuaScannerProfile(language), masked)
	if err != nil {
		return AnalyzerResult{}, err
	}
	if !maskComplete {
		state.builder.MarkIncomplete()
		_ = state.builder.AddDiagnostic(DiagnosticSpec{Code: language + "-unterminated-long-bracket", Message: language + " source contains an unterminated long-bracket string or comment", Severity: DiagnosticWarning, AffectsCoverage: true})
	}
	lines := BuildLogicalLines(scan.Tokens, LogicalLineProfile{Separators: []string{";"}})
	for _, line := range lines {
		if state.stopped || len(line.Tokens) == 0 {
			continue
		}
		functionIndex := -1
		if strings.EqualFold(line.Tokens[0].Text, "function") {
			functionIndex = 0
		} else if len(line.Tokens) > 1 && strings.EqualFold(line.Tokens[0].Text, "local") && strings.EqualFold(line.Tokens[1].Text, "function") {
			functionIndex = 1
		}
		nameStart, nameEnd := -1, -1
		nativeKind := "function"
		if functionIndex >= 0 {
			nameStart = phase8NextIdentifier(line.Tokens, functionIndex+1, len(line.Tokens))
			if nameStart >= 0 {
				nameEnd = nameStart + 1
				for nameEnd+1 < len(line.Tokens) && (line.Tokens[nameEnd].Text == "." || line.Tokens[nameEnd].Text == ":") && (line.Tokens[nameEnd+1].Kind == TokenIdentifier || line.Tokens[nameEnd+1].Kind == TokenKeyword) {
					nameEnd += 2
				}
			}
		} else if start, end, ok := phase8LuaAssignedFunctionName(line.Tokens); ok {
			nameStart, nameEnd = start, end
			nativeKind = "function-assignment"
		}
		if nameStart >= 0 && nameEnd > nameStart {
			name := tokenRangeText(line.Tokens, nameStart, nameEnd)
			if name != "" {
				state.add(SymbolSpec{Kind: SymbolKindFunction, NativeKind: nativeKind, Name: name, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameStart].StartOffset, End: line.Tokens[nameEnd-1].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			}
		}
		if luau {
			typeIndex := -1
			if strings.EqualFold(line.Tokens[0].Text, "type") {
				typeIndex = 0
			} else if len(line.Tokens) > 1 && strings.EqualFold(line.Tokens[0].Text, "export") && strings.EqualFold(line.Tokens[1].Text, "type") {
				typeIndex = 1
			}
			if typeIndex >= 0 {
				idx := phase8NextIdentifier(line.Tokens, typeIndex+1, len(line.Tokens))
				if idx >= 0 {
					tok := line.Tokens[idx]
					state.add(SymbolSpec{Kind: SymbolKindType, NativeKind: "type", Name: tok.Text, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
				}
			}
		}
	}
	for i := 0; i < len(scan.Tokens); i++ {
		if !strings.EqualFold(scan.Tokens[i].Text, "require") {
			continue
		}
		end := min(len(scan.Tokens), i+8)
		for j := i + 1; j < end; j++ {
			if scan.Tokens[j].Kind == TokenString {
				if value := phase8StringValue(scan.Tokens[j]); value != "" {
					state.addDependency(StructuralDependencyImport, value, scan.Tokens[j].StartOffset, scan.Tokens[j].EndOffset)
				}
				break
			}
			if scan.Tokens[j].Kind == TokenNewline || scan.Tokens[j].Kind == TokenEOF {
				break
			}
		}
	}
	return state.result()
}

func phase8LuaAssignedFunctionName(tokens []Token) (int, int, bool) {
	start := 0
	if len(tokens) > 0 && strings.EqualFold(tokens[0].Text, "local") {
		start = 1
	}
	if start >= len(tokens) || tokens[start].Kind != TokenIdentifier {
		return 0, 0, false
	}
	end := start + 1
	for end+1 < len(tokens) && (tokens[end].Text == "." || tokens[end].Text == ":") && (tokens[end+1].Kind == TokenIdentifier || tokens[end+1].Kind == TokenKeyword) {
		end += 2
	}
	equals := nextStructuralToken(tokens, end, len(tokens))
	if equals >= len(tokens) || tokens[equals].Text != "=" {
		return 0, 0, false
	}
	function := nextStructuralToken(tokens, equals+1, len(tokens))
	if function >= len(tokens) || !strings.EqualFold(tokens[function].Text, "function") {
		return 0, 0, false
	}
	return start, end, true
}

func (GroovyAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePhase7Brace(ctx, document, options, phase7BracePolicy{
		language: "groovy", analyzer: AnalyzerGroovy, profile: GroovyScannerProfile(),
		typeKinds:     map[string]SymbolKind{"class": SymbolKindClass, "interface": SymbolKindInterface, "trait": SymbolKindTrait, "enum": SymbolKindEnum},
		typeNative:    map[string]string{"class": "class", "interface": "interface", "trait": "trait", "enum": "enum"},
		modifiers:     setOf("abstract", "final", "private", "protected", "public", "static"),
		moduleKeyword: "package", moduleKind: SymbolKindPackage, lineTerminatedModule: true, importKeywords: setOf("import"),
		maskText: maskGroovySlashyStrings,
	})
}
