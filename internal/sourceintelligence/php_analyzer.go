package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// PHPAnalyzer performs bounded declaration-level PHP analysis without executing
// includes, autoloaders, Composer, or project code.
type PHPAnalyzer struct{}

func (PHPAnalyzer) ID() AnalyzerID   { return AnalyzerPHP }
func (PHPAnalyzer) Language() string { return "php" }
func (PHPAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_php_source", document.Path, err)
	}
	masked, heredocDiagnostics, err := maskPHPHeredocs(ctx, document.Text)
	if err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_php_source", document.Path, err)
	}
	scanDocument := document
	if masked != document.Text {
		clone := *document
		clone.Text = masked
		clone.lineStarts = buildLineStarts(masked)
		scanDocument = &clone
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: "php", Analyzer: string(AnalyzerPHP), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, scanDocument, PHPScannerProfile(), ScannerLimits{MaxTokens: scannerTokenBudget(scanDocument.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range append(heredocDiagnostics, scan.Diagnostics...) {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: "php-" + diagnostic.Code, Message: diagnostic.Message, Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
	}
	if !scan.Complete || len(heredocDiagnostics) > 0 {
		builder.MarkIncomplete()
	}
	parser := &phpParser{ctx: ctx, document: document, tokens: scan.Tokens, pairs: PairDelimiterTokens(scan.Tokens, nil), builder: builder}
	parser.parseScope(0, len(scan.Tokens), nil, false, "")
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_php_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

type phpParser struct {
	ctx          context.Context
	document     *SourceDocument
	tokens       []Token
	pairs        map[int]int
	builder      *SymbolBuilder
	dependencies []StructuralDependency
	relations    []StructuralRelation
	stopped      bool
}

var phpModifiers = map[string]struct{}{
	"abstract": {}, "final": {}, "private": {}, "protected": {}, "public": {}, "readonly": {}, "static": {},
}

func (p *phpParser) parseScope(start, end int, parent *SymbolParent, members bool, owner string) {
	currentParent := parent
	for index := start; index < end && !p.stopped; {
		if p.ctx.Err() != nil {
			return
		}
		index = p.skipTrivia(index, end)
		if index >= end || p.tokens[index].Kind == TokenEOF {
			return
		}
		if p.tokens[index].Text == "<?" || (p.tokens[index].Text == "<" && index+1 < end && p.tokens[index+1].Text == "?") {
			index++
			if index < end && p.tokens[index].Text == "?" {
				index++
			}
			if index < end && strings.EqualFold(p.tokens[index].Text, "php") {
				index++
			}
			continue
		}
		semantic := p.skipModifiers(index, end)
		if semantic >= end {
			return
		}
		if !members && p.token(semantic, "namespace") {
			next, ns, braced := p.parseNamespace(index, semantic, end, currentParent)
			if ns != nil && !braced {
				currentParent = ns
			}
			index = next
			continue
		}
		if p.token(semantic, "use") {
			if members {
				index = p.parseTraitUse(semantic, end, currentParent)
			} else {
				index = p.parseUse(semantic, end)
			}
			continue
		}
		if !members && p.isIncludeKeyword(semantic) {
			index = p.parseInclude(semantic, end)
			continue
		}
		if p.isTypeKeyword(semantic) {
			index = p.parseType(index, semantic, end, currentParent)
			continue
		}
		if p.token(semantic, "function") {
			index = p.parseFunction(index, semantic, end, currentParent, members, owner)
			continue
		}
		if members || p.token(semantic, "const") {
			if next, ok := p.parseValue(index, semantic, end, currentParent, members); ok {
				index = next
				continue
			}
		}
		index = p.skipStatement(index, end)
	}
}

func (p *phpParser) token(index int, text string) bool {
	return index >= 0 && index < len(p.tokens) && strings.EqualFold(p.tokens[index].Text, text)
}

func (p *phpParser) skipTrivia(index, end int) int {
	for index < end && (p.tokens[index].Kind == TokenNewline || p.tokens[index].Kind == TokenDirective) {
		index++
	}
	return index
}

func (p *phpParser) skipModifiers(index, end int) int {
	for index < end {
		if _, ok := phpModifiers[strings.ToLower(p.tokens[index].Text)]; ok {
			index++
			continue
		}
		return index
	}
	return end
}

func (p *phpParser) isTypeKeyword(index int) bool {
	return p.token(index, "class") || p.token(index, "interface") || p.token(index, "trait") || p.token(index, "enum")
}

func (p *phpParser) isIncludeKeyword(index int) bool {
	return p.token(index, "include") || p.token(index, "include_once") || p.token(index, "require") || p.token(index, "require_once")
}

func (p *phpParser) parseNamespace(declarationStart, keyword, end int, parent *SymbolParent) (int, *SymbolParent, bool) {
	base := p.tokens[keyword].Nesting
	terminator := p.findEither(keyword+1, end, base, ";", "{")
	if terminator < 0 {
		p.builder.MarkIncomplete()
		return end, nil, false
	}
	nameStart := p.skipTrivia(keyword+1, terminator)
	nameEnd := p.previousCode(terminator-1, nameStart)
	if nameEnd < nameStart {
		return terminator + 1, nil, false
	}
	name := tokenRangeText(p.tokens, nameStart, nameEnd+1)
	if name == "" {
		return terminator + 1, nil, false
	}
	declarationEnd := p.tokens[terminator].EndOffset
	var body *OffsetRange
	next := terminator + 1
	braced := p.tokens[terminator].Text == "{"
	if braced {
		close := p.pairs[terminator]
		if close <= terminator || close >= end {
			p.builder.MarkIncomplete()
			return end, nil, true
		}
		declarationEnd = p.tokens[close].EndOffset
		value := OffsetRange{Start: p.tokens[terminator].StartOffset, End: p.tokens[close].EndOffset}
		body = &value
		next = close + 1
	}
	symbol, added := p.add(SymbolSpec{Kind: SymbolKindNamespace, NativeKind: "namespace", Name: name, QualifiedName: name, Parent: parent,
		Declaration: OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: p.tokens[nameStart].StartOffset, End: p.tokens[nameEnd].EndOffset},
		Signature:   &OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: p.tokens[terminator].StartOffset}, Body: body, Evidence: SymbolEvidenceStructural})
	if !added {
		return next, nil, braced
	}
	nsParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
	if braced {
		close := p.pairs[terminator]
		p.parseScope(terminator+1, close, nsParent, false, "")
	}
	return next, nsParent, braced
}

func (p *phpParser) parseUse(keyword, end int) int {
	semicolon := p.findToken(keyword+1, end, ";", p.tokens[keyword].Nesting)
	if semicolon < 0 {
		p.builder.MarkIncomplete()
		return end
	}
	start := p.skipTrivia(keyword+1, semicolon)
	if p.token(start, "function") || p.token(start, "const") {
		start = p.skipTrivia(start+1, semicolon)
	}
	for _, part := range splitTokenRangeAt(p.tokens, start, semicolon, ",", p.tokens[keyword].Nesting) {
		valueEnd := part[1]
		for index := part[0]; index < part[1]; index++ {
			if p.token(index, "as") {
				valueEnd = index
				break
			}
		}
		last := p.previousCode(valueEnd-1, part[0])
		if last < part[0] {
			continue
		}
		value := tokenRangeText(p.tokens, part[0], last+1)
		if value == "" {
			continue
		}
		rangeValue, err := p.document.RangeFromUTF8Offsets(p.tokens[part[0]].StartOffset, p.tokens[last].EndOffset)
		if err == nil {
			p.dependencies = append(p.dependencies, StructuralDependency{Kind: StructuralDependencyImport, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural})
		}
	}
	return semicolon + 1
}

func (p *phpParser) parseInclude(keyword, end int) int {
	semicolon := p.findToken(keyword+1, end, ";", p.tokens[keyword].Nesting)
	if semicolon < 0 {
		semicolon = min(p.statementEnd(keyword, end), end-1)
	}
	for index := keyword + 1; index <= semicolon && index < end; index++ {
		if p.tokens[index].Kind != TokenString {
			if p.tokens[index].Kind == TokenIdentifier && strings.HasPrefix(p.tokens[index].Text, "$") {
				break
			}
			continue
		}
		value := phpStringLiteralValue(p.tokens[index].Text)
		if value != "" {
			rangeValue, err := p.document.RangeFromUTF8Offsets(p.tokens[index].StartOffset, p.tokens[index].EndOffset)
			if err == nil {
				p.dependencies = append(p.dependencies, StructuralDependency{Kind: StructuralDependencyInclude, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural})
			}
		}
		break
	}
	return min(semicolon+1, end)
}

func phpStringLiteralValue(value string) string {
	if len(value) < 2 || (value[0] != '\'' && value[0] != '"') || value[len(value)-1] != value[0] {
		return ""
	}
	return value[1 : len(value)-1]
}

func (p *phpParser) parseType(declarationStart, keyword, end int, parent *SymbolParent) int {
	nameIndex := nextIdentifierToken(p.tokens, keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	open := p.findToken(nameIndex+1, end, "{", p.tokens[keyword].Nesting+1)
	if open < 0 {
		p.builder.MarkIncomplete()
		return p.skipStatement(declarationStart, end)
	}
	close := p.pairs[open]
	if close <= open || close >= end {
		p.builder.MarkIncomplete()
		return end
	}
	kind := SymbolKindClass
	nativeKind := strings.ToLower(p.tokens[keyword].Text)
	switch nativeKind {
	case "interface":
		kind = SymbolKindInterface
	case "trait":
		kind = SymbolKindTrait
	case "enum":
		kind = SymbolKindEnum
	}
	modifiers := collectKnownModifiers(p.tokens, declarationStart, keyword, phpModifiers)
	symbol, added := p.add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: p.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: p.tokens[close].EndOffset},
		NameRange:   OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: p.tokens[open].StartOffset},
		Body:        &OffsetRange{Start: p.tokens[open].StartOffset, End: p.tokens[close].EndOffset},
		Visibility:  visibilityFromModifiers(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural})
	if added {
		p.collectTypeRelations(symbol.QualifiedName, nameIndex+1, open, p.tokens[keyword].Nesting, nativeKind)
		typeParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		p.parseScope(open+1, close, typeParent, true, symbol.Name)
	}
	return close + 1
}

func (p *phpParser) collectTypeRelations(source string, start, end, nesting int, nativeKind string) {
	for index := start; index < end; index++ {
		kind := ""
		switch strings.ToLower(p.tokens[index].Text) {
		case "extends":
			kind = "extends"
		case "implements":
			kind = "implements"
		default:
			continue
		}
		clauseEnd := end
		for cursor := index + 1; cursor < end; cursor++ {
			if p.token(cursor, "extends") || p.token(cursor, "implements") {
				clauseEnd = cursor
				break
			}
		}
		parts := splitTokenRangeAt(p.tokens, index+1, clauseEnd, ",", nesting)
		if nativeKind == "class" && kind == "extends" && len(parts) > 1 {
			parts = parts[:1]
		}
		for _, part := range parts {
			p.addRelation(kind, source, tokenRangeText(p.tokens, part[0], part[1]), part[0], part[1])
		}
		index = clauseEnd - 1
	}
}

func (p *phpParser) parseTraitUse(keyword, end int, parent *SymbolParent) int {
	semicolon := p.findToken(keyword+1, end, ";", p.tokens[keyword].Nesting)
	if semicolon < 0 {
		return p.skipStatement(keyword, end)
	}
	if parent != nil {
		for _, part := range splitTokenRangeAt(p.tokens, keyword+1, semicolon, ",", p.tokens[keyword].Nesting) {
			p.addRelation("uses-trait", parent.QualifiedName, tokenRangeText(p.tokens, part[0], part[1]), part[0], part[1])
		}
	}
	return semicolon + 1
}

func (p *phpParser) parseFunction(declarationStart, keyword, end int, parent *SymbolParent, member bool, owner string) int {
	cursor := p.skipTrivia(keyword+1, end)
	if p.token(cursor, "&") {
		cursor++
	}
	nameIndex := nextIdentifierToken(p.tokens, cursor, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	paren := p.findToken(nameIndex+1, end, "(", p.tokens[keyword].Nesting+1)
	if paren < 0 {
		return nameIndex + 1
	}
	closeParen := p.pairs[paren]
	if closeParen <= paren {
		p.builder.MarkIncomplete()
		return paren + 1
	}
	bodyOpen := -1
	semicolon := -1
	base := p.tokens[keyword].Nesting
	for index := closeParen + 1; index < end; index++ {
		if p.tokens[index].Text == "{" && p.tokens[index].Nesting == base+1 {
			bodyOpen = index
			break
		}
		if p.tokens[index].Text == ";" && p.tokens[index].Nesting == base {
			semicolon = index
			break
		}
	}
	kind := SymbolKindFunction
	nativeKind := "function-declaration"
	name := p.tokens[nameIndex].Text
	if member {
		kind = SymbolKindMethod
		nativeKind = "method-declaration"
		if strings.EqualFold(name, "__construct") {
			kind = SymbolKindConstructor
			nativeKind = "constructor-declaration"
			name = owner
		} else if strings.EqualFold(name, "__destruct") {
			kind = SymbolKindDestructor
			nativeKind = "destructor-declaration"
		}
	}
	declarationEnd := p.tokens[closeParen].EndOffset
	signatureEnd := declarationEnd
	next := closeParen + 1
	var body *OffsetRange
	if bodyOpen >= 0 {
		closeBody := p.pairs[bodyOpen]
		if closeBody <= bodyOpen || closeBody >= end {
			p.builder.MarkIncomplete()
			return end
		}
		declarationEnd = p.tokens[closeBody].EndOffset
		signatureEnd = p.tokens[bodyOpen].StartOffset
		value := OffsetRange{Start: p.tokens[bodyOpen].StartOffset, End: p.tokens[closeBody].EndOffset}
		body = &value
		next = closeBody + 1
		switch kind {
		case SymbolKindFunction:
			nativeKind = "function-definition"
		case SymbolKindMethod:
			nativeKind = "method-definition"
		case SymbolKindConstructor:
			nativeKind = "constructor-definition"
		case SymbolKindDestructor:
			nativeKind = "destructor-definition"
		}
	} else if semicolon >= 0 {
		declarationEnd = p.tokens[semicolon].EndOffset
		signatureEnd = p.tokens[semicolon].StartOffset
		next = semicolon + 1
	}
	modifiers := collectKnownModifiers(p.tokens, declarationStart, keyword, phpModifiers)
	p.add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: name, Parent: parent,
		Declaration: OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: signatureEnd}, Body: body,
		Visibility: visibilityFromModifiers(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
		Disambiguator: tokenRangeText(p.tokens, paren, closeParen+1) + ":" + nativeKind})
	return next
}

func (p *phpParser) parseValue(declarationStart, semantic, end int, parent *SymbolParent, members bool) (int, bool) {
	semicolon := p.findToken(semantic, end, ";", p.tokens[semantic].Nesting)
	if semicolon < 0 {
		return declarationStart + 1, false
	}
	if p.token(semantic, "const") {
		nameIndex := nextIdentifierToken(p.tokens, semantic+1, semicolon)
		if nameIndex < 0 {
			return semicolon + 1, false
		}
		p.add(SymbolSpec{Kind: SymbolKindConstant, NativeKind: "const", Name: p.tokens[nameIndex].Text, Parent: parent,
			Declaration: OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: p.tokens[semicolon].EndOffset},
			NameRange:   OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset},
			Visibility:  p.visibility(declarationStart, semantic), Modifiers: collectKnownModifiers(p.tokens, declarationStart, semantic, phpModifiers), Evidence: SymbolEvidenceStructural})
		return semicolon + 1, true
	}
	for index := semantic; index < semicolon; index++ {
		if p.tokens[index].Kind == TokenIdentifier && strings.HasPrefix(p.tokens[index].Text, "$") {
			kind := SymbolKindVariable
			nativeKind := "variable"
			if members {
				kind = SymbolKindProperty
				nativeKind = "property"
			}
			p.add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: p.tokens[index].Text, Parent: parent,
				Declaration: OffsetRange{Start: p.tokens[declarationStart].StartOffset, End: p.tokens[semicolon].EndOffset},
				NameRange:   OffsetRange{Start: p.tokens[index].StartOffset, End: p.tokens[index].EndOffset},
				Visibility:  p.visibility(declarationStart, semantic), Modifiers: collectKnownModifiers(p.tokens, declarationStart, index, phpModifiers), Evidence: SymbolEvidenceStructural})
			return semicolon + 1, true
		}
	}
	return semicolon + 1, false
}

func (p *phpParser) visibility(start, end int) Visibility {
	return visibilityFromModifiers(collectKnownModifiers(p.tokens, start, end, phpModifiers))
}

func (p *phpParser) addRelation(kind, source, target string, start, end int) {
	target = strings.TrimSpace(target)
	if source == "" || target == "" || start < 0 || end <= start || end > len(p.tokens) {
		return
	}
	rangeValue, err := p.document.RangeFromUTF8Offsets(p.tokens[start].StartOffset, p.tokens[end-1].EndOffset)
	if err == nil {
		p.relations = append(p.relations, StructuralRelation{Kind: kind, Source: source, Target: target, Range: rangeValue, Evidence: SymbolEvidenceStructural})
	}
}

func (p *phpParser) findEither(start, end, nesting int, a, b string) int {
	for index := start; index < end; index++ {
		if p.tokens[index].Nesting == nesting && (p.tokens[index].Text == a || p.tokens[index].Text == b) {
			return index
		}
	}
	return -1
}

func (p *phpParser) findToken(start, end int, text string, nesting int) int {
	for index := start; index < end; index++ {
		if p.tokens[index].Text == text && p.tokens[index].Nesting == nesting {
			return index
		}
	}
	return -1
}

func (p *phpParser) previousCode(index, start int) int {
	for index >= start {
		if p.tokens[index].Kind != TokenNewline && p.tokens[index].Kind != TokenDirective {
			return index
		}
		index--
	}
	return -1
}

func (p *phpParser) statementEnd(start, end int) int {
	base := p.tokens[start].Nesting
	for index := start; index < end; index++ {
		if p.tokens[index].Text == ";" && p.tokens[index].Nesting == base {
			return index
		}
		if p.tokens[index].Kind == TokenNewline && p.tokens[index].Nesting == base {
			return index
		}
		if p.tokens[index].Kind == TokenEOF {
			return index
		}
	}
	return end - 1
}

func (p *phpParser) skipStatement(start, end int) int {
	if start >= end {
		return end
	}
	terminator := p.statementEnd(start, end)
	if terminator < start {
		return start + 1
	}
	return min(terminator+1, end)
}

func (p *phpParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
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

func maskPHPHeredocs(ctx context.Context, text string) (string, []ScannerDiagnostic, error) {
	masked := []byte(text)
	changed := false
	var diagnostics []ScannerDiagnostic
	for at := 0; at < len(text); {
		if at&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return "", nil, err
			}
		}
		if strings.HasPrefix(text[at:], "//") || text[at] == '#' {
			for at < len(text) && text[at] != '\r' && text[at] != '\n' {
				at++
			}
			continue
		}
		if strings.HasPrefix(text[at:], "/*") {
			end := strings.Index(text[at+2:], "*/")
			if end < 0 {
				break
			}
			at += end + 4
			continue
		}
		if text[at] == '\'' || text[at] == '"' {
			quote := text[at]
			at++
			for at < len(text) {
				if text[at] == '\\' {
					at += min(2, len(text)-at)
					continue
				}
				if text[at] == quote {
					at++
					break
				}
				at++
			}
			continue
		}
		if !strings.HasPrefix(text[at:], "<<<") {
			at++
			continue
		}
		open := at
		cursor := at + 3
		for cursor < len(text) && (text[cursor] == ' ' || text[cursor] == '\t') {
			cursor++
		}
		quote := byte(0)
		if cursor < len(text) && (text[cursor] == '\'' || text[cursor] == '"') {
			quote = text[cursor]
			cursor++
		}
		nameStart := cursor
		for cursor < len(text) && (text[cursor] == '_' || text[cursor] >= 'A' && text[cursor] <= 'Z' || text[cursor] >= 'a' && text[cursor] <= 'z' || cursor > nameStart && text[cursor] >= '0' && text[cursor] <= '9') {
			cursor++
		}
		if cursor == nameStart {
			at += 3
			continue
		}
		name := text[nameStart:cursor]
		if quote != 0 {
			if cursor >= len(text) || text[cursor] != quote {
				at += 3
				continue
			}
			cursor++
		}
		openingValid := true
		for cursor < len(text) && text[cursor] != '\r' && text[cursor] != '\n' {
			if text[cursor] != ' ' && text[cursor] != '\t' {
				openingValid = false
				break
			}
			cursor++
		}
		if !openingValid {
			at += 3
			continue
		}
		if cursor < len(text) && text[cursor] == '\r' && cursor+1 < len(text) && text[cursor+1] == '\n' {
			cursor += 2
		} else if cursor < len(text) {
			cursor++
		}
		end := -1
		for lineStart := cursor; lineStart <= len(text); {
			lineEnd := lineStart
			for lineEnd < len(text) && text[lineEnd] != '\r' && text[lineEnd] != '\n' {
				lineEnd++
			}
			candidate := strings.TrimSpace(text[lineStart:lineEnd])
			candidate = strings.TrimSuffix(candidate, ";")
			candidate = strings.TrimSpace(candidate)
			if candidate == name {
				end = lineEnd
				break
			}
			if lineEnd >= len(text) {
				break
			}
			if text[lineEnd] == '\r' && lineEnd+1 < len(text) && text[lineEnd+1] == '\n' {
				lineStart = lineEnd + 2
			} else {
				lineStart = lineEnd + 1
			}
		}
		if end < 0 {
			diagnostics = append(diagnostics, ScannerDiagnostic{Code: "unterminated-heredoc", Message: "PHP heredoc/nowdoc literal is not terminated", StartOffset: open, EndOffset: len(text)})
			end = len(text)
		}
		for i := open; i < end; i++ {
			if masked[i] != '\r' && masked[i] != '\n' {
				masked[i] = ' '
			}
		}
		changed = true
		at = end
		continue
	}
	if !changed {
		return text, diagnostics, nil
	}
	return string(masked), diagnostics, nil
}
