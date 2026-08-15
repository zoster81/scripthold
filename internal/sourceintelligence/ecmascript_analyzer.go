package sourceintelligence

import (
	"context"

	"github.com/zoster81/scripthold/internal/operation"
)

// JavaScriptAnalyzer performs bounded declaration-level ECMAScript/JSX analysis.
type JavaScriptAnalyzer struct{}

func (JavaScriptAnalyzer) ID() AnalyzerID   { return AnalyzerJavaScript }
func (JavaScriptAnalyzer) Language() string { return "javascript" }
func (JavaScriptAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeECMAScript(ctx, document, options, false)
}

// TypeScriptAnalyzer performs bounded declaration-level TypeScript/TSX analysis.
type TypeScriptAnalyzer struct{}

func (TypeScriptAnalyzer) ID() AnalyzerID   { return AnalyzerTypeScript }
func (TypeScriptAnalyzer) Language() string { return "typescript" }
func (TypeScriptAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeECMAScript(ctx, document, options, true)
}

var ecmaModifiers = map[string]struct{}{
	"abstract": {}, "async": {}, "declare": {}, "default": {}, "export": {}, "override": {},
	"private": {}, "protected": {}, "public": {}, "readonly": {}, "static": {},
}

func analyzeECMAScript(ctx context.Context, document *SourceDocument, options AnalyzeOptions, typescript bool) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_ecmascript_source", document.Path, err)
	}
	language := "javascript"
	analyzer := AnalyzerJavaScript
	profile := JavaScriptScannerProfile()
	if typescript {
		language = "typescript"
		analyzer = AnalyzerTypeScript
		profile = TypeScriptScannerProfile()
	}
	scanDocument := document
	masked, err := maskECMAScriptRegexLiterals(ctx, document.Text)
	if err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_ecmascript_source", document.Path, err)
	}
	if masked != document.Text {
		clone := *document
		clone.Text = masked
		clone.lineStarts = buildLineStarts(masked)
		scanDocument = &clone
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: language, Analyzer: string(analyzer), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, scanDocument, profile, ScannerLimits{
		MaxTokens: scannerTokenBudget(scanDocument.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting,
	})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range scan.Diagnostics {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{
			Code: language + "-" + diagnostic.Code, Message: diagnostic.Message,
			Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true,
		})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	parser := &ecmaParser{
		ctx: ctx, document: document, tokens: scan.Tokens, pairs: PairDelimiterTokens(scan.Tokens, nil),
		builder: builder, typescript: typescript,
	}
	parser.parseScope(0, len(scan.Tokens), nil, false, "")
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_ecmascript_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

type ecmaParser struct {
	ctx          context.Context
	document     *SourceDocument
	tokens       []Token
	pairs        map[int]int
	builder      *SymbolBuilder
	typescript   bool
	dependencies []StructuralDependency
	relations    []StructuralRelation
	stopped      bool
}

func (parser *ecmaParser) parseScope(start, end int, parent *SymbolParent, members bool, owner string) {
	for index := start; index < end && !parser.stopped; {
		if parser.ctx.Err() != nil {
			return
		}
		index = parser.skipTrivia(index, end)
		if index >= end || parser.tokens[index].Kind == TokenEOF {
			return
		}
		if !members && parser.token(index, "import") {
			index = parser.parseImport(index, end)
			continue
		}
		if !members && parser.token(index, "export") {
			if next, ok := parser.parseReExport(index, end); ok {
				index = next
				continue
			}
		}
		semantic := parser.skipDeclarationPrefixes(index, end)
		if semantic >= end {
			return
		}
		if parser.token(semantic, "class") {
			index = parser.parseClass(index, semantic, end, parent)
			continue
		}
		if parser.typescript && parser.token(semantic, "interface") {
			index = parser.parseInterface(index, semantic, end, parent)
			continue
		}
		if parser.typescript && parser.token(semantic, "type") {
			index = parser.parseTypeAlias(index, semantic, end, parent)
			continue
		}
		if parser.typescript && parser.token(semantic, "enum") {
			index = parser.parseEnum(index, semantic, end, parent)
			continue
		}
		if parser.typescript && (parser.token(semantic, "namespace") || parser.token(semantic, "module")) {
			index = parser.parseNamespace(index, semantic, end, parent)
			continue
		}
		if parser.token(semantic, "function") {
			index = parser.parseFunction(index, semantic, end, parent, members)
			continue
		}
		if parser.token(semantic, "const") || parser.token(semantic, "let") || parser.token(semantic, "var") {
			index = parser.parseVariable(index, semantic, end, parent)
			continue
		}
		if members {
			if next, ok := parser.parseMember(index, semantic, end, parent, owner); ok {
				index = next
				continue
			}
		}
		index = parser.skipUnknownStatement(index, end)
	}
}

func (parser *ecmaParser) skipTrivia(index, end int) int {
	for index < end {
		switch parser.tokens[index].Kind {
		case TokenNewline, TokenDirective:
			index++
		default:
			return index
		}
	}
	return end
}

func (parser *ecmaParser) token(index int, text string) bool {
	return index >= 0 && index < len(parser.tokens) && parser.tokens[index].Text == text
}

func (parser *ecmaParser) skipDeclarationPrefixes(index, end int) int {
	for index < end {
		if _, ok := ecmaModifiers[parser.tokens[index].Text]; ok {
			index++
			continue
		}
		return index
	}
	return end
}

func (parser *ecmaParser) statementEnd(start, end int) int {
	if start >= end {
		return end
	}
	base := parser.tokens[start].Nesting
	for index := start; index < end; index++ {
		token := parser.tokens[index]
		if token.Kind == TokenEOF {
			return index
		}
		if token.Text == ";" && token.Nesting == base {
			return index
		}
		if token.Kind == TokenNewline && token.Nesting == base {
			return index
		}
	}
	return end - 1
}

func (parser *ecmaParser) skipUnknownStatement(start, end int) int {
	terminator := parser.statementEnd(start, end)
	if terminator < start {
		return start + 1
	}
	if terminator < end && parser.tokens[terminator].Kind != TokenEOF {
		return terminator + 1
	}
	return min(terminator+1, end)
}

func (parser *ecmaParser) parseImport(start, end int) int {
	terminator := parser.statementEnd(start, end)
	parser.collectStringDependency(start, terminator+1)
	return min(terminator+1, end)
}

func (parser *ecmaParser) parseReExport(start, end int) (int, bool) {
	terminator := parser.statementEnd(start, end)
	hasFrom := false
	for index := start + 1; index <= terminator && index < end; index++ {
		if parser.token(index, "from") {
			hasFrom = true
			break
		}
		// A declaration after export is not a re-export statement.
		if parser.token(index, "class") || parser.token(index, "function") ||
			parser.token(index, "const") || parser.token(index, "let") || parser.token(index, "var") ||
			parser.token(index, "interface") || parser.token(index, "type") ||
			parser.token(index, "enum") || parser.token(index, "namespace") {
			return start, false
		}
	}
	if !hasFrom {
		return start, false
	}
	parser.collectStringDependency(start, terminator+1)
	return min(terminator+1, end), true
}

func (parser *ecmaParser) collectStringDependency(start, end int) {
	for index := start; index < end && index < len(parser.tokens); index++ {
		if parser.tokens[index].Kind != TokenString {
			continue
		}
		value := ecmaStringLiteralValue(parser.tokens[index].Text)
		if value == "" {
			continue
		}
		rangeValue, err := parser.document.RangeFromUTF8Offsets(parser.tokens[index].StartOffset, parser.tokens[index].EndOffset)
		if err == nil {
			parser.dependencies = append(parser.dependencies, StructuralDependency{
				Kind: StructuralDependencyImport, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural,
			})
		}
		return
	}
}

func ecmaStringLiteralValue(value string) string {
	if len(value) < 2 {
		return ""
	}
	quote := value[0]
	if (quote != '"' && quote != '\'') || value[len(value)-1] != quote {
		return ""
	}
	return value[1 : len(value)-1]
}

func (parser *ecmaParser) parseVariable(declarationStart, keyword, end int, parent *SymbolParent) int {
	terminator := parser.statementEnd(keyword, end)
	nameIndex := parser.nextIdentifier(keyword+1, terminator+1)
	if nameIndex < 0 {
		return min(terminator+1, end)
	}
	arrow := false
	for index := nameIndex + 1; index <= terminator && index < end; index++ {
		if parser.tokens[index].Text == "=>" {
			arrow = true
		}
		if parser.tokens[index].Text == "require" && index+1 <= terminator {
			open := parser.skipTrivia(index+1, terminator+1)
			if open <= terminator && parser.tokens[open].Text == "(" {
				close := parser.pairs[open]
				if close > open && close <= terminator {
					for cursor := open + 1; cursor < close; cursor++ {
						if parser.tokens[cursor].Kind == TokenString {
							value := ecmaStringLiteralValue(parser.tokens[cursor].Text)
							if value != "" {
								rangeValue, err := parser.document.RangeFromUTF8Offsets(parser.tokens[cursor].StartOffset, parser.tokens[cursor].EndOffset)
								if err == nil {
									parser.dependencies = append(parser.dependencies, StructuralDependency{
										Kind: StructuralDependencyImport, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural,
									})
								}
							}
							break
						}
					}
				}
			}
		}
	}
	kind := SymbolKindVariable
	nativeKind := parser.tokens[keyword].Text
	if parser.token(keyword, "const") {
		kind = SymbolKindConstant
		nativeKind = "const"
	}
	if arrow {
		kind = SymbolKindFunction
		nativeKind = "arrow-function"
	}
	last := parser.previousCodeToken(terminator-1, nameIndex)
	if parser.tokens[terminator].Text == ";" {
		last = terminator
	}
	if last < nameIndex {
		last = nameIndex
	}
	var signature *OffsetRange
	if arrow {
		value := OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[last].EndOffset}
		signature = &value
	}
	parser.add(SymbolSpec{
		Kind: kind, NativeKind: nativeKind, Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[last].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   signature, Visibility: parser.visibility(declarationStart, keyword),
		Modifiers: parser.modifiers(declarationStart, keyword), Evidence: SymbolEvidenceStructural,
		Disambiguator: tokenRangeText(parser.tokens, nameIndex, min(terminator+1, end)),
	})
	return min(terminator+1, end)
}

func (parser *ecmaParser) parseFunction(declarationStart, keyword, end int, parent *SymbolParent, members bool) int {
	cursor := keyword + 1
	if parser.token(cursor, "*") {
		cursor++
	}
	nameIndex := parser.nextIdentifier(cursor, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	base := parser.tokens[keyword].Nesting
	paren := -1
	for index := nameIndex + 1; index < end; index++ {
		if parser.tokens[index].Text == "(" && parser.tokens[index].Nesting == base+1 {
			paren = index
			break
		}
		if parser.tokens[index].Kind == TokenNewline && parser.tokens[index].Nesting == base {
			return index + 1
		}
	}
	if paren < 0 {
		return nameIndex + 1
	}
	return parser.addCallable(declarationStart, nameIndex, paren, end, parent, members, parser.tokens[nameIndex].Text)
}

func (parser *ecmaParser) addCallable(declarationStart, nameIndex, paren, end int, parent *SymbolParent, members bool, name string) int {
	closeParen := parser.pairs[paren]
	if closeParen <= paren || closeParen >= end {
		parser.builder.MarkIncomplete()
		return paren + 1
	}
	base := parser.tokens[nameIndex].Nesting
	bodyOpen := -1
	terminator := -1
	for index := closeParen + 1; index < end; index++ {
		if parser.tokens[index].Text == "{" && parser.tokens[index].Nesting == base+1 {
			bodyOpen = index
			break
		}
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == base {
			terminator = index
			break
		}
		if parser.tokens[index].Kind == TokenNewline && parser.tokens[index].Nesting == base {
			terminator = index
			break
		}
	}
	kind := SymbolKindFunction
	nativeKind := "function-declaration"
	if members {
		kind = SymbolKindMethod
		nativeKind = "method-declaration"
	}
	declarationEnd := parser.tokens[closeParen].EndOffset
	signatureEnd := declarationEnd
	next := closeParen + 1
	var body *OffsetRange
	if bodyOpen >= 0 {
		closeBody := parser.pairs[bodyOpen]
		if closeBody <= bodyOpen || closeBody >= end {
			parser.builder.MarkIncomplete()
			return end
		}
		declarationEnd = parser.tokens[closeBody].EndOffset
		signatureEnd = parser.tokens[bodyOpen].StartOffset
		value := OffsetRange{Start: parser.tokens[bodyOpen].StartOffset, End: parser.tokens[closeBody].EndOffset}
		body = &value
		next = closeBody + 1
		if members {
			nativeKind = "method-definition"
		} else {
			nativeKind = "function-definition"
		}
	} else if terminator >= 0 {
		if previous := parser.previousCodeToken(terminator-1, closeParen); previous >= closeParen {
			declarationEnd = parser.tokens[previous].EndOffset
			signatureEnd = declarationEnd
		}
		if parser.tokens[terminator].Text == ";" {
			declarationEnd = parser.tokens[terminator].EndOffset
		}
		next = terminator + 1
	}
	parser.add(SymbolSpec{
		Kind: kind, NativeKind: nativeKind, Name: name, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: signatureEnd}, Body: body,
		Visibility: parser.visibility(declarationStart, nameIndex), Modifiers: parser.modifiers(declarationStart, nameIndex),
		Evidence:      SymbolEvidenceStructural,
		Disambiguator: tokenRangeText(parser.tokens, paren, closeParen+1) + ":" + nativeKind,
	})
	return next
}

func (parser *ecmaParser) parseClass(declarationStart, keyword, end int, parent *SymbolParent) int {
	nameIndex := parser.nextIdentifier(keyword+1, end)
	if nameIndex < 0 {
		parser.builder.MarkIncomplete()
		return keyword + 1
	}
	open := parser.findBodyOpen(nameIndex+1, end, parser.tokens[keyword].Nesting)
	if open < 0 {
		parser.builder.MarkIncomplete()
		return parser.skipUnknownStatement(declarationStart, end)
	}
	close := parser.pairs[open]
	if close <= open || close >= end {
		parser.builder.MarkIncomplete()
		return end
	}
	symbol, added := parser.add(SymbolSpec{
		Kind: SymbolKindClass, NativeKind: "class", Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[close].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[open].StartOffset},
		Body:        &OffsetRange{Start: parser.tokens[open].StartOffset, End: parser.tokens[close].EndOffset},
		Visibility:  parser.visibility(declarationStart, keyword), Modifiers: parser.modifiers(declarationStart, keyword),
		Evidence: SymbolEvidenceStructural,
	})
	if added {
		parser.collectTypeRelations(symbol.QualifiedName, nameIndex+1, open, parser.tokens[keyword].Nesting, false)
		classParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		parser.parseScope(open+1, close, classParent, true, symbol.Name)
	}
	return close + 1
}

func (parser *ecmaParser) parseInterface(declarationStart, keyword, end int, parent *SymbolParent) int {
	nameIndex := parser.nextIdentifier(keyword+1, end)
	if nameIndex < 0 {
		parser.builder.MarkIncomplete()
		return keyword + 1
	}
	open := parser.findBodyOpen(nameIndex+1, end, parser.tokens[keyword].Nesting)
	if open < 0 {
		parser.builder.MarkIncomplete()
		return parser.skipUnknownStatement(declarationStart, end)
	}
	close := parser.pairs[open]
	if close <= open || close >= end {
		parser.builder.MarkIncomplete()
		return end
	}
	symbol, added := parser.add(SymbolSpec{
		Kind: SymbolKindInterface, NativeKind: "interface", Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[close].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[open].StartOffset},
		Body:        &OffsetRange{Start: parser.tokens[open].StartOffset, End: parser.tokens[close].EndOffset},
		Visibility:  parser.visibility(declarationStart, keyword), Modifiers: parser.modifiers(declarationStart, keyword),
		Evidence: SymbolEvidenceStructural,
	})
	if added {
		parser.collectTypeRelations(symbol.QualifiedName, nameIndex+1, open, parser.tokens[keyword].Nesting, true)
		interfaceParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		parser.parseScope(open+1, close, interfaceParent, true, symbol.Name)
	}
	return close + 1
}

func (parser *ecmaParser) parseTypeAlias(declarationStart, keyword, end int, parent *SymbolParent) int {
	terminator := parser.statementEnd(keyword, end)
	nameIndex := parser.nextIdentifier(keyword+1, terminator+1)
	if nameIndex < 0 {
		return min(terminator+1, end)
	}
	last := parser.previousCodeToken(terminator-1, nameIndex)
	if parser.tokens[terminator].Text == ";" {
		last = terminator
	}
	parser.add(SymbolSpec{
		Kind: SymbolKindAlias, NativeKind: "type-alias", Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[last].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[last].EndOffset},
		Visibility:  parser.visibility(declarationStart, keyword), Modifiers: parser.modifiers(declarationStart, keyword),
		Evidence: SymbolEvidenceStructural,
	})
	return min(terminator+1, end)
}

func (parser *ecmaParser) parseEnum(declarationStart, keyword, end int, parent *SymbolParent) int {
	nameIndex := parser.nextIdentifier(keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	open := parser.findBodyOpen(nameIndex+1, end, parser.tokens[keyword].Nesting)
	if open < 0 {
		return parser.skipUnknownStatement(declarationStart, end)
	}
	close := parser.pairs[open]
	if close <= open {
		parser.builder.MarkIncomplete()
		return end
	}
	parser.add(SymbolSpec{
		Kind: SymbolKindEnum, NativeKind: "enum", Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[close].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[open].StartOffset},
		Body:        &OffsetRange{Start: parser.tokens[open].StartOffset, End: parser.tokens[close].EndOffset},
		Visibility:  parser.visibility(declarationStart, keyword), Modifiers: parser.modifiers(declarationStart, keyword),
		Evidence: SymbolEvidenceStructural,
	})
	return close + 1
}

func (parser *ecmaParser) parseNamespace(declarationStart, keyword, end int, parent *SymbolParent) int {
	nameIndex := parser.nextIdentifier(keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	open := parser.findBodyOpen(nameIndex+1, end, parser.tokens[keyword].Nesting)
	if open < 0 {
		return parser.skipUnknownStatement(declarationStart, end)
	}
	close := parser.pairs[open]
	if close <= open {
		parser.builder.MarkIncomplete()
		return end
	}
	symbol, added := parser.add(SymbolSpec{
		Kind: SymbolKindNamespace, NativeKind: parser.tokens[keyword].Text, Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[close].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[open].StartOffset},
		Body:        &OffsetRange{Start: parser.tokens[open].StartOffset, End: parser.tokens[close].EndOffset},
		Evidence:    SymbolEvidenceStructural,
	})
	if added {
		nsParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		parser.parseScope(open+1, close, nsParent, false, "")
	}
	return close + 1
}

func (parser *ecmaParser) parseMember(declarationStart, semantic, end int, parent *SymbolParent, owner string) (int, bool) {
	base := parser.tokens[semantic].Nesting
	nameIndex := semantic
	if parser.token(nameIndex, "get") || parser.token(nameIndex, "set") {
		afterKeyword := parser.skipTrivia(nameIndex+1, end)
		if afterKeyword < end && parser.tokens[afterKeyword].Text == "(" {
			return parser.addCallable(declarationStart, nameIndex, afterKeyword, end, parent, true, parser.tokens[nameIndex].Text), true
		}
		next := parser.nextIdentifier(nameIndex+1, end)
		if next >= 0 {
			nameIndex = next
		}
	}
	if parser.token(nameIndex, "constructor") {
		paren := parser.findParen(nameIndex+1, end, base)
		if paren < 0 {
			return declarationStart + 1, false
		}
		return parser.parseConstructor(declarationStart, nameIndex, paren, end, parent, owner), true
	}
	if parser.tokens[nameIndex].Kind != TokenIdentifier {
		return declarationStart + 1, false
	}
	paren := parser.findParen(nameIndex+1, end, base)
	equal := parser.findTokenAtNesting(nameIndex+1, end, "=", base)
	if paren >= 0 && (equal < 0 || paren < equal) {
		return parser.addCallable(declarationStart, nameIndex, paren, end, parent, true, parser.tokens[nameIndex].Text), true
	}
	terminator := parser.statementEnd(semantic, end)
	last := parser.previousCodeToken(terminator-1, nameIndex)
	if parser.tokens[terminator].Text == ";" {
		last = terminator
	}
	if last < nameIndex {
		last = nameIndex
	}
	parser.add(SymbolSpec{
		Kind: parser.memberValueKind(), NativeKind: "field", Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[last].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Visibility:  parser.visibility(declarationStart, semantic), Modifiers: parser.modifiers(declarationStart, semantic),
		Evidence: SymbolEvidenceStructural,
	})
	return min(terminator+1, end), true
}

func (parser *ecmaParser) parseConstructor(declarationStart, nameIndex, paren, end int, parent *SymbolParent, owner string) int {
	closeParen := parser.pairs[paren]
	if closeParen <= paren {
		parser.builder.MarkIncomplete()
		return paren + 1
	}
	base := parser.tokens[nameIndex].Nesting
	bodyOpen := parser.findTokenAtNesting(closeParen+1, end, "{", base+1)
	if bodyOpen < 0 {
		terminator := parser.statementEnd(closeParen+1, end)
		parser.add(SymbolSpec{
			Kind: SymbolKindConstructor, NativeKind: "constructor-declaration", Name: owner, Parent: parent,
			Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[terminator].EndOffset},
			NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
			Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[terminator].EndOffset},
			Evidence:    SymbolEvidenceStructural, Disambiguator: tokenRangeText(parser.tokens, paren, closeParen+1),
		})
		return min(terminator+1, end)
	}
	closeBody := parser.pairs[bodyOpen]
	if closeBody <= bodyOpen {
		parser.builder.MarkIncomplete()
		return end
	}
	parser.add(SymbolSpec{
		Kind: SymbolKindConstructor, NativeKind: "constructor-definition", Name: owner, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[closeBody].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[bodyOpen].StartOffset},
		Body:        &OffsetRange{Start: parser.tokens[bodyOpen].StartOffset, End: parser.tokens[closeBody].EndOffset},
		Evidence:    SymbolEvidenceStructural, Disambiguator: tokenRangeText(parser.tokens, paren, closeParen+1),
	})
	return closeBody + 1
}

func (parser *ecmaParser) memberValueKind() SymbolKind {
	if parser.typescript {
		return SymbolKindProperty
	}
	return SymbolKindField
}

func (parser *ecmaParser) collectTypeRelations(source string, start, end, nesting int, interfaceDecl bool) {
	for index := start; index < end; index++ {
		kind := ""
		switch parser.tokens[index].Text {
		case "extends":
			kind = "extends"
		case "implements":
			kind = "implements"
		default:
			continue
		}
		clauseEnd := end
		for cursor := index + 1; cursor < end; cursor++ {
			if parser.tokens[cursor].Text == "implements" || parser.tokens[cursor].Text == "extends" {
				clauseEnd = cursor
				break
			}
		}
		parts := splitECMATypeList(parser.tokens, index+1, clauseEnd, nesting)
		if !interfaceDecl && kind == "extends" && len(parts) > 1 {
			parts = parts[:1]
		}
		for _, part := range parts {
			target := tokenRangeText(parser.tokens, part[0], part[1])
			if target == "" {
				continue
			}
			rangeValue, err := parser.document.RangeFromUTF8Offsets(parser.tokens[part[0]].StartOffset, parser.tokens[part[1]-1].EndOffset)
			if err == nil {
				parser.relations = append(parser.relations, StructuralRelation{
					Kind: kind, Source: source, Target: target, Range: rangeValue, Evidence: SymbolEvidenceStructural,
				})
			}
		}
		index = clauseEnd - 1
	}
}

func splitECMATypeList(tokens []Token, start, end, nesting int) [][2]int {
	var result [][2]int
	partStart := start
	angle := 0
	for index := start; index < end; index++ {
		switch tokens[index].Text {
		case "<":
			angle++
		case ">":
			if angle > 0 {
				angle--
			}
		case ",":
			if angle == 0 && tokens[index].Nesting == nesting {
				if partStart < index {
					result = append(result, [2]int{partStart, index})
				}
				partStart = index + 1
			}
		}
	}
	if partStart < end {
		result = append(result, [2]int{partStart, end})
	}
	return result
}

func (parser *ecmaParser) findBodyOpen(start, end, base int) int {
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == "{" && parser.tokens[index].Nesting == base+1 {
			return index
		}
		if parser.tokens[index].Kind == TokenEOF {
			break
		}
	}
	return -1
}

func (parser *ecmaParser) findParen(start, end, base int) int {
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == "(" && parser.tokens[index].Nesting == base+1 {
			return index
		}
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == base {
			break
		}
		if parser.tokens[index].Kind == TokenNewline && parser.tokens[index].Nesting == base {
			break
		}
	}
	return -1
}

func (parser *ecmaParser) findTokenAtNesting(start, end int, text string, nesting int) int {
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == text && parser.tokens[index].Nesting == nesting {
			return index
		}
		if parser.tokens[index].Kind == TokenEOF {
			break
		}
	}
	return -1
}

func (parser *ecmaParser) nextIdentifier(index, end int) int {
	for index < end {
		if parser.tokens[index].Kind == TokenIdentifier {
			return index
		}
		if parser.tokens[index].Kind == TokenString || parser.tokens[index].Text == ";" {
			return -1
		}
		index++
	}
	return -1
}

func (parser *ecmaParser) previousCodeToken(index, start int) int {
	for index >= start {
		switch parser.tokens[index].Kind {
		case TokenNewline, TokenDirective:
			index--
		default:
			return index
		}
	}
	return -1
}

func (parser *ecmaParser) visibility(start, end int) Visibility {
	for index := start; index < end; index++ {
		switch parser.tokens[index].Text {
		case "public":
			return VisibilityPublic
		case "private":
			return VisibilityPrivate
		case "protected":
			return VisibilityProtected
		}
	}
	return ""
}

func (parser *ecmaParser) modifiers(start, end int) []string {
	var result []string
	for index := start; index < end; index++ {
		if _, ok := ecmaModifiers[parser.tokens[index].Text]; ok {
			result = append(result, parser.tokens[index].Text)
		}
	}
	return result
}

func (parser *ecmaParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
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
