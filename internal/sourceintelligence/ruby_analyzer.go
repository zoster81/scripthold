package sourceintelligence

import (
	"context"
	"strings"
	"unicode"

	"github.com/zoster81/scripthold/internal/operation"
)

// RubyAnalyzer performs bounded declaration-level Ruby analysis without
// evaluating DSLs, metaprogramming, require targets, or runtime dispatch.
type RubyAnalyzer struct{}

func (RubyAnalyzer) ID() AnalyzerID   { return AnalyzerRuby }
func (RubyAnalyzer) Language() string { return "ruby" }
func (RubyAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_ruby_source", document.Path, err)
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: "ruby", Analyzer: string(AnalyzerRuby), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, document, RubyScannerProfile(), ScannerLimits{MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range scan.Diagnostics {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: "ruby-" + diagnostic.Code, Message: diagnostic.Message, Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	parser := &rubyParser{ctx: ctx, document: document, builder: builder}
	parser.parse(BuildLogicalLines(scan.Tokens, LogicalLineProfile{}))
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_ruby_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

type rubyScope struct {
	kind       string
	parent     *SymbolParent
	owner      string
	visibility Visibility
}

type rubyParser struct {
	ctx          context.Context
	document     *SourceDocument
	builder      *SymbolBuilder
	dependencies []StructuralDependency
	relations    []StructuralRelation
	scopes       []rubyScope
	stopped      bool
}

func (p *rubyParser) parse(lines []LogicalLine) {
	for _, line := range lines {
		if p.stopped || p.ctx.Err() != nil || len(line.Tokens) == 0 {
			continue
		}
		p.parseLine(line)
	}
	if len(p.scopes) > 0 && !p.stopped {
		_ = p.builder.AddDiagnostic(DiagnosticSpec{Code: "ruby-unterminated-scope", Message: "Ruby source contains one or more scopes without matching end", Severity: DiagnosticWarning, AffectsCoverage: true})
	}
}

func (p *rubyParser) parseLine(line LogicalLine) {
	tokens := line.Tokens
	first := 0
	text := strings.ToLower(tokens[first].Text)
	if text == "end" {
		if len(p.scopes) == 0 {
			_ = p.builder.AddDiagnostic(DiagnosticSpec{Code: "ruby-unmatched-end", Message: "Ruby end has no matching open scope", Severity: DiagnosticWarning, Range: &OffsetRange{Start: tokens[first].StartOffset, End: tokens[first].EndOffset}, AffectsCoverage: true})
			return
		}
		p.scopes = p.scopes[:len(p.scopes)-1]
		return
	}
	if text == "require" || text == "require_relative" {
		p.parseRequire(tokens)
		return
	}
	if text == "module" {
		p.parseModule(line, first)
		return
	}
	if text == "class" {
		p.parseClass(line, first)
		return
	}
	if text == "def" {
		p.parseMethod(line, first)
		return
	}
	if text == "private_class_method" {
		for index := first + 1; index < len(tokens); index++ {
			if strings.EqualFold(tokens[index].Text, "def") {
				p.parseMethodWithVisibility(line, index, VisibilityPrivate)
				return
			}
		}
	}
	if text == "include" || text == "extend" {
		p.parseMixin(tokens, first)
		return
	}
	if text == "public" || text == "protected" || text == "private" {
		p.setVisibility(text)
		return
	}
	if p.parseConstant(line) {
		return
	}
	if rubyStartsAnonymousBlock(tokens) {
		p.scopes = append(p.scopes, rubyScope{kind: "block", parent: p.currentParent(), owner: p.currentOwner(), visibility: p.currentVisibility()})
	}
}

func (p *rubyParser) parseRequire(tokens []Token) {
	for _, token := range tokens[1:] {
		if token.Kind != TokenString {
			continue
		}
		value := rubyStringLiteral(token.Text)
		if value == "" {
			return
		}
		rangeValue, err := p.document.RangeFromUTF8Offsets(token.StartOffset, token.EndOffset)
		if err == nil {
			p.dependencies = append(p.dependencies, StructuralDependency{Kind: StructuralDependencyImport, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural})
		}
		return
	}
}

func rubyStringLiteral(value string) string {
	if len(value) < 2 || (value[0] != '\'' && value[0] != '"') || value[len(value)-1] != value[0] {
		return ""
	}
	return value[1 : len(value)-1]
}

func (p *rubyParser) parseModule(line LogicalLine, keyword int) {
	nameStart := rubyNextNameToken(line.Tokens, keyword+1)
	if nameStart < 0 {
		p.markMalformed(line, "ruby-module-name", "Ruby module declaration has no name")
		return
	}
	nameEnd := rubyQualifiedNameEnd(line.Tokens, nameStart)
	name := tokenRangeText(line.Tokens, nameStart, nameEnd)
	symbol, ok := p.add(SymbolSpec{Kind: SymbolKindModule, NativeKind: "module", Name: name, Parent: p.currentParent(),
		Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset},
		NameRange:   OffsetRange{Start: line.Tokens[nameStart].StartOffset, End: line.Tokens[nameEnd-1].EndOffset},
		Signature:   &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Visibility: p.currentVisibility(), Evidence: SymbolEvidenceStructural})
	if ok {
		parent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		p.scopes = append(p.scopes, rubyScope{kind: "module", parent: parent, owner: symbol.Name, visibility: VisibilityPublic})
	}
}

func (p *rubyParser) parseClass(line LogicalLine, keyword int) {
	tokens := line.Tokens
	cursor := keyword + 1
	if (cursor+1 < len(tokens) && tokens[cursor].Text == "<<" && strings.EqualFold(tokens[cursor+1].Text, "self")) ||
		(cursor+2 < len(tokens) && tokens[cursor].Text == "<" && tokens[cursor+1].Text == "<" && strings.EqualFold(tokens[cursor+2].Text, "self")) {
		p.scopes = append(p.scopes, rubyScope{kind: "singleton-class", parent: p.currentParent(), owner: p.currentOwner(), visibility: p.currentVisibility()})
		return
	}
	nameStart := rubyNextNameToken(tokens, cursor)
	if nameStart < 0 {
		p.markMalformed(line, "ruby-class-name", "Ruby class declaration has no name")
		return
	}
	nameEnd := rubyQualifiedNameEnd(tokens, nameStart)
	name := tokenRangeText(tokens, nameStart, nameEnd)
	symbol, ok := p.add(SymbolSpec{Kind: SymbolKindClass, NativeKind: "class", Name: name, Parent: p.currentParent(),
		Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset},
		NameRange:   OffsetRange{Start: tokens[nameStart].StartOffset, End: tokens[nameEnd-1].EndOffset},
		Signature:   &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Visibility: p.currentVisibility(), Evidence: SymbolEvidenceStructural})
	if !ok {
		return
	}
	for index := nameEnd; index < len(tokens); index++ {
		if tokens[index].Text != "<" {
			continue
		}
		targetStart := index + 1
		targetEnd := rubyQualifiedNameEnd(tokens, targetStart)
		if targetStart < len(tokens) && targetEnd > targetStart {
			p.addRelation("inherits", symbol.QualifiedName, tokenRangeText(tokens, targetStart, targetEnd), tokens[targetStart].StartOffset, tokens[targetEnd-1].EndOffset)
		}
		break
	}
	parent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
	p.scopes = append(p.scopes, rubyScope{kind: "class", parent: parent, owner: symbol.Name, visibility: VisibilityPublic})
}

func (p *rubyParser) parseMethod(line LogicalLine, keyword int) {
	p.parseMethodWithVisibility(line, keyword, p.currentVisibility())
}

func (p *rubyParser) parseMethodWithVisibility(line LogicalLine, keyword int, visibility Visibility) {
	tokens := line.Tokens
	cursor := keyword + 1
	if cursor >= len(tokens) {
		p.markMalformed(line, "ruby-method-name", "Ruby method declaration has no name")
		return
	}
	classMethod := false
	if strings.EqualFold(tokens[cursor].Text, "self") && cursor+1 < len(tokens) && tokens[cursor+1].Text == "." {
		classMethod = true
		cursor += 2
	}
	if cursor >= len(tokens) || (tokens[cursor].Kind != TokenIdentifier && tokens[cursor].Kind != TokenKeyword) {
		p.markMalformed(line, "ruby-method-name", "Ruby method declaration has no supported name")
		return
	}
	nameIndex := cursor
	name := tokens[nameIndex].Text
	kind := SymbolKindFunction
	nativeKind := "function"
	parent := p.currentParent()
	owner := p.currentOwner()
	if parent != nil {
		kind = SymbolKindMethod
		nativeKind = "method"
	}
	if classMethod || p.inScope("singleton-class") {
		nativeKind = "singleton-method"
	}
	if strings.EqualFold(name, "initialize") && parent != nil && owner != "" {
		kind = SymbolKindConstructor
		nativeKind = "initialize"
		name = owner
	}
	_, ok := p.add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: name, Parent: parent,
		Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset},
		NameRange:   OffsetRange{Start: tokens[nameIndex].StartOffset, End: tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Visibility: visibility, Evidence: SymbolEvidenceStructural,
		Disambiguator: tokenRangeText(tokens, nameIndex, len(tokens)) + ":" + nativeKind})
	if ok {
		p.scopes = append(p.scopes, rubyScope{kind: "method", parent: parent, owner: owner, visibility: visibility})
	}
}

func (p *rubyParser) parseMixin(tokens []Token, keyword int) {
	parent := p.currentParent()
	if parent == nil {
		return
	}
	start := rubyNextNameToken(tokens, keyword+1)
	if start < 0 {
		return
	}
	end := rubyQualifiedNameEnd(tokens, start)
	target := tokenRangeText(tokens, start, end)
	kind := "includes"
	if strings.EqualFold(tokens[keyword].Text, "extend") {
		kind = "extends-mixin"
	}
	p.addRelation(kind, parent.QualifiedName, target, tokens[start].StartOffset, tokens[end-1].EndOffset)
}

func (p *rubyParser) parseConstant(line LogicalLine) bool {
	tokens := line.Tokens
	if len(tokens) < 3 || tokens[0].Kind != TokenIdentifier || tokens[1].Text != "=" || !rubyConstantName(tokens[0].Text) {
		return false
	}
	p.add(SymbolSpec{Kind: SymbolKindConstant, NativeKind: "constant", Name: tokens[0].Text, Parent: p.currentParent(),
		Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset},
		NameRange:   OffsetRange{Start: tokens[0].StartOffset, End: tokens[0].EndOffset}, Visibility: p.currentVisibility(), Evidence: SymbolEvidenceStructural})
	return true
}

func rubyConstantName(value string) bool {
	if value == "" {
		return false
	}
	first, _ := utf8DecodeRune(value)
	return unicode.IsUpper(first)
}

func utf8DecodeRune(value string) (rune, int) {
	for _, r := range value {
		return r, len(string(r))
	}
	return 0, 0
}

func rubyNextNameToken(tokens []Token, start int) int {
	for index := start; index < len(tokens); index++ {
		if tokens[index].Kind == TokenIdentifier || tokens[index].Kind == TokenKeyword {
			return index
		}
	}
	return -1
}

func rubyQualifiedNameEnd(tokens []Token, start int) int {
	if start < 0 || start >= len(tokens) {
		return start
	}
	end := start + 1
	for end+1 < len(tokens) && ((tokens[end].Text == ":" && tokens[end+1].Text == ":") || tokens[end].Text == ".") {
		if tokens[end].Text == ":" {
			if end+2 >= len(tokens) || (tokens[end+2].Kind != TokenIdentifier && tokens[end+2].Kind != TokenKeyword) {
				break
			}
			end += 3
		} else {
			if tokens[end+1].Kind != TokenIdentifier && tokens[end+1].Kind != TokenKeyword {
				break
			}
			end += 2
		}
	}
	return min(end, len(tokens))
}

func rubyStartsAnonymousBlock(tokens []Token) bool {
	if len(tokens) == 0 {
		return false
	}
	switch strings.ToLower(tokens[0].Text) {
	case "if", "unless", "case", "begin", "while", "until", "for":
		return true
	}
	inlineDepth := 0
	for _, token := range tokens {
		switch strings.ToLower(token.Text) {
		case "begin", "case", "do":
			inlineDepth++
		case "end":
			if inlineDepth > 0 {
				inlineDepth--
			}
		}
	}
	return inlineDepth > 0
}

func (p *rubyParser) currentParent() *SymbolParent {
	for index := len(p.scopes) - 1; index >= 0; index-- {
		if p.scopes[index].parent != nil {
			value := *p.scopes[index].parent
			return &value
		}
	}
	return nil
}

func (p *rubyParser) currentOwner() string {
	for index := len(p.scopes) - 1; index >= 0; index-- {
		if p.scopes[index].owner != "" {
			return p.scopes[index].owner
		}
	}
	return ""
}

func (p *rubyParser) currentVisibility() Visibility {
	for index := len(p.scopes) - 1; index >= 0; index-- {
		if p.scopes[index].visibility != "" {
			return p.scopes[index].visibility
		}
	}
	return VisibilityPublic
}

func (p *rubyParser) setVisibility(value string) {
	visibility := VisibilityPublic
	switch value {
	case "private":
		visibility = VisibilityPrivate
	case "protected":
		visibility = VisibilityProtected
	}
	for index := len(p.scopes) - 1; index >= 0; index-- {
		if p.scopes[index].kind == "class" || p.scopes[index].kind == "module" || p.scopes[index].kind == "singleton-class" {
			p.scopes[index].visibility = visibility
			return
		}
	}
}

func (p *rubyParser) inScope(kind string) bool {
	for index := len(p.scopes) - 1; index >= 0; index-- {
		if p.scopes[index].kind == kind {
			return true
		}
		if p.scopes[index].kind == "class" || p.scopes[index].kind == "module" {
			return false
		}
	}
	return false
}

func (p *rubyParser) addRelation(kind, source, target string, start, end int) {
	if source == "" || strings.TrimSpace(target) == "" {
		return
	}
	rangeValue, err := p.document.RangeFromUTF8Offsets(start, end)
	if err == nil {
		p.relations = append(p.relations, StructuralRelation{Kind: kind, Source: source, Target: strings.TrimSpace(target), Range: rangeValue, Evidence: SymbolEvidenceStructural})
	}
}

func (p *rubyParser) markMalformed(line LogicalLine, code, message string) {
	value := OffsetRange{Start: line.StartOffset, End: line.EndOffset}
	_ = p.builder.AddDiagnostic(DiagnosticSpec{Code: code, Message: message, Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
}

func (p *rubyParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
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
