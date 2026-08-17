package sourceintelligence

import (
	"context"
	"regexp"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

type basicDialectPolicy struct {
	language       string
	analyzer       AnalyzerID
	profile        ScannerProfile
	metadataModule bool
	lineNumbers    bool
	pureBasic      bool
	freeBasic      bool
	vbScript       bool
}

type VB6Analyzer struct{}
type VBAAnalyzer struct{}
type VBScriptAnalyzer struct{}
type QBasicAnalyzer struct{}
type ClassicBasicAnalyzer struct{}
type FreeBasicAnalyzer struct{}
type PureBasicAnalyzer struct{}

func (VB6Analyzer) ID() AnalyzerID            { return AnalyzerVB6 }
func (VB6Analyzer) Language() string          { return "vb6" }
func (VBAAnalyzer) ID() AnalyzerID            { return AnalyzerVBA }
func (VBAAnalyzer) Language() string          { return "vba" }
func (VBScriptAnalyzer) ID() AnalyzerID       { return AnalyzerVBScript }
func (VBScriptAnalyzer) Language() string     { return "vbscript" }
func (QBasicAnalyzer) ID() AnalyzerID         { return AnalyzerQBasic }
func (QBasicAnalyzer) Language() string       { return "qbasic" }
func (ClassicBasicAnalyzer) ID() AnalyzerID   { return AnalyzerClassicBasic }
func (ClassicBasicAnalyzer) Language() string { return "classic-basic" }
func (FreeBasicAnalyzer) ID() AnalyzerID      { return AnalyzerFreeBasic }
func (FreeBasicAnalyzer) Language() string    { return "freebasic" }
func (PureBasicAnalyzer) ID() AnalyzerID      { return AnalyzerPureBasic }
func (PureBasicAnalyzer) Language() string    { return "purebasic" }

func (VB6Analyzer) Analyze(ctx context.Context, d *SourceDocument, o AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeBasicDialect(ctx, d, o, basicDialectPolicy{language: "vb6", analyzer: AnalyzerVB6, profile: VB6ScannerProfile(), metadataModule: true})
}
func (VBAAnalyzer) Analyze(ctx context.Context, d *SourceDocument, o AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeBasicDialect(ctx, d, o, basicDialectPolicy{language: "vba", analyzer: AnalyzerVBA, profile: VBAScannerProfile(), metadataModule: true})
}
func (VBScriptAnalyzer) Analyze(ctx context.Context, d *SourceDocument, o AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeBasicDialect(ctx, d, o, basicDialectPolicy{language: "vbscript", analyzer: AnalyzerVBScript, profile: VBScriptScannerProfile(), vbScript: true})
}
func (QBasicAnalyzer) Analyze(ctx context.Context, d *SourceDocument, o AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeBasicDialect(ctx, d, o, basicDialectPolicy{language: "qbasic", analyzer: AnalyzerQBasic, profile: QBasicScannerProfile(), lineNumbers: true})
}
func (ClassicBasicAnalyzer) Analyze(ctx context.Context, d *SourceDocument, o AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeBasicDialect(ctx, d, o, basicDialectPolicy{language: "classic-basic", analyzer: AnalyzerClassicBasic, profile: ClassicBasicScannerProfile(), lineNumbers: true})
}
func (FreeBasicAnalyzer) Analyze(ctx context.Context, d *SourceDocument, o AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeBasicDialect(ctx, d, o, basicDialectPolicy{language: "freebasic", analyzer: AnalyzerFreeBasic, profile: FreeBasicScannerProfile(), freeBasic: true})
}
func (PureBasicAnalyzer) Analyze(ctx context.Context, d *SourceDocument, o AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeBasicDialect(ctx, d, o, basicDialectPolicy{language: "purebasic", analyzer: AnalyzerPureBasic, profile: PureBasicScannerProfile(), pureBasic: true})
}

type basicLine struct {
	tokens     []Token
	start, end int
}
type basicParser struct {
	ctx          context.Context
	document     *SourceDocument
	builder      *SymbolBuilder
	policy       basicDialectPolicy
	lines        []basicLine
	pairs        map[int]int
	module       *SymbolParent
	dependencies []StructuralDependency
	stopped      bool
}

func analyzeBasicDialect(ctx context.Context, document *SourceDocument, options AnalyzeOptions, policy basicDialectPolicy) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_basic_source", document.Path, err)
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: policy.language, Analyzer: string(policy.analyzer), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, document, policy.profile, ScannerLimits{MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range scan.Diagnostics {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: policy.language + "-" + diagnostic.Code, Message: diagnostic.Message, Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	logical := BuildLogicalLines(scan.Tokens, LogicalLineProfile{Separators: []string{":"}})
	lines := make([]basicLine, 0, len(logical))
	for _, line := range logical {
		tokens := append([]Token(nil), line.Tokens...)
		if policy.lineNumbers && len(tokens) > 0 && tokens[0].Kind == TokenNumber {
			tokens = tokens[1:]
		}
		if len(tokens) > 0 {
			lines = append(lines, basicLine{tokens: tokens, start: line.StartOffset, end: line.EndOffset})
		}
	}
	parser := &basicParser{ctx: ctx, document: document, builder: builder, policy: policy, lines: lines}
	parser.pairs = parser.buildPairs()
	parser.dependencies = collectBasicDependencies(document, policy)
	parser.parseRange(0, len(lines), nil, "")
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_basic_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies}, nil
}

func (p *basicParser) buildPairs() map[int]int {
	pairs := map[int]int{}
	type open struct {
		index int
		label string
	}
	var stack []open
	for i, line := range p.lines {
		label, openScope := p.scopeOpen(line.tokens)
		if openScope {
			stack = append(stack, open{i, label})
			continue
		}
		close := p.scopeClose(line.tokens)
		if close == "" || len(stack) == 0 {
			continue
		}
		top := stack[len(stack)-1]
		if top.label == close {
			pairs[top.index] = i
			stack = stack[:len(stack)-1]
		}
	}
	return pairs
}

func basicSemanticIndex(tokens []Token) int {
	for index, token := range tokens {
		value := strings.ToLower(token.Text)
		switch value {
		case "public", "private", "protected", "friend", "shared":
			continue
		case "static":
			if index+1 < len(tokens) {
				next := strings.ToLower(tokens[index+1].Text)
				if next == "sub" || next == "function" || next == "property" || next == "procedure" {
					continue
				}
			}
		}
		return index
	}
	return -1
}

func (p *basicParser) freeBasicTypeAlias(tokens []Token, semantic int) bool {
	if !p.policy.freeBasic || semantic < 0 || semantic+2 >= len(tokens) || !strings.EqualFold(tokens[semantic].Text, "type") {
		return false
	}
	for index := semantic + 2; index < len(tokens); index++ {
		if strings.EqualFold(tokens[index].Text, "as") {
			return true
		}
	}
	return false
}

func (p *basicParser) scopeOpen(tokens []Token) (string, bool) {
	if len(tokens) == 0 {
		return "", false
	}
	semantic := basicSemanticIndex(tokens)
	if semantic < 0 {
		return "", false
	}
	first := strings.ToLower(tokens[semantic].Text)
	if p.freeBasicTypeAlias(tokens, semantic) {
		return "", false
	}
	if first == "declare" {
		return "", false
	}
	if semantic+1 >= len(tokens) || tokens[semantic+1].Text == "=" {
		return "", false
	}
	if p.policy.pureBasic {
		switch first {
		case "module":
			return "module", true
		case "structure":
			return "structure", true
		case "interface":
			return "interface", true
		case "enumeration":
			return "enumeration", true
		case "procedure", "procedurec", "proceduredll", "procedurecdll":
			return "procedure", true
		}
	}
	switch first {
	case "class":
		if p.policy.vbScript {
			return "class", true
		}
	case "type":
		return "type", true
	case "enum":
		return "enum", true
	case "namespace":
		if p.policy.freeBasic {
			return "namespace", true
		}
	case "sub":
		return "sub", true
	case "function":
		return "function", true
	case "property":
		return "property", true
	case "constructor":
		if p.policy.freeBasic {
			return "constructor", true
		}
	case "destructor":
		if p.policy.freeBasic {
			return "destructor", true
		}
	case "operator":
		if p.policy.freeBasic {
			return "operator", true
		}
	}
	return "", false
}

func (p *basicParser) scopeClose(tokens []Token) string {
	if len(tokens) == 0 {
		return ""
	}
	first := strings.ToLower(tokens[0].Text)
	if p.policy.pureBasic {
		switch first {
		case "endmodule":
			return "module"
		case "endstructure":
			return "structure"
		case "endinterface":
			return "interface"
		case "endenumeration":
			return "enumeration"
		case "endprocedure":
			return "procedure"
		}
	}
	if first != "end" || len(tokens) < 2 {
		return ""
	}
	switch strings.ToLower(tokens[1].Text) {
	case "class", "type", "enum", "namespace", "sub", "function", "property", "constructor", "destructor", "operator":
		return strings.ToLower(tokens[1].Text)
	}
	return ""
}

func (p *basicParser) parseRange(start, end int, parent *SymbolParent, scopeKind string) {
	currentParent := parent
	if currentParent == nil && p.module != nil {
		v := *p.module
		currentParent = &v
	}
	for i := start; i < end && !p.stopped; {
		if p.ctx.Err() != nil {
			return
		}
		line := p.lines[i]
		tokens := line.tokens
		if len(tokens) == 0 {
			i++
			continue
		}
		first := strings.ToLower(tokens[0].Text)
		if first == "rem" {
			i++
			continue
		}
		if p.policy.metadataModule && first == "attribute" && len(tokens) >= 4 && strings.EqualFold(tokens[1].Text, "VB_Name") && tokens[2].Text == "=" && tokens[3].Kind == TokenString {
			name := basicStringValue(tokens[3].Text)
			if name != "" {
				if symbol, ok := p.add(SymbolSpec{Kind: SymbolKindModule, NativeKind: "vb-module", Name: name, QualifiedName: name, Declaration: OffsetRange{Start: line.start, End: line.end}, NameRange: OffsetRange{Start: tokens[1].StartOffset, End: tokens[1].EndOffset}, Evidence: SymbolEvidenceStructural}); ok {
					v := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
					p.module = &v
					currentParent = &v
				}
			}
			i++
			continue
		}
		semantic := basicSemanticIndex(tokens)
		if semantic < 0 {
			i++
			continue
		}
		first = strings.ToLower(tokens[semantic].Text)
		if first == "declare" {
			p.parseDeclare(line, currentParent)
			i++
			continue
		}

		if first == "def" {
			p.parseDef(line, currentParent)
			i++
			continue
		}
		if p.freeBasicTypeAlias(tokens, semantic) {
			p.addNamed(line, semantic+1, SymbolKindType, "type-alias", currentParent)
			i++
			continue
		}
		label, isOpen := p.scopeOpen(tokens)
		if isOpen {
			close, ok := p.pairs[i]
			if !ok {
				p.builder.MarkIncomplete()
				_ = p.builder.AddDiagnostic(DiagnosticSpec{Code: p.policy.language + "-missing-end", Message: "declaration is missing its matching end", Severity: DiagnosticWarning, Range: &OffsetRange{Start: line.start, End: line.end}, AffectsCoverage: true})
				close = end
			}
			symbol, added := p.addScopeSymbol(line, label, currentParent)
			if !added {
				i++
				continue
			}
			child := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
			if close > i+1 {
				p.parseRange(i+1, close, child, label)
			}
			if ok {
				i = close + 1
			} else {
				return
			}
			continue
		}
		if first == "const" {
			p.addNamed(line, semantic+1, SymbolKindConstant, "const", currentParent)
			i++
			continue
		}
		if first == "dim" || first == "global" || first == "static" {
			kind := SymbolKindVariable
			if basicFieldScope(scopeKind) {
				kind = SymbolKindField
			}
			p.addNamed(line, semantic+1, kind, "variable", currentParent)
			i++
			continue
		}
		if basicFieldScope(scopeKind) && p.addTypeField(line, currentParent) {
			i++
			continue
		}
		i++
	}
}

func basicFieldScope(scopeKind string) bool {
	switch scopeKind {
	case "class", "type", "structure", "interface":
		return true
	default:
		return false
	}
}

func (p *basicParser) addScopeSymbol(line basicLine, label string, parent *SymbolParent) (NormalizedSymbol, bool) {
	tokens := line.tokens
	semantic := basicSemanticIndex(tokens)
	if semantic < 0 {
		return NormalizedSymbol{}, false
	}
	nameIndex := semantic + 1
	kind := SymbolKindFunction
	native := label
	if p.policy.pureBasic {
		switch label {
		case "module":
			kind = SymbolKindModule
		case "structure":
			kind = SymbolKindStruct
		case "interface":
			kind = SymbolKindInterface
		case "enumeration":
			kind = SymbolKindEnum
		case "procedure":
			kind = SymbolKindFunction
		}
	} else {
		switch label {
		case "class":
			kind = SymbolKindClass
		case "type":
			kind = SymbolKindType
		case "enum":
			kind = SymbolKindEnum
		case "namespace":
			kind = SymbolKindNamespace
		case "sub", "function", "property", "constructor", "destructor", "operator":
			if parent != nil && (p.policy.vbScript || p.policy.metadataModule) {
				kind = SymbolKindMethod
			} else {
				kind = SymbolKindFunction
			}
		}
	}
	if label == "property" && parent != nil {
		kind = SymbolKindProperty
	}
	if label == "constructor" {
		kind = SymbolKindConstructor
	}
	if label == "destructor" {
		kind = SymbolKindDestructor
	}
	if label == "operator" {
		kind = SymbolKindOperator
	}
	if label == "procedure" {
		nameIndex = 1
	}
	if label == "property" && nameIndex < len(tokens) {
		switch strings.ToLower(tokens[nameIndex].Text) {
		case "get", "let", "set":
			nameIndex++
		}
	}
	if nameIndex >= len(tokens) {
		return NormalizedSymbol{}, false
	}
	nameToken := tokens[nameIndex]
	if nameToken.Kind != TokenIdentifier && nameToken.Kind != TokenKeyword {
		return NormalizedSymbol{}, false
	}
	name := normalizeBasicName(nameToken.Text, p.policy)
	return p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: name, Parent: parent, Declaration: OffsetRange{Start: line.start, End: line.end}, NameRange: OffsetRange{Start: nameToken.StartOffset, End: nameToken.EndOffset}, Signature: &OffsetRange{Start: line.start, End: line.end}, Evidence: SymbolEvidenceStructural})
}

func (p *basicParser) parseDeclare(line basicLine, parent *SymbolParent) {
	if len(line.tokens) < 3 {
		return
	}
	semantic := basicSemanticIndex(line.tokens)
	if semantic < 0 || semantic+2 >= len(line.tokens) {
		return
	}
	kindIndex := semantic + 1
	k := strings.ToLower(line.tokens[kindIndex].Text)
	if k != "sub" && k != "function" {
		return
	}
	p.addNamed(line, kindIndex+1, SymbolKindFunction, "declare-"+k, parent)
}
func (p *basicParser) parseDef(line basicLine, parent *SymbolParent) {
	if len(line.tokens) < 2 {
		return
	}
	semantic := basicSemanticIndex(line.tokens)
	if semantic < 0 || semantic+1 >= len(line.tokens) {
		return
	}
	p.addNamed(line, semantic+1, SymbolKindFunction, "def-fn", parent)
}
func (p *basicParser) addNamed(line basicLine, index int, kind SymbolKind, native string, parent *SymbolParent) {
	if index >= len(line.tokens) {
		return
	}
	tok := line.tokens[index]
	if tok.Kind != TokenIdentifier && tok.Kind != TokenKeyword {
		return
	}
	name := normalizeBasicName(tok.Text, p.policy)
	p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: name, Parent: parent, Declaration: OffsetRange{Start: line.start, End: line.end}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Evidence: SymbolEvidenceStructural})
}
func (p *basicParser) addTypeField(line basicLine, parent *SymbolParent) bool {
	if len(line.tokens) == 0 {
		return false
	}
	semantic := basicSemanticIndex(line.tokens)
	if semantic < 0 || semantic >= len(line.tokens) {
		return false
	}
	first := line.tokens[semantic]
	if first.Kind != TokenIdentifier {
		return false
	}
	for _, tok := range line.tokens {
		if strings.EqualFold(tok.Text, "as") || p.policy.pureBasic && strings.HasPrefix(tok.Text, ".") {
			p.addNamed(line, semantic, SymbolKindField, "field", parent)
			return true
		}
	}
	if p.policy.pureBasic && semantic+1 < len(line.tokens) && line.tokens[semantic+1].Text == "." {
		p.addNamed(line, semantic, SymbolKindField, "field", parent)
		return true
	}
	return false
}
func (p *basicParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
	s, err := p.builder.Add(spec)
	if operation.KindOf(err) == operation.KindLimit {
		p.stopped = true
		return NormalizedSymbol{}, false
	}
	if err != nil {
		p.builder.MarkIncomplete()
		return NormalizedSymbol{}, false
	}
	return s, true
}

func normalizeBasicName(value string, policy basicDialectPolicy) string {
	if policy.lineNumbers {
		value = strings.TrimRight(value, "$%&!#")
	}
	return value
}
func basicStringValue(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return strings.ReplaceAll(value[1:len(value)-1], "\"\"", "\"")
	}
	return ""
}

var freeBasicIncludeRE = regexp.MustCompile(`(?im)^\s*#include(?:\s+once)?\s+["<]([^">]+)[">]`)
var pureBasicIncludeRE = regexp.MustCompile(`(?im)^\s*x?includefile\s+"([^"]+)"`)
var vbLibRE = regexp.MustCompile(`(?im)^\s*(?:public\s+|private\s+)?declare\s+(?:function|sub)\s+[^\r\n]+?\s+lib\s+"([^"]+)"`)

func collectBasicDependencies(document *SourceDocument, policy basicDialectPolicy) []StructuralDependency {
	var matches [][]int
	if policy.freeBasic {
		matches = freeBasicIncludeRE.FindAllStringSubmatchIndex(document.Text, -1)
	} else if policy.pureBasic {
		matches = pureBasicIncludeRE.FindAllStringSubmatchIndex(document.Text, -1)
	} else if policy.metadataModule {
		matches = vbLibRE.FindAllStringSubmatchIndex(document.Text, -1)
	}
	result := make([]StructuralDependency, 0, len(matches))
	for _, m := range matches {
		if len(m) < 4 || m[2] < 0 {
			continue
		}
		value := document.Text[m[2]:m[3]]
		r, err := document.RangeFromUTF8Offsets(m[2], m[3])
		if err == nil {
			result = append(result, StructuralDependency{Kind: StructuralDependencyInclude, Value: value, Range: r, Evidence: SymbolEvidenceStructural})
		}
	}
	return result
}
