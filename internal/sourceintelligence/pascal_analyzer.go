package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// PascalAnalyzer performs bounded declaration-level Pascal analysis.
type PascalAnalyzer struct{}

func (PascalAnalyzer) ID() AnalyzerID   { return AnalyzerPascal }
func (PascalAnalyzer) Language() string { return "pascal" }
func (PascalAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePascalFamily(ctx, document, options, false)
}

// DelphiAnalyzer extends the shared Pascal structural parser with Delphi/Object
// Pascal constructs such as qualified implementations, generics, and helpers.
type DelphiAnalyzer struct{}

func (DelphiAnalyzer) ID() AnalyzerID   { return AnalyzerDelphi }
func (DelphiAnalyzer) Language() string { return "delphi" }
func (DelphiAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePascalFamily(ctx, document, options, true)
}

func analyzePascalFamily(ctx context.Context, document *SourceDocument, options AnalyzeOptions, delphi bool) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_pascal_source", document.Path, err)
	}
	language := "pascal"
	analyzer := AnalyzerPascal
	profile := PascalScannerProfile()
	if delphi {
		language = "delphi"
		analyzer = AnalyzerDelphi
		profile = DelphiScannerProfile()
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
	scan, err := ScanSource(ctx, document, profile, ScannerLimits{MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range scan.Diagnostics {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: language + "-" + diagnostic.Code, Message: diagnostic.Message, Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	parser := &pascalParser{
		ctx: ctx, document: document, builder: builder, delphi: delphi, language: language,
		types: make(map[string]SymbolParent),
	}
	parser.parse(BuildLogicalLines(scan.Tokens, LogicalLineProfile{}))
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_pascal_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

type pascalScope struct {
	kind       string
	parent     SymbolParent
	owner      string
	visibility Visibility
	beginDepth int
}

type pascalParser struct {
	ctx          context.Context
	document     *SourceDocument
	builder      *SymbolBuilder
	delphi       bool
	language     string
	dependencies []StructuralDependency
	relations    []StructuralRelation
	types        map[string]SymbolParent
	scopes       []pascalScope
	module       *SymbolParent
	section      string
	mode         string
	stopped      bool
}

func (p *pascalParser) parse(lines []LogicalLine) {
	for _, line := range lines {
		if p.stopped || p.ctx.Err() != nil || len(line.Tokens) == 0 {
			continue
		}
		p.parseLine(line)
	}
	if len(p.scopes) > 0 && !p.stopped {
		_ = p.builder.AddDiagnostic(DiagnosticSpec{Code: p.language + "-unterminated-scope", Message: "Pascal source contains one or more declarations without a matching end", Severity: DiagnosticWarning, AffectsCoverage: true})
	}
}

func (p *pascalParser) parseLine(line LogicalLine) {
	tokens := line.Tokens
	first := 0
	word := strings.ToLower(tokens[first].Text)

	if p.handleEndOrBegin(tokens) {
		return
	}
	if word == "program" || word == "unit" || word == "package" {
		p.parseModule(line, first, word)
		p.mode = ""
		return
	}
	if word == "interface" && p.currentType() == nil {
		p.section = "interface-section"
		p.mode = ""
		return
	}
	if word == "implementation" && p.currentType() == nil {
		p.section = "implementation-section"
		p.mode = ""
		return
	}
	if word == "uses" {
		p.parseUses(tokens, first)
		p.mode = ""
		return
	}
	if scope := p.currentType(); scope != nil {
		if pascalVisibilityLine(tokens) {
			p.setTypeVisibility(tokens)
			return
		}
		if p.isRoutineKeyword(word) {
			p.parseRoutine(line, first, true)
			return
		}
		if word == "property" {
			p.parseProperty(line, first, &scope.parent)
			return
		}
		if p.parseTypeField(line, scope) {
			return
		}
	}

	if word == "type" {
		p.mode = "type"
		if len(tokens) > 1 {
			p.parseTypeDeclaration(LogicalLine{Tokens: tokens[1:], StartOffset: tokens[1].StartOffset, EndOffset: line.EndOffset})
		}
		return
	}
	if word == "const" {
		p.mode = "const"
		if len(tokens) > 1 {
			p.parseConstDeclaration(LogicalLine{Tokens: tokens[1:], StartOffset: tokens[1].StartOffset, EndOffset: line.EndOffset})
		}
		return
	}
	if word == "var" {
		p.mode = "var"
		if len(tokens) > 1 {
			p.parseVarDeclaration(LogicalLine{Tokens: tokens[1:], StartOffset: tokens[1].StartOffset, EndOffset: line.EndOffset})
		}
		return
	}
	if p.isRoutineKeyword(word) {
		p.mode = ""
		p.parseRoutine(line, first, false)
		return
	}
	switch p.mode {
	case "type":
		p.parseTypeDeclaration(line)
	case "const":
		p.parseConstDeclaration(line)
	case "var":
		p.parseVarDeclaration(line)
	}
}

func (p *pascalParser) handleEndOrBegin(tokens []Token) bool {
	if len(tokens) == 0 {
		return false
	}
	first := strings.ToLower(tokens[0].Text)
	if first == "begin" {
		if len(p.scopes) > 0 && p.scopes[len(p.scopes)-1].kind == "routine" {
			p.scopes[len(p.scopes)-1].beginDepth++
		}
		return true
	}
	if first != "end" {
		return false
	}
	if len(p.scopes) == 0 {
		return true
	}
	top := &p.scopes[len(p.scopes)-1]
	if top.kind == "routine" && top.beginDepth > 1 {
		top.beginDepth--
		return true
	}
	p.scopes = p.scopes[:len(p.scopes)-1]
	return true
}

func (p *pascalParser) parseModule(line LogicalLine, keyword int, nativeKind string) {
	nameStart := nextIdentifierToken(line.Tokens, keyword+1, len(line.Tokens))
	if nameStart < 0 {
		p.markMalformed(line, p.language+"-module-name", "Pascal module declaration has no name")
		return
	}
	nameEnd := pascalQualifiedNameEnd(line.Tokens, nameStart)
	name := tokenRangeText(line.Tokens, nameStart, nameEnd)
	symbol, ok := p.add(SymbolSpec{Kind: SymbolKindModule, NativeKind: nativeKind, Name: name, QualifiedName: name,
		Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset},
		NameRange:   OffsetRange{Start: line.Tokens[nameStart].StartOffset, End: line.Tokens[nameEnd-1].EndOffset},
		Signature:   &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
	if ok {
		parent := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		p.module = &parent
	}
}

func (p *pascalParser) parseUses(tokens []Token, keyword int) {
	end := len(tokens)
	for end > keyword+1 && tokens[end-1].Text == ";" {
		end--
	}
	for _, part := range splitTokenRangeAt(tokens, keyword+1, end, ",", tokens[keyword].Nesting) {
		start := part[0]
		last := part[1] - 1
		for start <= last && tokens[start].Kind == TokenString {
			start++
		}
		if start > last {
			continue
		}
		value := tokenRangeText(tokens, start, last+1)
		if value == "" {
			continue
		}
		rangeValue, err := p.document.RangeFromUTF8Offsets(tokens[start].StartOffset, tokens[last].EndOffset)
		if err == nil {
			p.dependencies = append(p.dependencies, StructuralDependency{Kind: StructuralDependencyImport, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural})
		}
	}
}

func (p *pascalParser) parseTypeDeclaration(line LogicalLine) {
	tokens := line.Tokens
	if len(tokens) < 3 {
		return
	}
	equal := pascalFindText(tokens, "=", 0)
	if equal <= 0 || equal+1 >= len(tokens) {
		return
	}
	nameIndex := nextIdentifierToken(tokens, 0, equal)
	if nameIndex < 0 {
		return
	}
	name := tokens[nameIndex].Text
	rhs := equal + 1
	for rhs < len(tokens) && tokens[rhs].Kind == TokenNewline {
		rhs++
	}
	if rhs >= len(tokens) {
		return
	}
	parent := p.currentDeclarationParent()
	sectionModifiers := p.sectionModifiers()
	if tokens[rhs].Text == "(" {
		p.add(SymbolSpec{Kind: SymbolKindEnum, NativeKind: "enum", Name: name, Parent: parent,
			Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: tokens[nameIndex].StartOffset, End: tokens[nameIndex].EndOffset},
			Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Modifiers: sectionModifiers, Evidence: SymbolEvidenceStructural})
		return
	}
	kindWord := strings.ToLower(tokens[rhs].Text)
	if kindWord != "class" && kindWord != "record" && kindWord != "interface" && kindWord != "object" {
		p.add(SymbolSpec{Kind: SymbolKindAlias, NativeKind: "type-alias", Name: name, Parent: parent,
			Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: tokens[nameIndex].StartOffset, End: tokens[nameIndex].EndOffset},
			Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Modifiers: sectionModifiers, Evidence: SymbolEvidenceStructural})
		return
	}
	kind := SymbolKindClass
	nativeKind := kindWord
	if kindWord == "record" {
		kind = SymbolKindRecord
	} else if kindWord == "interface" {
		kind = SymbolKindInterface
	} else if kindWord == "object" {
		kind = SymbolKindType
	}
	helperIndex := pascalFindFold(tokens, "helper", rhs+1)
	forIndex := pascalFindFold(tokens, "for", rhs+1)
	if helperIndex >= 0 && forIndex > helperIndex {
		kind = SymbolKindType
		nativeKind = kindWord + "-helper"
	}
	symbol, ok := p.add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: name, Parent: parent,
		Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: tokens[nameIndex].StartOffset, End: tokens[nameIndex].EndOffset},
		Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Modifiers: sectionModifiers, Evidence: SymbolEvidenceStructural})
	if !ok {
		return
	}
	typeParent := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
	p.types[symbol.Name] = typeParent
	p.types[symbol.QualifiedName] = typeParent
	if helperIndex >= 0 && forIndex > helperIndex {
		end := len(tokens)
		for end > forIndex+1 && tokens[end-1].Text == ";" {
			end--
		}
		p.addRelation("helper-for", symbol.QualifiedName, tokenRangeText(tokens, forIndex+1, end), tokens[forIndex+1].StartOffset, tokens[end-1].EndOffset)
	} else {
		p.collectPascalTypeRelations(symbol.QualifiedName, kindWord, tokens, rhs+1)
	}
	if !pascalContainsFold(tokens, "end") {
		p.scopes = append(p.scopes, pascalScope{kind: "type", parent: typeParent, owner: symbol.Name, visibility: VisibilityPublic})
	}
}

func (p *pascalParser) collectPascalTypeRelations(source, kindWord string, tokens []Token, start int) {
	open := pascalFindText(tokens, "(", start)
	if open < 0 {
		return
	}
	pairs := PairDelimiterTokens(tokens, nil)
	close := pairs[open]
	if close <= open {
		return
	}
	for index, part := range splitTokenRangeAt(tokens, open+1, close, ",", tokens[open].Nesting) {
		target := tokenRangeText(tokens, part[0], part[1])
		kind := "implements"
		if kindWord == "interface" || index == 0 {
			kind = "inherits"
		}
		p.addRelation(kind, source, target, tokens[part[0]].StartOffset, tokens[part[1]-1].EndOffset)
	}
}

func (p *pascalParser) parseTypeField(line LogicalLine, scope *pascalScope) bool {
	tokens := line.Tokens
	colon := pascalFindText(tokens, ":", 0)
	if colon <= 0 || pascalFindText(tokens, "=", 0) >= 0 {
		return false
	}
	added := false
	for _, part := range splitTokenRangeAt(tokens, 0, colon, ",", tokens[0].Nesting) {
		nameIndex := nextIdentifierToken(tokens, part[0], part[1])
		if nameIndex < 0 {
			continue
		}
		p.add(SymbolSpec{Kind: SymbolKindField, NativeKind: "field", Name: tokens[nameIndex].Text, Parent: &scope.parent,
			Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: tokens[nameIndex].StartOffset, End: tokens[nameIndex].EndOffset},
			Visibility: scope.visibility, Modifiers: p.sectionModifiers(), Evidence: SymbolEvidenceStructural})
		added = true
	}
	return added
}

func (p *pascalParser) parseProperty(line LogicalLine, keyword int, parent *SymbolParent) {
	nameIndex := nextIdentifierToken(line.Tokens, keyword+1, len(line.Tokens))
	if nameIndex < 0 {
		return
	}
	visibility := VisibilityPublic
	if scope := p.currentType(); scope != nil {
		visibility = scope.visibility
	}
	p.add(SymbolSpec{Kind: SymbolKindProperty, NativeKind: "property", Name: line.Tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset},
		Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Visibility: visibility, Modifiers: p.sectionModifiers(), Evidence: SymbolEvidenceStructural})
}

func (p *pascalParser) parseRoutine(line LogicalLine, keyword int, member bool) {
	tokens := line.Tokens
	kindWord := strings.ToLower(tokens[keyword].Text)
	nameStart := nextIdentifierToken(tokens, keyword+1, len(tokens))
	if nameStart < 0 {
		p.markMalformed(line, p.language+"-routine-name", "Pascal routine declaration has no name")
		return
	}
	nameEnd := pascalRoutineNameEnd(tokens, nameStart)
	nameToken := previousIdentifierToken(tokens, nameEnd-1, nameStart)
	if nameToken < 0 {
		return
	}
	sourceName := tokens[nameToken].Text
	parent := p.currentDeclarationParent()
	owner := ""
	qualifiedOwner := pascalOwnerBeforeRoutine(tokens, nameStart, nameEnd)
	if qualifiedOwner != "" {
		baseOwner := pascalBaseTypeName(qualifiedOwner)
		if resolved, ok := p.types[baseOwner]; ok {
			value := resolved
			parent = &value
			owner = baseOwner
		}
	}
	if member {
		if scope := p.currentType(); scope != nil {
			value := scope.parent
			parent = &value
			owner = scope.owner
		}
	}
	name := sourceName
	kind := SymbolKindFunction
	if member || qualifiedOwner != "" {
		kind = SymbolKindMethod
	}
	if kindWord == "constructor" {
		kind = SymbolKindConstructor
		if owner != "" {
			name = owner
		}
	} else if kindWord == "destructor" {
		kind = SymbolKindDestructor
	}
	forward := pascalContainsFold(tokens, "forward")
	declarationOnly := member || p.section == "interface-section"
	nativeKind := kindWord + "-definition"
	if forward {
		nativeKind = kindWord + "-forward"
	} else if declarationOnly {
		nativeKind = kindWord + "-declaration"
	}
	visibility := VisibilityPublic
	if scope := p.currentType(); scope != nil {
		visibility = scope.visibility
	}
	modifiers := p.sectionModifiers()
	for _, modifier := range []string{"overload", "virtual", "override", "abstract", "static", "inline"} {
		if pascalContainsFold(tokens, modifier) {
			modifiers = append(modifiers, modifier)
		}
	}
	symbol, ok := p.add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: name, Parent: parent,
		Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: tokens[nameToken].StartOffset, End: tokens[nameToken].EndOffset},
		Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Visibility: visibility, Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
		Disambiguator: tokenRangeText(tokens, nameStart, len(tokens)) + ":" + nativeKind})
	if !ok || forward || declarationOnly {
		return
	}
	parentValue := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
	p.scopes = append(p.scopes, pascalScope{kind: "routine", parent: parentValue, owner: symbol.Name, visibility: VisibilityPublic})
}

func (p *pascalParser) parseConstDeclaration(line LogicalLine) {
	tokens := line.Tokens
	if len(tokens) < 2 {
		return
	}
	equal := pascalFindText(tokens, "=", 0)
	if equal <= 0 {
		return
	}
	nameIndex := previousIdentifierToken(tokens, equal-1, 0)
	if nameIndex < 0 {
		return
	}
	p.add(SymbolSpec{Kind: SymbolKindConstant, NativeKind: "const", Name: tokens[nameIndex].Text, Parent: p.currentDeclarationParent(),
		Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: tokens[nameIndex].StartOffset, End: tokens[nameIndex].EndOffset},
		Modifiers: p.sectionModifiers(), Evidence: SymbolEvidenceStructural})
}

func (p *pascalParser) parseVarDeclaration(line LogicalLine) {
	tokens := line.Tokens
	colon := pascalFindText(tokens, ":", 0)
	if colon <= 0 {
		return
	}
	for _, part := range splitTokenRangeAt(tokens, 0, colon, ",", tokens[0].Nesting) {
		nameIndex := nextIdentifierToken(tokens, part[0], part[1])
		if nameIndex < 0 {
			continue
		}
		p.add(SymbolSpec{Kind: SymbolKindVariable, NativeKind: "var", Name: tokens[nameIndex].Text, Parent: p.currentDeclarationParent(),
			Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: tokens[nameIndex].StartOffset, End: tokens[nameIndex].EndOffset},
			Modifiers: p.sectionModifiers(), Evidence: SymbolEvidenceStructural})
	}
}

func (p *pascalParser) currentType() *pascalScope {
	if len(p.scopes) == 0 || p.scopes[len(p.scopes)-1].kind != "type" {
		return nil
	}
	return &p.scopes[len(p.scopes)-1]
}

func (p *pascalParser) currentDeclarationParent() *SymbolParent {
	if len(p.scopes) > 0 {
		value := p.scopes[len(p.scopes)-1].parent
		return &value
	}
	if p.module != nil {
		value := *p.module
		return &value
	}
	return nil
}

func (p *pascalParser) setTypeVisibility(tokens []Token) {
	if len(p.scopes) == 0 || p.scopes[len(p.scopes)-1].kind != "type" {
		return
	}
	visibility := VisibilityPublic
	for _, token := range tokens {
		switch strings.ToLower(token.Text) {
		case "private":
			visibility = VisibilityPrivate
		case "protected":
			visibility = VisibilityProtected
		case "published", "public":
			visibility = VisibilityPublic
		}
	}
	p.scopes[len(p.scopes)-1].visibility = visibility
}

func pascalVisibilityLine(tokens []Token) bool {
	if len(tokens) == 0 || len(tokens) > 2 {
		return false
	}
	for _, token := range tokens {
		switch strings.ToLower(token.Text) {
		case "strict", "private", "protected", "published", "public":
		default:
			return false
		}
	}
	return true
}

func (p *pascalParser) sectionModifiers() []string {
	if p.section == "" {
		return nil
	}
	return []string{p.section}
}

func (p *pascalParser) isRoutineKeyword(value string) bool {
	switch value {
	case "procedure", "function", "constructor", "destructor":
		return true
	default:
		return false
	}
}

func pascalFindText(tokens []Token, text string, start int) int {
	for index := max(start, 0); index < len(tokens); index++ {
		if tokens[index].Text == text {
			return index
		}
	}
	return -1
}

func pascalFindFold(tokens []Token, text string, start int) int {
	for index := max(start, 0); index < len(tokens); index++ {
		if strings.EqualFold(tokens[index].Text, text) {
			return index
		}
	}
	return -1
}

func pascalContainsFold(tokens []Token, text string) bool {
	return pascalFindFold(tokens, text, 0) >= 0
}

func pascalQualifiedNameEnd(tokens []Token, start int) int {
	end := start + 1
	for end+1 < len(tokens) && tokens[end].Text == "." && tokens[end+1].Kind == TokenIdentifier {
		end += 2
	}
	return min(end, len(tokens))
}

func pascalRoutineNameEnd(tokens []Token, start int) int {
	angle := 0
	end := start
	for end < len(tokens) {
		switch tokens[end].Text {
		case "<":
			angle++
		case ">":
			if angle > 0 {
				angle--
			}
		case "(":
			if angle == 0 {
				return end
			}
		case ";", ":":
			if angle == 0 {
				return end
			}
		}
		end++
	}
	return end
}

func pascalOwnerBeforeRoutine(tokens []Token, start, end int) string {
	lastDot := -1
	angle := 0
	for index := start; index < end; index++ {
		switch tokens[index].Text {
		case "<":
			angle++
		case ">":
			if angle > 0 {
				angle--
			}
		case ".":
			if angle == 0 {
				lastDot = index
			}
		}
	}
	if lastDot <= start {
		return ""
	}
	return tokenRangeText(tokens, start, lastDot)
}

func pascalBaseTypeName(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.LastIndexByte(value, '.'); index >= 0 {
		value = value[index+1:]
	}
	if index := strings.IndexByte(value, '<'); index >= 0 {
		value = value[:index]
	}
	return value
}

func (p *pascalParser) addRelation(kind, source, target string, start, end int) {
	target = strings.TrimSpace(target)
	if source == "" || target == "" || start < 0 || end <= start {
		return
	}
	rangeValue, err := p.document.RangeFromUTF8Offsets(start, end)
	if err == nil {
		p.relations = append(p.relations, StructuralRelation{Kind: kind, Source: source, Target: target, Range: rangeValue, Evidence: SymbolEvidenceStructural})
	}
}

func (p *pascalParser) markMalformed(line LogicalLine, code, message string) {
	value := OffsetRange{Start: line.StartOffset, End: line.EndOffset}
	_ = p.builder.AddDiagnostic(DiagnosticSpec{Code: code, Message: message, Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
}

func (p *pascalParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
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
