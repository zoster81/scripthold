package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// MQL4Analyzer and MQL5Analyzer deliberately keep distinct provider identities.
// They reuse only the declaration-safe C++ structural subset and add MQL-specific
// dependency/input evidence without pretending to evaluate the preprocessor.
type MQL4Analyzer struct{}
type MQL5Analyzer struct{}

func (MQL4Analyzer) ID() AnalyzerID   { return AnalyzerMQL4 }
func (MQL4Analyzer) Language() string { return "mql4" }
func (MQL5Analyzer) ID() AnalyzerID   { return AnalyzerMQL5 }
func (MQL5Analyzer) Language() string { return "mql5" }

func (MQL4Analyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeMQL(ctx, document, options, "mql4", AnalyzerMQL4)
}
func (MQL5Analyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeMQL(ctx, document, options, "mql5", AnalyzerMQL5)
}

func analyzeMQL(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_mql_source", document.Path, err)
	}
	source, err := (CPPAnalyzer{}).Analyze(ctx, document, options)
	if err != nil {
		return AnalyzerResult{}, err
	}
	for index := range source.Analysis.Symbols {
		symbol := &source.Analysis.Symbols[index]
		if symbol.Kind != SymbolKindVariable && symbol.Kind != SymbolKindConstant {
			continue
		}
		declaration, _, _, _ := symbol.SourceOffsets()
		if declaration.Start < 0 || declaration.End > len(document.Text) || declaration.End <= declaration.Start {
			continue
		}
		prefix := strings.ToLower(strings.TrimSpace(document.Text[declaration.Start:declaration.End]))
		switch {
		case strings.HasPrefix(prefix, "input "):
			symbol.NativeKind = "input-variable"
			symbol.Modifiers = append(symbol.Modifiers, "input")
		case strings.HasPrefix(prefix, "sinput "):
			symbol.NativeKind = "static-input-variable"
			symbol.Modifiers = append(symbol.Modifiers, "sinput")
		case strings.HasPrefix(prefix, "extern "):
			symbol.NativeKind = "extern-variable"
			symbol.Modifiers = append(symbol.Modifiers, "extern")
		}
	}
	analysis, err := reprojectAnalyzerSymbols(ctx, document, source, options, language, analyzer, "", 0, nil)
	if err != nil {
		return AnalyzerResult{}, err
	}
	copyPhase7AdapterDiagnostics(&analysis, source.Analysis, language, options.Limits.MaxDiagnostics)
	dependencies := append([]StructuralDependency(nil), source.Dependencies...)
	imports, conditional, err := collectMQLDirectives(ctx, document, language)
	if err != nil {
		return AnalyzerResult{}, err
	}
	dependencies = appendUniqueDependencies(dependencies, imports)
	if conditional {
		builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: language, Analyzer: string(analyzer), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: language + "-conditional-preprocessor", Message: "conditional preprocessing is not evaluated; declarations may depend on macro state", Severity: DiagnosticWarning, AffectsCoverage: true})
		analysis.CoverageComplete = false
		for _, diagnostic := range builder.Result().Diagnostics {
			appendPhase7Diagnostic(&analysis, diagnostic, options.Limits.MaxDiagnostics)
		}
	}
	return AnalyzerResult{Analysis: analysis, Dependencies: dependencies, Relations: source.Relations}, nil
}

func collectMQLDirectives(ctx context.Context, document *SourceDocument, language string) ([]StructuralDependency, bool, error) {
	scan, err := ScanSource(ctx, document, MQLScannerProfile(language), ScannerLimits{MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: 2048})
	if err != nil {
		return nil, false, err
	}
	var result []StructuralDependency
	conditional := false
	for _, token := range scan.Tokens {
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
		if !strings.HasPrefix(lower, "#import") {
			continue
		}
		rest := strings.TrimSpace(trimmed[len("#import"):])
		value := phase7QuotedOrAngleValue(rest)
		if value == "" {
			continue
		}
		rangeValue, rangeErr := document.RangeFromUTF8Offsets(token.StartOffset, token.EndOffset)
		if rangeErr == nil {
			result = append(result, StructuralDependency{Kind: StructuralDependencyImport, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural})
		}
	}
	return result, conditional, nil
}

func appendUniqueDependencies(base, extra []StructuralDependency) []StructuralDependency {
	seen := make(map[string]struct{}, len(base)+len(extra))
	result := make([]StructuralDependency, 0, len(base)+len(extra))
	for _, dependency := range append(append([]StructuralDependency(nil), base...), extra...) {
		key := string(dependency.Kind) + "\x00" + dependency.Value
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, dependency)
	}
	return result
}

func phase7QuotedOrAngleValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 2 {
		return ""
	}
	switch value[0] {
	case '"':
		if end := strings.IndexByte(value[1:], '"'); end >= 0 {
			return value[1 : end+1]
		}
	case '<':
		if end := strings.IndexByte(value[1:], '>'); end >= 0 {
			return value[1 : end+1]
		}
	}
	return ""
}

// ArduinoAnalyzer reuses the proven C++ declaration parser but reprojects every
// symbol under its own provider identity. Arduino-generated prototypes/build
// semantics are intentionally outside this source-only adapter.
type ArduinoAnalyzer struct{}

func (ArduinoAnalyzer) ID() AnalyzerID   { return AnalyzerArduino }
func (ArduinoAnalyzer) Language() string { return "arduino" }
func (ArduinoAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	source, err := (CPPAnalyzer{}).Analyze(ctx, document, options)
	if err != nil {
		return AnalyzerResult{}, err
	}
	analysis, err := reprojectAnalyzerSymbols(ctx, document, source, options, "arduino", AnalyzerArduino, "", 0, nil)
	if err != nil {
		return AnalyzerResult{}, err
	}
	copyPhase7AdapterDiagnostics(&analysis, source.Analysis, "arduino", options.Limits.MaxDiagnostics)
	return AnalyzerResult{Analysis: analysis, Dependencies: source.Dependencies, Relations: source.Relations}, nil
}

func copyPhase7AdapterDiagnostics(target *AnalysisResult, source AnalysisResult, language string, maxDiagnostics int) {
	for _, diagnostic := range source.Diagnostics {
		if strings.HasPrefix(diagnostic.Code, "cpp-") {
			diagnostic.Code = language + strings.TrimPrefix(diagnostic.Code, "cpp")
		}
		appendPhase7Diagnostic(target, diagnostic, maxDiagnostics)
	}
	if source.DiagnosticsTruncated {
		target.DiagnosticsTruncated = true
		target.CoverageComplete = false
	}
}

func appendPhase7Diagnostic(target *AnalysisResult, diagnostic AnalysisDiagnostic, maxDiagnostics int) {
	if maxDiagnostics > 0 && len(target.Diagnostics) >= maxDiagnostics {
		target.DiagnosticsTruncated = true
		target.CoverageComplete = false
		return
	}
	target.Diagnostics = append(target.Diagnostics, diagnostic)
}

// Dart/D/Solidity/Apex use one bounded brace parser with explicit per-language
// policies. It shares token/scanner mechanics only; native constructs remain
// separate in the policy and emitted NativeKind values.
type DartAnalyzer struct{}
type DAnalyzer struct{}
type SolidityAnalyzer struct{}
type ApexAnalyzer struct{}

func (DartAnalyzer) ID() AnalyzerID       { return AnalyzerDart }
func (DartAnalyzer) Language() string     { return "dart" }
func (DAnalyzer) ID() AnalyzerID          { return AnalyzerD }
func (DAnalyzer) Language() string        { return "d" }
func (SolidityAnalyzer) ID() AnalyzerID   { return AnalyzerSolidity }
func (SolidityAnalyzer) Language() string { return "solidity" }
func (ApexAnalyzer) ID() AnalyzerID       { return AnalyzerApex }
func (ApexAnalyzer) Language() string     { return "apex" }

type phase7BracePolicy struct {
	language             string
	analyzer             AnalyzerID
	profile              ScannerProfile
	typeKinds            map[string]SymbolKind
	typeNative           map[string]string
	modifiers            map[string]struct{}
	moduleKeyword        string
	moduleKind           SymbolKind
	lineTerminatedModule bool
	importKeywords       map[string]struct{}
	explicitFuncs        map[string]string
	maskText             func(string) (string, bool)
	caseInsensitive      bool
}

func (DartAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePhase7Brace(ctx, document, options, phase7BracePolicy{
		language: "dart", analyzer: AnalyzerDart, profile: DartScannerProfile(),
		typeKinds:  map[string]SymbolKind{"class": SymbolKindClass, "enum": SymbolKindEnum, "mixin": SymbolKindTrait, "extension": SymbolKindType},
		typeNative: map[string]string{"class": "class", "enum": "enum", "mixin": "mixin", "extension": "extension"},
		modifiers:  setOf("abstract", "base", "final", "interface", "sealed"), importKeywords: setOf("import", "export", "part"),
	})
}
func (DAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePhase7Brace(ctx, document, options, phase7BracePolicy{
		language: "d", analyzer: AnalyzerD, profile: DScannerProfile(),
		typeKinds:  map[string]SymbolKind{"class": SymbolKindClass, "struct": SymbolKindStruct, "interface": SymbolKindInterface, "enum": SymbolKindEnum, "union": SymbolKindType},
		typeNative: map[string]string{"class": "class", "struct": "struct", "interface": "interface", "enum": "enum", "union": "union"},
		modifiers:  setOf("abstract", "final", "immutable", "private", "protected", "public", "shared", "static"), moduleKeyword: "module", importKeywords: setOf("import"),
		maskText: maskDQStringLiterals,
	})
}

func maskDQStringLiterals(text string) (string, bool) {
	masked := []byte(text)
	for at := 0; at < len(text); {
		if strings.HasPrefix(text[at:], "//") {
			at = phase7DLineEnd(text, at+2)
			continue
		}
		if strings.HasPrefix(text[at:], "/*") {
			if relative := strings.Index(text[at+2:], "*/"); relative >= 0 {
				at += 2 + relative + 2
				continue
			}
			break
		}
		if strings.HasPrefix(text[at:], "/+") {
			if end, ok := phase7DNestedCommentEnd(text, at); ok {
				at = end
				continue
			}
			break
		}
		if text[at] == '"' || text[at] == '\'' || text[at] == '`' {
			at = phase7DOrdinaryStringEnd(text, at)
			continue
		}
		if text[at] != 'q' || at > 0 && phase7DIdentifierByte(text[at-1]) || at+1 >= len(text) {
			at++
			continue
		}
		end, ok := 0, false
		switch text[at+1] {
		case '{':
			end, ok = phase7DBalancedQStringEnd(text, at+1, '{', '}', false)
		case '"':
			end, ok = phase7DQuotedQStringEnd(text, at)
		}
		if end == 0 {
			at++
			continue
		}
		if !ok {
			phase8MaskRange(masked, at, len(text))
			return string(masked), false
		}
		phase8MaskRange(masked, at, end)
		at = end
	}
	return string(masked), true
}

func phase7DQuotedQStringEnd(text string, at int) (int, bool) {
	content := at + 2
	if content >= len(text) {
		return len(text), false
	}
	switch text[content] {
	case '{':
		return phase7DBalancedQStringEnd(text, content, '{', '}', true)
	case '[':
		return phase7DBalancedQStringEnd(text, content, '[', ']', true)
	case '(':
		return phase7DBalancedQStringEnd(text, content, '(', ')', true)
	case '<':
		return phase7DBalancedQStringEnd(text, content, '<', '>', true)
	}
	if !phase7DIdentifierStart(text[content]) {
		for cursor := content + 1; cursor+1 < len(text); cursor++ {
			if text[cursor] == text[content] && text[cursor+1] == '"' {
				return cursor + 2, true
			}
		}
		return len(text), false
	}
	cursor := content + 1
	for cursor < len(text) && phase7DIdentifierByte(text[cursor]) {
		cursor++
	}
	delimiter := text[content:cursor]
	for cursor < len(text) {
		lineStart := cursor
		if text[lineStart] == '\r' || text[lineStart] == '\n' {
			lineStart = phase7DNextLine(text, lineStart)
		}
		lineEnd := phase7DLineEnd(text, lineStart)
		line := text[lineStart:lineEnd]
		terminator := delimiter + "\""
		if strings.HasPrefix(line, terminator) {
			return lineStart + len(terminator), true
		}
		if lineEnd >= len(text) {
			break
		}
		cursor = phase7DNextLine(text, lineEnd)
	}
	return len(text), false
}

func phase7DBalancedQStringEnd(text string, open int, left, right byte, quoted bool) (int, bool) {
	depth := 1
	for cursor := open + 1; cursor < len(text); cursor++ {
		switch text[cursor] {
		case left:
			depth++
		case right:
			depth--
			if depth == 0 {
				end := cursor + 1
				if quoted {
					if end >= len(text) || text[end] != '"' {
						return len(text), false
					}
					end++
				}
				return end, true
			}
		}
	}
	return len(text), false
}

func phase7DNestedCommentEnd(text string, at int) (int, bool) {
	depth := 1
	for cursor := at + 2; cursor < len(text); cursor++ {
		if strings.HasPrefix(text[cursor:], "/+") {
			depth++
			cursor++
			continue
		}
		if strings.HasPrefix(text[cursor:], "+/") {
			depth--
			cursor++
			if depth == 0 {
				return cursor + 1, true
			}
		}
	}
	return len(text), false
}

func phase7DOrdinaryStringEnd(text string, at int) int {
	delimiter := text[at]
	for cursor := at + 1; cursor < len(text); cursor++ {
		if delimiter != '`' && text[cursor] == '\\' && cursor+1 < len(text) {
			cursor++
			continue
		}
		if text[cursor] == delimiter {
			return cursor + 1
		}
		if delimiter != '`' && (text[cursor] == '\r' || text[cursor] == '\n') {
			return cursor
		}
	}
	return len(text)
}

func phase7DLineEnd(text string, at int) int {
	for at < len(text) && text[at] != '\r' && text[at] != '\n' {
		at++
	}
	return at
}

func phase7DNextLine(text string, at int) int {
	if at < len(text) && text[at] == '\r' {
		at++
		if at < len(text) && text[at] == '\n' {
			at++
		}
		return at
	}
	if at < len(text) && text[at] == '\n' {
		return at + 1
	}
	return at
}

func phase7DIdentifierStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func phase7DIdentifierByte(value byte) bool {
	return phase7DIdentifierStart(value) || value >= '0' && value <= '9'
}
func (SolidityAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePhase7Brace(ctx, document, options, phase7BracePolicy{
		language: "solidity", analyzer: AnalyzerSolidity, profile: SolidityScannerProfile(),
		typeKinds:  map[string]SymbolKind{"contract": SymbolKindClass, "interface": SymbolKindInterface, "library": SymbolKindModule, "struct": SymbolKindStruct, "enum": SymbolKindEnum},
		typeNative: map[string]string{"contract": "contract", "interface": "interface", "library": "library", "struct": "struct", "enum": "enum"},
		modifiers:  setOf("abstract"), importKeywords: setOf("import"), explicitFuncs: map[string]string{"function": "function", "constructor": "constructor", "modifier": "modifier", "fallback": "fallback", "receive": "receive", "event": "event"},
	})
}
func (ApexAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePhase7Brace(ctx, document, options, phase7BracePolicy{
		language: "apex", analyzer: AnalyzerApex, profile: ApexScannerProfile(), caseInsensitive: true,
		typeKinds:  map[string]SymbolKind{"class": SymbolKindClass, "interface": SymbolKindInterface, "enum": SymbolKindEnum},
		typeNative: map[string]string{"class": "class", "interface": "interface", "enum": "enum"},
		modifiers:  setOf("abstract", "global", "private", "protected", "public", "virtual", "with", "without", "sharing"), explicitFuncs: map[string]string{"trigger": "trigger"},
	})
}

func setOf(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func analyzePhase7Brace(ctx context.Context, document *SourceDocument, options AnalyzeOptions, policy phase7BracePolicy) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_phase7_brace_source", document.Path, err)
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: policy.language, Analyzer: string(policy.analyzer), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scanDocument := document
	maskComplete := true
	if policy.maskText != nil {
		masked, complete := policy.maskText(document.Text)
		maskComplete = complete
		if len(masked) != len(document.Text) {
			return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "phase 7 masked source must preserve byte offsets")
		}
		clone := *document
		clone.Text = masked
		scanDocument = &clone
	}
	scan, err := ScanSource(ctx, scanDocument, policy.profile, ScannerLimits{MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
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
	if !maskComplete {
		builder.MarkIncomplete()
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: policy.language + "-unterminated-opaque-region", Message: policy.language + " source contains an unterminated language-specific opaque region", Severity: DiagnosticWarning, AffectsCoverage: true})
	}
	parser := &phase7BraceParser{ctx: ctx, document: document, tokens: scan.Tokens, pairs: PairDelimiterTokens(scan.Tokens, nil), builder: builder, policy: policy}
	parent := parser.parseModuleAndDependencies()
	parser.parseScope(0, len(scan.Tokens), parent, false, "")
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_phase7_brace_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

type phase7BraceParser struct {
	ctx          context.Context
	document     *SourceDocument
	tokens       []Token
	pairs        map[int]int
	builder      *SymbolBuilder
	policy       phase7BracePolicy
	dependencies []StructuralDependency
	relations    []StructuralRelation
	module       *SymbolParent
	stopped      bool
}

func (p *phase7BraceParser) eq(value, want string) bool {
	if p.policy.caseInsensitive {
		return strings.EqualFold(value, want)
	}
	return value == want
}
func (p *phase7BraceParser) lower(value string) string {
	if p.policy.caseInsensitive {
		return strings.ToLower(value)
	}
	return value
}

func (p *phase7BraceParser) parseModuleAndDependencies() *SymbolParent {
	for i := 0; i < len(p.tokens); i++ {
		if p.tokens[i].Nesting != 0 || p.tokens[i].Kind == TokenNewline || p.tokens[i].Kind == TokenEOF {
			continue
		}
		text := p.lower(p.tokens[i].Text)
		if p.policy.moduleKeyword != "" && p.eq(text, p.policy.moduleKeyword) && p.module == nil {
			terminator := p.findSameDepth(i+1, len(p.tokens), ";", p.tokens[i].Nesting)
			declarationEnd := -1
			if terminator > i+1 {
				declarationEnd = p.tokens[terminator].EndOffset
			} else if p.policy.lineTerminatedModule {
				terminator = p.lineEnd(i)
				if terminator > i+1 {
					declarationEnd = p.tokens[terminator-1].EndOffset
				}
			}
			if terminator > i+1 && declarationEnd >= 0 {
				start := nextIdentifierToken(p.tokens, i+1, terminator)
				if start >= 0 {
					name := tokenRangeText(p.tokens, start, terminator)
					if name != "" {
						kind := p.policy.moduleKind
						if kind == "" {
							kind = SymbolKindModule
						}
						symbol, ok := p.add(SymbolSpec{Kind: kind, NativeKind: p.policy.moduleKeyword, Name: name, QualifiedName: name, Declaration: OffsetRange{Start: p.tokens[i].StartOffset, End: declarationEnd}, NameRange: OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[terminator-1].EndOffset}, Signature: &OffsetRange{Start: p.tokens[i].StartOffset, End: declarationEnd}, Evidence: SymbolEvidenceStructural})
						if ok {
							value := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
							p.module = &value
						}
					}
				}
			}
		}
		if _, ok := p.policy.importKeywords[text]; ok {
			p.collectImport(i)
		}
	}
	return p.module
}

func (p *phase7BraceParser) collectImport(start int) {
	depth := p.tokens[start].Nesting
	end := p.findSameDepth(start+1, len(p.tokens), ";", depth)
	if end < 0 {
		end = p.lineEnd(start)
	}
	if end <= start+1 {
		return
	}
	for i := start + 1; i < end; i++ {
		if p.tokens[i].Kind != TokenString {
			continue
		}
		value := phase7StringValue(p.tokens[i].Text)
		if value != "" {
			p.addDependency(StructuralDependencyImport, value, p.tokens[i].StartOffset, p.tokens[i].EndOffset)
			return
		}
	}
	first := nextIdentifierToken(p.tokens, start+1, end)
	if first < 0 {
		return
	}
	last := end
	for last > first && (p.tokens[last-1].Text == "," || p.tokens[last-1].Text == ";") {
		last--
	}
	value := tokenRangeText(p.tokens, first, last)
	if value != "" {
		p.addDependency(StructuralDependencyImport, value, p.tokens[first].StartOffset, p.tokens[last-1].EndOffset)
	}
}

func phase7StringValue(value string) string {
	if len(value) < 2 {
		return ""
	}
	quote := value[0]
	if (quote != '\'' && quote != '"') || value[len(value)-1] != quote {
		return ""
	}
	return value[1 : len(value)-1]
}

func (p *phase7BraceParser) parseScope(start, end int, parent *SymbolParent, members bool, owner string) {
	for i := start; i < end && !p.stopped; {
		if p.ctx.Err() != nil {
			return
		}
		i = nextStructuralToken(p.tokens, i, end)
		if i >= end || p.tokens[i].Kind == TokenEOF {
			return
		}
		if p.tokens[i].Kind == TokenDirective {
			i++
			continue
		}
		if keyword, ok := p.typeKeyword(i, end); ok {
			i = p.parseType(i, keyword, end, parent)
			continue
		}
		if native, ok := p.policy.explicitFuncs[p.lower(p.tokens[i].Text)]; ok {
			i = p.parseExplicit(i, end, parent, members, owner, native)
			continue
		}
		if next, ok := p.parseCStyle(i, end, parent, members, owner); ok {
			i = next
			continue
		}
		i++
	}
}

func (p *phase7BraceParser) typeKeyword(start, end int) (int, bool) {
	limit := min(end, start+16)
	for i := start; i < limit; i++ {
		text := p.lower(p.tokens[i].Text)
		if _, ok := p.policy.typeKinds[text]; ok {
			return i, true
		}
		if _, ok := p.policy.modifiers[text]; ok {
			continue
		}
		if p.tokens[i].Text == "{" || p.tokens[i].Text == ";" || p.tokens[i].Kind == TokenNewline {
			break
		}
		if p.tokens[i].Kind == TokenIdentifier && i > start {
			break
		}
	}
	return -1, false
}

func (p *phase7BraceParser) parseType(start, keyword, end int, parent *SymbolParent) int {
	nameIndex := nextIdentifierToken(p.tokens, keyword+1, end)
	if nameIndex < 0 {
		p.builder.MarkIncomplete()
		return keyword + 1
	}
	depth := p.tokens[keyword].Nesting
	open := -1
	semicolon := -1
	for i := nameIndex + 1; i < end; i++ {
		if p.tokens[i].Text == "{" && p.tokens[i].Nesting == depth+1 {
			open = i
			break
		}
		if p.tokens[i].Text == ";" && p.tokens[i].Nesting == depth {
			semicolon = i
			break
		}
	}
	terminator := semicolon
	close := -1
	if open >= 0 {
		close = p.pairs[open]
		if close <= open || close >= end {
			p.builder.MarkIncomplete()
			return end
		}
		terminator = close
	}
	if terminator < 0 {
		p.builder.MarkIncomplete()
		return end
	}
	native := p.lower(p.tokens[keyword].Text)
	kind := p.policy.typeKinds[native]
	if override := p.policy.typeNative[native]; override != "" {
		native = override
	}
	declarationEnd := p.tokens[terminator].EndOffset
	signatureEnd := declarationEnd
	var body *OffsetRange
	if open >= 0 {
		signatureEnd = p.tokens[open].StartOffset
		value := OffsetRange{Start: p.tokens[open].StartOffset, End: p.tokens[close].EndOffset}
		body = &value
	}
	modifiers := p.collectModifiers(start, keyword)
	symbol, added := p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: p.tokens[nameIndex].Text, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: declarationEnd}, NameRange: OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: p.tokens[start].StartOffset, End: signatureEnd}, Body: body, Visibility: phase7Visibility(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural})
	if added && open >= 0 {
		p.collectTypeRelations(symbol.QualifiedName, nameIndex+1, open, depth)
		if kind != SymbolKindEnum {
			child := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
			p.parseScope(open+1, close, child, true, symbol.Name)
		}
	}
	return terminator + 1
}

func (p *phase7BraceParser) collectTypeRelations(source string, start, end, depth int) {
	colon := -1
	for i := start; i < end; i++ {
		if p.tokens[i].Text == ":" && p.tokens[i].Nesting == depth {
			colon = i
			break
		}
	}
	if colon >= 0 {
		p.addRelationParts("inherits", source, colon+1, end, depth)
		return
	}
	for i := start; i < end; i++ {
		keyword := p.lower(p.tokens[i].Text)
		kind := ""
		switch keyword {
		case "extends", "is":
			kind = "inherits"
		case "implements":
			kind = "implements"
		case "with":
			kind = "mixin"
		}
		if kind == "" {
			continue
		}
		segmentEnd := end
		for j := i + 1; j < end; j++ {
			other := p.lower(p.tokens[j].Text)
			if other == "extends" || other == "is" || other == "implements" || other == "with" {
				segmentEnd = j
				break
			}
		}
		p.addRelationParts(kind, source, i+1, segmentEnd, depth)
		i = segmentEnd - 1
	}
}

func (p *phase7BraceParser) addRelationParts(kind, source string, start, end, depth int) {
	for _, part := range splitTokenRangeAt(p.tokens, start, end, ",", depth) {
		left, right := part[0], part[1]
		for left < right {
			text := p.lower(p.tokens[left].Text)
			if text == "public" || text == "private" || text == "protected" || text == "virtual" {
				left++
				continue
			}
			break
		}
		if left >= right {
			continue
		}
		target := tokenRangeText(p.tokens, left, right)
		if target == "" {
			continue
		}
		rangeValue, err := p.document.RangeFromUTF8Offsets(p.tokens[left].StartOffset, p.tokens[right-1].EndOffset)
		if err == nil {
			p.relations = append(p.relations, StructuralRelation{Kind: kind, Source: source, Target: target, Range: rangeValue, Evidence: SymbolEvidenceStructural})
		}
	}
}

func (p *phase7BraceParser) parseExplicit(start, end int, parent *SymbolParent, members bool, owner, native string) int {
	depth := p.tokens[start].Nesting
	kind := SymbolKindFunction
	nameIndex := -1
	name := ""
	switch native {
	case "constructor":
		kind = SymbolKindConstructor
		name = owner
		if name == "" {
			name = "constructor"
		}
	case "modifier":
		kind = SymbolKindMethod
		nameIndex = nextIdentifierToken(p.tokens, start+1, end)
	case "event":
		kind = SymbolKindEvent
		nameIndex = nextIdentifierToken(p.tokens, start+1, end)
	case "trigger":
		kind = SymbolKindFunction
		nameIndex = nextIdentifierToken(p.tokens, start+1, end)
	case "fallback", "receive":
		if members {
			kind = SymbolKindMethod
		}
		name = native
	default:
		if members {
			kind = SymbolKindMethod
		}
		nameIndex = nextIdentifierToken(p.tokens, start+1, end)
	}
	if nameIndex >= 0 {
		name = p.tokens[nameIndex].Text
	}
	if name == "" {
		return start + 1
	}
	paren := -1
	for i := start + 1; i < end; i++ {
		if p.tokens[i].Text == "(" && p.tokens[i].Nesting == depth+1 {
			paren = i
			break
		}
		if p.tokens[i].Text == ";" && p.tokens[i].Nesting == depth {
			break
		}
	}
	search := start + 1
	if paren >= 0 {
		if closeParen := p.pairs[paren]; closeParen > paren {
			search = closeParen + 1
		}
	}
	terminator, bodyOpen := p.functionTerminator(search, end, depth)
	if terminator < 0 {
		return start + 1
	}
	declarationEnd := p.tokens[terminator].EndOffset
	var body *OffsetRange
	next := terminator + 1
	if bodyOpen >= 0 {
		close := p.pairs[bodyOpen]
		if close <= bodyOpen {
			p.builder.MarkIncomplete()
			return end
		}
		declarationEnd = p.tokens[close].EndOffset
		value := OffsetRange{Start: p.tokens[bodyOpen].StartOffset, End: p.tokens[close].EndOffset}
		body = &value
		next = close + 1
	}
	nameRange := OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[start].EndOffset}
	if nameIndex >= 0 {
		nameRange = OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset}
	}
	if native == "constructor" && owner != "" {
		nameRange = OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[start].EndOffset}
	}
	p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: name, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: declarationEnd}, NameRange: nameRange, Signature: &OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[terminator].StartOffset}, Body: body, Evidence: SymbolEvidenceStructural, Disambiguator: p.parameterDisambiguator(paren)})
	return next
}

func (p *phase7BraceParser) parseCStyle(start, end int, parent *SymbolParent, members bool, owner string) (int, bool) {
	depth := p.tokens[start].Nesting
	terminator := -1
	for i := start; i < end; i++ {
		if p.tokens[i].Text == ";" && p.tokens[i].Nesting == depth {
			terminator = i
			break
		}
		if p.tokens[i].Text == "{" && p.tokens[i].Nesting == depth+1 {
			terminator = i
			break
		}
		if p.tokens[i].Kind == TokenEOF {
			break
		}
	}
	if terminator < 0 {
		return start + 1, false
	}
	paren := -1
	for i := start; i < terminator; i++ {
		if p.tokens[i].Text == "(" && p.tokens[i].Nesting == depth+1 {
			paren = i
			break
		}
	}
	if paren >= 0 {
		nameIndex := previousIdentifierToken(p.tokens, paren-1, start)
		name := ""
		kind := SymbolKindFunction
		native := "function"
		if nameIndex >= 0 {
			name = p.tokens[nameIndex].Text
		}
		if name == "" && paren > start && p.eq(p.tokens[paren-1].Text, "this") {
			name = owner
			kind = SymbolKindConstructor
			native = "constructor"
		}
		if name != "" {
			lowerName := strings.ToLower(name)
			if lowerName != "if" && lowerName != "for" && lowerName != "while" && lowerName != "switch" && lowerName != "catch" && lowerName != "return" {
				if members && kind == SymbolKindFunction {
					kind = SymbolKindMethod
					native = "method"
				}
				if members && owner != "" && p.eq(name, owner) {
					kind = SymbolKindConstructor
					native = "constructor"
				}
				if nameIndex > start && p.tokens[nameIndex-1].Text == "~" && (p.eq(name, owner) || p.eq(name, "this")) {
					kind = SymbolKindDestructor
					native = "destructor"
				}
				bodyOpen := -1
				declarationEnd := p.tokens[terminator].EndOffset
				next := terminator + 1
				var body *OffsetRange
				if p.tokens[terminator].Text == "{" {
					bodyOpen = terminator
					close := p.pairs[bodyOpen]
					if close > bodyOpen {
						declarationEnd = p.tokens[close].EndOffset
						value := OffsetRange{Start: p.tokens[bodyOpen].StartOffset, End: p.tokens[close].EndOffset}
						body = &value
						next = close + 1
					}
				}
				nameRange := OffsetRange{Start: p.tokens[paren-1].StartOffset, End: p.tokens[paren-1].EndOffset}
				if nameIndex >= 0 {
					nameRange = OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset}
				}
				p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: name, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: declarationEnd}, NameRange: nameRange, Signature: &OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[terminator].StartOffset}, Body: body, Visibility: phase7Visibility(p.collectModifiers(start, paren)), Modifiers: p.collectModifiers(start, paren), Evidence: SymbolEvidenceStructural, Disambiguator: p.parameterDisambiguator(paren)})
				return next, true
			}
		}
	}
	if p.tokens[terminator].Text == "{" {
		if close := p.pairs[terminator]; close > terminator {
			return close + 1, false
		}
		return terminator + 1, false
	}
	return p.parseVariable(start, terminator, parent, members)
}

func (p *phase7BraceParser) parseVariable(start, semicolon int, parent *SymbolParent, members bool) (int, bool) {
	if start >= semicolon {
		return semicolon + 1, false
	}
	first := p.lower(p.tokens[start].Text)
	if first == "module" || first == "import" || first == "export" || first == "part" || first == "pragma" || first == "using" || first == "return" {
		return semicolon + 1, false
	}
	limit := semicolon
	for i := start; i < semicolon; i++ {
		if p.tokens[i].Text == "=" && p.tokens[i].Nesting == p.tokens[start].Nesting {
			limit = i
			break
		}
	}
	nameIndex := previousIdentifierToken(p.tokens, limit-1, start)
	if nameIndex < 0 {
		return semicolon + 1, false
	}
	if _, isModifier := p.policy.modifiers[p.lower(p.tokens[nameIndex].Text)]; isModifier {
		return semicolon + 1, false
	}
	kind := SymbolKindVariable
	native := "variable"
	if members {
		kind = SymbolKindField
		native = "field"
	}
	for i := start; i < nameIndex; i++ {
		text := p.lower(p.tokens[i].Text)
		if text == "const" || text == "immutable" || text == "final" {
			if !members {
				kind = SymbolKindConstant
				native = "constant"
			}
			break
		}
	}
	modifiers := p.collectModifiers(start, nameIndex)
	p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: p.tokens[nameIndex].Text, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[semicolon].EndOffset}, NameRange: OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset}, Visibility: phase7Visibility(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural})
	return semicolon + 1, true
}

func (p *phase7BraceParser) collectModifiers(start, end int) []string {
	var result []string
	for i := start; i < end; i++ {
		text := p.lower(p.tokens[i].Text)
		if _, ok := p.policy.modifiers[text]; ok || text == "public" || text == "private" || text == "protected" || text == "static" || text == "external" || text == "internal" || text == "global" || text == "override" || text == "virtual" {
			result = append(result, text)
		}
	}
	return result
}

func phase7Visibility(modifiers []string) Visibility {
	for _, modifier := range modifiers {
		switch strings.ToLower(modifier) {
		case "public", "global":
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

func (p *phase7BraceParser) functionTerminator(start, end, depth int) (terminator, bodyOpen int) {
	for i := start; i < end; i++ {
		if p.tokens[i].Text == ";" && p.tokens[i].Nesting == depth {
			return i, -1
		}
		if p.tokens[i].Text == "{" && p.tokens[i].Nesting == depth+1 {
			return i, i
		}
	}
	return -1, -1
}

func (p *phase7BraceParser) parameterDisambiguator(paren int) string {
	if paren < 0 {
		return ""
	}
	if close := p.pairs[paren]; close > paren {
		return tokenRangeText(p.tokens, paren, close+1)
	}
	return ""
}

func (p *phase7BraceParser) findSameDepth(start, end int, text string, depth int) int {
	for i := start; i < end; i++ {
		if p.tokens[i].Text == text && p.tokens[i].Nesting == depth {
			return i
		}
	}
	return -1
}
func (p *phase7BraceParser) lineEnd(start int) int {
	for i := start; i < len(p.tokens); i++ {
		if p.tokens[i].Kind == TokenNewline || p.tokens[i].Kind == TokenEOF {
			return i
		}
	}
	return len(p.tokens)
}
func (p *phase7BraceParser) addDependency(kind StructuralDependencyKind, value string, start, end int) {
	rangeValue, err := p.document.RangeFromUTF8Offsets(start, end)
	if err == nil {
		p.dependencies = appendUniqueDependencies(p.dependencies, []StructuralDependency{{Kind: kind, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural}})
	}
}
func (p *phase7BraceParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
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
