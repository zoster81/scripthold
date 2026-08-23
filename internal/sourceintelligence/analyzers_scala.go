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
	var interpolationExpressions []OffsetRange
	masked, interpolationExpressions = maskScalaInterpolatedStrings(masked)
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
	parser := &scalaParser{ctx: ctx, document: document, lines: lines, builder: builder, braceCloses: braceCloses, interpolationExpressions: interpolationExpressions}
	parser.parseRange(0, len(lines), nil, "")
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_scala_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

type scalaParser struct {
	ctx                      context.Context
	document                 *SourceDocument
	lines                    []LogicalLine
	builder                  *SymbolBuilder
	braceCloses              map[int]Token
	interpolationExpressions []OffsetRange
	packageRoot              *SymbolParent
	dependencies             []StructuralDependency
	relations                []StructuralRelation
	stopped                  bool
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
		signatureEnd := line.EndOffset
		if body != nil {
			declarationEnd = body.End
			if body.Start < signatureEnd {
				signatureEnd = body.Start
			}
		}
		nameToken := line.Tokens[nameIndex]
		modifiers := scalaModifiers(line.Tokens[:keywordIndex])
		symbol, err := parser.builder.Add(SymbolSpec{
			Kind: kind, NativeKind: nativeKind, Name: nameToken.Text, Parent: rootParent,
			Declaration: OffsetRange{Start: line.StartOffset, End: declarationEnd},
			NameRange:   OffsetRange{Start: nameToken.StartOffset, End: nameToken.EndOffset},
			Signature:   &OffsetRange{Start: line.StartOffset, End: signatureEnd}, Body: body,
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
	for tokenIndex := keywordIndex + 1; tokenIndex < len(line.Tokens); tokenIndex++ {
		token := line.Tokens[tokenIndex]
		if token.Text != "{" || scalaOffsetInRanges(token.StartOffset, parser.interpolationExpressions) || !scalaDeclarationOwnsBrace(nativeKind, line.Tokens, keywordIndex, tokenIndex) {
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
	lineIndent := scalaPhysicalIndent(parser.document.Text, line.StartOffset)
	if index+1 < end && scalaPhysicalIndent(parser.document.Text, parser.lines[index+1].StartOffset) > lineIndent {
		scopeEnd := index + 1
		for scopeEnd < end && scalaPhysicalIndent(parser.document.Text, parser.lines[scopeEnd].StartOffset) > lineIndent {
			scopeEnd++
		}
		body := &OffsetRange{Start: parser.lines[index+1].StartOffset, End: parser.lines[scopeEnd-1].EndOffset}
		return scopeEnd, body
	}
	if (nativeKind == "class" || nativeKind == "trait" || nativeKind == "object" || nativeKind == "enum") && scalaLineHasTokenAtNesting(line.Tokens, ":", line.Tokens[keywordIndex].Nesting) {
		parser.builder.MarkIncomplete()
		_ = parser.builder.AddDiagnostic(DiagnosticSpec{
			Code: "scala-empty-scope", Message: "declaration has no indented or braced body",
			Severity: DiagnosticWarning, Range: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, AffectsCoverage: true,
		})
	}
	return index + 1, nil
}

func scalaDeclarationOwnsBrace(nativeKind string, tokens []Token, keywordIndex, openIndex int) bool {
	switch nativeKind {
	case "val", "var", "type":
		return false
	case "def":
		previous := previousStructuralToken(tokens, openIndex-1, keywordIndex)
		if previous < keywordIndex {
			return false
		}
		if tokens[previous].Text == "=" {
			return true
		}
		if tokens[previous].Text != ")" {
			return false
		}
		return !scalaLineHasTokenAtNesting(tokens[keywordIndex+1:openIndex], "=", tokens[keywordIndex].Nesting)
	default:
		return true
	}
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
