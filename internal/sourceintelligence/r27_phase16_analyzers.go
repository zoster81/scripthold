package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// ScalaAnalyzer provides bounded declaration-level Scala 2/3 structure. It
// recognizes both brace-owned and indentation-owned declaration scopes without
// attempting compiler/type/macro semantics.
type ScalaAnalyzer struct{}

func (ScalaAnalyzer) ID() AnalyzerID   { return AnalyzerScala }
func (ScalaAnalyzer) Language() string { return "scala" }

func (ScalaAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_scala_source", document.Path, err)
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: "scala", Analyzer: string(AnalyzerScala), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scanDocument := document
	masked := maskScalaTripleQuotedStrings(document.Text)
	masked = maskScalaSymbolLiterals(masked)
	if masked != document.Text {
		clone := *document
		clone.Text = masked
		scanDocument = &clone
	}
	scan, err := ScanSource(ctx, scanDocument, ScalaScannerProfile(), ScannerLimits{
		MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting,
	})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range scan.Diagnostics {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{
			Code: "scala-" + diagnostic.Code, Message: diagnostic.Message,
			Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true,
		})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	lines := BuildLogicalLines(scan.Tokens, LogicalLineProfile{Separators: []string{";"}, TrackIndentation: true, SkipDirectives: true})
	pairs := PairDelimiterTokens(scan.Tokens, nil)
	braceCloses := make(map[int]Token)
	for openIndex, closeIndex := range pairs {
		if openIndex < 0 || openIndex >= len(scan.Tokens) || closeIndex <= openIndex || closeIndex >= len(scan.Tokens) {
			continue
		}
		if scan.Tokens[openIndex].Text == "{" {
			braceCloses[scan.Tokens[openIndex].StartOffset] = scan.Tokens[closeIndex]
		}
	}
	parser := &scalaParser{ctx: ctx, document: document, lines: lines, builder: builder, braceCloses: braceCloses}
	parser.parseRange(0, len(lines), nil, "")
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_scala_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

type scalaParser struct {
	ctx          context.Context
	document     *SourceDocument
	lines        []LogicalLine
	builder      *SymbolBuilder
	braceCloses  map[int]Token
	packageRoot  *SymbolParent
	dependencies []StructuralDependency
	relations    []StructuralRelation
	stopped      bool
}

func (parser *scalaParser) parseRange(start, end int, parent *SymbolParent, parentKind string) {
	for index := start; index < end && !parser.stopped; {
		if parser.ctx.Err() != nil {
			return
		}
		line := parser.lines[index]
		if len(line.Tokens) == 0 {
			index++
			continue
		}
		rootParent := parent
		if rootParent == nil {
			rootParent = parser.packageRoot
		}
		if scalaTokenEqual(line.Tokens[0], "package") {
			parser.parsePackage(line)
			index++
			continue
		}
		if scalaTokenEqual(line.Tokens[0], "import") || scalaTokenEqual(line.Tokens[0], "export") {
			parser.parseImport(line)
			index++
			continue
		}
		kind, nativeKind, keywordIndex, nameIndex := scalaDeclaration(line.Tokens, parentKind)
		if kind == "" || nameIndex < 0 {
			index++
			continue
		}
		scopeEnd, body := parser.scope(index, end, keywordIndex, nativeKind)
		declarationEnd := line.EndOffset
		if body != nil {
			declarationEnd = body.End
		}
		nameToken := line.Tokens[nameIndex]
		modifiers := scalaModifiers(line.Tokens[:keywordIndex])
		symbol, err := parser.builder.Add(SymbolSpec{
			Kind: kind, NativeKind: nativeKind, Name: nameToken.Text, Parent: rootParent,
			Declaration: OffsetRange{Start: line.StartOffset, End: declarationEnd},
			NameRange:   OffsetRange{Start: nameToken.StartOffset, End: nameToken.EndOffset},
			Signature:   &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Body: body,
			Visibility: scalaVisibility(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
		})
		if operation.KindOf(err) == operation.KindLimit {
			parser.stopped = true
			return
		}
		if err != nil {
			parser.builder.MarkIncomplete()
			index = max(index+1, scopeEnd)
			continue
		}
		if nativeKind == "class" || nativeKind == "trait" || nativeKind == "object" || nativeKind == "enum" {
			parser.collectTypeRelations(symbol.QualifiedName, line.Tokens, nameIndex+1)
			if scopeEnd > index+1 {
				child := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
				parser.parseRange(index+1, scopeEnd, child, nativeKind)
			}
		}
		index = max(index+1, scopeEnd)
	}
}

func (parser *scalaParser) parsePackage(line LogicalLine) {
	if len(line.Tokens) < 2 {
		return
	}
	end := scalaHeaderEnd(line.Tokens, 1)
	if end <= 1 {
		return
	}
	name := tokenRangeText(line.Tokens, 1, end)
	if name == "" {
		return
	}
	nameStart := line.Tokens[1].StartOffset
	nameEnd := line.Tokens[end-1].EndOffset
	symbol, err := parser.builder.Add(SymbolSpec{
		Kind: SymbolKindPackage, NativeKind: "package", Name: name, QualifiedName: name,
		Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset},
		NameRange:   OffsetRange{Start: nameStart, End: nameEnd}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset},
		Evidence: SymbolEvidenceStructural,
	})
	if operation.KindOf(err) == operation.KindLimit {
		parser.stopped = true
		return
	}
	if err != nil {
		parser.builder.MarkIncomplete()
		return
	}
	parser.packageRoot = &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
}

func (parser *scalaParser) parseImport(line LogicalLine) {
	if len(line.Tokens) < 2 {
		return
	}
	end := scalaHeaderEnd(line.Tokens, 1)
	for index := 1; index < end; index++ {
		if line.Tokens[index].Text == "{" || line.Tokens[index].Text == "*" || line.Tokens[index].Text == "_" {
			end = index
			break
		}
	}
	if end <= 1 {
		return
	}
	value := tokenRangeText(line.Tokens, 1, end)
	value = strings.TrimSuffix(strings.TrimSpace(value), ".")
	if value == "" {
		return
	}
	rangeValue, err := parser.document.RangeFromUTF8Offsets(line.Tokens[1].StartOffset, line.Tokens[end-1].EndOffset)
	if err == nil {
		parser.dependencies = append(parser.dependencies, StructuralDependency{
			Kind: StructuralDependencyImport, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural,
		})
	}
}

func (parser *scalaParser) scope(index, end, keywordIndex int, nativeKind string) (int, *OffsetRange) {
	line := parser.lines[index]
	for _, token := range line.Tokens[keywordIndex+1:] {
		if token.Text != "{" {
			continue
		}
		close, ok := parser.braceCloses[token.StartOffset]
		if !ok {
			parser.builder.MarkIncomplete()
			return index + 1, nil
		}
		scopeEnd := index + 1
		for scopeEnd < end && parser.lines[scopeEnd].StartOffset < close.StartOffset {
			scopeEnd++
		}
		body := &OffsetRange{Start: token.StartOffset, End: close.EndOffset}
		return scopeEnd, body
	}
	if index+1 < end && parser.lines[index+1].Indent > line.Indent {
		scopeEnd := index + 1
		for scopeEnd < end && parser.lines[scopeEnd].Indent > line.Indent {
			scopeEnd++
		}
		body := &OffsetRange{Start: parser.lines[index+1].StartOffset, End: parser.lines[scopeEnd-1].EndOffset}
		return scopeEnd, body
	}
	if (nativeKind == "class" || nativeKind == "trait" || nativeKind == "object" || nativeKind == "enum") && scalaLineHasToken(line.Tokens, ":") {
		parser.builder.MarkIncomplete()
		_ = parser.builder.AddDiagnostic(DiagnosticSpec{
			Code: "scala-empty-scope", Message: "declaration has no indented or braced body",
			Severity: DiagnosticWarning, Range: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, AffectsCoverage: true,
		})
	}
	return index + 1, nil
}

func (parser *scalaParser) collectTypeRelations(source string, tokens []Token, start int) {
	for index := start; index < len(tokens); index++ {
		kind := ""
		switch tokens[index].Text {
		case "extends":
			kind = "inherits"
		case "with":
			kind = "mixin"
		}
		if kind == "" {
			continue
		}
		left := index + 1
		right := left
		for right < len(tokens) {
			text := tokens[right].Text
			if text == "with" || text == "derives" || text == ":" || text == "{" || text == "=" || text == "," {
				break
			}
			right++
		}
		if right <= left {
			continue
		}
		target := tokenRangeText(tokens, left, right)
		if target == "" {
			continue
		}
		rangeValue, err := parser.document.RangeFromUTF8Offsets(tokens[left].StartOffset, tokens[right-1].EndOffset)
		if err == nil {
			parser.relations = append(parser.relations, StructuralRelation{
				Kind: kind, Source: source, Target: target, Range: rangeValue, Evidence: SymbolEvidenceStructural,
			})
		}
		index = right - 1
	}
}

func scalaDeclaration(tokens []Token, parentKind string) (SymbolKind, string, int, int) {
	if len(tokens) < 2 {
		return "", "", -1, -1
	}
	keyword := -1
	for index, token := range tokens {
		switch token.Text {
		case "class", "trait", "object", "enum", "def", "val", "var", "type", "given":
			keyword = index
		}
		if keyword >= 0 {
			break
		}
		if !scalaModifier(token.Text) && token.Text != "case" {
			return "", "", -1, -1
		}
	}
	if keyword < 0 {
		return "", "", -1, -1
	}
	nameIndex := nextIdentifierToken(tokens, keyword+1, len(tokens))
	if nameIndex < 0 {
		return "", "", keyword, -1
	}
	native := tokens[keyword].Text
	switch native {
	case "class":
		return SymbolKindClass, native, keyword, nameIndex
	case "trait":
		return SymbolKindTrait, native, keyword, nameIndex
	case "object":
		return SymbolKindModule, native, keyword, nameIndex
	case "enum":
		return SymbolKindEnum, native, keyword, nameIndex
	case "def":
		if scalaTypeOwner(parentKind) {
			return SymbolKindMethod, native, keyword, nameIndex
		}
		return SymbolKindFunction, native, keyword, nameIndex
	case "val", "var", "given":
		if scalaTypeOwner(parentKind) {
			return SymbolKindField, native, keyword, nameIndex
		}
		return SymbolKindVariable, native, keyword, nameIndex
	case "type":
		return SymbolKindType, native, keyword, nameIndex
	default:
		return "", "", -1, -1
	}
}

func scalaTypeOwner(kind string) bool {
	switch kind {
	case "class", "trait", "object", "enum":
		return true
	default:
		return false
	}
}

func scalaModifier(value string) bool {
	switch value {
	case "abstract", "final", "implicit", "lazy", "open", "opaque", "override", "private", "protected", "sealed", "transparent", "inline":
		return true
	default:
		return false
	}
}

func scalaModifiers(tokens []Token) []string {
	result := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if scalaModifier(token.Text) || token.Text == "case" {
			result = append(result, token.Text)
		}
	}
	return result
}

func scalaVisibility(modifiers []string) Visibility {
	for _, modifier := range modifiers {
		switch modifier {
		case "private":
			return VisibilityPrivate
		case "protected":
			return VisibilityProtected
		}
	}
	return VisibilityPublic
}

func maskScalaTripleQuotedStrings(text string) string {
	masked := []byte(text)
	for at := 0; at+2 < len(text); {
		if text[at] != '"' || text[at+1] != '"' || text[at+2] != '"' || at > 0 && text[at-1] == '"' {
			at++
			continue
		}
		cursor := at + 3
		closed := false
		for cursor+2 < len(text) {
			if text[cursor] != '"' {
				cursor++
				continue
			}
			runEnd := cursor
			for runEnd < len(text) && text[runEnd] == '"' {
				runEnd++
			}
			if runEnd-cursor >= 3 {
				phase8MaskRange(masked, at, runEnd)
				at = runEnd
				closed = true
				break
			}
			cursor = runEnd
		}
		if !closed {
			at += 3
		}
	}
	return string(masked)
}

func maskScalaSymbolLiterals(text string) string {
	masked := []byte(text)
	for at := 0; at < len(text); at++ {
		if text[at] != '\'' || at+1 >= len(text) || !isScalaSymbolIdentifierByte(text[at+1]) {
			continue
		}
		end := at + 2
		for end < len(text) && isScalaSymbolIdentifierByte(text[end]) {
			end++
		}
		if end < len(text) && text[end] == '\'' {
			continue
		}
		phase8MaskRange(masked, at, end)
		at = end - 1
	}
	return string(masked)
}

func isScalaSymbolIdentifierByte(value byte) bool {
	return value == '_' || value == '$' || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func scalaTokenEqual(token Token, value string) bool { return token.Text == value }

func scalaLineHasToken(tokens []Token, value string) bool {
	for _, token := range tokens {
		if token.Text == value {
			return true
		}
	}
	return false
}

func scalaHeaderEnd(tokens []Token, start int) int {
	end := len(tokens)
	for index := start; index < len(tokens); index++ {
		switch tokens[index].Text {
		case ":", "{", "=":
			return index
		}
	}
	return end
}

// FlowAnalyzer reuses the proven typed-ECMAScript structural parser only after
// normalizing Flow-only syntax with byte-length-preserving masking. It does not
// claim TypeScript namespace/module semantics or Flow type checking.
type FlowAnalyzer struct{}

func (FlowAnalyzer) ID() AnalyzerID   { return AnalyzerFlow }
func (FlowAnalyzer) Language() string { return "flow" }

func (FlowAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_flow_source", document.Path, err)
	}
	masked := maskFlowOnlyKeywords(document.Text)
	clone := *document
	clone.Text = masked
	clone.lineStarts = buildLineStarts(masked)
	source, err := (TypeScriptAnalyzer{}).Analyze(ctx, &clone, options)
	if err != nil {
		return AnalyzerResult{}, err
	}
	filtered := source
	filtered.Analysis.Symbols = filtered.Analysis.Symbols[:0]
	for _, symbol := range source.Analysis.Symbols {
		if symbol.NativeKind == "namespace" || symbol.NativeKind == "module" {
			continue
		}
		filtered.Analysis.Symbols = append(filtered.Analysis.Symbols, symbol)
	}
	analysis, err := reprojectAnalyzerSymbols(ctx, document, filtered, options, "flow", AnalyzerFlow, "", 0, nil)
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range source.Analysis.Diagnostics {
		if strings.HasPrefix(diagnostic.Code, "typescript-") {
			diagnostic.Code = "flow-" + strings.TrimPrefix(diagnostic.Code, "typescript-")
		}
		if options.Limits.MaxDiagnostics > 0 && len(analysis.Diagnostics) >= options.Limits.MaxDiagnostics {
			analysis.DiagnosticsTruncated = true
			analysis.CoverageComplete = false
			break
		}
		analysis.Diagnostics = append(analysis.Diagnostics, diagnostic)
	}
	if source.Analysis.DiagnosticsTruncated {
		analysis.DiagnosticsTruncated = true
		analysis.CoverageComplete = false
	}
	for index := range analysis.Symbols {
		symbol := &analysis.Symbols[index]
		if symbol.Kind != SymbolKindType && symbol.Kind != SymbolKindAlias {
			continue
		}
		declaration, _, _, _ := symbol.SourceOffsets()
		if declaration.Start < 0 || declaration.End > len(document.Text) || declaration.End <= declaration.Start {
			continue
		}
		if strings.Contains(document.Text[declaration.Start:declaration.End], "opaque type") {
			symbol.NativeKind = "opaque-type"
		}
	}
	return AnalyzerResult{Analysis: analysis, Dependencies: source.Dependencies, Relations: source.Relations}, nil
}

func maskFlowOnlyKeywords(text string) string {
	bytes := []byte(text)
	for index := 0; index+len("opaque") <= len(bytes); index++ {
		if string(bytes[index:index+len("opaque")]) != "opaque" {
			continue
		}
		if index > 0 && isFlowIdentifierByte(bytes[index-1]) {
			continue
		}
		end := index + len("opaque")
		if end < len(bytes) && isFlowIdentifierByte(bytes[end]) {
			continue
		}
		next := end
		for next < len(bytes) && isFlowWhitespace(bytes[next]) {
			next++
		}
		if next+len("type") > len(bytes) || string(bytes[next:next+len("type")]) != "type" {
			continue
		}
		typeEnd := next + len("type")
		if typeEnd < len(bytes) && isFlowIdentifierByte(bytes[typeEnd]) {
			continue
		}
		for cursor := index; cursor < end; cursor++ {
			bytes[cursor] = ' '
		}
		index = end - 1
	}
	return string(bytes)
}

func isFlowIdentifierByte(value byte) bool {
	return value == '_' || value == '$' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func isFlowWhitespace(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

// ScalaScannerProfile covers the declaration-oriented Scala 2/3 lexical subset
// used by the Phase 16 recognizer. Newlines remain visible even inside braces so
// brace-owned and indentation-owned declarations can share one logical-line pass.
func ScalaScannerProfile() ScannerProfile {
	return ScannerProfile{
		Name: "scala",
		Keywords: []string{
			"abstract", "case", "class", "def", "derives", "enum", "export", "extends", "final", "given", "implicit", "import", "inline", "lazy", "object", "opaque", "open", "override", "package", "private", "protected", "sealed", "trait", "transparent", "type", "val", "var", "with",
		},
		Identifier:   DefaultIdentifierPolicy(),
		LineComments: []string{"//"},
		BlockComments: []BlockCommentRule{
			{Start: "/*", End: "*/", Nestable: true},
		},
		Strings: []StringRule{
			{Prefixes: []string{"s", "f", "raw", ""}, Delimiter: "\"\"\"", Multiline: true, BackslashEscapes: true},
			{Prefixes: []string{"s", "f", "raw", ""}, Delimiter: "\"", BackslashEscapes: true},
			{Prefixes: []string{""}, Delimiter: "'", BackslashEscapes: true},
		},
		Indentation: true,
	}
}
