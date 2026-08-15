package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// CAnalyzer performs bounded declaration-level ISO C analysis.
type CAnalyzer struct{}

func (CAnalyzer) ID() AnalyzerID   { return AnalyzerC }
func (CAnalyzer) Language() string { return "c" }
func (CAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeCFamily(ctx, document, options, false)
}

// CPPAnalyzer performs bounded declaration-level C++ analysis without invoking
// a compiler, build system, compile database, or macro expander.
type CPPAnalyzer struct{}

func (CPPAnalyzer) ID() AnalyzerID   { return AnalyzerCPP }
func (CPPAnalyzer) Language() string { return "cpp" }
func (CPPAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeCFamily(ctx, document, options, true)
}

var cFamilyModifiers = map[string]struct{}{
	"auto": {}, "const": {}, "constexpr": {}, "consteval": {}, "explicit": {}, "extern": {}, "friend": {}, "inline": {},
	"mutable": {}, "private": {}, "protected": {}, "public": {}, "register": {}, "static": {}, "thread_local": {},
	"typedef": {}, "virtual": {}, "volatile": {}, "override": {}, "final": {}, "signed": {}, "unsigned": {}, "long": {}, "short": {},
}

func analyzeCFamily(ctx context.Context, document *SourceDocument, options AnalyzeOptions, cpp bool) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_c_family_source", document.Path, err)
	}
	language := "c"
	analyzer := AnalyzerC
	profile := CScannerProfile()
	scanDocument := document
	var rawDiagnostics []ScannerDiagnostic
	if cpp {
		language = "cpp"
		analyzer = AnalyzerCPP
		profile = CPPScannerProfile()
		masked, diagnostics, err := maskCPPRawStrings(ctx, document.Text)
		if err != nil {
			return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_cpp_source", document.Path, err)
		}
		rawDiagnostics = diagnostics
		if masked != document.Text {
			clone := *document
			clone.Text = masked
			clone.lineStarts = buildLineStarts(masked)
			scanDocument = &clone
		}
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
	scan, err := ScanSource(ctx, scanDocument, profile, ScannerLimits{MaxTokens: scannerTokenBudget(scanDocument.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range append(rawDiagnostics, scan.Diagnostics...) {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: language + "-" + diagnostic.Code, Message: diagnostic.Message, Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
	}
	if !scan.Complete || len(rawDiagnostics) > 0 {
		builder.MarkIncomplete()
	}
	dependencies, conditional := collectCFamilyDirectives(document, scan.Tokens)
	if conditional {
		_ = builder.AddDiagnostic(DiagnosticSpec{
			Code: language + "-conditional-preprocessor", Message: "conditional preprocessing is not evaluated; declarations may depend on macro state",
			Severity: DiagnosticWarning, AffectsCoverage: true,
		})
	}
	parser := &cFamilyParser{
		ctx: ctx, document: document, tokens: scan.Tokens, pairs: PairDelimiterTokens(scan.Tokens, nil), builder: builder,
		cpp: cpp, dependencies: dependencies, types: make(map[string]SymbolParent),
	}
	parser.parseScope(0, len(scan.Tokens), nil, false, "")
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_c_family_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

func collectCFamilyDirectives(document *SourceDocument, tokens []Token) ([]StructuralDependency, bool) {
	var result []StructuralDependency
	conditional := false
	for _, token := range tokens {
		if token.Kind != TokenDirective {
			continue
		}
		trimmed := strings.TrimSpace(token.Text)
		lower := strings.ToLower(trimmed)
		for _, prefix := range []string{"#if", "#ifdef", "#ifndef", "#elif", "#else", "#endif"} {
			if lower == prefix || strings.HasPrefix(lower, prefix+" ") || strings.HasPrefix(lower, prefix+"\t") {
				conditional = true
				break
			}
		}
		if !strings.HasPrefix(lower, "#include") {
			continue
		}
		rest := strings.TrimSpace(trimmed[len("#include"):])
		value := ""
		if len(rest) >= 2 && rest[0] == '<' {
			if end := strings.IndexByte(rest[1:], '>'); end >= 0 {
				value = rest[1 : end+1]
			}
		} else if len(rest) >= 2 && rest[0] == '"' {
			if end := strings.IndexByte(rest[1:], '"'); end >= 0 {
				value = rest[1 : end+1]
			}
		}
		if value == "" {
			continue
		}
		rangeValue, err := document.RangeFromUTF8Offsets(token.StartOffset, token.EndOffset)
		if err == nil {
			result = append(result, StructuralDependency{Kind: StructuralDependencyInclude, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural})
		}
	}
	return result, conditional
}

type cFamilyParser struct {
	ctx          context.Context
	document     *SourceDocument
	tokens       []Token
	pairs        map[int]int
	builder      *SymbolBuilder
	cpp          bool
	dependencies []StructuralDependency
	relations    []StructuralRelation
	types        map[string]SymbolParent
	stopped      bool
}

func (parser *cFamilyParser) parseScope(start, end int, parent *SymbolParent, members bool, owner string) {
	for index := start; index < end && !parser.stopped; {
		if parser.ctx.Err() != nil {
			return
		}
		index = nextStructuralToken(parser.tokens, index, end)
		if index >= end || parser.tokens[index].Kind == TokenEOF {
			return
		}
		if parser.cpp && parser.token(index, "namespace") {
			index = parser.parseNamespace(index, end, parent)
			continue
		}
		if keyword, declarationStart, ok := parser.typeDeclarationAt(index, end); ok {
			index = parser.parseType(declarationStart, keyword, end, parent)
			continue
		}
		if parser.cpp && parser.token(index, "using") {
			if next, ok := parser.parseUsingAlias(index, end, parent); ok {
				index = next
				continue
			}
		}
		if members && parser.isAccessLabel(index, end) {
			index += 2
			continue
		}
		if next, ok := parser.parseFunctionOrVariable(index, end, parent, members, owner); ok {
			index = next
			continue
		}
		index++
	}
}

func (parser *cFamilyParser) token(index int, value string) bool {
	return index >= 0 && index < len(parser.tokens) && parser.tokens[index].Text == value
}

func (parser *cFamilyParser) typeDeclarationAt(start, end int) (keyword, declarationStart int, ok bool) {
	cursor := start
	declarationStart = start
	if parser.cpp && parser.token(cursor, "template") {
		cursor++
		angle := 0
		seenOpen := false
		for cursor < end {
			if parser.tokens[cursor].Text == "<" {
				angle++
				seenOpen = true
			} else if parser.tokens[cursor].Text == ">" && angle > 0 {
				angle--
				if angle == 0 && seenOpen {
					cursor++
					break
				}
			}
			cursor++
		}
		cursor = nextStructuralToken(parser.tokens, cursor, end)
	}
	for cursor < end {
		text := strings.ToLower(parser.tokens[cursor].Text)
		if _, modifier := cFamilyModifiers[text]; modifier && text != "typedef" {
			cursor++
			continue
		}
		if text == "struct" || text == "class" || text == "union" || text == "enum" {
			if text == "class" && !parser.cpp {
				return 0, start, false
			}
			return cursor, declarationStart, true
		}
		return 0, start, false
	}
	return 0, start, false
}

func (parser *cFamilyParser) parseNamespace(start, end int, parent *SymbolParent) int {
	depth := parser.tokens[start].Nesting
	open := -1
	for index := start + 1; index < end; index++ {
		if parser.tokens[index].Text == "{" && parser.tokens[index].Nesting == depth+1 {
			open = index
			break
		}
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == depth {
			return index + 1
		}
	}
	if open < 0 {
		parser.builder.MarkIncomplete()
		return start + 1
	}
	close := parser.pairs[open]
	if close <= open || close >= end {
		parser.builder.MarkIncomplete()
		return end
	}
	nameStart := nextIdentifierToken(parser.tokens, start+1, open)
	nameEnd := previousIdentifierToken(parser.tokens, open-1, start+1)
	if nameStart < 0 || nameEnd < nameStart {
		return close + 1
	}
	name := tokenRangeText(parser.tokens, nameStart, nameEnd+1)
	nameRange := OffsetRange{Start: parser.tokens[nameStart].StartOffset, End: parser.tokens[nameEnd].EndOffset}
	symbol, added := parser.add(SymbolSpec{
		Kind: SymbolKindNamespace, NativeKind: "namespace", Name: name, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[close].EndOffset}, NameRange: nameRange,
		Signature: &OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[open].StartOffset},
		Body:      &OffsetRange{Start: parser.tokens[open].StartOffset, End: parser.tokens[close].EndOffset}, Evidence: SymbolEvidenceStructural,
	})
	if added {
		nsParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		parser.parseScope(open+1, close, nsParent, false, "")
	}
	return close + 1
}

func (parser *cFamilyParser) parseType(start, keyword, end int, parent *SymbolParent) int {
	depth := parser.tokens[keyword].Nesting
	nativeKind := strings.ToLower(parser.tokens[keyword].Text)
	cursor := keyword + 1
	if nativeKind == "enum" && parser.cpp && cursor < end && (parser.token(cursor, "class") || parser.token(cursor, "struct")) {
		cursor++
	}
	nameIndex := nextIdentifierToken(parser.tokens, cursor, end)
	if nameIndex < 0 {
		parser.builder.MarkIncomplete()
		return keyword + 1
	}
	open, semicolon := -1, -1
	for index := nameIndex + 1; index < end; index++ {
		if parser.tokens[index].Text == "{" && parser.tokens[index].Nesting == depth+1 {
			open = index
			break
		}
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == depth {
			semicolon = index
			break
		}
	}
	terminator := semicolon
	close := -1
	if open >= 0 {
		close = parser.pairs[open]
		if close <= open || close >= end {
			parser.builder.MarkIncomplete()
			return end
		}
		terminator = close
	}
	if terminator < 0 {
		parser.builder.MarkIncomplete()
		return end
	}
	kind := SymbolKindStruct
	switch nativeKind {
	case "class":
		kind = SymbolKindClass
	case "enum":
		kind = SymbolKindEnum
	case "union":
		kind = SymbolKindType
	}
	declarationStart := parser.tokens[start].StartOffset
	declarationEnd := parser.tokens[terminator].EndOffset
	var body *OffsetRange
	signatureEnd := declarationEnd
	if open >= 0 {
		signatureEnd = parser.tokens[open].StartOffset
		value := OffsetRange{Start: parser.tokens[open].StartOffset, End: parser.tokens[close].EndOffset}
		body = &value
	}
	modifiers := collectKnownModifiers(parser.tokens, start, keyword, cFamilyModifiers)
	symbol, added := parser.add(SymbolSpec{
		Kind: kind, NativeKind: nativeKind, Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: declarationStart, End: declarationEnd},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: declarationStart, End: signatureEnd}, Body: body,
		Visibility: visibilityFromModifiers(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
	})
	if !added {
		return terminator + 1
	}
	parser.types[symbol.QualifiedName] = SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
	if parser.cpp && open >= 0 {
		parser.collectCPPBaseRelations(symbol.QualifiedName, nameIndex+1, open, depth)
	}
	if open >= 0 && nativeKind != "enum" {
		typeParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		parser.parseScope(open+1, close, typeParent, true, symbol.Name)
	}
	return terminator + 1
}

func (parser *cFamilyParser) collectCPPBaseRelations(source string, start, end, nesting int) {
	colon := -1
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == ":" && parser.tokens[index].Nesting == nesting {
			colon = index
			break
		}
	}
	if colon < 0 {
		return
	}
	drop := map[string]struct{}{"public": {}, "private": {}, "protected": {}, "virtual": {}}
	for _, part := range splitTokenRangeAt(parser.tokens, colon+1, end, ",", nesting) {
		target := normalizedTypeSpelling(parser.tokens, part[0], part[1], drop)
		if target == "" {
			continue
		}
		rangeValue, err := parser.document.RangeFromUTF8Offsets(parser.tokens[part[0]].StartOffset, parser.tokens[part[1]-1].EndOffset)
		if err == nil {
			parser.relations = append(parser.relations, StructuralRelation{Kind: "inherits", Source: source, Target: target, Range: rangeValue, Evidence: SymbolEvidenceStructural})
		}
	}
}

func (parser *cFamilyParser) parseUsingAlias(start, end int, parent *SymbolParent) (int, bool) {
	depth := parser.tokens[start].Nesting
	semicolon := -1
	for index := start + 1; index < end; index++ {
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == depth {
			semicolon = index
			break
		}
	}
	if semicolon < 0 {
		return start + 1, false
	}
	nameIndex := nextIdentifierToken(parser.tokens, start+1, semicolon)
	if nameIndex < 0 || !parser.hasToken(nameIndex+1, semicolon, "=") {
		return semicolon + 1, false
	}
	_, added := parser.add(SymbolSpec{
		Kind: SymbolKindAlias, NativeKind: "using-alias", Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[semicolon].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[semicolon].EndOffset}, Evidence: SymbolEvidenceStructural,
	})
	return semicolon + 1, added
}

func (parser *cFamilyParser) parseFunctionOrVariable(start, end int, parent *SymbolParent, members bool, owner string) (int, bool) {
	depth := parser.tokens[start].Nesting
	terminator := -1
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == depth {
			terminator = index
			break
		}
		if parser.tokens[index].Text == "{" && parser.tokens[index].Nesting == depth+1 {
			terminator = index
			break
		}
		if parser.tokens[index].Kind == TokenEOF {
			break
		}
	}
	if terminator < 0 {
		return start + 1, false
	}
	if _, functionPointer := parser.functionPointerDeclarator(start, terminator); !functionPointer {
		paren := parser.firstFunctionParen(start, terminator, depth)
		if paren >= 0 {
			if next, ok := parser.parseFunction(start, paren, terminator, end, parent, members, owner); ok {
				return next, true
			}
		}
	}
	if parser.tokens[terminator].Text == "{" {
		close := parser.pairs[terminator]
		if close > terminator {
			return close + 1, false
		}
		return terminator + 1, false
	}
	return parser.parseVariable(start, terminator, parent, members)
}

func (parser *cFamilyParser) firstFunctionParen(start, end, depth int) int {
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == "=" && parser.tokens[index].Nesting == depth {
			return -1
		}
		if parser.tokens[index].Text != "(" || parser.tokens[index].Nesting != depth+1 {
			continue
		}
		previous := previousStructuralToken(parser.tokens, index-1, start)
		if previous < 0 {
			continue
		}
		if parser.tokens[previous].Kind == TokenIdentifier || parser.tokens[previous].Text == "]" || parser.tokens[previous].Text == ")" {
			return index
		}
	}
	return -1
}

func (parser *cFamilyParser) parseFunction(start, paren, terminator, end int, parent *SymbolParent, members bool, owner string) (int, bool) {
	closeParen := parser.pairs[paren]
	if closeParen <= paren || closeParen >= end {
		parser.builder.MarkIncomplete()
		return terminator + 1, false
	}
	operatorIndex := -1
	if parser.cpp {
		for index := start; index < paren; index++ {
			if parser.token(index, "operator") {
				operatorIndex = index
				break
			}
		}
	}
	nameIndex := previousIdentifierToken(parser.tokens, paren-1, start)
	if operatorIndex >= 0 {
		nameIndex = operatorIndex
	}
	if nameIndex < 0 {
		return terminator + 1, false
	}
	name := parser.tokens[nameIndex].Text
	effectiveParent := parent
	effectiveMembers := members
	effectiveOwner := owner
	if parser.cpp {
		if qualifiedParent, qualifiedOwner, ok := parser.qualifiedFunctionOwner(start, nameIndex, parent); ok {
			effectiveParent = qualifiedParent
			effectiveMembers = true
			effectiveOwner = qualifiedOwner
		}
	}
	kind := SymbolKindFunction
	nativeKind := "function-declaration"
	if effectiveMembers {
		kind = SymbolKindMethod
		nativeKind = "method-declaration"
	}
	if parser.cpp {
		if operatorIndex >= 0 {
			name = "operator" + tokenRangeText(parser.tokens, operatorIndex+1, paren)
			kind = SymbolKindOperator
			nativeKind = "operator-declaration"
		} else if effectiveMembers && nameIndex > start && parser.tokens[nameIndex-1].Text == "~" && name == effectiveOwner {
			kind = SymbolKindDestructor
			nativeKind = "destructor-declaration"
		} else if effectiveMembers && name == effectiveOwner {
			kind = SymbolKindConstructor
			nativeKind = "constructor-declaration"
		}
	}
	bodyOpen := -1
	if parser.tokens[terminator].Text == "{" {
		bodyOpen = terminator
	} else {
		for index := closeParen + 1; index < end; index++ {
			if parser.tokens[index].Text == "{" && parser.tokens[index].Nesting == parser.tokens[start].Nesting+1 {
				bodyOpen = index
				break
			}
			if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == parser.tokens[start].Nesting {
				break
			}
		}
	}
	declarationEnd := parser.tokens[terminator].EndOffset
	var body *OffsetRange
	next := terminator + 1
	if bodyOpen >= 0 {
		close := parser.pairs[bodyOpen]
		if close <= bodyOpen || close >= end {
			parser.builder.MarkIncomplete()
			return end, false
		}
		declarationEnd = parser.tokens[close].EndOffset
		value := OffsetRange{Start: parser.tokens[bodyOpen].StartOffset, End: parser.tokens[close].EndOffset}
		body = &value
		next = close + 1
		switch kind {
		case SymbolKindFunction:
			nativeKind = "function-definition"
		case SymbolKindMethod:
			nativeKind = "method-definition"
		case SymbolKindConstructor:
			nativeKind = "constructor-definition"
		case SymbolKindDestructor:
			nativeKind = "destructor-definition"
		case SymbolKindOperator:
			nativeKind = "operator-definition"
		}
	}
	modifiers := collectKnownModifiers(parser.tokens, start, paren, cFamilyModifiers)
	nameStart := parser.tokens[nameIndex].StartOffset
	nameEnd := parser.tokens[nameIndex].EndOffset
	if kind == SymbolKindOperator {
		last := previousStructuralToken(parser.tokens, paren-1, nameIndex)
		if last >= nameIndex {
			nameEnd = parser.tokens[last].EndOffset
		}
	}
	_, added := parser.add(SymbolSpec{
		Kind: kind, NativeKind: nativeKind, Name: name, Parent: effectiveParent,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: nameStart, End: nameEnd},
		Signature:   &OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[bodyStartOrTerminator(bodyOpen, terminator)].StartOffset}, Body: body,
		Visibility: visibilityFromModifiers(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
		Disambiguator: tokenRangeText(parser.tokens, paren, closeParen+1),
	})
	return next, added
}

func (parser *cFamilyParser) qualifiedFunctionOwner(start, nameIndex int, lexicalParent *SymbolParent) (*SymbolParent, string, bool) {
	separator := -1
	for index := nameIndex - 1; index > start; index-- {
		if parser.tokens[index].Text == ":" && parser.tokens[index-1].Text == ":" {
			separator = index - 1
			break
		}
	}
	if separator < 0 {
		return nil, "", false
	}
	cursor := separator - 1
	if cursor < start {
		return nil, "", false
	}
	if parser.tokens[cursor].Text == ">" {
		angle := 1
		cursor--
		for cursor >= start && angle > 0 {
			switch parser.tokens[cursor].Text {
			case ">":
				angle++
			case "<":
				angle--
			}
			cursor--
		}
	}
	qualifierIndex := previousIdentifierToken(parser.tokens, cursor, start)
	if qualifierIndex < 0 {
		return nil, "", false
	}
	qualifier := parser.tokens[qualifierIndex].Text
	candidate := qualifier
	if lexicalParent != nil && lexicalParent.QualifiedName != "" {
		candidate = lexicalParent.QualifiedName + "." + qualifier
	}
	if parent, ok := parser.types[candidate]; ok {
		value := parent
		return &value, qualifier, true
	}
	var match *SymbolParent
	for qualified, parent := range parser.types {
		if qualified != qualifier && !strings.HasSuffix(qualified, "."+qualifier) {
			continue
		}
		if match != nil {
			return nil, "", false
		}
		value := parent
		match = &value
	}
	if match == nil {
		return nil, "", false
	}
	return match, qualifier, true
}
func bodyStartOrTerminator(bodyOpen, terminator int) int {
	if bodyOpen >= 0 {
		return bodyOpen
	}
	return terminator
}

func (parser *cFamilyParser) parseVariable(start, semicolon int, parent *SymbolParent, members bool) (int, bool) {
	if parser.hasToken(start, semicolon, "typedef") {
		nameIndex := previousIdentifierToken(parser.tokens, semicolon-1, start)
		if nameIndex >= 0 {
			_, added := parser.add(SymbolSpec{Kind: SymbolKindAlias, NativeKind: "typedef", Name: parser.tokens[nameIndex].Text, Parent: parent,
				Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[semicolon].EndOffset},
				NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset}, Evidence: SymbolEvidenceStructural})
			return semicolon + 1, added
		}
	}
	limit := semicolon
	for index := start; index < semicolon; index++ {
		if parser.tokens[index].Text == "=" && parser.tokens[index].Nesting == parser.tokens[start].Nesting {
			limit = index
			break
		}
	}
	nameIndex, functionPointer := parser.functionPointerDeclarator(start, limit)
	if !functionPointer {
		nameIndex = previousIdentifierToken(parser.tokens, limit-1, start)
	}
	if nameIndex < 0 {
		return semicolon + 1, false
	}
	lowerName := strings.ToLower(parser.tokens[nameIndex].Text)
	if _, isModifier := cFamilyModifiers[lowerName]; isModifier {
		return semicolon + 1, false
	}
	kind := SymbolKindVariable
	nativeKind := "variable"
	if members {
		kind = SymbolKindField
		nativeKind = "field"
	}
	if parser.hasToken(start, nameIndex, "const") || parser.hasToken(start, nameIndex, "constexpr") {
		if !members {
			kind = SymbolKindConstant
			nativeKind = "constant"
		}
	}
	modifiers := collectKnownModifiers(parser.tokens, start, nameIndex, cFamilyModifiers)
	_, added := parser.add(SymbolSpec{
		Kind: kind, NativeKind: nativeKind, Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[semicolon].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Visibility:  visibilityFromModifiers(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
	})
	return semicolon + 1, added
}

func (parser *cFamilyParser) functionPointerDeclarator(start, end int) (int, bool) {
	for open := start; open < end; open++ {
		if parser.tokens[open].Text != "(" {
			continue
		}
		close := parser.pairs[open]
		if close <= open || close >= end {
			continue
		}
		starSeen := false
		nameIndex := -1
		for index := open + 1; index < close; index++ {
			if parser.tokens[index].Text == "*" {
				starSeen = true
				continue
			}
			if starSeen && parser.tokens[index].Kind == TokenIdentifier {
				nameIndex = index
			}
		}
		if !starSeen || nameIndex < 0 {
			continue
		}
		parameters := nextStructuralToken(parser.tokens, close+1, end)
		if parameters >= end || parser.tokens[parameters].Text != "(" {
			continue
		}
		parameterClose := parser.pairs[parameters]
		if parameterClose > parameters && parameterClose <= end {
			return nameIndex, true
		}
	}
	return -1, false
}
func (parser *cFamilyParser) hasToken(start, end int, text string) bool {
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == text {
			return true
		}
	}
	return false
}

func (parser *cFamilyParser) isAccessLabel(index, end int) bool {
	if index+1 >= end || parser.tokens[index+1].Text != ":" {
		return false
	}
	text := parser.tokens[index].Text
	return text == "public" || text == "private" || text == "protected"
}

func (parser *cFamilyParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
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

func maskCPPRawStrings(ctx context.Context, text string) (string, []ScannerDiagnostic, error) {
	masked := []byte(text)
	changed := false
	var diagnostics []ScannerDiagnostic
	for index := 0; index < len(text); {
		if index&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return "", nil, err
			}
		}
		if strings.HasPrefix(text[index:], "//") {
			if end := strings.IndexAny(text[index+2:], "\r\n"); end >= 0 {
				index += end + 2
			} else {
				break
			}
			continue
		}
		if strings.HasPrefix(text[index:], "/*") {
			end := strings.Index(text[index+2:], "*/")
			if end < 0 {
				break
			}
			index += end + 4
			continue
		}
		if prefixBytes, delimiterStart, ok := cppRawStringStart(text, index); ok {
			openParen := strings.IndexByte(text[delimiterStart:], '(')
			if openParen < 0 || openParen > 16 {
				index += prefixBytes
				continue
			}
			openParen += delimiterStart
			delimiter := text[delimiterStart:openParen]
			if !validCPPRawDelimiter(delimiter) {
				index += prefixBytes
				continue
			}
			closing := ")" + delimiter + "\""
			relativeEnd := strings.Index(text[openParen+1:], closing)
			end := len(text)
			if relativeEnd >= 0 {
				end = openParen + 1 + relativeEnd + len(closing)
			} else {
				diagnostics = append(diagnostics, ScannerDiagnostic{Code: "unterminated-raw-string", Message: "C++ raw string literal is not terminated", StartOffset: index, EndOffset: len(text)})
			}
			for cursor := index; cursor < end; cursor++ {
				if masked[cursor] != '\r' && masked[cursor] != '\n' {
					masked[cursor] = ' '
				}
			}
			changed = true
			index = end
			continue
		}
		if text[index] == '"' || text[index] == '\'' {
			quote := text[index]
			index++
			for index < len(text) {
				if text[index] == '\\' {
					index += min(2, len(text)-index)
					continue
				}
				if text[index] == quote {
					index++
					break
				}
				index++
			}
			continue
		}
		index++
	}
	if !changed {
		return text, diagnostics, nil
	}
	return string(masked), diagnostics, nil
}

func cppRawStringStart(text string, index int) (prefixBytes, delimiterStart int, ok bool) {
	for _, prefix := range []string{"u8R\"", "uR\"", "UR\"", "LR\"", "R\""} {
		if strings.HasPrefix(text[index:], prefix) {
			if index > 0 {
				previous := text[index-1]
				if previous == '_' || previous >= '0' && previous <= '9' || previous >= 'A' && previous <= 'Z' || previous >= 'a' && previous <= 'z' {
					return 0, 0, false
				}
			}
			return len(prefix), index + len(prefix), true
		}
	}
	return 0, 0, false
}

func validCPPRawDelimiter(value string) bool {
	if len(value) > 16 {
		return false
	}
	for index := 0; index < len(value); index++ {
		current := value[index]
		if current <= ' ' || current == '\\' || current == ')' || current == '(' {
			return false
		}
	}
	return true
}
