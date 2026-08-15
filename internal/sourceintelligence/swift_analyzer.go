package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// SwiftAnalyzer performs bounded declaration-level Swift analysis without
// compiler/type checking, macro expansion, package resolution, or execution.
type SwiftAnalyzer struct{}

func (SwiftAnalyzer) ID() AnalyzerID   { return AnalyzerSwift }
func (SwiftAnalyzer) Language() string { return "swift" }
func (SwiftAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_swift_source", document.Path, err)
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: "swift", Analyzer: string(AnalyzerSwift), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, document, SwiftScannerProfile(), ScannerLimits{MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range scan.Diagnostics {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: "swift-" + diagnostic.Code, Message: diagnostic.Message, Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	parser := &swiftParser{
		ctx: ctx, document: document, tokens: scan.Tokens, pairs: PairDelimiterTokens(scan.Tokens, nil), builder: builder,
		types: make(map[string]SymbolParent),
	}
	parser.parseScope(0, len(scan.Tokens), nil, false, "")
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_swift_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

var swiftModifiers = map[string]struct{}{
	"convenience": {}, "fileprivate": {}, "final": {}, "internal": {}, "mutating": {}, "nonisolated": {}, "open": {},
	"override": {}, "private": {}, "public": {}, "required": {}, "static": {},
}

type swiftParser struct {
	ctx          context.Context
	document     *SourceDocument
	tokens       []Token
	pairs        map[int]int
	builder      *SymbolBuilder
	dependencies []StructuralDependency
	relations    []StructuralRelation
	types        map[string]SymbolParent
	stopped      bool
}

func (p *swiftParser) parseScope(start, end int, parent *SymbolParent, members bool, owner string) {
	for index := start; index < end && !p.stopped; {
		if p.ctx.Err() != nil {
			return
		}
		index = p.skipTrivia(index, end)
		if index >= end || p.tokens[index].Kind == TokenEOF {
			return
		}
		declarationStart := index
		index = p.skipAttributes(index, end)
		index = p.skipTrivia(index, end)
		semantic := p.skipModifiers(index, end)
		if semantic >= end {
			return
		}
		if !members && p.token(semantic, "import") {
			index = p.parseImport(semantic, end)
			continue
		}
		switch strings.ToLower(p.tokens[semantic].Text) {
		case "class", "struct", "enum", "protocol":
			index = p.parseType(declarationStart, semantic, end, parent)
		case "extension":
			index = p.parseExtension(declarationStart, semantic, end, parent)
		case "func":
			index = p.parseFunction(declarationStart, semantic, end, parent, members)
		case "init":
			if members {
				index = p.parseInitializer(declarationStart, semantic, end, parent, owner)
			} else {
				index = p.skipStatement(declarationStart, end)
			}
		case "deinit":
			if members {
				index = p.parseDeinitializer(declarationStart, semantic, end, parent, owner)
			} else {
				index = p.skipStatement(declarationStart, end)
			}
		case "var", "let":
			index = p.parseProperty(declarationStart, semantic, end, parent, members)
		case "typealias", "associatedtype":
			index = p.parseAlias(declarationStart, semantic, end, parent)
		default:
			index = p.skipStatement(declarationStart, end)
		}
	}
}

func (p *swiftParser) token(index int, text string) bool {
	return index >= 0 && index < len(p.tokens) && p.tokens[index].Text == text
}

func (p *swiftParser) skipTrivia(index, end int) int {
	for index < end && (p.tokens[index].Kind == TokenNewline || p.tokens[index].Kind == TokenDirective) {
		index++
	}
	return index
}

func (p *swiftParser) skipAttributes(index, end int) int {
	for index < end && p.tokens[index].Text == "@" {
		cursor := index + 1
		if cursor >= end || (p.tokens[cursor].Kind != TokenIdentifier && p.tokens[cursor].Kind != TokenKeyword) {
			return index
		}
		cursor++
		for cursor+1 < end && p.tokens[cursor].Text == "." && (p.tokens[cursor+1].Kind == TokenIdentifier || p.tokens[cursor+1].Kind == TokenKeyword) {
			cursor += 2
		}
		if cursor < end && p.tokens[cursor].Text == "(" {
			close := p.pairs[cursor]
			if close <= cursor || close >= end {
				p.builder.MarkIncomplete()
				return index
			}
			cursor = close + 1
		}
		index = p.skipTrivia(cursor, end)
	}
	return index
}

func (p *swiftParser) skipModifiers(index, end int) int {
	for index < end {
		if _, ok := swiftModifiers[strings.ToLower(p.tokens[index].Text)]; ok {
			index++
			continue
		}
		return index
	}
	return end
}

func (p *swiftParser) parseImport(keyword, end int) int {
	terminator := p.lineTerminator(keyword+1, end, p.tokens[keyword].Nesting)
	if terminator < 0 {
		terminator = end
	}
	cursor := p.skipTrivia(keyword+1, terminator)
	if cursor < terminator {
		switch p.tokens[cursor].Text {
		case "class", "struct", "enum", "protocol", "func", "var", "let", "typealias":
			cursor++
		}
	}
	last := p.previousCode(terminator-1, cursor)
	if cursor < terminator && last >= cursor {
		value := tokenRangeText(p.tokens, cursor, last+1)
		if value != "" {
			rangeValue, err := p.document.RangeFromUTF8Offsets(p.tokens[cursor].StartOffset, p.tokens[last].EndOffset)
			if err == nil {
				p.dependencies = append(p.dependencies, StructuralDependency{Kind: StructuralDependencyImport, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural})
			}
		}
	}
	if terminator < end {
		return terminator + 1
	}
	return end
}

func (p *swiftParser) parseType(declarationStart, keyword, end int, parent *SymbolParent) int {
	nameIndex := nextIdentifierToken(p.tokens, keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	base := p.tokens[keyword].Nesting
	open := p.findToken(nameIndex+1, end, "{", base+1)
	if open < 0 {
		p.builder.MarkIncomplete()
		return p.skipStatement(declarationStart, end)
	}
	close := p.pairs[open]
	if close <= open || close >= end {
		p.builder.MarkIncomplete()
		return end
	}
	nativeKind := strings.ToLower(p.tokens[keyword].Text)
	kind := SymbolKindClass
	switch nativeKind {
	case "struct":
		kind = SymbolKindStruct
	case "enum":
		kind = SymbolKindEnum
	case "protocol":
		kind = SymbolKindInterface
	}
	modifiers := collectKnownModifiers(p.tokens, declarationStart, keyword, swiftModifiers)
	symbol, added := p.add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: p.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: p.tokens[close].EndOffset},
		NameRange:   OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: p.tokens[open].StartOffset},
		Body:        &OffsetRange{Start: p.tokens[open].StartOffset, End: p.tokens[close].EndOffset},
		Visibility:  swiftVisibility(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural})
	if added {
		parentValue := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		p.types[symbol.QualifiedName] = parentValue
		p.types[symbol.Name] = parentValue
		p.collectTypeRelations(symbol.QualifiedName, nativeKind, nameIndex+1, open, base)
		p.parseScope(open+1, close, &parentValue, true, symbol.Name)
	}
	return close + 1
}

func (p *swiftParser) parseExtension(_ int, keyword, end int, lexicalParent *SymbolParent) int {
	base := p.tokens[keyword].Nesting
	open := p.findToken(keyword+1, end, "{", base+1)
	if open < 0 {
		p.builder.MarkIncomplete()
		return p.skipStatement(keyword, end)
	}
	close := p.pairs[open]
	if close <= open || close >= end {
		p.builder.MarkIncomplete()
		return end
	}
	colon := p.findToken(keyword+1, open, ":", base)
	targetEnd := open
	if colon >= 0 {
		targetEnd = colon
	}
	start := p.skipTrivia(keyword+1, targetEnd)
	last := p.previousCode(targetEnd-1, start)
	if start >= targetEnd || last < start {
		return close + 1
	}
	target := tokenRangeText(p.tokens, start, last+1)
	parent := p.resolveTypeParent(target, lexicalParent)
	if colon >= 0 {
		for _, part := range swiftSplitTypeList(p.tokens, colon+1, open, base) {
			p.addRelation("conforms", parent.QualifiedName, tokenRangeText(p.tokens, part[0], part[1]), part[0], part[1])
		}
	}
	p.parseScope(open+1, close, &parent, true, swiftBaseTypeName(target))
	return close + 1
}

func (p *swiftParser) resolveTypeParent(target string, lexicalParent *SymbolParent) SymbolParent {
	if value, ok := p.types[target]; ok {
		return value
	}
	base := swiftBaseTypeName(target)
	if value, ok := p.types[base]; ok {
		return value
	}
	qualified := target
	if lexicalParent != nil && !strings.Contains(target, ".") {
		qualified = lexicalParent.QualifiedName + "." + target
	}
	return SymbolParent{QualifiedName: qualified}
}

func swiftBaseTypeName(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.IndexByte(value, '<'); index >= 0 {
		value = value[:index]
	}
	if index := strings.LastIndexByte(value, '.'); index >= 0 {
		value = value[index+1:]
	}
	return value
}

func (p *swiftParser) collectTypeRelations(source, nativeKind string, start, end, nesting int) {
	colon := p.findToken(start, end, ":", nesting)
	if colon < 0 {
		return
	}
	parts := swiftSplitTypeList(p.tokens, colon+1, end, nesting)
	for index, part := range parts {
		kind := "conforms"
		if nativeKind == "class" && index == 0 {
			kind = "inherits"
		} else if nativeKind == "protocol" {
			kind = "inherits"
		}
		p.addRelation(kind, source, tokenRangeText(p.tokens, part[0], part[1]), part[0], part[1])
	}
}

func swiftSplitTypeList(tokens []Token, start, end, nesting int) [][2]int {
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

func (p *swiftParser) parseFunction(declarationStart, keyword, end int, parent *SymbolParent, member bool) int {
	nameIndex := nextIdentifierToken(p.tokens, keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	paren := p.findToken(nameIndex+1, end, "(", p.tokens[keyword].Nesting+1)
	if paren < 0 {
		return p.skipStatement(declarationStart, end)
	}
	return p.addCallable(declarationStart, nameIndex, paren, end, parent, member, p.tokens[nameIndex].Text, "func")
}

func (p *swiftParser) parseInitializer(declarationStart, keyword, end int, parent *SymbolParent, owner string) int {
	paren := p.findToken(keyword+1, end, "(", p.tokens[keyword].Nesting+1)
	if paren < 0 {
		return p.skipStatement(declarationStart, end)
	}
	return p.addCallable(declarationStart, keyword, paren, end, parent, true, owner, "init")
}

func (p *swiftParser) parseDeinitializer(declarationStart, keyword, end int, parent *SymbolParent, owner string) int {
	base := p.tokens[keyword].Nesting
	bodyOpen := p.findToken(keyword+1, end, "{", base+1)
	if bodyOpen < 0 {
		return p.skipStatement(declarationStart, end)
	}
	close := p.pairs[bodyOpen]
	if close <= bodyOpen {
		p.builder.MarkIncomplete()
		return end
	}
	p.add(SymbolSpec{Kind: SymbolKindDestructor, NativeKind: "deinit", Name: owner, Parent: parent,
		Declaration: OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: p.tokens[close].EndOffset},
		NameRange:   OffsetRange{Start: p.tokens[keyword].StartOffset, End: p.tokens[keyword].EndOffset},
		Signature:   &OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: p.tokens[bodyOpen].StartOffset},
		Body:        &OffsetRange{Start: p.tokens[bodyOpen].StartOffset, End: p.tokens[close].EndOffset}, Evidence: SymbolEvidenceStructural})
	return close + 1
}

func (p *swiftParser) addCallable(declarationStart, nameIndex, paren, end int, parent *SymbolParent, member bool, name, nativeKind string) int {
	closeParen := p.pairs[paren]
	if closeParen <= paren || closeParen >= end {
		p.builder.MarkIncomplete()
		return paren + 1
	}
	base := p.tokens[nameIndex].Nesting
	bodyOpen := -1
	terminator := -1
	for index := closeParen + 1; index < end; index++ {
		if p.tokens[index].Text == "{" && p.tokens[index].Nesting == base+1 {
			bodyOpen = index
			break
		}
		if p.tokens[index].Kind == TokenNewline && p.tokens[index].Nesting == base {
			terminator = index
			break
		}
		if p.tokens[index].Text == ";" && p.tokens[index].Nesting == base {
			terminator = index
			break
		}
	}
	kind := SymbolKindFunction
	if member {
		kind = SymbolKindMethod
	}
	if nativeKind == "init" {
		kind = SymbolKindConstructor
	}
	declarationEnd := p.tokens[closeParen].EndOffset
	signatureEnd := declarationEnd
	next := closeParen + 1
	var body *OffsetRange
	if bodyOpen >= 0 {
		close := p.pairs[bodyOpen]
		if close <= bodyOpen || close >= end {
			p.builder.MarkIncomplete()
			return end
		}
		declarationEnd = p.tokens[close].EndOffset
		signatureEnd = p.tokens[bodyOpen].StartOffset
		value := OffsetRange{Start: p.tokens[bodyOpen].StartOffset, End: p.tokens[close].EndOffset}
		body = &value
		next = close + 1
		nativeKind += "-definition"
	} else if terminator >= 0 {
		previous := p.previousCode(terminator-1, closeParen)
		if previous >= closeParen {
			declarationEnd = p.tokens[previous].EndOffset
			signatureEnd = declarationEnd
		}
		next = terminator + 1
		nativeKind += "-declaration"
	}
	modifiers := collectKnownModifiers(p.tokens, declarationStart, nameIndex, swiftModifiers)
	p.add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: name, Parent: parent,
		Declaration: OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: signatureEnd}, Body: body,
		Visibility: swiftVisibility(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
		Disambiguator: tokenRangeText(p.tokens, paren, closeParen+1) + ":" + nativeKind})
	return next
}

func (p *swiftParser) parseProperty(declarationStart, keyword, end int, parent *SymbolParent, member bool) int {
	nameIndex := nextIdentifierToken(p.tokens, keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	base := p.tokens[keyword].Nesting
	terminator := p.lineTerminator(nameIndex+1, end, base)
	bodyOpen := p.findToken(nameIndex+1, chooseSwiftSearchEnd(terminator, end), "{", base+1)
	declarationEnd := p.tokens[nameIndex].EndOffset
	next := nameIndex + 1
	var body *OffsetRange
	if bodyOpen >= 0 && (terminator < 0 || bodyOpen < terminator) {
		close := p.pairs[bodyOpen]
		if close <= bodyOpen || close >= end {
			p.builder.MarkIncomplete()
			return end
		}
		declarationEnd = p.tokens[close].EndOffset
		value := OffsetRange{Start: p.tokens[bodyOpen].StartOffset, End: p.tokens[close].EndOffset}
		body = &value
		next = close + 1
	} else if terminator >= 0 {
		previous := p.previousCode(terminator-1, nameIndex)
		if previous >= nameIndex {
			declarationEnd = p.tokens[previous].EndOffset
		}
		next = terminator + 1
	}
	kind := SymbolKindVariable
	if member {
		kind = SymbolKindProperty
	}
	modifiers := collectKnownModifiers(p.tokens, declarationStart, keyword, swiftModifiers)
	p.add(SymbolSpec{Kind: kind, NativeKind: p.tokens[keyword].Text, Name: p.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset}, Body: body,
		Visibility: swiftVisibility(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural})
	return next
}

func chooseSwiftSearchEnd(terminator, end int) int {
	if terminator >= 0 {
		return terminator + 1
	}
	return end
}

func (p *swiftParser) parseAlias(declarationStart, keyword, end int, parent *SymbolParent) int {
	nameIndex := nextIdentifierToken(p.tokens, keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	terminator := p.lineTerminator(nameIndex+1, end, p.tokens[keyword].Nesting)
	last := nameIndex
	if terminator >= 0 {
		if previous := p.previousCode(terminator-1, nameIndex); previous >= nameIndex {
			last = previous
		}
	}
	p.add(SymbolSpec{Kind: SymbolKindAlias, NativeKind: p.tokens[keyword].Text, Name: p.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: p.tokens[last].EndOffset},
		NameRange:   OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: p.tokens[last].EndOffset}, Evidence: SymbolEvidenceStructural})
	if terminator >= 0 {
		return terminator + 1
	}
	return last + 1
}

func swiftVisibility(modifiers []string) Visibility {
	for _, modifier := range modifiers {
		switch modifier {
		case "open", "public":
			return VisibilityPublic
		case "private", "fileprivate":
			return VisibilityPrivate
		case "internal":
			return VisibilityInternal
		}
	}
	return ""
}

func (p *swiftParser) addRelation(kind, source, target string, start, end int) {
	target = strings.TrimSpace(target)
	if source == "" || target == "" || start < 0 || end <= start || end > len(p.tokens) {
		return
	}
	rangeValue, err := p.document.RangeFromUTF8Offsets(p.tokens[start].StartOffset, p.tokens[end-1].EndOffset)
	if err == nil {
		p.relations = append(p.relations, StructuralRelation{Kind: kind, Source: source, Target: target, Range: rangeValue, Evidence: SymbolEvidenceStructural})
	}
}

func (p *swiftParser) findToken(start, end int, text string, nesting int) int {
	for index := start; index < end; index++ {
		if p.tokens[index].Text == text && p.tokens[index].Nesting == nesting {
			return index
		}
		if p.tokens[index].Kind == TokenEOF {
			break
		}
	}
	return -1
}

func (p *swiftParser) lineTerminator(start, end, nesting int) int {
	for index := start; index < end; index++ {
		if (p.tokens[index].Kind == TokenNewline || p.tokens[index].Text == ";") && p.tokens[index].Nesting == nesting {
			return index
		}
		if p.tokens[index].Kind == TokenEOF {
			return index
		}
	}
	return -1
}

func (p *swiftParser) previousCode(index, start int) int {
	for index >= start {
		if p.tokens[index].Kind != TokenNewline && p.tokens[index].Kind != TokenDirective {
			return index
		}
		index--
	}
	return -1
}

func (p *swiftParser) skipStatement(start, end int) int {
	if start >= end {
		return end
	}
	base := p.tokens[start].Nesting
	for index := start; index < end; index++ {
		if p.tokens[index].Text == "{" && p.tokens[index].Nesting == base+1 {
			if close := p.pairs[index]; close > index {
				return close + 1
			}
		}
		if (p.tokens[index].Kind == TokenNewline || p.tokens[index].Text == ";") && p.tokens[index].Nesting == base {
			return index + 1
		}
		if p.tokens[index].Kind == TokenEOF {
			return index
		}
	}
	return end
}

func (p *swiftParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
	symbol, err := p.builder.Add(spec)
	if operation.KindOf(err) == operation.KindLimit {
		p.stopped = true
		return NormalizedSymbol{}, false
	}
	if err != nil {
		p.builder.MarkIncomplete()
		return NormalizedSymbol{}, false
	}
	return symbol, true
}
