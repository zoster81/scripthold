package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// VBNetAnalyzer performs case-insensitive, explicit-End declaration analysis.
type VBNetAnalyzer struct{}

func (VBNetAnalyzer) ID() AnalyzerID   { return AnalyzerVBNet }
func (VBNetAnalyzer) Language() string { return "vbnet" }

func (VBNetAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_vbnet_source", document.Path, err)
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: "vbnet", Analyzer: string(AnalyzerVBNet), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxTokens := scannerTokenBudget(document.Text)
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, document, VBNetScannerProfile(), ScannerLimits{MaxTokens: maxTokens, MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range scan.Diagnostics {
		rangeValue := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: "vbnet-" + diagnostic.Code, Message: diagnostic.Message, Severity: DiagnosticWarning, Range: &rangeValue, AffectsCoverage: true})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	statements := buildVBStatements(scan.Tokens)
	parser := &vbnetParser{ctx: ctx, document: document, statements: statements, builder: builder, scopePairs: pairVBNamedScopes(statements)}
	parser.parseRange(0, len(statements), nil, "", "")
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_vbnet_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

type vbStatement struct {
	tokens      []Token
	startOffset int
	endOffset   int
}

type vbScopeOpen struct {
	index int
	label string
}

type vbnetParser struct {
	ctx          context.Context
	document     *SourceDocument
	statements   []vbStatement
	builder      *SymbolBuilder
	scopePairs   map[int]int
	dependencies []StructuralDependency
	relations    []StructuralRelation
	stopped      bool
}

var vbModifiers = map[string]struct{}{
	"public": {}, "private": {}, "protected": {}, "friend": {}, "shared": {}, "partial": {}, "mustinherit": {},
	"notinheritable": {}, "overloads": {}, "overrides": {}, "overridable": {}, "notoverridable": {}, "mustoverride": {},
	"readonly": {}, "writeonly": {}, "async": {}, "iterator": {}, "default": {}, "shadows": {}, "custom": {}, "declare": {},
	"ansi": {}, "auto": {}, "unicode": {}, "widening": {}, "narrowing": {},
}

func buildVBStatements(tokens []Token) []vbStatement {
	lines := BuildLogicalLines(tokens, LogicalLineProfile{Separators: []string{":"}, SkipDirectives: true})
	result := make([]vbStatement, 0, len(lines))
	for _, line := range lines {
		result = append(result, vbStatement{tokens: line.Tokens, startOffset: line.StartOffset, endOffset: line.EndOffset})
	}
	return result
}

func pairVBNamedScopes(statements []vbStatement) map[int]int {
	pairs := make(map[int]int)
	var stack []vbScopeOpen
	for index, statement := range statements {
		if label := vbTypeScopeLabel(statement); label != "" {
			stack = append(stack, vbScopeOpen{index: index, label: label})
			continue
		}
		if closeLabel := vbEndLabel(statement); closeLabel != "" && len(stack) > 0 {
			top := stack[len(stack)-1]
			if strings.EqualFold(top.label, closeLabel) {
				stack = stack[:len(stack)-1]
				pairs[top.index] = index
			}
		}
	}
	return pairs
}

func vbTypeScopeLabel(statement vbStatement) string {
	_, keyword := vbDeclarationKeyword(statement)
	switch keyword {
	case "namespace", "module", "class", "structure", "interface", "enum":
		return keyword
	default:
		return ""
	}
}

func vbEndLabel(statement vbStatement) string {
	if len(statement.tokens) < 2 || !strings.EqualFold(statement.tokens[0].Text, "end") {
		return ""
	}
	return strings.ToLower(statement.tokens[1].Text)
}

func (parser *vbnetParser) parseRange(start, end int, parent *SymbolParent, ownerName, ownerKind string) {
	for index := start; index < end && !parser.stopped; {
		if parser.ctx.Err() != nil {
			return
		}
		statement := parser.statements[index]
		if len(statement.tokens) == 0 {
			index++
			continue
		}
		if strings.EqualFold(statement.tokens[0].Text, "imports") {
			parser.parseImports(statement)
			index++
			continue
		}
		if strings.EqualFold(statement.tokens[0].Text, "inherits") || strings.EqualFold(statement.tokens[0].Text, "implements") {
			parser.parseRelation(statement, parent)
			index++
			continue
		}
		keywordIndex, keyword := vbDeclarationKeyword(statement)
		switch keyword {
		case "namespace", "module", "class", "structure", "interface", "enum":
			closeIndex, hasClose := parser.scopePairs[index]
			if !hasClose {
				closeIndex = index
				parser.builder.MarkIncomplete()
				_ = parser.builder.AddDiagnostic(DiagnosticSpec{Code: "vbnet-missing-end", Message: "declaration is missing End " + keyword, Severity: DiagnosticWarning, Range: &OffsetRange{Start: statement.startOffset, End: statement.endOffset}, AffectsCoverage: true})
			}
			symbol, added := parser.addType(statement, keywordIndex, keyword, closeIndex, parent)
			if !added {
				index++
				continue
			}
			child := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
			if hasClose {
				parser.parseRange(index+1, closeIndex, child, symbol.Name, keyword)
				index = closeIndex + 1
			} else {
				parser.parseRange(index+1, end, child, symbol.Name, keyword)
				return
			}
			continue
		case "sub", "function", "property":
			index = parser.parseCallable(index, end, statement, keywordIndex, keyword, parent, ownerName, ownerKind)
			continue
		case "event":
			index = parser.parseEvent(index, end, statement, keywordIndex, parent)
			continue
		case "const":
			parser.addSingleLine(statement, keywordIndex, SymbolKindConstant, "const", parent)
			index++
			continue
		}
		if ownerKind != "" && ownerKind != "namespace" && vbLooksLikeField(statement, keywordIndex) {
			parser.addField(statement, parent)
		}
		index++
	}
}

func vbDeclarationKeyword(statement vbStatement) (int, string) {
	for index, token := range statement.tokens {
		text := strings.ToLower(token.Text)
		if _, modifier := vbModifiers[text]; modifier {
			continue
		}
		if text == "dim" {
			return index, text
		}
		switch text {
		case "namespace", "module", "class", "structure", "interface", "enum", "sub", "function", "property", "event", "const":
			return index, text
		default:
			return index, text
		}
	}
	return -1, ""
}

func (parser *vbnetParser) addType(statement vbStatement, keywordIndex int, keyword string, closeIndex int, parent *SymbolParent) (NormalizedSymbol, bool) {
	nameIndex := vbNextIdentifier(statement, keywordIndex+1)
	if nameIndex < 0 {
		parser.builder.MarkIncomplete()
		return NormalizedSymbol{}, false
	}
	kind := map[string]SymbolKind{"namespace": SymbolKindNamespace, "module": SymbolKindModule, "class": SymbolKindClass, "structure": SymbolKindStruct, "interface": SymbolKindInterface, "enum": SymbolKindEnum}[keyword]
	declarationEnd := statement.endOffset
	var body *OffsetRange
	if closeIndex > 0 && closeIndex < len(parser.statements) && closeIndex != parser.statementIndex(statement) {
		declarationEnd = parser.statements[closeIndex].endOffset
		value := OffsetRange{Start: statement.endOffset, End: parser.statements[closeIndex].startOffset}
		body = &value
	}
	modifiers := vbCollectModifiers(statement, keywordIndex)
	symbol, err := parser.builder.Add(SymbolSpec{
		Kind: kind, NativeKind: keyword, Name: statement.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: statement.startOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: statement.tokens[nameIndex].StartOffset, End: statement.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: statement.startOffset, End: statement.endOffset}, Body: body,
		Visibility: vbVisibility(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
	})
	if operation.KindOf(err) == operation.KindLimit {
		parser.stopped = true
		return NormalizedSymbol{}, false
	}
	if err != nil {
		parser.builder.MarkIncomplete()
		return NormalizedSymbol{}, false
	}
	return symbol, true
}

func (parser *vbnetParser) statementIndex(target vbStatement) int {
	for index := range parser.statements {
		if parser.statements[index].startOffset == target.startOffset && parser.statements[index].endOffset == target.endOffset {
			return index
		}
	}
	return -1
}

func (parser *vbnetParser) parseCallable(index, end int, statement vbStatement, keywordIndex int, keyword string, parent *SymbolParent, ownerName, ownerKind string) int {
	var nameIndex int
	constructor := keyword == "sub" && keywordIndex+1 < len(statement.tokens) && strings.EqualFold(statement.tokens[keywordIndex+1].Text, "new")
	if constructor {
		nameIndex = keywordIndex + 1
	} else {
		nameIndex = vbNextIdentifier(statement, keywordIndex+1)
	}
	if nameIndex < 0 {
		return index + 1
	}
	name := statement.tokens[nameIndex].Text
	kind := SymbolKindMethod
	nativeKind := keyword
	endLabel := keyword
	if constructor {
		kind = SymbolKindConstructor
		name = ownerName
		nativeKind = "constructor"
	}
	if keyword == "property" {
		kind = SymbolKindProperty
	}
	modifiers := vbCollectModifiers(statement, keywordIndex)
	bodyless := ownerKind == "interface" || vbHasModifier(modifiers, "mustoverride") || vbHasModifier(modifiers, "declare")
	closeIndex := -1
	if !bodyless {
		closeIndex = parser.findEnd(index+1, end, endLabel)
	}
	declarationEnd := statement.endOffset
	var body *OffsetRange
	if closeIndex >= 0 {
		declarationEnd = parser.statements[closeIndex].endOffset
		value := OffsetRange{Start: statement.endOffset, End: parser.statements[closeIndex].startOffset}
		body = &value
	} else if !bodyless && keyword != "property" {
		parser.builder.MarkIncomplete()
		_ = parser.builder.AddDiagnostic(DiagnosticSpec{Code: "vbnet-missing-end", Message: "declaration is missing End " + endLabel, Severity: DiagnosticWarning, Range: &OffsetRange{Start: statement.startOffset, End: statement.endOffset}, AffectsCoverage: true})
	}
	nameToken := statement.tokens[nameIndex]
	_, err := parser.builder.Add(SymbolSpec{
		Kind: kind, NativeKind: nativeKind, Name: name, Parent: parent,
		Declaration: OffsetRange{Start: statement.startOffset, End: declarationEnd}, NameRange: OffsetRange{Start: nameToken.StartOffset, End: nameToken.EndOffset},
		Signature: &OffsetRange{Start: statement.startOffset, End: statement.endOffset}, Body: body,
		Visibility: vbVisibility(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
	})
	if operation.KindOf(err) == operation.KindLimit {
		parser.stopped = true
	}
	if closeIndex >= 0 {
		return closeIndex + 1
	}
	return index + 1
}

func (parser *vbnetParser) parseEvent(index, end int, statement vbStatement, keywordIndex int, parent *SymbolParent) int {
	nameIndex := vbNextIdentifier(statement, keywordIndex+1)
	if nameIndex < 0 {
		return index + 1
	}
	modifiers := vbCollectModifiers(statement, keywordIndex)
	closeIndex := -1
	if vbHasModifier(modifiers, "custom") {
		closeIndex = parser.findEnd(index+1, end, "event")
		if closeIndex < 0 {
			parser.builder.MarkIncomplete()
			_ = parser.builder.AddDiagnostic(DiagnosticSpec{Code: "vbnet-missing-end", Message: "declaration is missing End event", Severity: DiagnosticWarning, Range: &OffsetRange{Start: statement.startOffset, End: statement.endOffset}, AffectsCoverage: true})
		}
	}
	declarationEnd := statement.endOffset
	var body *OffsetRange
	if closeIndex >= 0 {
		declarationEnd = parser.statements[closeIndex].endOffset
		value := OffsetRange{Start: statement.endOffset, End: parser.statements[closeIndex].startOffset}
		body = &value
	}
	nameToken := statement.tokens[nameIndex]
	_, err := parser.builder.Add(SymbolSpec{
		Kind: SymbolKindEvent, NativeKind: "event", Name: nameToken.Text, Parent: parent,
		Declaration: OffsetRange{Start: statement.startOffset, End: declarationEnd}, NameRange: OffsetRange{Start: nameToken.StartOffset, End: nameToken.EndOffset},
		Signature: &OffsetRange{Start: statement.startOffset, End: statement.endOffset}, Body: body,
		Visibility: vbVisibility(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
	})
	if operation.KindOf(err) == operation.KindLimit {
		parser.stopped = true
	}
	if closeIndex >= 0 {
		return closeIndex + 1
	}
	return index + 1
}

func (parser *vbnetParser) findEnd(start, end int, label string) int {
	var lambdaStack []string
	for index := start; index < end; index++ {
		statement := parser.statements[index]
		if lambdaLabel := vbMultilineLambdaLabel(statement); lambdaLabel != "" {
			lambdaStack = append(lambdaStack, lambdaLabel)
		}
		closeLabel := vbEndLabel(statement)
		if closeLabel == "" {
			continue
		}
		if len(lambdaStack) > 0 {
			top := lambdaStack[len(lambdaStack)-1]
			if strings.EqualFold(top, closeLabel) {
				lambdaStack = lambdaStack[:len(lambdaStack)-1]
			}
			continue
		}
		if strings.EqualFold(closeLabel, label) {
			return index
		}
	}
	return -1
}

func vbMultilineLambdaLabel(statement vbStatement) string {
	declarationIndex, declarationKeyword := vbDeclarationKeyword(statement)
	for index, token := range statement.tokens {
		label := strings.ToLower(token.Text)
		if label != "sub" && label != "function" {
			continue
		}
		if index == declarationIndex && strings.EqualFold(label, declarationKeyword) {
			continue
		}
		if index+1 >= len(statement.tokens) || statement.tokens[index+1].Text != "(" {
			continue
		}
		closeIndex := vbMatchingCloseParen(statement.tokens, index+1)
		if closeIndex < 0 {
			continue
		}
		if label == "sub" && closeIndex == len(statement.tokens)-1 {
			return label
		}
		if label == "function" {
			if closeIndex == len(statement.tokens)-1 || closeIndex+1 < len(statement.tokens) && strings.EqualFold(statement.tokens[closeIndex+1].Text, "as") {
				return label
			}
		}
	}
	return ""
}

func vbMatchingCloseParen(tokens []Token, openIndex int) int {
	depth := 0
	for index := openIndex; index < len(tokens); index++ {
		switch tokens[index].Text {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func (parser *vbnetParser) addSingleLine(statement vbStatement, keywordIndex int, kind SymbolKind, nativeKind string, parent *SymbolParent) {
	nameIndex := vbNextIdentifier(statement, keywordIndex+1)
	if nameIndex < 0 {
		return
	}
	modifiers := vbCollectModifiers(statement, keywordIndex)
	_, err := parser.builder.Add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: statement.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: statement.startOffset, End: statement.endOffset}, NameRange: OffsetRange{Start: statement.tokens[nameIndex].StartOffset, End: statement.tokens[nameIndex].EndOffset},
		Signature: &OffsetRange{Start: statement.startOffset, End: statement.endOffset}, Visibility: vbVisibility(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural})
	if operation.KindOf(err) == operation.KindLimit {
		parser.stopped = true
	}
}

func (parser *vbnetParser) addField(statement vbStatement, parent *SymbolParent) {
	keywordIndex, keyword := vbDeclarationKeyword(statement)
	start := keywordIndex
	if keyword == "dim" {
		start++
	}
	nameIndex := vbNextIdentifier(statement, start)
	if nameIndex < 0 {
		return
	}
	modifiers := vbCollectModifiers(statement, keywordIndex)
	_, err := parser.builder.Add(SymbolSpec{Kind: SymbolKindField, NativeKind: "field", Name: statement.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: statement.startOffset, End: statement.endOffset}, NameRange: OffsetRange{Start: statement.tokens[nameIndex].StartOffset, End: statement.tokens[nameIndex].EndOffset},
		Signature: &OffsetRange{Start: statement.startOffset, End: statement.endOffset}, Visibility: vbVisibility(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural})
	if operation.KindOf(err) == operation.KindLimit {
		parser.stopped = true
	}
}

func vbLooksLikeField(statement vbStatement, keywordIndex int) bool {
	if len(statement.tokens) == 0 {
		return false
	}
	if keywordIndex >= 0 && strings.EqualFold(statement.tokens[keywordIndex].Text, "dim") {
		return true
	}
	for _, token := range statement.tokens {
		if strings.EqualFold(token.Text, "as") {
			return true
		}
	}
	return false
}

func (parser *vbnetParser) parseImports(statement vbStatement) {
	if len(statement.tokens) < 2 {
		return
	}
	start := 1
	alias := ""
	if len(statement.tokens) > 3 && statement.tokens[1].Kind == TokenIdentifier && statement.tokens[2].Text == "=" {
		alias = statement.tokens[1].Text
		start = 3
	}
	value := vbJoinedTokens(statement.tokens[start:])
	if value == "" {
		return
	}
	rangeValue, err := parser.document.RangeFromUTF8Offsets(statement.startOffset, statement.endOffset)
	if err == nil {
		parser.dependencies = append(parser.dependencies, StructuralDependency{Kind: StructuralDependencyImport, Value: value, Alias: alias, Range: rangeValue, Evidence: SymbolEvidenceStructural})
	}
}

func (parser *vbnetParser) parseRelation(statement vbStatement, parent *SymbolParent) {
	if parent == nil || len(statement.tokens) < 2 {
		return
	}
	target := vbJoinedTokens(statement.tokens[1:])
	if target == "" {
		return
	}
	rangeValue, err := parser.document.RangeFromUTF8Offsets(statement.startOffset, statement.endOffset)
	if err == nil {
		parser.relations = append(parser.relations, StructuralRelation{Kind: strings.ToLower(statement.tokens[0].Text), Source: parent.QualifiedName, Target: target, Range: rangeValue, Evidence: SymbolEvidenceStructural})
	}
}

func vbNextIdentifier(statement vbStatement, start int) int {
	for index := start; index < len(statement.tokens); index++ {
		if statement.tokens[index].Kind == TokenIdentifier {
			return index
		}
		if statement.tokens[index].Text == "[" && index+2 < len(statement.tokens) && statement.tokens[index+2].Text == "]" {
			name := statement.tokens[index+1]
			if name.Kind == TokenIdentifier || name.Kind == TokenKeyword {
				return index + 1
			}
		}
	}
	return -1
}

func vbCollectModifiers(statement vbStatement, keywordIndex int) []string {
	var result []string
	for index := 0; index < keywordIndex && index < len(statement.tokens); index++ {
		text := strings.ToLower(statement.tokens[index].Text)
		if _, ok := vbModifiers[text]; ok {
			result = append(result, text)
		}
	}
	return result
}

func vbHasModifier(modifiers []string, want string) bool {
	for _, modifier := range modifiers {
		if modifier == want {
			return true
		}
	}
	return false
}

func vbVisibility(modifiers []string) Visibility {
	for _, modifier := range modifiers {
		switch modifier {
		case "public":
			return VisibilityPublic
		case "private":
			return VisibilityPrivate
		case "protected":
			return VisibilityProtected
		case "friend":
			return VisibilityFriend
		}
	}
	return ""
}

func vbJoinedTokens(tokens []Token) string {
	var builder strings.Builder
	previousWord := false
	for _, token := range tokens {
		if token.Kind == TokenString || token.Kind == TokenNewline || token.Kind == TokenDirective {
			continue
		}
		word := token.Kind == TokenIdentifier || token.Kind == TokenKeyword || token.Kind == TokenNumber
		if builder.Len() > 0 && previousWord && word {
			builder.WriteByte(' ')
		}
		builder.WriteString(token.Text)
		previousWord = word
	}
	return strings.TrimSpace(builder.String())
}
