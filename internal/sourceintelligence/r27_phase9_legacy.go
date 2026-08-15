package sourceintelligence

import (
	"context"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type FortranAnalyzer struct{}
type COBOLAnalyzer struct{}
type AdaAnalyzer struct{}

func (FortranAnalyzer) ID() AnalyzerID   { return AnalyzerFortran }
func (FortranAnalyzer) Language() string { return "fortran" }
func (COBOLAnalyzer) ID() AnalyzerID     { return AnalyzerCOBOL }
func (COBOLAnalyzer) Language() string   { return "cobol" }
func (AdaAnalyzer) ID() AnalyzerID       { return AnalyzerAda }
func (AdaAnalyzer) Language() string     { return "ada" }

func (FortranAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newPhase9Builder(ctx, document, options, "fortran", AnalyzerFortran)
	if err != nil {
		return AnalyzerResult{}, err
	}
	if strings.EqualFold(filepath.Ext(document.Path), ".f") {
		return analyzeFortranFixed(ctx, document, options, builder)
	}
	scan, lines, err := phase9ScanLogicalLines(ctx, document, FortranScannerProfile(), options.MaxNesting)
	if err != nil {
		return AnalyzerResult{}, err
	}
	phase9ApplyScanDiagnostics(builder, scan, "fortran")
	dependencies := []StructuralDependency{}
	var scopes []phase9Scope
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		phase9ParseFortranTokens(document, builder, line.Tokens, line.StartOffset, line.EndOffset, &scopes, &dependencies)
	}
	phase9MarkUnclosedScopes(builder, "fortran", scopes)
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func analyzeFortranFixed(ctx context.Context, document *SourceDocument, options AnalyzeOptions, builder *SymbolBuilder) (AnalyzerResult, error) {
	lines, err := BuildSourceLines(ctx, document, LineModelProfile{Kind: LineModelFixed, Fixed: FixedLineProfile{
		CommentColumnOne: []string{"C", "c", "*", "!"}, LabelStartColumn: 1, LabelEndColumn: 6,
		ContinuationColumn: 6, CodeStartColumn: 7, CodeEndColumn: 73,
	}}, LineModelLimits{MaxLines: max(4096, len(document.Text)+1), MaxLineBytes: 1024 * 1024})
	if err != nil {
		return AnalyzerResult{}, err
	}
	dependencies := []StructuralDependency{}
	var scopes []phase9Scope
	var statementRanges []OffsetRange
	statementStart := -1
	statementEnd := -1
	flush := func() error {
		if statementStart < 0 || len(statementRanges) == 0 {
			statementRanges = statementRanges[:0]
			statementStart = -1
			statementEnd = -1
			return nil
		}
		text := phase9MaskedFixedStatement(document.Text, statementStart, statementEnd, statementRanges)
		fake := &SourceDocument{Path: document.Path, Text: text, Encoding: "utf-8", lineStarts: buildLineStarts(text)}
		scan, logical, scanErr := phase9ScanLogicalLines(ctx, fake, FortranScannerProfile(), options.MaxNesting)
		if scanErr != nil {
			return scanErr
		}
		if !scan.Complete {
			builder.MarkIncomplete()
		}
		if len(logical) > 0 {
			tokens := append([]Token(nil), logical[0].Tokens...)
			for index := range tokens {
				tokens[index].StartOffset += statementStart
				tokens[index].EndOffset += statementStart
			}
			phase9ParseFortranTokens(document, builder, tokens, statementStart, statementEnd, &scopes, &dependencies)
		}
		statementRanges = statementRanges[:0]
		statementStart = -1
		statementEnd = -1
		return nil
	}
	for _, line := range lines {
		if line.Comment || line.Code.End <= line.Code.Start {
			continue
		}
		if !line.Continuation {
			if err := flush(); err != nil {
				return AnalyzerResult{}, err
			}
			statementStart = line.Code.Start
		} else if statementStart < 0 {
			builder.MarkIncomplete()
			statementStart = line.Code.Start
		}
		statementRanges = append(statementRanges, line.Code)
		statementEnd = line.Code.End
	}
	if err := flush(); err != nil {
		return AnalyzerResult{}, err
	}
	phase9MarkUnclosedScopes(builder, "fortran", scopes)
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func phase9MaskedFixedStatement(text string, start, end int, ranges []OffsetRange) string {
	if start < 0 || end < start || end > len(text) {
		return ""
	}
	result := make([]byte, end-start)
	for index := range result {
		result[index] = ' '
	}
	for _, value := range ranges {
		if value.Start < start || value.End > end || value.End <= value.Start {
			continue
		}
		copy(result[value.Start-start:value.End-start], text[value.Start:value.End])
	}
	return string(result)
}

func phase9ParseFortranTokens(document *SourceDocument, builder *SymbolBuilder, tokens []Token, start, end int, scopes *[]phase9Scope, dependencies *[]StructuralDependency) {
	if len(tokens) == 0 {
		return
	}
	first := strings.ToLower(tokens[0].Text)
	if first == "end" {
		label := ""
		if len(tokens) > 1 {
			label = strings.ToLower(tokens[1].Text)
		}
		if label == "module" || label == "program" || label == "submodule" || label == "type" {
			*scopes = phase9PopScope(*scopes, label, true)
		}
		return
	}
	if first == "use" {
		name := -1
		for index := 1; index < len(tokens); index++ {
			if strings.EqualFold(tokens[index].Text, "intrinsic") || strings.EqualFold(tokens[index].Text, "non_intrinsic") {
				continue
			}
			if tokens[index].Kind == TokenIdentifier {
				name = index
				break
			}
		}
		if name >= 0 {
			phase9AddDependency(document, dependencies, StructuralDependencyImport, tokens[name].Text, tokens[name].StartOffset, tokens[name].EndOffset)
		}
		return
	}
	parent := phase9ParentFromScopes(*scopes)
	switch first {
	case "module", "submodule", "program":
		if first == "module" && len(tokens) > 1 && strings.EqualFold(tokens[1].Text, "procedure") {
			return
		}
		nameIndex := phase9FirstIdentifier(tokens, 1)
		if nameIndex < 0 {
			return
		}
		kind := SymbolKindModule
		symbol, ok := phase9AddSymbol(builder, SymbolSpec{Kind: kind, NativeKind: first, Name: tokens[nameIndex].Text, Parent: parent,
			Declaration: OffsetRange{Start: start, End: end}, NameRange: OffsetRange{Start: tokens[nameIndex].StartOffset, End: tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: start, End: end}, Evidence: SymbolEvidenceStructural})
		if ok {
			*scopes = append(*scopes, phase9Scope{label: first, parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
		}
	case "type":
		if len(tokens) > 1 && tokens[1].Text == "(" {
			return
		}
		nameIndex := -1
		for index := 1; index < len(tokens); index++ {
			if tokens[index].Kind == TokenIdentifier && !strings.EqualFold(tokens[index].Text, "is") {
				nameIndex = index
			}
		}
		if nameIndex < 0 {
			return
		}
		symbol, ok := phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindType, NativeKind: "derived-type", Name: tokens[nameIndex].Text, Parent: parent,
			Declaration: OffsetRange{Start: start, End: end}, NameRange: OffsetRange{Start: tokens[nameIndex].StartOffset, End: tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: start, End: end}, Evidence: SymbolEvidenceStructural})
		if ok {
			*scopes = append(*scopes, phase9Scope{label: "type", parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
		}
	default:
		keyword := -1
		native := ""
		for index := 0; index < len(tokens); index++ {
			lower := strings.ToLower(tokens[index].Text)
			if lower == "subroutine" || lower == "function" {
				keyword = index
				native = lower
				break
			}
		}
		if keyword < 0 {
			return
		}
		nameIndex := phase9FirstIdentifier(tokens, keyword+1)
		if nameIndex < 0 {
			return
		}
		phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindFunction, NativeKind: native, Name: tokens[nameIndex].Text, Parent: parent,
			Declaration: OffsetRange{Start: start, End: end}, NameRange: OffsetRange{Start: tokens[nameIndex].StartOffset, End: tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: start, End: end}, Evidence: SymbolEvidenceStructural})
	}
}

func (COBOLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newPhase9Builder(ctx, document, options, "cobol", AnalyzerCOBOL)
	if err != nil {
		return AnalyzerResult{}, err
	}
	fixedFormat := !phase9COBOLLooksFreeForm(document.Text)
	lineProfile := LineModelProfile{Kind: LineModelFree}
	if fixedFormat {
		lineProfile = LineModelProfile{Kind: LineModelFixed, Fixed: FixedLineProfile{
			LabelStartColumn: 1, LabelEndColumn: 7, ContinuationColumn: 7, CodeStartColumn: 8, CodeEndColumn: 73,
		}}
	}
	lines, err := BuildSourceLines(ctx, document, lineProfile, LineModelLimits{MaxLines: max(4096, len(document.Text)+1), MaxLineBytes: 1024 * 1024})
	if err != nil {
		return AnalyzerResult{}, err
	}
	var program *SymbolParent
	dependencies := []StructuralDependency{}
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		if fixedFormat && cobolFixedComment(document.Text[line.Physical.Start:line.Physical.End]) || line.Code.End <= line.Code.Start {
			continue
		}
		code := strings.TrimSpace(document.Text[line.Code.Start:line.Code.End])
		var complete bool
		code, complete = phase9COBOLStripInlineComment(code)
		if !complete {
			builder.MarkIncomplete()
			value := line.Code
			_ = builder.AddDiagnostic(DiagnosticSpec{Code: "cobol-unterminated-string", Message: "COBOL source contains an unterminated quoted literal", Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
		}
		if code == "" {
			continue
		}
		upper := strings.ToUpper(code)
		switch {
		case strings.HasPrefix(upper, "PROGRAM-ID."):
			name := phase9COBOLWordAfter(code, "PROGRAM-ID.")
			if name == "" {
				continue
			}
			nameStart := phase9FindFoldRange(document.Text, line.Code, name)
			symbol, ok := phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindModule, NativeKind: "program-id", Name: name,
				Declaration: line.Code, NameRange: nameStart, Signature: &line.Code, Evidence: SymbolEvidenceStructural})
			if ok {
				value := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
				program = &value
			}
		case strings.HasSuffix(upper, " SECTION."):
			name := strings.TrimSpace(code[:len(code)-len(" SECTION.")])
			if name == "" || strings.ContainsAny(name, " \t") {
				continue
			}
			nameRange := phase9FindFoldRange(document.Text, line.Code, name)
			phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindFunction, NativeKind: "section", Name: name, Parent: program,
				Declaration: line.Code, NameRange: nameRange, Signature: &line.Code, Evidence: SymbolEvidenceStructural})
		}
		if index := strings.Index(upper, "COPY "); index >= 0 {
			rest := strings.TrimSpace(code[index+len("COPY "):])
			name := phase9COBOLLeadingWord(rest)
			if name != "" {
				absolute := line.Code.Start + index + len("COPY ") + strings.Index(code[index+len("COPY "):], name)
				phase9AddDependency(document, &dependencies, StructuralDependencyInclude, name, absolute, absolute+len(name))
			}
		}
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func phase9COBOLStripInlineComment(text string) (string, bool) {
	quote := byte(0)
	for index := 0; index < len(text); index++ {
		value := text[index]
		if quote != 0 {
			if value == quote {
				if index+1 < len(text) && text[index+1] == quote {
					index++
					continue
				}
				quote = 0
			}
			continue
		}
		if value == '\'' || value == '"' {
			quote = value
			continue
		}
		if value == '*' && index+1 < len(text) && text[index+1] == '>' {
			return strings.TrimSpace(text[:index]), true
		}
	}
	return strings.TrimSpace(text), quote == 0
}

func phase9COBOLLooksFreeForm(text string) bool {
	for start := 0; start < len(text); {
		end := start
		for end < len(text) && text[end] != '\r' && text[end] != '\n' {
			end++
		}
		trimmed := strings.TrimSpace(text[start:end])
		if trimmed != "" && !strings.HasPrefix(trimmed, "*>") {
			leading := len(text[start:end]) - len(strings.TrimLeft(text[start:end], " \t"))
			upper := strings.ToUpper(trimmed)
			return leading < 7 && (strings.HasPrefix(upper, "IDENTIFICATION ") || strings.HasPrefix(upper, "PROGRAM-ID.") || strings.HasPrefix(upper, "ENVIRONMENT ") || strings.HasPrefix(upper, "DATA ") || strings.HasPrefix(upper, "PROCEDURE "))
		}
		if end >= len(text) {
			break
		}
		if text[end] == '\r' && end+1 < len(text) && text[end+1] == '\n' {
			start = end + 2
		} else {
			start = end + 1
		}
	}
	return false
}

func cobolFixedComment(line string) bool {
	if utf8.RuneCountInString(line) < 7 {
		return false
	}
	offset := scalarColumnOffset(line, 7)
	if offset >= len(line) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(line[offset:])
	return r == '*' || r == '/' || r == 'D' || r == 'd'
}

func phase9COBOLWordAfter(text, prefix string) string {
	if len(text) < len(prefix) {
		return ""
	}
	return phase9COBOLLeadingWord(strings.TrimSpace(text[len(prefix):]))
}

func phase9COBOLLeadingWord(text string) string {
	end := 0
	for end < len(text) {
		value := text[end]
		if !(value == '-' || value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9') {
			break
		}
		end++
	}
	if end == 0 {
		return ""
	}
	return text[:end]
}

func phase9FindFoldRange(text string, scope OffsetRange, value string) OffsetRange {
	if scope.Start < 0 || scope.End > len(text) || scope.End <= scope.Start || value == "" {
		return OffsetRange{}
	}
	index := strings.Index(strings.ToLower(text[scope.Start:scope.End]), strings.ToLower(value))
	if index < 0 {
		return OffsetRange{}
	}
	start := scope.Start + index
	return OffsetRange{Start: start, End: start + len(value)}
}

func (AdaAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newPhase9Builder(ctx, document, options, "ada", AnalyzerAda)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, lines, err := phase9ScanLogicalLines(ctx, document, AdaScannerProfile(), options.MaxNesting)
	if err != nil {
		return AnalyzerResult{}, err
	}
	phase9ApplyScanDiagnostics(builder, scan, "ada")
	dependencies := []StructuralDependency{}
	var scopes []phase9Scope
	for _, line := range lines {
		if len(line.Tokens) == 0 {
			continue
		}
		first := strings.ToLower(line.Tokens[0].Text)
		if first == "with" {
			end := phase9LineEndToken(line.Tokens)
			for _, part := range splitTokenRangeAt(line.Tokens, 1, end, ",", line.Tokens[0].Nesting) {
				partEnd := part[1]
				for partEnd > part[0] && line.Tokens[partEnd-1].Text == ";" {
					partEnd--
				}
				if partEnd <= part[0] {
					continue
				}
				value := tokenRangeText(line.Tokens, part[0], partEnd)
				if value != "" {
					phase9AddDependency(document, &dependencies, StructuralDependencyImport, value, line.Tokens[part[0]].StartOffset, line.Tokens[partEnd-1].EndOffset)
				}
			}
			continue
		}
		if first == "end" {
			if len(scopes) > 0 && len(line.Tokens) > 1 {
				closing := line.Tokens[1].Text
				current := scopes[len(scopes)-1]
				if strings.EqualFold(closing, "package") || strings.EqualFold(closing, phase9QualifiedTail(current.parent.QualifiedName)) {
					scopes = scopes[:len(scopes)-1]
				}
			}
			continue
		}
		parent := phase9ParentFromScopes(scopes)
		if first == "package" {
			nameStart := 1
			if len(line.Tokens) > 1 && strings.EqualFold(line.Tokens[1].Text, "body") {
				nameStart = 2
			}
			nameIndex := phase9FirstIdentifier(line.Tokens, nameStart)
			if nameIndex < 0 {
				continue
			}
			symbol, ok := phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindPackage, NativeKind: "package", Name: line.Tokens[nameIndex].Text, Parent: parent,
				Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			if ok {
				scopes = append(scopes, phase9Scope{label: "package", parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
			}
			continue
		}
		if first == "type" || first == "subtype" || first == "task" || first == "protected" {
			nameIndex := phase9FirstIdentifier(line.Tokens, 1)
			if nameIndex >= 0 {
				phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindType, NativeKind: first, Name: line.Tokens[nameIndex].Text, Parent: parent,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			}
			continue
		}
		if first == "procedure" || first == "function" {
			nameIndex := phase9FirstIdentifier(line.Tokens, 1)
			if nameIndex >= 0 {
				phase9AddSymbol(builder, SymbolSpec{Kind: SymbolKindFunction, NativeKind: first, Name: line.Tokens[nameIndex].Text, Parent: parent,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			}
		}
	}
	phase9MarkUnclosedScopes(builder, "ada", scopes)
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}
