package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

type ZigAnalyzer struct{}
type NimAnalyzer struct{}
type ALAnalyzer struct{}

func (ZigAnalyzer) ID() AnalyzerID   { return AnalyzerZig }
func (ZigAnalyzer) Language() string { return "zig" }
func (NimAnalyzer) ID() AnalyzerID   { return AnalyzerNim }
func (NimAnalyzer) Language() string { return "nim" }
func (ALAnalyzer) ID() AnalyzerID    { return AnalyzerAL }
func (ALAnalyzer) Language() string  { return "al" }

func (ZigAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: "zig", Analyzer: string(AnalyzerZig), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, document, ZigScannerProfile(), ScannerLimits{MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range scan.Diagnostics {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: "zig-" + diagnostic.Code, Message: diagnostic.Message, Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	parser := &zigParser{ctx: ctx, document: document, tokens: scan.Tokens, pairs: PairDelimiterTokens(scan.Tokens, nil), builder: builder}
	parser.collectImports()
	parser.parseScope(0, len(scan.Tokens), nil, false)
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies}, nil
}

type zigParser struct {
	ctx          context.Context
	document     *SourceDocument
	tokens       []Token
	pairs        map[int]int
	builder      *SymbolBuilder
	dependencies []StructuralDependency
	stopped      bool
}

func (p *zigParser) collectImports() {
	for i := 0; i+3 < len(p.tokens); i++ {
		if p.tokens[i].Text != "@" || !strings.EqualFold(p.tokens[i+1].Text, "import") || p.tokens[i+2].Text != "(" {
			continue
		}
		close := p.pairs[i+2]
		if close <= i+2 {
			continue
		}
		for j := i + 3; j < close; j++ {
			if p.tokens[j].Kind != TokenString {
				continue
			}
			value := phase7StringValue(p.tokens[j].Text)
			if value != "" {
				r, err := p.document.RangeFromUTF8Offsets(p.tokens[j].StartOffset, p.tokens[j].EndOffset)
				if err == nil {
					p.dependencies = appendUniqueDependencies(p.dependencies, []StructuralDependency{{Kind: StructuralDependencyImport, Value: value, Range: r, Evidence: SymbolEvidenceStructural}})
				}
			}
			break
		}
	}
}

func (p *zigParser) parseScope(start, end int, parent *SymbolParent, members bool) {
	for i := start; i < end && !p.stopped; {
		if p.ctx.Err() != nil {
			return
		}
		i = nextStructuralToken(p.tokens, i, end)
		if i >= end || p.tokens[i].Kind == TokenEOF {
			return
		}
		semantic := i
		if strings.EqualFold(p.tokens[semantic].Text, "pub") || strings.EqualFold(p.tokens[semantic].Text, "extern") {
			semantic = nextStructuralToken(p.tokens, semantic+1, end)
		}
		if semantic < end && strings.EqualFold(p.tokens[semantic].Text, "fn") {
			i = p.parseFunction(i, semantic, end, parent, members)
			continue
		}
		if semantic < end && (strings.EqualFold(p.tokens[semantic].Text, "const") || strings.EqualFold(p.tokens[semantic].Text, "var")) {
			if next, ok := p.parseBinding(i, semantic, end, parent, members); ok {
				i = next
				continue
			}
		}
		if members && p.tokens[i].Kind == TokenIdentifier && i+1 < end && p.tokens[i+1].Text == ":" {
			i = p.parseField(i, end, parent)
			continue
		}
		i++
	}
}

func (p *zigParser) parseBinding(start, keyword, end int, parent *SymbolParent, members bool) (int, bool) {
	nameIndex := nextIdentifierToken(p.tokens, keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1, false
	}
	depth := p.tokens[keyword].Nesting
	semicolon := -1
	equal := -1
	for i := nameIndex + 1; i < end; i++ {
		if p.tokens[i].Text == "=" && p.tokens[i].Nesting == depth {
			equal = i
		}
		if p.tokens[i].Text == ";" && p.tokens[i].Nesting == depth {
			semicolon = i
			break
		}
	}
	if semicolon < 0 {
		return keyword + 1, false
	}
	if equal >= 0 {
		kindIndex := nextStructuralToken(p.tokens, equal+1, semicolon)
		if kindIndex < semicolon {
			native := strings.ToLower(p.tokens[kindIndex].Text)
			kind := SymbolKind("")
			switch native {
			case "struct":
				kind = SymbolKindStruct
			case "enum":
				kind = SymbolKindEnum
			case "union", "opaque":
				kind = SymbolKindType
			}
			if kind != "" {
				open := -1
				for j := kindIndex + 1; j < semicolon; j++ {
					if p.tokens[j].Text == "{" {
						open = j
						break
					}
				}
				if open >= 0 {
					close := p.pairs[open]
					if close > open {
						symbol, ok := p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: p.tokens[nameIndex].Text, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[semicolon].EndOffset}, NameRange: OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[open].StartOffset}, Body: &OffsetRange{Start: p.tokens[open].StartOffset, End: p.tokens[close].EndOffset}, Evidence: SymbolEvidenceStructural})
						if ok && kind != SymbolKindEnum {
							child := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
							p.parseScope(open+1, close, child, true)
						}
						return semicolon + 1, true
					}
				}
			}
		}
	}
	kind := SymbolKindConstant
	native := "const"
	if strings.EqualFold(p.tokens[keyword].Text, "var") {
		kind = SymbolKindVariable
		native = "var"
	}
	if members {
		kind = SymbolKindField
		native = "field"
	}
	p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: p.tokens[nameIndex].Text, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[semicolon].EndOffset}, NameRange: OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset}, Evidence: SymbolEvidenceStructural})
	return semicolon + 1, true
}

func (p *zigParser) parseFunction(start, keyword, end int, parent *SymbolParent, members bool) int {
	nameIndex := nextIdentifierToken(p.tokens, keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	paren := -1
	for i := nameIndex + 1; i < end; i++ {
		if p.tokens[i].Text == "(" {
			paren = i
			break
		}
		if p.tokens[i].Kind == TokenNewline {
			break
		}
	}
	if paren < 0 {
		return keyword + 1
	}
	closeParen := p.pairs[paren]
	if closeParen <= paren {
		p.builder.MarkIncomplete()
		return keyword + 1
	}
	open := -1
	semicolon := -1
	for i := closeParen + 1; i < end; i++ {
		if p.tokens[i].Text == "{" {
			open = i
			break
		}
		if p.tokens[i].Text == ";" {
			semicolon = i
			break
		}
	}
	terminator := semicolon
	declarationEnd := p.tokens[closeParen].EndOffset
	var body *OffsetRange
	next := closeParen + 1
	if open >= 0 {
		close := p.pairs[open]
		if close <= open {
			p.builder.MarkIncomplete()
			return end
		}
		terminator = open
		declarationEnd = p.tokens[close].EndOffset
		value := OffsetRange{Start: p.tokens[open].StartOffset, End: p.tokens[close].EndOffset}
		body = &value
		next = close + 1
	} else if semicolon >= 0 {
		declarationEnd = p.tokens[semicolon].EndOffset
		next = semicolon + 1
	}
	kind := SymbolKindFunction
	native := "fn"
	if members {
		kind = SymbolKindMethod
		native = "method"
	}
	p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: p.tokens[nameIndex].Text, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: declarationEnd}, NameRange: OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[terminator].StartOffset}, Body: body, Evidence: SymbolEvidenceStructural, Disambiguator: tokenRangeText(p.tokens, paren, closeParen+1)})
	return next
}

func (p *zigParser) parseField(start, end int, parent *SymbolParent) int {
	depth := p.tokens[start].Nesting
	terminator := -1
	for i := start + 1; i < end; i++ {
		if (p.tokens[i].Text == "," || p.tokens[i].Text == ";") && p.tokens[i].Nesting == depth {
			terminator = i
			break
		}
	}
	if terminator < 0 {
		return start + 1
	}
	p.add(SymbolSpec{Kind: SymbolKindField, NativeKind: "field", Name: p.tokens[start].Text, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[terminator].EndOffset}, NameRange: OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[start].EndOffset}, Evidence: SymbolEvidenceStructural})
	return terminator + 1
}
func (p *zigParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
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

func (NimAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: "nim", Analyzer: string(AnalyzerNim), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, document, NimScannerProfile(), ScannerLimits{MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range scan.Diagnostics {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: "nim-" + diagnostic.Code, Message: diagnostic.Message, Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	parser := &nimParser{ctx: ctx, document: document, builder: builder}
	parser.parse(BuildLogicalLines(scan.Tokens, LogicalLineProfile{TrackIndentation: true}))
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

type nimParser struct {
	ctx               context.Context
	document          *SourceDocument
	builder           *SymbolBuilder
	dependencies      []StructuralDependency
	relations         []StructuralRelation
	typeSectionIndent int
	inTypeSection     bool
	currentType       *SymbolParent
	currentTypeIndent int
	stopped           bool
}

func (p *nimParser) parse(lines []LogicalLine) {
	for _, line := range lines {
		if p.stopped || p.ctx.Err() != nil || len(line.Tokens) == 0 {
			continue
		}
		if p.currentType != nil && line.Indent <= p.currentTypeIndent {
			p.currentType = nil
		}
		first := strings.ToLower(strings.TrimSuffix(line.Tokens[0].Text, "*"))
		if first == "type" && len(line.Tokens) == 1 {
			p.inTypeSection = true
			p.typeSectionIndent = line.Indent
			p.currentType = nil
			continue
		}
		if p.inTypeSection && line.Indent <= p.typeSectionIndent && first != "type" {
			p.inTypeSection = false
			p.currentType = nil
		}
		if first == "import" || first == "include" {
			p.importLine(line)
			continue
		}
		if p.inTypeSection && line.Indent > p.typeSectionIndent {
			if p.typeDecl(line) {
				continue
			}
		}
		if p.currentType != nil && line.Indent > p.currentTypeIndent {
			if p.field(line) {
				continue
			}
		}
		switch first {
		case "proc", "func", "method", "iterator", "template":
			p.callable(line, first)
		case "const", "let", "var":
			p.binding(line, first)
		}
	}
}
func nimNameToken(token Token) (string, OffsetRange) {
	name := strings.TrimSuffix(token.Text, "*")
	end := token.EndOffset
	if strings.HasSuffix(token.Text, "*") {
		end--
	}
	return name, OffsetRange{Start: token.StartOffset, End: end}
}
func (p *nimParser) importLine(line LogicalLine) {
	for _, part := range splitTokenRangeAt(line.Tokens, 1, len(line.Tokens), ",", line.Tokens[0].Nesting) {
		if part[0] >= part[1] {
			continue
		}
		value := tokenRangeText(line.Tokens, part[0], part[1])
		if value == "" {
			continue
		}
		r, err := p.document.RangeFromUTF8Offsets(line.Tokens[part[0]].StartOffset, line.Tokens[part[1]-1].EndOffset)
		if err == nil {
			p.dependencies = appendUniqueDependencies(p.dependencies, []StructuralDependency{{Kind: StructuralDependencyImport, Value: value, Range: r, Evidence: SymbolEvidenceStructural}})
		}
	}
}
func (p *nimParser) typeDecl(line LogicalLine) bool {
	if len(line.Tokens) < 3 || line.Tokens[0].Kind != TokenIdentifier {
		return false
	}
	equal := -1
	for i := 1; i < len(line.Tokens); i++ {
		if line.Tokens[i].Text == "=" {
			equal = i
			break
		}
	}
	if equal < 0 {
		return false
	}
	name, nr := nimNameToken(line.Tokens[0])
	if name == "" {
		return false
	}
	kind := SymbolKindType
	native := "type"
	relationTarget := ""
	for i := equal + 1; i < len(line.Tokens); i++ {
		text := strings.ToLower(line.Tokens[i].Text)
		switch text {
		case "enum":
			kind = SymbolKindEnum
			native = "enum"
		case "object":
			if kind != SymbolKindEnum {
				kind = SymbolKindClass
				native = "object"
			}
		case "ref":
			native = "ref-object"
		case "of":
			if i+1 < len(line.Tokens) {
				relationTarget = tokenRangeText(line.Tokens, i+1, len(line.Tokens))
			}
		}
	}
	symbol, ok := p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: name, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: nr, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
	if ok {
		parent := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		p.currentType = &parent
		p.currentTypeIndent = line.Indent
		if relationTarget != "" {
			r, err := p.document.RangeFromUTF8Offsets(line.Tokens[equal+1].StartOffset, line.Tokens[len(line.Tokens)-1].EndOffset)
			if err == nil {
				p.relations = append(p.relations, StructuralRelation{Kind: "inherits", Source: symbol.QualifiedName, Target: relationTarget, Range: r, Evidence: SymbolEvidenceStructural})
			}
		}
	}
	return true
}
func (p *nimParser) field(line LogicalLine) bool {
	if len(line.Tokens) < 2 || line.Tokens[0].Kind != TokenIdentifier || line.Tokens[1].Text != ":" {
		return false
	}
	name, nr := nimNameToken(line.Tokens[0])
	if name == "" {
		return false
	}
	p.add(SymbolSpec{Kind: SymbolKindField, NativeKind: "field", Name: name, Parent: p.currentType, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: nr, Evidence: SymbolEvidenceStructural})
	return true
}
func (p *nimParser) callable(line LogicalLine, native string) {
	idx := nextIdentifierToken(line.Tokens, 1, len(line.Tokens))
	if idx < 0 {
		return
	}
	name, nr := nimNameToken(line.Tokens[idx])
	if name == "" {
		return
	}
	p.add(SymbolSpec{Kind: SymbolKindFunction, NativeKind: native, Name: name, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: nr, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
}
func (p *nimParser) binding(line LogicalLine, native string) {
	idx := nextIdentifierToken(line.Tokens, 1, len(line.Tokens))
	if idx < 0 {
		return
	}
	name, nr := nimNameToken(line.Tokens[idx])
	if name == "" {
		return
	}
	kind := SymbolKindVariable
	if native == "const" {
		kind = SymbolKindConstant
	}
	p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: name, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: nr, Evidence: SymbolEvidenceStructural})
}
func (p *nimParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
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

func (ALAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: "al", Analyzer: string(AnalyzerAL), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, document, ALScannerProfile(), ScannerLimits{
		MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting,
	})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range scan.Diagnostics {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{
			Code: "al-" + diagnostic.Code, Message: diagnostic.Message, Severity: DiagnosticWarning,
			Range: &value, AffectsCoverage: true,
		})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	parser := &alParser{
		ctx: ctx, document: document, tokens: scan.Tokens,
		pairs: PairDelimiterTokens(scan.Tokens, nil), builder: builder,
	}
	parent := parser.namespaceAndUsing()
	parser.parseScope(0, len(scan.Tokens), parent, false)
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

type alParser struct {
	ctx          context.Context
	document     *SourceDocument
	tokens       []Token
	pairs        map[int]int
	builder      *SymbolBuilder
	dependencies []StructuralDependency
	relations    []StructuralRelation
	namespace    *SymbolParent
	stopped      bool
}

func (p *alParser) namespaceAndUsing() *SymbolParent {
	for i := 0; i < len(p.tokens); i++ {
		if p.tokens[i].Nesting != 0 {
			continue
		}
		text := strings.ToLower(p.tokens[i].Text)
		if text == "namespace" && p.namespace == nil {
			end := p.sameDepth(i+1, ";", 0)
			if end > i+1 {
				start := nextIdentifierToken(p.tokens, i+1, end)
				if start >= 0 {
					name := tokenRangeText(p.tokens, start, end)
					symbol, ok := p.add(SymbolSpec{
						Kind: SymbolKindNamespace, NativeKind: "namespace", Name: name, QualifiedName: name,
						Declaration: OffsetRange{Start: p.tokens[i].StartOffset, End: p.tokens[end].EndOffset},
						NameRange:   OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[end-1].EndOffset},
						Signature:   &OffsetRange{Start: p.tokens[i].StartOffset, End: p.tokens[end].EndOffset},
						Evidence:    SymbolEvidenceStructural,
					})
					if ok {
						value := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
						p.namespace = &value
					}
				}
			}
		}
		if text == "using" {
			end := p.sameDepth(i+1, ";", 0)
			if end <= i+1 {
				continue
			}
			start := nextIdentifierToken(p.tokens, i+1, end)
			if start < 0 {
				continue
			}
			value := tokenRangeText(p.tokens, start, end)
			rangeValue, err := p.document.RangeFromUTF8Offsets(p.tokens[start].StartOffset, p.tokens[end-1].EndOffset)
			if err == nil {
				p.dependencies = appendUniqueDependencies(p.dependencies, []StructuralDependency{{
					Kind: StructuralDependencyImport, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural,
				}})
			}
		}
	}
	return p.namespace
}

func (p *alParser) parseScope(start, end int, parent *SymbolParent, members bool) {
	objects := map[string]SymbolKind{
		"codeunit": SymbolKindModule, "table": SymbolKindType, "tableextension": SymbolKindType,
		"page": SymbolKindType, "pageextension": SymbolKindType, "report": SymbolKindType,
		"reportextension": SymbolKindType, "query": SymbolKindType, "xmlport": SymbolKindType,
		"enum": SymbolKindEnum, "enumextension": SymbolKindEnum, "interface": SymbolKindInterface,
		"permissionset": SymbolKindType,
	}
	for i := start; i < end && !p.stopped; {
		if p.ctx.Err() != nil {
			return
		}
		i = nextStructuralToken(p.tokens, i, end)
		if i >= end || p.tokens[i].Kind == TokenEOF {
			return
		}
		semantic := i
		if strings.EqualFold(p.tokens[semantic].Text, "local") || strings.EqualFold(p.tokens[semantic].Text, "protected") {
			semantic = nextStructuralToken(p.tokens, semantic+1, end)
		}
		if semantic >= end {
			return
		}
		lower := strings.ToLower(p.tokens[semantic].Text)
		if kind, ok := objects[lower]; ok {
			i = p.object(i, semantic, end, parent, kind)
			continue
		}
		if lower == "procedure" || lower == "trigger" {
			i = p.procedure(i, semantic, end, parent, members, lower)
			continue
		}
		if lower == "field" && members {
			if next, ok := p.field(i, end, parent); ok {
				i = next
				continue
			}
		}
		i++
	}
}

func (p *alParser) object(start, keyword, end int, parent *SymbolParent, kind SymbolKind) int {
	nameIndex := nextIdentifierToken(p.tokens, keyword+1, end)
	if nameIndex >= 0 && p.tokens[nameIndex].Kind == TokenNumber {
		nameIndex = nextIdentifierToken(p.tokens, nameIndex+1, end)
	}
	if nameIndex < 0 {
		return keyword + 1
	}
	depth := p.tokens[keyword].Nesting
	open := -1
	for i := nameIndex + 1; i < end; i++ {
		if p.tokens[i].Text == "{" && p.tokens[i].Nesting == depth+1 {
			open = i
			break
		}
		if p.tokens[i].Text == ";" && p.tokens[i].Nesting == depth {
			return i + 1
		}
	}
	if open < 0 {
		return keyword + 1
	}
	close := p.pairs[open]
	if close <= open {
		p.builder.MarkIncomplete()
		return end
	}
	native := strings.ToLower(p.tokens[keyword].Text)
	symbol, ok := p.add(SymbolSpec{
		Kind: kind, NativeKind: native, Name: p.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[close].EndOffset},
		NameRange:   OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[open].StartOffset},
		Body:        &OffsetRange{Start: p.tokens[open].StartOffset, End: p.tokens[close].EndOffset},
		Evidence:    SymbolEvidenceStructural,
	})
	if ok {
		child := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		for i := nameIndex + 1; i < open; i++ {
			if !strings.EqualFold(p.tokens[i].Text, "extends") || i+1 >= open {
				continue
			}
			target := alTokenValue(p.tokens[i+1])
			if target == "" {
				continue
			}
			rangeValue, err := p.document.RangeFromUTF8Offsets(p.tokens[i+1].StartOffset, p.tokens[i+1].EndOffset)
			if err == nil {
				p.relations = append(p.relations, StructuralRelation{
					Kind: "extends", Source: symbol.QualifiedName, Target: target,
					Range: rangeValue, Evidence: SymbolEvidenceStructural,
				})
			}
		}
		if kind != SymbolKindEnum {
			p.parseScope(open+1, close, child, true)
		}
	}
	return close + 1
}

func (p *alParser) procedure(start, keyword, end int, parent *SymbolParent, members bool, native string) int {
	nameIndex := nextIdentifierToken(p.tokens, keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	paren := -1
	for i := nameIndex + 1; i < end; i++ {
		if p.tokens[i].Text == "(" {
			paren = i
			break
		}
		if p.tokens[i].Kind == TokenNewline {
			break
		}
	}
	declarationEnd := p.tokens[nameIndex].EndOffset
	next := nameIndex + 1
	if paren >= 0 {
		closeParen := p.pairs[paren]
		if closeParen > paren {
			declarationEnd = p.tokens[closeParen].EndOffset
			next = closeParen + 1
		}
	}
	kind := SymbolKindFunction
	if members {
		kind = SymbolKindMethod
	}
	p.add(SymbolSpec{
		Kind: kind, NativeKind: native, Name: p.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: p.tokens[start].StartOffset, End: declarationEnd}, Evidence: SymbolEvidenceStructural,
	})
	return next
}

func (p *alParser) field(start, end int, parent *SymbolParent) (int, bool) {
	paren := -1
	for i := start + 1; i < end; i++ {
		if p.tokens[i].Text == "(" {
			paren = i
			break
		}
		if p.tokens[i].Kind == TokenNewline {
			return start + 1, false
		}
	}
	if paren < 0 {
		return start + 1, false
	}
	close := p.pairs[paren]
	if close <= paren {
		return start + 1, false
	}
	semicolonCount := 0
	nameIndex := -1
	for i := paren + 1; i < close; i++ {
		if p.tokens[i].Text == ";" {
			semicolonCount++
			continue
		}
		if semicolonCount == 1 && (p.tokens[i].Kind == TokenIdentifier || p.tokens[i].Kind == TokenString) {
			nameIndex = i
			break
		}
	}
	if nameIndex < 0 {
		return close + 1, false
	}
	name := alTokenValue(p.tokens[nameIndex])
	if name == "" {
		return close + 1, false
	}
	nameRange := OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset}
	if p.tokens[nameIndex].Kind == TokenString && len(p.tokens[nameIndex].Text) >= 2 {
		nameRange.Start++
		nameRange.End--
	}
	p.add(SymbolSpec{
		Kind: SymbolKindField, NativeKind: "field", Name: name, Parent: parent,
		Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[close].EndOffset},
		NameRange:   nameRange, Evidence: SymbolEvidenceStructural,
	})
	return close + 1, true
}

func alTokenValue(token Token) string {
	if token.Kind == TokenString {
		return phase7StringValue(token.Text)
	}
	return strings.Trim(token.Text, "\"")
}

func (p *alParser) sameDepth(start int, text string, depth int) int {
	for i := start; i < len(p.tokens); i++ {
		if p.tokens[i].Text == text && p.tokens[i].Nesting == depth {
			return i
		}
	}
	return -1
}

func (p *alParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
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
