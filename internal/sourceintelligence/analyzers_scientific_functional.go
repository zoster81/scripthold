package sourceintelligence

import (
	"context"
	"strings"
	"unicode/utf8"
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
	scanDocument, unterminatedCharacterVector, err := phase9MaskMATLABCharacterVectors(ctx, document, octave)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, lines, err := phase9ScanLogicalLines(ctx, scanDocument, MATLABScannerProfile(language), options.MaxNesting)
	if err != nil {
		return AnalyzerResult{}, err
	}
	phase9ApplyScanDiagnostics(builder, scan, language)
	if unterminatedCharacterVector != nil {
		builder.MarkIncomplete()
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: language + "-unterminated-character-vector", Message: language + " source contains an unterminated single-quoted character vector", Severity: DiagnosticWarning, Range: unterminatedCharacterVector, AffectsCoverage: true})
	}
	dependencies := []StructuralDependency{}
	var scopes []phase9Scope
	functionFile := false
	firstCodeSeen := false
	explicitFunctionEndSeen := false
	implicitFunctionBoundaries := phase9MATLABLikeUsesImplicitFunctionBoundaries(lines, octave)
	for _, line := range lines {
		if len(line.Tokens) == 0 {
			continue
		}
		first := strings.ToLower(line.Tokens[0].Text)
		if !firstCodeSeen {
			functionFile = first == "function"
			firstCodeSeen = true
		}
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
			if len(scopes) == 0 {
				builder.MarkIncomplete()
				_ = builder.AddDiagnostic(DiagnosticSpec{Code: language + "-unmatched-scope-terminator", Message: language + " source contains an unmatched structural scope terminator", Severity: DiagnosticWarning, AffectsCoverage: true})
				continue
			}
			if scopes[len(scopes)-1].label == "function" {
				explicitFunctionEndSeen = true
			}
			scopes = scopes[:len(scopes)-1]
			continue
		}
		scopeOpeningLine := phase9MATLABLikeScopeOpeningTokens(line.Tokens, octave)
		if !scopeOpeningLine {
			phase9MATLABLikeVisitTrailingScopeTransitions(line.Tokens, octave, func(opening bool, label string) bool {
				if opening {
					scopes = append(scopes, phase9Scope{label: label})
					return true
				}
				if len(scopes) == 0 {
					builder.MarkIncomplete()
					_ = builder.AddDiagnostic(DiagnosticSpec{Code: language + "-unmatched-scope-terminator", Message: language + " source contains an unmatched structural scope terminator", Severity: DiagnosticWarning, AffectsCoverage: true})
					return false
				}
				if scopes[len(scopes)-1].label == "function" {
					explicitFunctionEndSeen = true
				}
				scopes = scopes[:len(scopes)-1]
				return true
			})
		}
		switch first {
		case "methods", "properties", "if", "for", "while", "switch", "try", "parfor", "spmd":
			if (first == "methods" || first == "properties") && !scopeOpeningLine {
				continue
			}
			if !phase9MATLABLikeLineClosesOwnScope(line.Tokens, octave) {
				scopes = append(scopes, phase9Scope{label: first})
				for _, label := range phase9MATLABLikeUnclosedTrailingScopes(line.Tokens, octave) {
					scopes = append(scopes, phase9Scope{label: label})
				}
			}
			continue
		case "arguments":
			if !octave && !phase9MATLABLikeLineClosesOwnScope(line.Tokens, false) {
				scopes = append(scopes, phase9Scope{label: first})
				for _, label := range phase9MATLABLikeUnclosedTrailingScopes(line.Tokens, false) {
					scopes = append(scopes, phase9Scope{label: label})
				}
			}
			continue
		case "unwind_protect":
			if octave && !phase9MATLABLikeLineClosesOwnScope(line.Tokens, true) {
				scopes = append(scopes, phase9Scope{label: first})
				for _, label := range phase9MATLABLikeUnclosedTrailingScopes(line.Tokens, true) {
					scopes = append(scopes, phase9Scope{label: label})
				}
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
			if implicitFunctionBoundaries && len(scopes) == 1 && scopes[0].label == "function" {
				scopes = scopes[:0]
			}
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
			if phase9MATLABLikeLineClosesOwnScope(line.Tokens, octave) {
				explicitFunctionEndSeen = true
				continue
			}
			functionScope := phase9Scope{label: "function"}
			if ok {
				functionScope.parent = SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
			}
			scopes = append(scopes, functionScope)
			for _, label := range phase9MATLABLikeUnclosedTrailingScopes(line.Tokens, octave) {
				scopes = append(scopes, phase9Scope{label: label})
			}
		}
	}
	if functionFile && len(scopes) == 1 && scopes[0].label == "function" && (octave || !explicitFunctionEndSeen) {
		scopes = scopes[:0]
	}
	phase9MarkUnclosedScopes(builder, language, scopes)
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func phase9MaskMATLABCharacterVectors(ctx context.Context, document *SourceDocument, octave bool) (*SourceDocument, *OffsetRange, error) {
	if document == nil || !strings.Contains(document.Text, "'") {
		return document, nil, nil
	}
	text := document.Text
	masked := []byte(text)
	changed := false
	var unterminated *OffsetRange
	for at := 0; at < len(text); {
		if at&0x3fff == 0 {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
		}
		if blockCommentDelimiterMatches(text, at, "%{", true) {
			at = phase9MATLABBlockCommentEnd(text, at)
			continue
		}
		if text[at] == '%' || octave && text[at] == '#' {
			for at < len(text) && text[at] != '\r' && text[at] != '\n' {
				at++
			}
			continue
		}
		if text[at] == '"' {
			end, _ := phase9MATLABQuotedEnd(text, at, '"')
			at = max(end, at+1)
			continue
		}
		if text[at] == '\'' && phase9MATLABCharacterVectorStart(text, at) {
			end, complete := phase9MATLABQuotedEnd(text, at, '\'')
			phase8MaskRange(masked, at, end)
			changed = true
			if !complete && unterminated == nil {
				value := OffsetRange{Start: at, End: end}
				unterminated = &value
			}
			at = max(end, at+1)
			continue
		}
		_, size := utf8.DecodeRuneInString(text[at:])
		at += max(size, 1)
	}
	if !changed {
		return document, unterminated, nil
	}
	clone := *document
	clone.Text = string(masked)
	clone.lineStarts = buildLineStarts(clone.Text)
	return &clone, unterminated, nil
}

func phase9MATLABBlockCommentEnd(text string, start int) int {
	for at := start + len("%{"); at < len(text); {
		relative := strings.Index(text[at:], "%}")
		if relative < 0 {
			return len(text)
		}
		closeOffset := at + relative
		if blockCommentDelimiterMatches(text, closeOffset, "%}", true) {
			return closeOffset + len("%}")
		}
		at = closeOffset + len("%}")
	}
	return len(text)
}

func phase9MATLABCharacterVectorStart(text string, at int) bool {
	if at < 0 || at >= len(text) || text[at] != '\'' {
		return false
	}
	if at == 0 {
		return true
	}
	previous := text[at-1]
	if previous == ' ' || previous == '\t' || previous == '\r' || previous == '\n' || previous == '\f' || previous == '\v' {
		return true
	}
	return strings.ContainsRune("([{,:;=+-*/\\^&|~<>", rune(previous))
}

func phase9MATLABQuotedEnd(text string, start int, delimiter byte) (int, bool) {
	if start < 0 || start >= len(text) || text[start] != delimiter {
		return start, false
	}
	for at := start + 1; at < len(text); {
		if text[at] == '\r' || text[at] == '\n' {
			return at, false
		}
		if text[at] == delimiter {
			if at+1 < len(text) && text[at+1] == delimiter {
				at += 2
				continue
			}
			return at + 1, true
		}
		_, size := utf8.DecodeRuneInString(text[at:])
		at += max(size, 1)
	}
	return len(text), false
}

func phase9MATLABLikeUsesImplicitFunctionBoundaries(lines []LogicalLine, octave bool) bool {
	var scopes []string
	boundaries := 0
	explicitFunctionEnd := false
	for _, line := range lines {
		if len(line.Tokens) == 0 {
			continue
		}
		first := strings.ToLower(line.Tokens[0].Text)
		if phase9MATLABLikeScopeTerminator(first, octave) {
			if len(scopes) == 0 {
				return false
			}
			if scopes[len(scopes)-1] == "function" {
				explicitFunctionEnd = true
			}
			scopes = scopes[:len(scopes)-1]
			continue
		}
		scopeOpeningLine := phase9MATLABLikeScopeOpeningTokens(line.Tokens, octave)
		if !scopeOpeningLine {
			complete := phase9MATLABLikeVisitTrailingScopeTransitions(line.Tokens, octave, func(opening bool, label string) bool {
				if opening {
					scopes = append(scopes, label)
					return true
				}
				if len(scopes) == 0 {
					return false
				}
				if scopes[len(scopes)-1] == "function" {
					explicitFunctionEnd = true
				}
				scopes = scopes[:len(scopes)-1]
				return true
			})
			if !complete {
				return false
			}
		}
		switch first {
		case "methods", "properties", "if", "for", "while", "switch", "try", "parfor", "spmd":
			if (first == "methods" || first == "properties") && !scopeOpeningLine {
				continue
			}
			if !phase9MATLABLikeLineClosesOwnScope(line.Tokens, octave) {
				scopes = append(scopes, first)
				scopes = append(scopes, phase9MATLABLikeUnclosedTrailingScopes(line.Tokens, octave)...)
			}
		case "arguments":
			if !octave && !phase9MATLABLikeLineClosesOwnScope(line.Tokens, false) {
				scopes = append(scopes, first)
				scopes = append(scopes, phase9MATLABLikeUnclosedTrailingScopes(line.Tokens, false)...)
			}
		case "unwind_protect":
			if octave && !phase9MATLABLikeLineClosesOwnScope(line.Tokens, true) {
				scopes = append(scopes, first)
				scopes = append(scopes, phase9MATLABLikeUnclosedTrailingScopes(line.Tokens, true)...)
			}
		case "classdef":
			scopes = append(scopes, "class")
		case "function":
			if phase9MATLABLikeLineClosesOwnScope(line.Tokens, octave) {
				explicitFunctionEnd = true
				continue
			}
			if len(scopes) == 1 && scopes[0] == "function" {
				boundaries++
				continue
			}
			scopes = append(scopes, "function")
			scopes = append(scopes, phase9MATLABLikeUnclosedTrailingScopes(line.Tokens, octave)...)
		}
	}
	return boundaries > 0 && !explicitFunctionEnd && len(scopes) == 1 && scopes[0] == "function"
}

func phase9MATLABLikeScopeOpeningLine(keyword string, octave bool) bool {
	switch keyword {
	case "classdef", "function", "methods", "properties", "if", "for", "while", "switch", "try", "parfor", "spmd":
		return true
	case "arguments":
		return !octave
	case "unwind_protect":
		return octave
	default:
		return false
	}
}

func phase9MATLABLikeScopeOpeningTokens(tokens []Token, octave bool) bool {
	if len(tokens) == 0 {
		return false
	}
	keyword := strings.ToLower(tokens[0].Text)
	if !phase9MATLABLikeScopeOpeningLine(keyword, octave) {
		return false
	}
	if keyword != "methods" && keyword != "properties" {
		return true
	}
	base := tokens[0].Nesting
	for index := 1; index < len(tokens); index++ {
		if tokens[index].Nesting == base && tokens[index].Text == "=" {
			return false
		}
	}
	return true
}

func phase9MATLABLikeVisitTrailingScopeTransitions(tokens []Token, octave bool, visit func(opening bool, label string) bool) bool {
	if len(tokens) < 2 || visit == nil {
		return true
	}
	base := tokens[0].Nesting
	for index := 1; index < len(tokens); index++ {
		if tokens[index].Nesting != base {
			continue
		}
		keyword := strings.ToLower(tokens[index].Text)
		if phase9MATLABLikeInlineScopeOpener(keyword, octave) {
			if !visit(true, keyword) {
				return false
			}
			continue
		}
		if phase9MATLABLikeScopeTerminator(keyword, octave) && !visit(false, "") {
			return false
		}
	}
	return true
}

func phase9MATLABLikeUnclosedTrailingScopes(tokens []Token, octave bool) []string {
	if len(tokens) < 2 {
		return nil
	}
	base := tokens[0].Nesting
	var scopes []string
	for index := 1; index < len(tokens); index++ {
		if tokens[index].Nesting != base {
			continue
		}
		keyword := strings.ToLower(tokens[index].Text)
		if phase9MATLABLikeInlineScopeOpener(keyword, octave) {
			scopes = append(scopes, keyword)
			continue
		}
		if !phase9MATLABLikeScopeTerminator(keyword, octave) {
			continue
		}
		if len(scopes) == 0 {
			break
		}
		scopes = scopes[:len(scopes)-1]
	}
	return scopes
}

func phase9MATLABLikeLineClosesOwnScope(tokens []Token, octave bool) bool {
	if len(tokens) < 2 {
		return false
	}
	base := tokens[0].Nesting
	depth := 1
	for index := 1; index < len(tokens); index++ {
		if tokens[index].Nesting != base {
			continue
		}
		keyword := strings.ToLower(tokens[index].Text)
		if phase9MATLABLikeInlineScopeOpener(keyword, octave) {
			depth++
			continue
		}
		if phase9MATLABLikeScopeTerminator(keyword, octave) {
			depth--
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

func phase9MATLABLikeInlineScopeOpener(keyword string, octave bool) bool {
	switch keyword {
	case "if", "for", "while", "switch", "try", "parfor", "spmd":
		return true
	case "unwind_protect":
		return octave
	default:
		return false
	}
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
	scanDocument, err := phase9MaskSingleQuotedCharacterLiterals(ctx, document, juliaCharacterLiteralEnd)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, lines, err := phase9ScanLogicalLines(ctx, scanDocument, JuliaScannerProfile(), options.MaxNesting)
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
			for _, part := range splitCommaTokenRangeAt(line.Tokens, 1, len(line.Tokens), line.Tokens[0].Nesting) {
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
				if ok && !phase9JuliaLineClosesOwnScope(line.Tokens, nameIndex+1) {
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
				if !phase9JuliaLineClosesOwnScope(line.Tokens, nameIndex+1) {
					scope := phase9Scope{label: "function"}
					if ok {
						scope.parent = SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
					}
					scopes = append(scopes, scope)
				}
			}
		default:
			if line.Tokens[0].Kind == TokenIdentifier && phase9JuliaCompactFunction(line.Tokens) {
				phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindFunction, NativeKind: "compact-function", Name: line.Tokens[0].Text, Parent: parent,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[0].StartOffset, End: line.Tokens[0].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
				continue
			}
			if first == "if" || first == "for" || first == "while" || first == "begin" || first == "let" || first == "try" || first == "quote" {
				if !phase9JuliaLineClosesOwnScope(line.Tokens, 1) {
					scopes = append(scopes, phase9Scope{label: first})
				}
			}
		}
	}
	phase9MarkUnclosedScopes(builder, "julia", scopes)
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func phase9JuliaLineClosesOwnScope(tokens []Token, bodyStart int) bool {
	if len(tokens) == 0 {
		return false
	}
	base := tokens[0].Nesting
	depth := 1
	for index := max(bodyStart, 1); index < len(tokens); index++ {
		if tokens[index].Nesting != base {
			continue
		}
		if strings.EqualFold(tokens[index].Text, "end") {
			depth--
			if depth == 0 {
				return true
			}
			continue
		}
		if phase9JuliaInlineBlockOpener(tokens, index, base) {
			depth++
		}
	}
	return false
}

func phase9JuliaInlineBlockOpener(tokens []Token, index, base int) bool {
	if index < 0 || index >= len(tokens) || tokens[index].Nesting != base {
		return false
	}
	switch strings.ToLower(tokens[index].Text) {
	case "if", "for", "while", "begin", "let", "try", "quote", "function", "macro", "module", "baremodule", "do":
		return true
	case "mutable":
		next := nextStructuralToken(tokens, index+1, len(tokens))
		return next < len(tokens) && tokens[next].Nesting == base && strings.EqualFold(tokens[next].Text, "struct")
	case "struct":
		previous := previousStructuralToken(tokens, index-1, 0)
		return previous < 0 || tokens[previous].Nesting != base || !strings.EqualFold(tokens[previous].Text, "mutable")
	default:
		return false
	}
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
	if ok && !phase9JuliaLineClosesOwnScope(line.Tokens, nameIndex+1) {
		*scopes = append(*scopes, phase9Scope{label: "struct", parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
	}
}

func phase9MaskSingleQuotedCharacterLiterals(ctx context.Context, document *SourceDocument, literalEnd func(string, int) (int, bool)) (*SourceDocument, error) {
	if document == nil || literalEnd == nil || !strings.Contains(document.Text, "'") {
		return document, nil
	}
	masked := []byte(document.Text)
	changed := false
	for at := 0; at < len(document.Text); {
		if at&0x3fff == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if document.Text[at] == '\'' {
			if end, ok := literalEnd(document.Text, at); ok {
				phase8MaskRange(masked, at, end)
				changed = true
				at = end
				continue
			}
		}
		_, size := utf8.DecodeRuneInString(document.Text[at:])
		at += max(size, 1)
	}
	if !changed {
		return document, nil
	}
	clone := *document
	clone.Text = string(masked)
	clone.lineStarts = buildLineStarts(clone.Text)
	return &clone, nil
}

func juliaCharacterLiteralEnd(text string, start int) (int, bool) {
	at := start + 1
	if start < 0 || start >= len(text) || text[start] != '\'' || at >= len(text) || text[at] == '\r' || text[at] == '\n' {
		return 0, false
	}
	if text[at] != '\\' {
		value, size := utf8.DecodeRuneInString(text[at:])
		if value == '\r' || value == '\n' || value == '\'' || size <= 0 {
			return 0, false
		}
		at += size
	} else {
		at++
		if at >= len(text) || text[at] == '\r' || text[at] == '\n' {
			return 0, false
		}
		switch text[at] {
		case 'x':
			at++
			startDigits := at
			for at < len(text) && at-startDigits < 2 && phase9ASCIIHexDigit(text[at]) {
				at++
			}
			if at == startDigits {
				return 0, false
			}
		case 'u':
			at++
			if !phase9ConsumeFixedHex(text, &at, 4) {
				return 0, false
			}
		case 'U':
			at++
			if !phase9ConsumeFixedHex(text, &at, 8) {
				return 0, false
			}
		default:
			if !strings.ContainsRune("0abefnrtv\\\"'", rune(text[at])) {
				return 0, false
			}
			at++
		}
	}
	if at >= len(text) || text[at] != '\'' {
		return 0, false
	}
	return at + 1, true
}

func haskellCharacterLiteralEnd(text string, start int) (int, bool) {
	at := start + 1
	if start < 0 || start >= len(text) || text[start] != '\'' || at >= len(text) || text[at] == '\r' || text[at] == '\n' {
		return 0, false
	}
	if text[at] != '\\' {
		value, size := utf8.DecodeRuneInString(text[at:])
		if value == '\r' || value == '\n' || value == '\'' || size <= 0 {
			return 0, false
		}
		at += size
	} else {
		at++
		if at >= len(text) || text[at] == '\r' || text[at] == '\n' {
			return 0, false
		}
		switch {
		case strings.ContainsRune("abfnrtv\\\"'", rune(text[at])):
			at++
		case text[at] >= '0' && text[at] <= '9':
			for at < len(text) && text[at] >= '0' && text[at] <= '9' {
				at++
			}
		case text[at] == 'o':
			at++
			startDigits := at
			for at < len(text) && text[at] >= '0' && text[at] <= '7' {
				at++
			}
			if at == startDigits {
				return 0, false
			}
		case text[at] == 'x':
			at++
			startDigits := at
			for at < len(text) && phase9ASCIIHexDigit(text[at]) {
				at++
			}
			if at == startDigits {
				return 0, false
			}
		case text[at] == '^':
			at++
			if at >= len(text) || text[at] < '@' || text[at] > '_' {
				return 0, false
			}
			at++
		case text[at] >= 'A' && text[at] <= 'Z':
			nameStart := at
			for at < len(text) && (text[at] >= 'A' && text[at] <= 'Z' || text[at] >= '0' && text[at] <= '9') {
				at++
			}
			if !phase9HaskellNamedCharacterEscape(text[nameStart:at]) {
				return 0, false
			}
		default:
			return 0, false
		}
	}
	if at >= len(text) || text[at] != '\'' {
		return 0, false
	}
	return at + 1, true
}

func ocamlCharacterLiteralEnd(text string, start int) (int, bool) {
	at := start + 1
	if start < 0 || start >= len(text) || text[start] != '\'' || at >= len(text) || text[at] == '\r' || text[at] == '\n' {
		return 0, false
	}
	if text[at] != '\\' {
		value, size := utf8.DecodeRuneInString(text[at:])
		if value == utf8.RuneError && size == 1 || value == '\r' || value == '\n' || value == '\'' || size <= 0 {
			return 0, false
		}
		at += size
	} else {
		at++
		if at >= len(text) || text[at] == '\r' || text[at] == '\n' {
			return 0, false
		}
		switch {
		case strings.ContainsRune("\\\"'ntbr ", rune(text[at])):
			at++
		case text[at] >= '0' && text[at] <= '9':
			if at+3 > len(text) {
				return 0, false
			}
			for index := 0; index < 3; index++ {
				if text[at+index] < '0' || text[at+index] > '9' {
					return 0, false
				}
			}
			at += 3
		case text[at] == 'x':
			at++
			if !phase9ConsumeFixedHex(text, &at, 2) {
				return 0, false
			}
		default:
			return 0, false
		}
	}
	if at >= len(text) || text[at] != '\'' {
		return 0, false
	}
	return at + 1, true
}

func phase9ConsumeFixedHex(text string, at *int, count int) bool {
	if at == nil || count <= 0 || *at < 0 || *at+count > len(text) {
		return false
	}
	for index := 0; index < count; index++ {
		if !phase9ASCIIHexDigit(text[*at+index]) {
			return false
		}
	}
	*at += count
	return true
}

func phase9ASCIIHexDigit(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func phase9HaskellNamedCharacterEscape(value string) bool {
	switch value {
	case "NUL", "SOH", "STX", "ETX", "EOT", "ENQ", "ACK", "BEL", "BS", "HT", "LF", "VT", "FF", "CR", "SO", "SI", "DLE", "DC1", "DC2", "DC3", "DC4", "NAK", "SYN", "ETB", "CAN", "EM", "SUB", "ESC", "FS", "GS", "RS", "US", "SP", "DEL":
		return true
	default:
		return false
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
	scanDocument, err := phase9MaskHaskellQuasiQuotes(ctx, document)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scanDocument, err = phase9MaskSingleQuotedCharacterLiterals(ctx, scanDocument, haskellCharacterLiteralEnd)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, lines, err := phase9ScanLogicalLines(ctx, scanDocument, HaskellScannerProfile(), options.MaxNesting)
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

func phase9MaskHaskellQuasiQuotes(ctx context.Context, document *SourceDocument) (*SourceDocument, error) {
	if document == nil || !strings.Contains(document.Text, "|]") {
		return document, nil
	}
	masked := []byte(document.Text)
	changed := false
	for at := 0; at < len(document.Text); {
		if at&0x3fff == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if document.Text[at] != '[' {
			_, size := utf8.DecodeRuneInString(document.Text[at:])
			at += max(size, 1)
			continue
		}
		contentStart, ok := phase9HaskellQuasiQuoteContentStart(document.Text, at)
		if !ok {
			at++
			continue
		}
		relativeEnd := strings.Index(document.Text[contentStart:], "|]")
		if relativeEnd < 0 {
			at = contentStart
			continue
		}
		end := contentStart + relativeEnd + 2
		phase8MaskRange(masked, at, end)
		changed = true
		at = end
	}
	if !changed {
		return document, nil
	}
	clone := *document
	clone.Text = string(masked)
	clone.lineStarts = buildLineStarts(clone.Text)
	return &clone, nil
}

func phase9HaskellQuasiQuoteContentStart(text string, start int) (int, bool) {
	cursor := start + 1
	if start < 0 || start >= len(text) || text[start] != '[' || cursor >= len(text) || !phase9HaskellQuoterStart(text[cursor]) {
		return 0, false
	}
	for cursor++; cursor < len(text); cursor++ {
		switch value := text[cursor]; {
		case value == '|':
			return cursor + 1, true
		case phase9HaskellQuoterContinue(value):
			continue
		default:
			return 0, false
		}
	}
	return 0, false
}

func phase9HaskellQuoterStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func phase9HaskellQuoterContinue(value byte) bool {
	return phase9HaskellQuoterStart(value) || value >= '0' && value <= '9' || value == '\'' || value == '.'
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
	scanDocument, err := phase9MaskSingleQuotedCharacterLiterals(ctx, document, ocamlCharacterLiteralEnd)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, lines, err := phase9ScanLogicalLines(ctx, scanDocument, OCamlScannerProfile(), options.MaxNesting)
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
