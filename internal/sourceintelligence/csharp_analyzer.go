package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// CSharpAnalyzer performs bounded declaration-level C# analysis over the shared scanner.
type CSharpAnalyzer struct{}

func (CSharpAnalyzer) ID() AnalyzerID   { return AnalyzerCSharp }
func (CSharpAnalyzer) Language() string { return "csharp" }

func (CSharpAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_csharp_source", document.Path, err)
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: "csharp", Analyzer: string(AnalyzerCSharp), IncludeSignatures: options.IncludeSignatures,
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
	scan, err := ScanSource(ctx, document, CSharpScannerProfile(), ScannerLimits{
		MaxTokens: maxTokens, MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting,
	})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range scan.Diagnostics {
		rangeValue := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: "csharp-" + diagnostic.Code, Message: diagnostic.Message, Severity: DiagnosticWarning, Range: &rangeValue, AffectsCoverage: true})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	parser := &csharpParser{ctx: ctx, document: document, tokens: scan.Tokens, builder: builder, pairs: buildTokenPairs(scan.Tokens)}
	parser.parseScope(0, len(scan.Tokens), nil, false, "")
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_csharp_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies}, nil
}

type csharpParser struct {
	ctx          context.Context
	document     *SourceDocument
	tokens       []Token
	builder      *SymbolBuilder
	pairs        map[int]int
	dependencies []StructuralDependency
	stopped      bool
}

var csharpModifiers = map[string]struct{}{
	"public": {}, "private": {}, "protected": {}, "internal": {}, "file": {}, "static": {}, "abstract": {},
	"sealed": {}, "partial": {}, "readonly": {}, "volatile": {}, "virtual": {}, "override": {}, "new": {},
	"async": {}, "extern": {}, "unsafe": {}, "required": {}, "ref": {},
}

func (parser *csharpParser) parseScope(start, end int, parent *SymbolParent, members bool, ownerName string) {
	currentParent := parent
	for index := start; index < end && !parser.stopped; {
		if parser.ctx.Err() != nil {
			return
		}
		index = parser.nextUseful(index, end)
		if index >= end || parser.tokens[index].Kind == TokenEOF {
			return
		}
		if !members && parser.tokenEqual(index, "using") {
			index = parser.parseUsing(index, end)
			continue
		}
		if !members && parser.tokenEqual(index, "namespace") {
			next, namespaceParent := parser.parseNamespace(index, end, currentParent)
			if namespaceParent != nil {
				currentParent = namespaceParent
			}
			index = next
			continue
		}
		if keyword, declarationStart, modifiers, ok := parser.typeDeclarationAt(index, end); ok {
			index = parser.parseType(declarationStart, keyword, end, currentParent, modifiers)
			continue
		}
		if !members && parser.tokens[index].Text == "[" {
			next, ok := parser.skipAttributeLists(index, end)
			if !ok {
				return
			}
			index = next
			continue
		}
		if members {
			index = parser.parseMember(index, end, currentParent, ownerName)
		} else {
			index++
		}
	}
}

func (parser *csharpParser) parseUsing(start, end int) int {
	depth := parser.tokens[start].Nesting
	semicolon := parser.findToken(start+1, end, ";", depth)
	if semicolon < 0 {
		parser.builder.MarkIncomplete()
		return end
	}
	alias := ""
	valueStart := start + 1
	if valueStart+1 < semicolon && parser.tokens[valueStart].Kind == TokenIdentifier && parser.tokenEqual(valueStart+1, "=") {
		alias = parser.tokens[valueStart].Text
		valueStart += 2
	}
	var parts []string
	for index := valueStart; index < semicolon; index++ {
		if parser.tokens[index].Kind == TokenIdentifier || parser.tokens[index].Kind == TokenKeyword || parser.tokens[index].Text == "." {
			parts = append(parts, parser.tokens[index].Text)
		}
	}
	value := strings.TrimSpace(strings.Join(parts, ""))
	if value != "" {
		rangeValue, err := parser.document.RangeFromUTF8Offsets(parser.tokens[start].StartOffset, parser.tokens[semicolon].EndOffset)
		if err == nil {
			parser.dependencies = append(parser.dependencies, StructuralDependency{Kind: StructuralDependencyImport, Value: value, Alias: alias, Range: rangeValue, Evidence: SymbolEvidenceStructural})
		}
	}
	return semicolon + 1
}

func (parser *csharpParser) parseNamespace(start, end int, parent *SymbolParent) (int, *SymbolParent) {
	cursor := start + 1
	var nameParts []string
	for cursor < end {
		token := parser.tokens[cursor]
		if token.Text == "{" || token.Text == ";" {
			break
		}
		if token.Kind == TokenIdentifier || token.Text == "." {
			nameParts = append(nameParts, token.Text)
		}
		cursor++
	}
	name := strings.Join(nameParts, "")
	if name == "" || cursor >= end {
		parser.builder.MarkIncomplete()
		return max(cursor, start+1), nil
	}
	terminator := parser.tokens[cursor]
	nameToken := parser.lastIdentifier(start+1, cursor)
	if nameToken < 0 {
		parser.builder.MarkIncomplete()
		return cursor + 1, nil
	}
	declarationEnd := terminator.EndOffset
	var body *OffsetRange
	closeIndex := -1
	if terminator.Text == "{" {
		closeIndex = parser.pairs[cursor]
		if closeIndex <= cursor || closeIndex >= end {
			parser.builder.MarkIncomplete()
			closeIndex = end - 1
		} else {
			declarationEnd = parser.tokens[closeIndex].EndOffset
			value := OffsetRange{Start: terminator.StartOffset, End: parser.tokens[closeIndex].EndOffset}
			body = &value
		}
	}
	spec := SymbolSpec{Kind: SymbolKindNamespace, NativeKind: "namespace", Name: name, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: parser.tokens[nameToken].StartOffset, End: parser.tokens[nameToken].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[start].StartOffset, End: terminator.StartOffset}, Body: body, Evidence: SymbolEvidenceStructural}
	symbol, ok := parser.add(spec)
	if !ok {
		return cursor + 1, nil
	}
	namespaceParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
	if terminator.Text == ";" {
		return cursor + 1, namespaceParent
	}
	parser.parseScope(cursor+1, closeIndex, namespaceParent, false, "")
	return closeIndex + 1, nil
}

func (parser *csharpParser) typeDeclarationAt(start, end int) (keyword, declarationStart int, modifiers []string, ok bool) {
	cursor := start
	declarationStart = start
	for cursor < end {
		if parser.tokens[cursor].Text == "[" {
			closeIndex := parser.pairs[cursor]
			if closeIndex <= cursor {
				return 0, start, nil, false
			}
			cursor = parser.nextUseful(closeIndex+1, end)
			continue
		}
		text := strings.ToLower(parser.tokens[cursor].Text)
		if _, exists := csharpModifiers[text]; exists {
			modifiers = append(modifiers, text)
			cursor = parser.nextUseful(cursor+1, end)
			continue
		}
		if text == "class" || text == "struct" || text == "interface" || text == "record" || text == "enum" {
			return cursor, declarationStart, modifiers, true
		}
		return 0, start, nil, false
	}
	return 0, start, nil, false
}

func (parser *csharpParser) parseType(start, keyword, end int, parent *SymbolParent, modifiers []string) int {
	cursor := keyword + 1
	nativeKind := strings.ToLower(parser.tokens[keyword].Text)
	if nativeKind == "record" && cursor < end && (parser.tokenEqual(cursor, "class") || parser.tokenEqual(cursor, "struct")) {
		cursor++
	}
	nameIndex := parser.nextIdentifier(cursor, end)
	if nameIndex < 0 {
		parser.builder.MarkIncomplete()
		return keyword + 1
	}
	depth := parser.tokens[keyword].Nesting
	terminator := parser.findDeclarationTerminator(nameIndex+1, end, depth)
	if terminator < 0 {
		parser.builder.MarkIncomplete()
		return end
	}
	declarationEnd := parser.tokens[terminator].EndOffset
	var body *OffsetRange
	closeIndex := -1
	if parser.tokens[terminator].Text == "{" {
		closeIndex = parser.pairs[terminator]
		if closeIndex > terminator && closeIndex < end {
			declarationEnd = parser.tokens[closeIndex].EndOffset
			value := OffsetRange{Start: parser.tokens[terminator].StartOffset, End: parser.tokens[closeIndex].EndOffset}
			body = &value
		} else {
			parser.builder.MarkIncomplete()
			closeIndex = end - 1
		}
	}
	kind := map[string]SymbolKind{"class": SymbolKindClass, "struct": SymbolKindStruct, "interface": SymbolKindInterface, "record": SymbolKindRecord, "enum": SymbolKindEnum}[nativeKind]
	signatureEnd := parser.tokens[terminator].StartOffset
	if parser.tokens[terminator].Text == ";" {
		signatureEnd = parser.tokens[terminator].EndOffset
	}
	symbol, added := parser.add(SymbolSpec{
		Kind: kind, NativeKind: nativeKind, Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[start].StartOffset, End: signatureEnd}, Body: body,
		Visibility: csharpVisibility(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
	})
	if !added {
		return terminator + 1
	}
	if closeIndex > terminator {
		typeParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		parser.parseScope(terminator+1, closeIndex, typeParent, true, symbol.Name)
		return closeIndex + 1
	}
	return terminator + 1
}

func (parser *csharpParser) parseMember(start, end int, parent *SymbolParent, ownerName string) int {
	start = parser.nextUseful(start, end)
	if start >= end {
		return end
	}
	if keyword, declarationStart, modifiers, ok := parser.typeDeclarationAt(start, end); ok {
		return parser.parseType(declarationStart, keyword, end, parent, modifiers)
	}
	declarationStart := start
	memberStart, ok := parser.skipAttributeLists(start, end)
	if !ok {
		return end
	}
	start = memberStart
	if start >= end {
		return end
	}
	depth := parser.tokens[start].Nesting
	terminator := parser.findMemberTerminator(start, end, depth)
	if terminator < 0 {
		return start + 1
	}
	closeIndex := -1
	declarationEnd := parser.tokens[terminator].EndOffset
	if parser.tokens[terminator].Text == "{" {
		closeIndex = parser.pairs[terminator]
		if closeIndex <= terminator || closeIndex >= end {
			parser.builder.MarkIncomplete()
			return end
		}
		declarationEnd = parser.tokens[closeIndex].EndOffset
	}
	segmentEnd := terminator
	modifiers := parser.collectModifiers(start, segmentEnd)
	visibility := csharpVisibility(modifiers)
	paren := parser.findTopOpening(start, segmentEnd, "(", depth+1)
	if paren >= 0 {
		nameIndex := parser.lastIdentifier(start, paren)
		if nameIndex >= 0 {
			kind := SymbolKindMethod
			nativeKind := "method"
			if parser.tokens[nameIndex].Text == ownerName {
				kind, nativeKind = SymbolKindConstructor, "constructor"
			}
			if parser.tokenEqual(nameIndex-1, "~") {
				kind, nativeKind = SymbolKindDestructor, "destructor"
			}
			if parser.tokenEqual(nameIndex-1, "operator") {
				kind, nativeKind = SymbolKindOperator, "operator"
			}
			if closeParen := parser.pairs[paren]; closeParen > paren && parser.containsToken(paren+1, closeParen, "this") {
				modifiers = append(modifiers, "extension")
			}
			signatureEnd := parser.tokens[terminator].StartOffset
			var body *OffsetRange
			if closeIndex > terminator {
				value := OffsetRange{Start: parser.tokens[terminator].StartOffset, End: parser.tokens[closeIndex].EndOffset}
				body = &value
			} else if arrow := parser.findToken(start, terminator, "=>", depth); arrow >= 0 {
				value := OffsetRange{Start: parser.tokens[arrow].StartOffset, End: parser.tokens[terminator].EndOffset}
				body = &value
			}
			parser.add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: parser.tokens[nameIndex].Text, Parent: parent,
				Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: declarationEnd},
				NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
				Signature:   &OffsetRange{Start: parser.tokens[start].StartOffset, End: signatureEnd}, Body: body,
				Visibility: visibility, Modifiers: modifiers, Evidence: SymbolEvidenceStructural})
		}
		if closeIndex > terminator {
			return closeIndex + 1
		}
		return terminator + 1
	}

	indexerName := parser.indexerNameIndex(start, segmentEnd, depth)
	nameIndex := indexerName
	if nameIndex < 0 {
		nameIndex = parser.memberNameIndex(start, segmentEnd)
	}
	if nameIndex >= 0 {
		kind, nativeKind := SymbolKindField, "field"
		if indexerName >= 0 {
			kind, nativeKind = SymbolKindProperty, "indexer"
		} else if parser.containsToken(start, nameIndex, "const") {
			kind, nativeKind = SymbolKindConstant, "const"
		} else if parser.containsToken(start, nameIndex, "event") {
			kind, nativeKind = SymbolKindEvent, "event"
		} else if closeIndex > terminator || parser.findToken(start, terminator, "=>", depth) >= 0 {
			kind, nativeKind = SymbolKindProperty, "property"
		}
		signatureEnd := parser.tokens[terminator].StartOffset
		var body *OffsetRange
		if closeIndex > terminator {
			value := OffsetRange{Start: parser.tokens[terminator].StartOffset, End: parser.tokens[closeIndex].EndOffset}
			body = &value
		}
		parser.add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: parser.tokens[nameIndex].Text, Parent: parent,
			Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: declarationEnd},
			NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
			Signature:   &OffsetRange{Start: parser.tokens[start].StartOffset, End: signatureEnd}, Body: body,
			Visibility: visibility, Modifiers: modifiers, Evidence: SymbolEvidenceStructural})
		if kind == SymbolKindField && closeIndex < 0 {
			for _, extra := range parser.additionalFieldNames(nameIndex+1, terminator, depth) {
				parser.add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: parser.tokens[extra].Text, Parent: parent,
					Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: declarationEnd},
					NameRange:   OffsetRange{Start: parser.tokens[extra].StartOffset, End: parser.tokens[extra].EndOffset},
					Visibility:  visibility, Modifiers: modifiers, Evidence: SymbolEvidenceStructural})
			}
		}
	}
	if closeIndex > terminator {
		return closeIndex + 1
	}
	return terminator + 1
}

func (parser *csharpParser) skipAttributeLists(start, end int) (int, bool) {
	cursor := start
	for cursor < end && parser.tokens[cursor].Text == "[" {
		closeIndex, ok := parser.pairs[cursor]
		if !ok || closeIndex <= cursor || closeIndex >= end {
			parser.builder.MarkIncomplete()
			return cursor, false
		}
		cursor = parser.nextUseful(closeIndex+1, end)
	}
	return cursor, true
}

func (parser *csharpParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
	symbol, err := parser.builder.Add(spec)
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

func (parser *csharpParser) nextUseful(index, end int) int {
	for index < end && (parser.tokens[index].Kind == TokenNewline || parser.tokens[index].Kind == TokenDirective || parser.tokens[index].Kind == TokenString) {
		index++
	}
	return index
}

func (parser *csharpParser) nextIdentifier(index, end int) int {
	for index < end {
		if parser.tokens[index].Kind == TokenIdentifier {
			return index
		}
		if parser.tokens[index].Text == "{" || parser.tokens[index].Text == ";" {
			return -1
		}
		index++
	}
	return -1
}

func (parser *csharpParser) lastIdentifier(start, end int) int {
	for index := end - 1; index >= start; index-- {
		if parser.tokens[index].Kind == TokenIdentifier {
			return index
		}
	}
	return -1
}

func (parser *csharpParser) findDeclarationTerminator(start, end, depth int) int {
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == depth {
			return index
		}
		if parser.tokens[index].Text == "{" && parser.tokens[index].Nesting == depth+1 {
			return index
		}
	}
	return -1
}

func (parser *csharpParser) findMemberTerminator(start, end, depth int) int {
	for index := start; index < end; index++ {
		token := parser.tokens[index]
		if token.Text == ";" && token.Nesting == depth {
			return index
		}
		if token.Text == "{" && token.Nesting == depth+1 {
			return index
		}
	}
	return -1
}

func (parser *csharpParser) findTopOpening(start, end int, text string, nesting int) int {
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == text && parser.tokens[index].Nesting == nesting {
			return index
		}
	}
	return -1
}

func (parser *csharpParser) findToken(start, end int, text string, nesting int) int {
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == text && parser.tokens[index].Nesting == nesting {
			return index
		}
	}
	return -1
}

func (parser *csharpParser) containsToken(start, end int, text string) bool {
	for index := start; index < end; index++ {
		if parser.tokenEqual(index, text) {
			return true
		}
	}
	return false
}

func (parser *csharpParser) collectModifiers(start, end int) []string {
	var modifiers []string
	for index := start; index < end; index++ {
		text := strings.ToLower(parser.tokens[index].Text)
		if _, ok := csharpModifiers[text]; ok {
			modifiers = append(modifiers, text)
		}
	}
	return modifiers
}

func (parser *csharpParser) indexerNameIndex(start, end, depth int) int {
	for index := start; index+1 < end; index++ {
		if parser.tokens[index].Nesting == depth && strings.EqualFold(parser.tokens[index].Text, "this") && parser.tokens[index+1].Text == "[" {
			return index
		}
	}
	return -1
}

func (parser *csharpParser) memberNameIndex(start, end int) int {
	arrow := -1
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == "=>" {
			arrow = index
			break
		}
	}
	if arrow >= 0 {
		return parser.lastIdentifier(start, arrow)
	}
	return parser.lastIdentifier(start, end)
}

func (parser *csharpParser) additionalFieldNames(start, end, depth int) []int {
	var result []int
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == "," && parser.tokens[index].Nesting == depth {
			if name := parser.nextIdentifier(index+1, end); name >= 0 {
				result = append(result, name)
			}
		}
	}
	return result
}

func (parser *csharpParser) tokenEqual(index int, text string) bool {
	return index >= 0 && index < len(parser.tokens) && strings.EqualFold(parser.tokens[index].Text, text)
}

func buildTokenPairs(tokens []Token) map[int]int {
	pairs := make(map[int]int)
	var stack []int
	for index, token := range tokens {
		switch token.Text {
		case "(", "[", "{":
			stack = append(stack, index)
		case ")", "]", "}":
			if len(stack) == 0 {
				continue
			}
			open := stack[len(stack)-1]
			if matchingToken(tokens[open].Text, token.Text) {
				stack = stack[:len(stack)-1]
				pairs[open] = index
				pairs[index] = open
			}
		}
	}
	return pairs
}

func matchingToken(open, close string) bool {
	return open == "(" && close == ")" || open == "[" && close == "]" || open == "{" && close == "}"
}

func csharpVisibility(modifiers []string) Visibility {
	for _, modifier := range modifiers {
		switch modifier {
		case "public":
			return VisibilityPublic
		case "private":
			return VisibilityPrivate
		case "protected":
			return VisibilityProtected
		case "internal":
			return VisibilityInternal
		}
	}
	return ""
}
