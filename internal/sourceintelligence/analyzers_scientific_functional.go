package sourceintelligence

import (
	"context"
	"strings"
)

type MATLABAnalyzer struct{}
type OctaveAnalyzer struct{}
type JuliaAnalyzer struct{}
type RAnalyzer struct{}
type HaskellAnalyzer struct{}
type OCamlAnalyzer struct{}

func (MATLABAnalyzer) ID() AnalyzerID    { return AnalyzerMATLAB }
func (MATLABAnalyzer) Language() string  { return "matlab" }
func (OctaveAnalyzer) ID() AnalyzerID    { return AnalyzerOctave }
func (OctaveAnalyzer) Language() string  { return "octave" }
func (JuliaAnalyzer) ID() AnalyzerID     { return AnalyzerJulia }
func (JuliaAnalyzer) Language() string   { return "julia" }
func (RAnalyzer) ID() AnalyzerID         { return AnalyzerR }
func (RAnalyzer) Language() string       { return "r" }
func (HaskellAnalyzer) ID() AnalyzerID   { return AnalyzerHaskell }
func (HaskellAnalyzer) Language() string { return "haskell" }
func (OCamlAnalyzer) ID() AnalyzerID     { return AnalyzerOCaml }
func (OCamlAnalyzer) Language() string   { return "ocaml" }

func (MATLABAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeMATLABLike(ctx, document, options, "matlab", AnalyzerMATLAB, false)
}

func (OctaveAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeMATLABLike(ctx, document, options, "octave", AnalyzerOctave, true)
}

func analyzeMATLABLike(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID, octave bool) (AnalyzerResult, error) {
	builder, err := newPhase9Builder(ctx, document, options, language, analyzer)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, lines, err := phase9ScanLogicalLines(ctx, document, MATLABScannerProfile(language), options.MaxNesting)
	if err != nil {
		return AnalyzerResult{}, err
	}
	phase9ApplyScanDiagnostics(builder, scan, language)
	dependencies := []StructuralDependency{}
	var scopes []phase9Scope
	for _, line := range lines {
		if len(line.Tokens) == 0 {
			continue
		}
		first := strings.ToLower(line.Tokens[0].Text)
		if first == "import" {
			for index := 1; index < len(line.Tokens); index++ {
				if line.Tokens[index].Kind != TokenIdentifier {
					continue
				}
				phase9AddDependency(document, &dependencies, StructuralDependencyImport, line.Tokens[index].Text, line.Tokens[index].StartOffset, line.Tokens[index].EndOffset)
			}
			continue
		}
		if octave && first == "pkg" && len(line.Tokens) >= 3 && strings.EqualFold(line.Tokens[1].Text, "load") {
			nameIndex := phase9FirstIdentifier(line.Tokens, 2)
			if nameIndex >= 0 {
				phase9AddDependency(document, &dependencies, StructuralDependencyImport, line.Tokens[nameIndex].Text, line.Tokens[nameIndex].StartOffset, line.Tokens[nameIndex].EndOffset)
			}
			continue
		}
		if phase9MATLABLikeScopeTerminator(first, octave) {
			if len(scopes) > 0 {
				scopes = scopes[:len(scopes)-1]
			}
			continue
		}
		switch first {
		case "methods", "properties", "if", "for", "while", "switch", "try", "parfor", "spmd":
			scopes = append(scopes, phase9Scope{label: first})
			continue
		case "unwind_protect":
			if octave {
				scopes = append(scopes, phase9Scope{label: first})
			}
			continue
		case "classdef":
			nameIndex := phase9FirstIdentifier(line.Tokens, 1)
			if nameIndex < 0 {
				continue
			}
			parent := phase9ParentFromScopes(scopes)
			symbol, ok := phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindClass, NativeKind: "classdef", Name: line.Tokens[nameIndex].Text, Parent: parent,
				Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			if ok {
				scopes = append(scopes, phase9Scope{label: "class", parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
			}
			continue
		case "function":
			nameIndex := phase9MATLABFunctionName(line.Tokens)
			if nameIndex < 0 {
				continue
			}
			parent := phase9ParentFromScopes(scopes)
			kind := SymbolKindFunction
			if parent != nil {
				kind = SymbolKindMethod
			}
			symbol, ok := phase9AddSymbol(builder, SymbolSpec{Kind: kind, NativeKind: "function", Name: line.Tokens[nameIndex].Text, Parent: parent,
				Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			functionScope := phase9Scope{label: "function"}
			if ok {
				functionScope.parent = SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
			}
			scopes = append(scopes, functionScope)
		}
	}
	phase9MarkUnclosedScopes(builder, language, scopes)
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func phase9MATLABLikeScopeTerminator(keyword string, octave bool) bool {
	switch keyword {
	case "end", "endfunction", "endclassdef":
		return true
	}
	if !octave {
		return false
	}
	switch keyword {
	case "endif", "endfor", "endwhile", "endswitch", "end_try_catch", "endparfor", "endmethods", "endproperties", "end_unwind_protect":
		return true
	default:
		return false
	}
}

func phase9MATLABFunctionName(tokens []Token) int {
	if len(tokens) < 2 {
		return -1
	}
	for index := 1; index < len(tokens); index++ {
		if tokens[index].Text == "=" {
			return phase9FirstIdentifier(tokens, index+1)
		}
	}
	return phase9FirstIdentifier(tokens, 1)
}

func (JuliaAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newPhase9Builder(ctx, document, options, "julia", AnalyzerJulia)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, lines, err := phase9ScanLogicalLines(ctx, document, JuliaScannerProfile(), options.MaxNesting)
	if err != nil {
		return AnalyzerResult{}, err
	}
	phase9ApplyScanDiagnostics(builder, scan, "julia")
	dependencies := []StructuralDependency{}
	var scopes []phase9Scope
	for _, line := range lines {
		if len(line.Tokens) == 0 {
			continue
		}
		first := strings.ToLower(line.Tokens[0].Text)
		if first == "using" || first == "import" {
			for _, part := range splitTokenRangeAt(line.Tokens, 1, len(line.Tokens), ",", line.Tokens[0].Nesting) {
				name := phase9FirstIdentifier(line.Tokens, part[0])
				if name >= part[0] && name < part[1] {
					phase9AddDependency(document, &dependencies, StructuralDependencyImport, line.Tokens[name].Text, line.Tokens[name].StartOffset, line.Tokens[name].EndOffset)
				}
			}
			continue
		}
		if first == "end" {
			if len(scopes) > 0 {
				scopes = scopes[:len(scopes)-1]
			}
			continue
		}
		parent := phase9ParentFromScopes(scopes)
		switch first {
		case "module", "baremodule":
			nameIndex := phase9FirstIdentifier(line.Tokens, 1)
			if nameIndex >= 0 {
				symbol, ok := phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindModule, NativeKind: first, Name: line.Tokens[nameIndex].Text, Parent: parent,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
				if ok {
					scopes = append(scopes, phase9Scope{label: "module", parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
				}
			}
		case "struct":
			phase9AddJuliaType(builder, line, parent, false, &scopes)
		case "mutable":
			if len(line.Tokens) > 1 && strings.EqualFold(line.Tokens[1].Text, "struct") {
				phase9AddJuliaType(builder, line, parent, true, &scopes)
			}
		case "abstract", "primitive":
			if len(line.Tokens) > 1 && strings.EqualFold(line.Tokens[1].Text, "type") {
				nameIndex := phase9FirstIdentifier(line.Tokens, 2)
				if nameIndex >= 0 {
					phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindType, NativeKind: first + "-type", Name: line.Tokens[nameIndex].Text, Parent: parent,
						Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
				}
			}
		case "function", "macro":
			nameIndex := phase9FirstIdentifier(line.Tokens, 1)
			if nameIndex >= 0 {
				symbol, ok := phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindFunction, NativeKind: first, Name: line.Tokens[nameIndex].Text, Parent: parent,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
				scope := phase9Scope{label: "function"}
				if ok {
					scope.parent = SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
				}
				scopes = append(scopes, scope)
			}
		default:
			if line.Tokens[0].Kind == TokenIdentifier && phase9JuliaCompactFunction(line.Tokens) {
				phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindFunction, NativeKind: "compact-function", Name: line.Tokens[0].Text, Parent: parent,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[0].StartOffset, End: line.Tokens[0].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
				continue
			}
			if first == "if" || first == "for" || first == "while" || first == "begin" || first == "let" || first == "try" || first == "quote" {
				scopes = append(scopes, phase9Scope{label: first})
			}
		}
	}
	phase9MarkUnclosedScopes(builder, "julia", scopes)
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func phase9JuliaCompactFunction(tokens []Token) bool {
	if len(tokens) < 4 || tokens[1].Text != "(" {
		return false
	}
	pairs := PairDelimiterTokens(tokens, nil)
	close := pairs[1]
	return close > 1 && close+1 < len(tokens) && tokens[close+1].Text == "="
}

func phase9AddJuliaType(builder *SymbolBuilder, line LogicalLine, parent *SymbolParent, mutable bool, scopes *[]phase9Scope) {
	start := 1
	if mutable {
		start = 2
	}
	nameIndex := phase9FirstIdentifier(line.Tokens, start)
	if nameIndex < 0 {
		return
	}
	native := "struct"
	if mutable {
		native = "mutable-struct"
	}
	symbol, ok := phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindStruct, NativeKind: native, Name: line.Tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
	if ok {
		*scopes = append(*scopes, phase9Scope{label: "struct", parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
	}
}

func (RAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newPhase9Builder(ctx, document, options, "r", AnalyzerR)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, lines, err := phase9ScanLogicalLines(ctx, document, RScannerProfile(), options.MaxNesting)
	if err != nil {
		return AnalyzerResult{}, err
	}
	phase9ApplyScanDiagnostics(builder, scan, "r")
	dependencies := []StructuralDependency{}
	for _, line := range lines {
		if len(line.Tokens) == 0 {
			continue
		}
		first := strings.ToLower(line.Tokens[0].Text)
		if (first == "library" || first == "require") && len(line.Tokens) >= 3 {
			if tokenIndex, value, ok := phase9RStaticPackageArgument(line.Tokens); ok {
				phase9AddDependency(document, &dependencies, StructuralDependencyImport, value, line.Tokens[tokenIndex].StartOffset, line.Tokens[tokenIndex].EndOffset)
			}
			continue
		}
		if line.Tokens[0].Kind != TokenIdentifier {
			continue
		}
		assignment := -1
		for index := 1; index < len(line.Tokens); index++ {
			if line.Tokens[index].Text == "<-" || line.Tokens[index].Text == "=" {
				assignment = index
				break
			}
		}
		if assignment < 0 || phase9TokenIndexFold(line.Tokens, "function", assignment+1) < 0 {
			continue
		}
		phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindFunction, NativeKind: "assigned-function", Name: line.Tokens[0].Text,
			Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[0].StartOffset, End: line.Tokens[0].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func phase9RStaticPackageArgument(tokens []Token) (int, string, bool) {
	characterOnly := false
	for index := 1; index+2 < len(tokens); index++ {
		if strings.EqualFold(tokens[index].Text, "character.only") && tokens[index+1].Text == "=" && strings.EqualFold(tokens[index+2].Text, "TRUE") {
			characterOnly = true
			break
		}
	}
	for index := 1; index < len(tokens); index++ {
		if tokens[index].Text == "(" {
			continue
		}
		if tokens[index].Text == "," || tokens[index].Text == ")" {
			break
		}
		switch tokens[index].Kind {
		case TokenString:
			value := phase9StringTokenValue(tokens[index])
			return index, value, value != ""
		case TokenIdentifier:
			if characterOnly {
				return -1, "", false
			}
			return index, tokens[index].Text, true
		}
	}
	return -1, "", false
}

func (HaskellAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newPhase9Builder(ctx, document, options, "haskell", AnalyzerHaskell)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, lines, err := phase9ScanLogicalLines(ctx, document, HaskellScannerProfile(), options.MaxNesting)
	if err != nil {
		return AnalyzerResult{}, err
	}
	phase9ApplyScanDiagnostics(builder, scan, "haskell")
	dependencies := []StructuralDependency{}
	var module *SymbolParent
	seenFunctions := make(map[string]struct{})
	for _, line := range lines {
		if len(line.Tokens) == 0 {
			continue
		}
		first := strings.ToLower(line.Tokens[0].Text)
		if first == "module" {
			nameIndex := phase9FirstIdentifier(line.Tokens, 1)
			if nameIndex >= 0 {
				symbol, ok := phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindModule, NativeKind: "module", Name: line.Tokens[nameIndex].Text,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
				if ok {
					value := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
					module = &value
				}
			}
			continue
		}
		if first == "import" {
			nameIndex := phase9FirstIdentifier(line.Tokens, 1)
			if nameIndex >= 0 {
				phase9AddDependency(document, &dependencies, StructuralDependencyImport, line.Tokens[nameIndex].Text, line.Tokens[nameIndex].StartOffset, line.Tokens[nameIndex].EndOffset)
			}
			continue
		}
		if first == "data" || first == "newtype" || first == "type" || first == "class" {
			nameIndex := phase9FirstIdentifier(line.Tokens, 1)
			if nameIndex >= 0 {
				kind := SymbolKindType
				if first == "class" {
					kind = SymbolKindInterface
				}
				phase9AddSymbol(builder, SymbolSpec{Kind: kind, NativeKind: first, Name: line.Tokens[nameIndex].Text, Parent: module,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			}
			continue
		}
		if line.Tokens[0].Kind == TokenIdentifier && phase9HasDoubleColon(line.Tokens, 1) {
			name := line.Tokens[0].Text
			if _, exists := seenFunctions[name]; !exists {
				phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindFunction, NativeKind: "type-signature", Name: name, Parent: module,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[0].StartOffset, End: line.Tokens[0].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
				seenFunctions[name] = struct{}{}
			}
			continue
		}
		if line.Tokens[0].Kind == TokenIdentifier {
			equals := phase9TokenIndexFold(line.Tokens, "=", 1)
			if equals > 0 {
				name := line.Tokens[0].Text
				if _, exists := seenFunctions[name]; !exists {
					kind := SymbolKindVariable
					native := "value-binding"
					if equals > 1 {
						kind = SymbolKindFunction
						native = "function-binding"
					}
					phase9AddSymbol(builder, SymbolSpec{Kind: kind, NativeKind: native, Name: name, Parent: module,
						Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[0].StartOffset, End: line.Tokens[0].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
					seenFunctions[name] = struct{}{}
				}
			}
		}
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func phase9HasDoubleColon(tokens []Token, start int) bool {
	for index := max(start, 0); index+1 < len(tokens); index++ {
		if tokens[index].Text == ":" && tokens[index+1].Text == ":" {
			return true
		}
	}
	return false
}

func (OCamlAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newPhase9Builder(ctx, document, options, "ocaml", AnalyzerOCaml)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, lines, err := phase9ScanLogicalLines(ctx, document, OCamlScannerProfile(), options.MaxNesting)
	if err != nil {
		return AnalyzerResult{}, err
	}
	phase9ApplyScanDiagnostics(builder, scan, "ocaml")
	dependencies := []StructuralDependency{}
	var scopes []phase9Scope
	for _, line := range lines {
		if len(line.Tokens) == 0 {
			continue
		}
		first := strings.ToLower(line.Tokens[0].Text)
		if first == "open" || first == "include" {
			nameIndex := phase9FirstIdentifier(line.Tokens, 1)
			if nameIndex >= 0 {
				phase9AddDependency(document, &dependencies, StructuralDependencyImport, line.Tokens[nameIndex].Text, line.Tokens[nameIndex].StartOffset, line.Tokens[nameIndex].EndOffset)
			}
			continue
		}
		if first == "end" {
			if len(scopes) > 0 {
				scopes = scopes[:len(scopes)-1]
			}
			continue
		}
		parent := phase9ParentFromScopes(scopes)
		switch first {
		case "module":
			nameIndex := phase9FirstIdentifier(line.Tokens, 1)
			if nameIndex >= 0 {
				symbol, ok := phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindModule, NativeKind: "module", Name: line.Tokens[nameIndex].Text, Parent: parent,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
				if ok && phase9TokenIndexFold(line.Tokens, "struct", nameIndex+1) >= 0 {
					scopes = append(scopes, phase9Scope{label: "module", parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
				}
			}
		case "type":
			nameIndex := phase9FirstIdentifier(line.Tokens, 1)
			if nameIndex >= 0 {
				phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindType, NativeKind: "type", Name: line.Tokens[nameIndex].Text, Parent: parent,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			}
		case "class":
			nameIndex := phase9FirstIdentifier(line.Tokens, 1)
			if nameIndex >= 0 {
				phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindClass, NativeKind: "class", Name: line.Tokens[nameIndex].Text, Parent: parent,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			}
		case "let":
			start := 1
			if len(line.Tokens) > 1 && strings.EqualFold(line.Tokens[1].Text, "rec") {
				start = 2
			}
			nameIndex := phase9FirstIdentifier(line.Tokens, start)
			if nameIndex >= 0 {
				kind := SymbolKindVariable
				native := "value-binding"
				equals := phase9TokenIndexFold(line.Tokens, "=", nameIndex+1)
				if equals > nameIndex+1 && line.Tokens[nameIndex+1].Text != ":" || phase9TokenIndexFold(line.Tokens, "fun", nameIndex+1) >= 0 || phase9TokenIndexFold(line.Tokens, "function", nameIndex+1) >= 0 {
					kind = SymbolKindFunction
					native = "function-binding"
				}
				phase9AddSymbol(builder, SymbolSpec{Kind: kind, NativeKind: native, Name: line.Tokens[nameIndex].Text, Parent: parent,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			}
		}
	}
	phase9MarkUnclosedScopes(builder, "ocaml", scopes)
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}
