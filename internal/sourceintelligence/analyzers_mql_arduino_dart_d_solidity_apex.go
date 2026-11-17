package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// MQL4Analyzer and MQL5Analyzer deliberately keep distinct provider identities.
// They reuse only the declaration-safe C++ structural subset, add MQL-specific
// evidence, and resolve only deterministic predefined dialect macros.
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
	scannerLimits := ScannerLimits{MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: 2048}
	scan, err := ScanSource(ctx, document, MQLScannerProfile(language), scannerLimits)
	if err != nil {
		return AnalyzerResult{}, err
	}
	projectionDocument := document
	if projected, changed := projectMQLDialectConditionals(document.Text, scan.Tokens, language); changed {
		clone := *document
		clone.Text = projected
		clone.lineStarts = buildLineStarts(projected)
		projectionDocument = &clone
		scan, err = ScanSource(ctx, projectionDocument, MQLScannerProfile(language), scannerLimits)
		if err != nil {
			return AnalyzerResult{}, err
		}
	}
	interfaceNameOffsets := make(map[int]struct{})
	analysisDocument := projectionDocument
	if projected, changed := projectMQLForCPP(projectionDocument.Text, scan.Tokens, language, interfaceNameOffsets); changed {
		clone := *projectionDocument
		clone.Text = projected
		clone.lineStarts = buildLineStarts(projected)
		analysisDocument = &clone
	}
	// MQL resolves only predefined dialect conditionals; unknown/user macro state must retain single-pass C++ fail-closed delimiter semantics.
	source, err := analyzeCFamilySingle(ctx, analysisDocument, options, true)
	if err != nil {
		return AnalyzerResult{}, err
	}
	interfaceIDs := make(map[string]struct{})
	for index := range source.Analysis.Symbols {
		symbol := &source.Analysis.Symbols[index]
		declaration, nameRange, _, _ := symbol.SourceOffsets()
		if declaration.Start < 0 || declaration.End > len(document.Text) || declaration.End <= declaration.Start {
			continue
		}
		declarationText := strings.TrimSpace(document.Text[declaration.Start:declaration.End])
		if symbol.Kind == SymbolKindClass {
			if _, ok := interfaceNameOffsets[nameRange.Start]; ok {
				symbol.Kind = SymbolKindInterface
				symbol.NativeKind = "interface"
				symbol.Visibility = VisibilityPublic
				interfaceIDs[symbol.ID] = struct{}{}
				continue
			}
		}
		if symbol.Kind != SymbolKindVariable && symbol.Kind != SymbolKindConstant {
			continue
		}
		prefix := strings.ToLower(declarationText)
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
	for index := range source.Analysis.Symbols {
		symbol := &source.Analysis.Symbols[index]
		if _, ok := interfaceIDs[symbol.ParentID]; ok {
			symbol.Visibility = VisibilityPublic
		}
	}
	analysis, err := reprojectAnalyzerSymbols(ctx, document, source, options, language, analyzer, "", 0, nil)
	if err != nil {
		return AnalyzerResult{}, err
	}
	relabelBraceAdapterDiagnostics(&analysis, language)
	dependencies := append([]StructuralDependency(nil), source.Dependencies...)
	imports := collectMQLDirectives(document, scan.Tokens)
	dependencies = appendUniqueDependencies(dependencies, imports)
	return AnalyzerResult{Analysis: analysis, Dependencies: dependencies, Relations: source.Relations}, nil
}

type mqlDialectConditionalFrame struct {
	opener      Token
	elseToken   Token
	endifToken  Token
	activeFirst bool
	hasElse     bool
	hasElif     bool
	malformed   bool
	known       bool
}

func projectMQLDialectConditionals(text string, tokens []Token, language string) (string, bool) {
	var stack []*mqlDialectConditionalFrame
	var completed []mqlDialectConditionalFrame
	for _, token := range tokens {
		if token.Kind != TokenDirective {
			continue
		}
		keyword, rest := cFamilyDirectiveKeywordAndRest(token.Text)
		switch keyword {
		case "#if":
			stack = append(stack, &mqlDialectConditionalFrame{opener: token})
		case "#ifdef", "#ifndef":
			active, known := mqlDialectConditionalValue(keyword, rest, language)
			stack = append(stack, &mqlDialectConditionalFrame{opener: token, activeFirst: active, known: known})
		case "#elif":
			if len(stack) > 0 {
				stack[len(stack)-1].hasElif = true
			}
		case "#else":
			if len(stack) > 0 {
				frame := stack[len(stack)-1]
				if frame.hasElse {
					frame.malformed = true
				} else {
					frame.hasElse = true
					frame.elseToken = token
				}
			}
		case "#endif":
			if len(stack) == 0 {
				continue
			}
			frame := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			frame.endifToken = token
			if frame.known && !frame.hasElif && !frame.malformed {
				completed = append(completed, *frame)
			}
		}
	}
	if len(completed) == 0 {
		return text, false
	}
	projected := []byte(text)
	for _, frame := range completed {
		maskRangePreservingLines(projected, frame.opener.StartOffset, frame.opener.EndOffset)
		if frame.hasElse {
			maskRangePreservingLines(projected, frame.elseToken.StartOffset, frame.elseToken.EndOffset)
		}
		maskRangePreservingLines(projected, frame.endifToken.StartOffset, frame.endifToken.EndOffset)
		if frame.activeFirst {
			if frame.hasElse {
				maskRangePreservingLines(projected, frame.elseToken.EndOffset, frame.endifToken.StartOffset)
			}
		} else {
			inactiveEnd := frame.endifToken.StartOffset
			if frame.hasElse {
				inactiveEnd = frame.elseToken.StartOffset
			}
			maskRangePreservingLines(projected, frame.opener.EndOffset, inactiveEnd)
		}
	}
	return string(projected), true
}

func mqlDialectConditionalValue(keyword, rest, language string) (bool, bool) {
	rest = strings.TrimSpace(rest)
	if comment := strings.Index(rest, "//"); comment >= 0 {
		rest = strings.TrimSpace(rest[:comment])
	}
	if comment := strings.Index(rest, "/*"); comment >= 0 {
		rest = strings.TrimSpace(rest[:comment])
	}
	var defined bool
	switch rest {
	case "__MQL4__":
		defined = language == "mql4"
	case "__MQL5__":
		defined = language == "mql5"
	case "__MQL__":
		defined = true
	case "__cplusplus":
		defined = false
	default:
		return false, false
	}
	if keyword == "#ifndef" {
		defined = !defined
	}
	return defined, true
}

func projectMQLForCPP(text string, tokens []Token, language string, interfaceNameOffsets map[int]struct{}) (string, bool) {
	var projected []byte
	ensureProjection := func() {
		if projected == nil {
			projected = []byte(text)
		}
	}
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		if language == "mql5" && index+2 < len(tokens) && token.Nesting == 0 && strings.EqualFold(token.Text, "input") {
			lineEnd, _ := physicalLineBounds(text, token.StartOffset)
			group := tokens[index+1]
			label := tokens[index+2]
			if group.StartOffset < lineEnd && label.StartOffset < lineEnd && strings.EqualFold(group.Text, "group") && label.Kind == TokenString {
				ensureProjection()
				maskRangePreservingLines(projected, token.StartOffset, lineEnd)
				index += 2
				continue
			}
		}
		nameIndex, ok := mqlInterfaceNameToken(tokens, index)
		if !ok {
			continue
		}
		if interfaceNameOffsets != nil {
			interfaceNameOffsets[tokens[nameIndex].StartOffset] = struct{}{}
		}
		ensureProjection()
		copy(projected[token.StartOffset:token.EndOffset], "class")
		for offset := token.StartOffset + len("class"); offset < token.EndOffset; offset++ {
			projected[offset] = ' '
		}
	}
	if projected == nil {
		return text, false
	}
	return string(projected), true
}

func mqlInterfaceNameToken(tokens []Token, index int) (int, bool) {
	if index < 0 || index >= len(tokens) {
		return 0, false
	}
	keyword := tokens[index]
	if keyword.Nesting != 0 || !strings.EqualFold(keyword.Text, "interface") {
		return 0, false
	}
	name := nextStructuralToken(tokens, index+1, len(tokens))
	if name >= len(tokens) || tokens[name].Kind != TokenIdentifier {
		return 0, false
	}
	for cursor := name + 1; cursor < len(tokens); cursor++ {
		token := tokens[cursor]
		if token.Kind == TokenEOF {
			return 0, false
		}
		if token.Text == ";" && token.Nesting == keyword.Nesting {
			return 0, false
		}
		if token.Text == "{" && token.Nesting == keyword.Nesting+1 {
			return name, true
		}
	}
	return 0, false
}

func collectMQLDirectives(document *SourceDocument, tokens []Token) []StructuralDependency {
	var result []StructuralDependency
	for _, token := range tokens {
		if token.Kind != TokenDirective {
			continue
		}
		trimmed := strings.TrimSpace(token.Text)
		lower := strings.ToLower(trimmed)
		if !strings.HasPrefix(lower, "#import") {
			continue
		}
		rest := strings.TrimSpace(trimmed[len("#import"):])
		value := quotedOrAngleValue(rest)
		if value == "" {
			continue
		}
		rangeValue, rangeErr := document.RangeFromUTF8Offsets(token.StartOffset, token.EndOffset)
		if rangeErr == nil {
			result = append(result, StructuralDependency{Kind: StructuralDependencyImport, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural})
		}
	}
	return result
}

func quotedOrAngleValue(value string) string {
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
	relabelBraceAdapterDiagnostics(&analysis, "arduino")
	return AnalyzerResult{Analysis: analysis, Dependencies: source.Dependencies, Relations: source.Relations}, nil
}

func relabelBraceAdapterDiagnostics(target *AnalysisResult, language string) {
	for index := range target.Diagnostics {
		diagnostic := &target.Diagnostics[index]
		if strings.HasPrefix(diagnostic.Code, "cpp-") {
			diagnostic.Code = language + strings.TrimPrefix(diagnostic.Code, "cpp")
		}
	}
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

type braceLanguagePolicy struct {
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
	supportsAnnotations  bool
}

func (DartAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeBraceLanguage(ctx, document, options, braceLanguagePolicy{
		language: "dart", analyzer: AnalyzerDart, profile: DartScannerProfile(),
		typeKinds:  map[string]SymbolKind{"class": SymbolKindClass, "enum": SymbolKindEnum, "mixin": SymbolKindTrait, "extension": SymbolKindType},
		typeNative: map[string]string{"class": "class", "enum": "enum", "mixin": "mixin", "extension": "extension"},
		modifiers:  setOf("abstract", "base", "final", "interface", "sealed"), importKeywords: setOf("import", "export", "part"), supportsAnnotations: true,
	})
}
func (DAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeBraceLanguage(ctx, document, options, braceLanguagePolicy{
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
			at = dLineEnd(text, at+2)
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
			if end, ok := dNestedCommentEnd(text, at); ok {
				at = end
				continue
			}
			break
		}
		if text[at] == '"' || text[at] == '\'' || text[at] == '`' {
			at = dOrdinaryStringEnd(text, at)
			continue
		}
		if text[at] != 'q' || at > 0 && dIdentifierByte(text[at-1]) || at+1 >= len(text) {
			at++
			continue
		}
		end, ok := 0, false
		switch text[at+1] {
		case '{':
			end, ok = dBalancedQStringEnd(text, at+1, '{', '}', false)
		case '"':
			end, ok = dQuotedQStringEnd(text, at)
		}
		if end == 0 {
			at++
			continue
		}
		if !ok {
			maskRangePreservingLines(masked, at, len(text))
			return string(masked), false
		}
		maskRangePreservingLines(masked, at, end)
		at = end
	}
	return string(masked), true
}

func dQuotedQStringEnd(text string, at int) (int, bool) {
	content := at + 2
	if content >= len(text) {
		return len(text), false
	}
	switch text[content] {
	case '{':
		return dBalancedQStringEnd(text, content, '{', '}', true)
	case '[':
		return dBalancedQStringEnd(text, content, '[', ']', true)
	case '(':
		return dBalancedQStringEnd(text, content, '(', ')', true)
	case '<':
		return dBalancedQStringEnd(text, content, '<', '>', true)
	}
	if !dIdentifierStart(text[content]) {
		for cursor := content + 1; cursor+1 < len(text); cursor++ {
			if text[cursor] == text[content] && text[cursor+1] == '"' {
				return cursor + 2, true
			}
		}
		return len(text), false
	}
	cursor := content + 1
	for cursor < len(text) && dIdentifierByte(text[cursor]) {
		cursor++
	}
	delimiter := text[content:cursor]
	for cursor < len(text) {
		lineStart := cursor
		if text[lineStart] == '\r' || text[lineStart] == '\n' {
			lineStart = dNextLine(text, lineStart)
		}
		lineEnd := dLineEnd(text, lineStart)
		line := text[lineStart:lineEnd]
		terminator := delimiter + "\""
		if strings.HasPrefix(line, terminator) {
			return lineStart + len(terminator), true
		}
		if lineEnd >= len(text) {
			break
		}
		cursor = dNextLine(text, lineEnd)
	}
	return len(text), false
}

func dBalancedQStringEnd(text string, open int, left, right byte, quoted bool) (int, bool) {
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

func dNestedCommentEnd(text string, at int) (int, bool) {
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

func dOrdinaryStringEnd(text string, at int) int {
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

func dLineEnd(text string, at int) int {
	for at < len(text) && text[at] != '\r' && text[at] != '\n' {
		at++
	}
	return at
}

func dNextLine(text string, at int) int {
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

func dIdentifierStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func dIdentifierByte(value byte) bool {
	return dIdentifierStart(value) || value >= '0' && value <= '9'
}
func (SolidityAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeBraceLanguage(ctx, document, options, braceLanguagePolicy{
		language: "solidity", analyzer: AnalyzerSolidity, profile: SolidityScannerProfile(),
		typeKinds:  map[string]SymbolKind{"contract": SymbolKindClass, "interface": SymbolKindInterface, "library": SymbolKindModule, "struct": SymbolKindStruct, "enum": SymbolKindEnum},
		typeNative: map[string]string{"contract": "contract", "interface": "interface", "library": "library", "struct": "struct", "enum": "enum"},
		modifiers:  setOf("abstract"), importKeywords: setOf("import"), explicitFuncs: map[string]string{"function": "function", "constructor": "constructor", "modifier": "modifier", "fallback": "fallback", "receive": "receive", "event": "event", "error": "error"},
	})
}
func (ApexAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeBraceLanguage(ctx, document, options, braceLanguagePolicy{
		language: "apex", analyzer: AnalyzerApex, profile: ApexScannerProfile(), caseInsensitive: true,
		typeKinds:  map[string]SymbolKind{"class": SymbolKindClass, "interface": SymbolKindInterface, "enum": SymbolKindEnum},
		typeNative: map[string]string{"class": "class", "interface": "interface", "enum": "enum"},
		modifiers:  setOf("abstract", "global", "private", "protected", "public", "virtual", "with", "without", "sharing"), explicitFuncs: map[string]string{"trigger": "trigger"}, supportsAnnotations: true,
	})
}

func setOf(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func analyzeBraceLanguage(ctx context.Context, document *SourceDocument, options AnalyzeOptions, policy braceLanguagePolicy) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_brace_language_source", document.Path, err)
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
			return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "masked source must preserve byte offsets")
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
	parser := &braceLanguageParser{ctx: ctx, document: document, tokens: scan.Tokens, pairs: PairDelimiterTokens(scan.Tokens, nil), builder: builder, policy: policy}
	parent := parser.parseModuleAndDependencies()
	parser.parseScope(0, len(scan.Tokens), parent, false, "")
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_brace_language_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

type braceLanguageParser struct {
	ctx          context.Context
	document     *SourceDocument
	tokens       []Token
	pairs        map[int]int
	builder      *SymbolBuilder
	policy       braceLanguagePolicy
	dependencies []StructuralDependency
	relations    []StructuralRelation
	module       *SymbolParent
	stopped      bool
}

func (p *braceLanguageParser) eq(value, want string) bool {
	if p.policy.caseInsensitive {
		return strings.EqualFold(value, want)
	}
	return value == want
}
func (p *braceLanguageParser) lower(value string) string {
	if p.policy.caseInsensitive {
		return strings.ToLower(value)
	}
	return value
}

func (p *braceLanguageParser) parseModuleAndDependencies() *SymbolParent {
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

func (p *braceLanguageParser) collectImport(start int) {
	depth := p.tokens[start].Nesting
	end := p.findSameDepth(start+1, len(p.tokens), ";", depth)
	if end < 0 {
		end = p.lineEnd(start)
	}
	if end <= start+1 {
		return
	}
	if p.policy.language == "d" {
		p.collectDImports(start, end, depth)
		return
	}
	if p.policy.language == "dart" &&
		(p.tokens[start].Text == "import" || p.tokens[start].Text == "export") {
		for i := start + 1; i < end; i++ {
			if p.tokens[i].Text != "if" || p.tokens[i].Nesting != depth {
				continue
			}
			// The default URI is authored source evidence, but platform-guarded
			// alternatives cannot be represented by an unconditional dependency.
			p.builder.MarkIncomplete()
			span := OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[end-1].EndOffset}
			_ = p.builder.AddDiagnostic(DiagnosticSpec{
				Code:     "dart-conditional-uri-alternatives",
				Message:  "Dart conditional import/export alternatives are not indexed; inspect the authored platform guards",
				Severity: DiagnosticWarning, Range: &span, AffectsCoverage: true,
			})
			break
		}
	}
	for i := start + 1; i < end; i++ {
		if p.tokens[i].Kind != TokenString {
			continue
		}
		value := quotedStringValue(p.tokens[i].Text)
		if value != "" {
			if p.policy.language == "dart" && p.tokens[start].Text == "import" {
				// Dart's optional prefix belongs to the local binding, not
				// to the imported URI. Only adjacent directive tokens count.
				next := i + 1
				for next < end && p.tokens[next].Kind == TokenNewline {
					next++
				}
				if next < end && p.tokens[next].Text == "deferred" {
					next++
					for next < end && p.tokens[next].Kind == TokenNewline {
						next++
					}
				}
				if next+1 < end && p.tokens[next].Text == "as" &&
					p.tokens[next+1].Kind == TokenIdentifier {
					rangeValue, err := p.document.RangeFromUTF8Offsets(p.tokens[i].StartOffset, p.tokens[i].EndOffset)
					if err == nil {
						p.dependencies = appendUniqueDependencies(p.dependencies, []StructuralDependency{{
							Kind: StructuralDependencyImport, Value: value, Alias: p.tokens[next+1].Text,
							Range: rangeValue, Evidence: SymbolEvidenceStructural,
						}})
					}
					return
				}
			}
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
	if p.policy.language == "groovy" && last-first >= 3 &&
		p.tokens[last-2].Text == "as" && p.tokens[last-1].Kind == TokenIdentifier {
		// Groovy's local alias is not part of the imported type/member.
		value := tokenRangeText(p.tokens, first, last-2)
		if value != "" {
			rangeValue, err := p.document.RangeFromUTF8Offsets(p.tokens[first].StartOffset, p.tokens[last-3].EndOffset)
			if err == nil {
				p.dependencies = appendUniqueDependencies(p.dependencies, []StructuralDependency{{
					Kind: StructuralDependencyImport, Value: value, Alias: p.tokens[last-1].Text,
					Range: rangeValue, Evidence: SymbolEvidenceStructural,
				}})
			}
		}
		return
	}
	value := tokenRangeText(p.tokens, first, last)
	if value != "" {
		p.addDependency(StructuralDependencyImport, value, p.tokens[first].StartOffset, p.tokens[last-1].EndOffset)
	}
}

func (p *braceLanguageParser) collectDImports(start, end, depth int) {
	// Only module names before the first selective ':' are dependencies.
	// A comma before that ':' separates modules; commas after it separate members.
	selector := end
	for i := start + 1; i < end; i++ {
		if p.tokens[i].Nesting == depth && p.tokens[i].Text == ":" {
			selector = i
			break
		}
	}
	invalid := false
	for first := start + 1; first < selector; {
		last := selector
		for i := first; i < selector; i++ {
			if p.tokens[i].Nesting == depth && p.tokens[i].Text == "," {
				last = i
				break
			}
		}
		if !p.collectDImportModule(first, last) {
			invalid = true
		}
		first = last + 1
	}
	if selector == start+1 || selector < end && !p.dImportSelectorsSupported(selector+1, end, depth) {
		invalid = true
	}
	if invalid {
		p.builder.MarkIncomplete()
		span := OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[end-1].EndOffset}
		_ = p.builder.AddDiagnostic(DiagnosticSpec{
			Code:     "d-unsupported-import-form",
			Message:  "D import has unsupported module or selector syntax; inspect the authored directive",
			Severity: DiagnosticWarning, Range: &span, AffectsCoverage: true,
		})
	}
}

func (p *braceLanguageParser) collectDImportModule(first, end int) bool {
	next := func(i int) int {
		for i < end && p.tokens[i].Kind == TokenNewline {
			i++
		}
		return i
	}
	i := next(first)
	if i == end || p.tokens[i].Kind != TokenIdentifier {
		return false
	}
	alias := ""
	if at := next(i + 1); at < end && p.tokens[at].Text == "=" {
		alias = p.tokens[i].Text
		i = next(at + 1)
	}
	moduleStart := i
	wantIdentifier := true
	lastIdentifier := -1
	for i = next(i); i < end; i = next(i + 1) {
		token := p.tokens[i]
		if wantIdentifier {
			if token.Kind != TokenIdentifier {
				return false
			}
			lastIdentifier = i
		} else if token.Text != "." {
			return false
		}
		wantIdentifier = !wantIdentifier
	}
	if wantIdentifier || lastIdentifier < 0 {
		return false
	}
	value := tokenRangeText(p.tokens, moduleStart, lastIdentifier+1)
	rangeValue, err := p.document.RangeFromUTF8Offsets(p.tokens[moduleStart].StartOffset, p.tokens[lastIdentifier].EndOffset)
	if err != nil || value == "" {
		return false
	}
	p.dependencies = appendUniqueDependencies(p.dependencies, []StructuralDependency{{
		Kind: StructuralDependencyImport, Value: value, Alias: alias,
		Range: rangeValue, Evidence: SymbolEvidenceStructural,
	}})
	return true
}

func (p *braceLanguageParser) dImportSelectorsSupported(first, end, depth int) bool {
	// Member selectors are not modeled as independent module dependencies.
	// Only simple identifiers and local aliases are accepted as understood syntax.
	for first < end {
		if p.tokens[first].Kind == TokenNewline {
			first++
			continue
		}
		if p.tokens[first].Nesting != depth || p.tokens[first].Kind != TokenIdentifier {
			return false
		}
		first++
		for first < end && p.tokens[first].Kind == TokenNewline {
			first++
		}
		if first < end && p.tokens[first].Text == "=" {
			first++
			for first < end && p.tokens[first].Kind == TokenNewline {
				first++
			}
			if first == end || p.tokens[first].Nesting != depth || p.tokens[first].Kind != TokenIdentifier {
				return false
			}
			first++
		}
		for first < end && p.tokens[first].Kind == TokenNewline {
			first++
		}
		if first == end {
			return true
		}
		if p.tokens[first].Nesting != depth || p.tokens[first].Text != "," {
			return false
		}
		first++
	}
	return false
}

func quotedStringValue(value string) string {
	if len(value) < 2 {
		return ""
	}
	quote := value[0]
	if (quote != '\'' && quote != '"') || value[len(value)-1] != quote {
		return ""
	}
	return value[1 : len(value)-1]
}

func (p *braceLanguageParser) parseScope(start, end int, parent *SymbolParent, members bool, owner string) {
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
		// Groovy package and import headers are already indexed. They do not
		// declare callables: scanning through them can mislabel a later
		// parameterized annotation and swallow its entire class.
		if p.policy.lineTerminatedModule && p.tokens[i].Nesting == 0 {
			word := p.lower(p.tokens[i].Text)
			_, imported := p.policy.importKeywords[word]
			if p.eq(word, p.policy.moduleKeyword) || imported {
				lineEnd := p.lineEnd(i)
				if semi := p.findSameDepth(i+1, lineEnd, ";", 0); semi >= 0 {
					i = semi + 1
				} else {
					i = lineEnd
				}
				continue
			}
		}
		if p.policy.language == "d" {
			if next, consumed := p.skipDStructuralPrelude(i, end, parent, members, owner); consumed {
				i = next
				continue
			}
		}
		// D unittest bodies contain executable statements, not declarations.
		if p.policy.language == "d" && p.tokens[i].Text == "unittest" {
			open := nextStructuralToken(p.tokens, i+1, end)
			if open < end && p.tokens[open].Text == "{" {
				if close := p.pairs[open]; close > open && close < end {
					i = close + 1
					continue
				}
			}
			p.builder.MarkIncomplete()
			i++
			continue
		}
		// D static assertions are compile-time checks, not callable definitions.
		if p.policy.language == "d" && p.tokens[i].Text == "static" {
			assertion := nextStructuralToken(p.tokens, i+1, end)
			if assertion < end && p.tokens[assertion].Text == "assert" {
				open := nextStructuralToken(p.tokens, assertion+1, end)
				if open < end && p.tokens[open].Text == "(" {
					if close := p.pairs[open]; close > open && close < end {
						semi := nextStructuralToken(p.tokens, close+1, end)
						if semi < end && p.tokens[semi].Text == ";" {
							i = semi + 1
							continue
						}
					}
				}
				p.builder.MarkIncomplete()
				i = assertion + 1
				continue
			}
		}
		// Solidity imports may contain named-brace lists. Consume the entire
		// directive before parsing declarations, or its braces can hide the
		// following contract and orphan all of that contract's members.
		if p.policy.language == "solidity" && p.tokens[i].Text == "import" {
			if terminator := p.findSameDepth(i+1, end, ";", p.tokens[i].Nesting); terminator >= 0 {
				i = terminator + 1
				continue
			}
			p.builder.MarkIncomplete()
			return
		}
		if p.policy.supportsAnnotations && p.tokens[i].Text == "@" {
			i = p.skipBraceAnnotation(i, end)
			continue
		}
		if keyword, ok := p.typeKeyword(i, end); ok {
			i = p.parseType(i, keyword, end, parent, members, owner)
			continue
		}
		if native, ok := p.policy.explicitFuncs[p.lower(p.tokens[i].Text)]; ok {
			i = p.parseExplicit(i, end, parent, members, owner, native)
			continue
		}
		if p.policy.language == "dart" {
			if next, consumed := p.parseDartGetter(i, end, parent); consumed {
				i = next
				continue
			}
		}
		if p.policy.language == "groovy" && members {
			if next, consumed := p.parseGroovyContainerField(i, end, parent); consumed {
				i = next
				continue
			}
			if next, consumed := p.parseGroovySimpleField(i, end, parent); consumed {
				i = next
				continue
			}
		}
		if next, consumed := p.parseCStyle(i, end, parent, members, owner); consumed {
			i = next
			continue
		}
		i++
	}
}

// parseDartGetter recognizes explicit get declarations before the generic
// callable parser can mistake invocations inside a getter body for methods.
func (p *braceLanguageParser) parseDartGetter(start, end int, parent *SymbolParent) (int, bool) {
	depth := p.tokens[start].Nesting
	limit := min(end, start+20)
	for i := start; i < limit; i++ {
		token := p.tokens[i]
		if token.Nesting < depth || token.Kind == TokenEOF {
			break
		}
		if token.Nesting == depth && token.Text == "get" {
			name := nextStructuralToken(p.tokens, i+1, end)
			if name >= end || p.tokens[name].Kind != TokenIdentifier {
				return 0, false
			}
			bodyStart := nextStructuralToken(p.tokens, name+1, end)
			if bodyStart >= end {
				return 0, false
			}
			var body *OffsetRange
			var signatureEnd int
			var declarationEnd int
			var next int
			switch p.tokens[bodyStart].Text {
			case ";":
				// A bodyless external getter is a declaration, not a field.
				if p.tokens[start].Text != "external" || p.tokens[bodyStart].Nesting != depth {
					return 0, false
				}
				signatureEnd = p.tokens[bodyStart].EndOffset
				declarationEnd = signatureEnd
				next = bodyStart + 1
			case "{":
				if p.tokens[bodyStart].Nesting != depth+1 {
					return 0, false
				}
				close := p.pairs[bodyStart]
				if close <= bodyStart || close >= end {
					p.builder.MarkIncomplete()
					return end, true
				}
				signatureEnd = p.tokens[bodyStart].StartOffset
				declarationEnd = p.tokens[close].EndOffset
				value := OffsetRange{Start: p.tokens[bodyStart].StartOffset, End: declarationEnd}
				body = &value
				next = close + 1
			case "=>":
				if p.tokens[bodyStart].Nesting != depth {
					return 0, false
				}
				semi := p.findSameDepth(bodyStart+1, end, ";", depth)
				if semi < 0 {
					p.builder.MarkIncomplete()
					return end, true
				}
				signatureEnd = p.tokens[bodyStart].StartOffset
				declarationEnd = p.tokens[semi].EndOffset
				value := OffsetRange{Start: p.tokens[bodyStart].StartOffset, End: declarationEnd}
				body = &value
				next = semi + 1
			default:
				return 0, false
			}
			mods := p.collectModifiers(start, i)
			p.add(SymbolSpec{
				Kind: SymbolKindProperty, NativeKind: "getter", Name: p.tokens[name].Text, Parent: parent,
				Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: declarationEnd},
				NameRange:   OffsetRange{Start: p.tokens[name].StartOffset, End: p.tokens[name].EndOffset},
				Signature:   &OffsetRange{Start: p.tokens[start].StartOffset, End: signatureEnd},
				Body:        body, Visibility: braceLanguageVisibility(mods), Modifiers: mods, Evidence: SymbolEvidenceStructural,
			})
			return next, true
		}
		if token.Nesting == depth && (token.Text == ";" || token.Text == "(" || token.Text == "=" || token.Text == "=>") {
			break
		}
		if token.Nesting == depth+1 && token.Text == "{" {
			break
		}
	}
	return 0, false
}

// parseGroovyContainerField skips balanced closure/map/list initializers as
// class-field values. Their internal calls and nested closures are not methods,
// and the next class declaration must remain visible to the symbol index.
func (p *braceLanguageParser) parseGroovyContainerField(start, end int, parent *SymbolParent) (int, bool) {
	depth := p.tokens[start].Nesting
	lineEnd := min(p.lineEnd(start), end)
	for i := start + 1; i < lineEnd; i++ {
		if p.tokens[i].Nesting != depth || p.tokens[i].Text != "=" {
			continue
		}
		open := nextStructuralToken(p.tokens, i+1, end)
		if open >= end || (p.tokens[open].Text != "{" && p.tokens[open].Text != "[") ||
			p.tokens[open].Nesting != depth+1 {
			return 0, false
		}
		close := p.pairs[open]
		if close <= open || close >= end {
			p.builder.MarkIncomplete()
			return open + 1, true
		}
		if _, added := p.parseVariable(start, close, parent, true); !added {
			p.builder.MarkIncomplete()
		}
		return close + 1, true
	}
	return 0, false
}

// parseGroovySimpleField handles semicolon-free class fields without treating
// the next line's method declaration as a continuation. Complex Groovy
// declarations remain outside this deliberately narrow structural subset.
func (p *braceLanguageParser) parseGroovySimpleField(start, end int, parent *SymbolParent) (int, bool) {
	lineEnd := p.lineEnd(start)
	if lineEnd >= end || p.tokens[lineEnd].Kind != TokenNewline {
		return 0, false
	}
	depth := p.tokens[start].Nesting
	lhsEnd := lineEnd
	for i := start; i < lineEnd; i++ {
		if p.tokens[i].Text == ";" && p.tokens[i].Nesting == depth {
			return 0, false
		}
		if p.tokens[i].Text == "=" && p.tokens[i].Nesting == depth {
			lhsEnd = i
			break
		}
	}
	typeStart := start
	for typeStart < lhsEnd {
		if _, modifier := p.policy.modifiers[p.lower(p.tokens[typeStart].Text)]; !modifier {
			break
		}
		typeStart++
	}
	if typeStart >= lhsEnd {
		return 0, false
	}
	first := p.tokens[typeStart]
	if first.Kind != TokenIdentifier && first.Text != "def" {
		switch first.Text {
		case "boolean", "byte", "short", "int", "long", "float", "double", "char":
		default:
			return 0, false
		}
	}
	nameIndex := previousIdentifierToken(p.tokens, lhsEnd-1, typeStart+1)
	if nameIndex <= typeStart || nameIndex != lhsEnd-1 {
		return 0, false
	}
	// Only recognize a typed declaration on the left of the assignment.
	// Nested generic delimiters are types, while calls, member accesses and
	// executable expressions must never become class declarations.
	genericDepth := 0
	for i := typeStart + 1; i < nameIndex; i++ {
		switch p.tokens[i].Text {
		case "<":
			genericDepth++
		case ">", ">>", ">>>":
			closeCount := len(p.tokens[i].Text)
			if closeCount > genericDepth {
				return 0, false
			}
			genericDepth -= closeCount
		case ",":
			if genericDepth == 0 {
				return 0, false
			}
		case "[", "]":
		default:
			if genericDepth == 0 || p.tokens[i].Kind != TokenIdentifier {
				return 0, false
			}
		}
	}
	if genericDepth != 0 {
		return 0, false
	}
	return p.parseVariable(start, lineEnd, parent, true)
}

// skipDStructuralPrelude keeps annotations, imports and visibility groups
// from consuming the declaration that follows. Mixin expansion is not modeled.
func (p *braceLanguageParser) skipDStructuralPrelude(start, end int, parent *SymbolParent, members bool, owner string) (int, bool) {
	token := p.tokens[start].Text
	depth := p.tokens[start].Nesting
	if token == "@" {
		return p.skipBraceAnnotation(start, end), true
	}
	if token == "alias" {
		semi := p.findSameDepth(start+1, end, ";", depth)
		if semi < 0 {
			p.builder.MarkIncomplete()
			return start + 1, true
		}
		name := nextStructuralToken(p.tokens, start+1, semi)
		equals := nextStructuralToken(p.tokens, name+1, semi)
		if name < semi && p.tokens[name].Kind == TokenIdentifier && equals < semi && p.tokens[equals].Text == "=" {
			p.add(SymbolSpec{Kind: SymbolKindAlias, NativeKind: "alias", Name: p.tokens[name].Text, Parent: parent,
				Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[semi].EndOffset},
				NameRange:   OffsetRange{Start: p.tokens[name].StartOffset, End: p.tokens[name].EndOffset},
				Evidence:    SymbolEvidenceStructural})
		} else {
			// Other D alias forms need a separate structural oracle.
			p.builder.MarkIncomplete()
		}
		return semi + 1, true
	}
	if token == "mixin" || token == "import" || token == "module" {
		if semi := p.findSameDepth(start+1, end, ";", depth); semi >= 0 {
			if token == "mixin" {
				p.builder.MarkIncomplete()
			}
			return semi + 1, true
		}
		p.builder.MarkIncomplete()
		return start + 1, true
	}
	switch token {
	case "public", "private", "protected", "package", "export":
		open := nextStructuralToken(p.tokens, start+1, end)
		if open < end && p.tokens[open].Text == "{" && p.tokens[open].Nesting == depth+1 {
			if close := p.pairs[open]; close > open && close < end {
				p.parseScope(open+1, close, parent, members, owner)
				return close + 1, true
			}
			p.builder.MarkIncomplete()
		}
		if open < end && p.tokens[open].Text == ":" {
			return open + 1, true
		}
	}
	return start, false
}

// skipBraceAnnotation consumes metadata without mistaking its arguments for a
// callable declaration. The following declaration remains available to parseScope.
func (p *braceLanguageParser) skipBraceAnnotation(start, end int) int {
	name := nextStructuralToken(p.tokens, start+1, end)
	if p.policy.language == "d" && name < end && p.tokens[name].Text == "(" {
		if close := p.pairs[name]; close > name && close < end {
			return close + 1
		}
		p.builder.MarkIncomplete()
		return start + 1
	}
	if name >= end || p.tokens[name].Kind != TokenIdentifier {
		p.builder.MarkIncomplete()
		return start + 1
	}
	next := name + 1
	for next+1 < end && p.tokens[next].Text == "." && p.tokens[next+1].Kind == TokenIdentifier {
		next += 2
	}
	if next < end && p.tokens[next].Text == "(" {
		if close := p.pairs[next]; close > next && close < end {
			return close + 1
		}
		p.builder.MarkIncomplete()
	}
	return next
}

func (p *braceLanguageParser) typeKeyword(start, end int) (int, bool) {
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

func (p *braceLanguageParser) parseType(start, keyword, end int, parent *SymbolParent, members bool, owner string) int {
	depth := p.tokens[keyword].Nesting
	native := p.lower(p.tokens[keyword].Text)
	if p.policy.language == "d" && (native == "struct" || native == "union") {
		open := nextStructuralToken(p.tokens, keyword+1, end)
		if open < end && p.tokens[open].Text == "{" && p.tokens[open].Nesting == depth+1 {
			close := p.pairs[open]
			if close <= open || close >= end {
				p.builder.MarkIncomplete()
				return end
			}
			p.parseScope(open+1, close, parent, members, owner)
			return close + 1
		}
	}
	nameIndex := nextIdentifierToken(p.tokens, keyword+1, end)
	dartExtensionType := false
	if p.policy.language == "dart" && native == "extension" {
		next := nextStructuralToken(p.tokens, keyword+1, end)
		if next < end && p.tokens[next].Text == "type" {
			dartExtensionType = true
			next = nextStructuralToken(p.tokens, next+1, end)
			if next < end && p.tokens[next].Text == "const" {
				next = nextStructuralToken(p.tokens, next+1, end)
			}
			if next >= end || p.tokens[next].Kind != TokenIdentifier ||
				p.tokens[next].Text == "on" || p.tokens[next].Text == "implements" ||
				p.tokens[next].Text == "type" || p.tokens[next].Text == "const" {
				return p.skipUnsupportedDartExtensionType(keyword, end)
			}
			nameIndex = next
			afterName := nextStructuralToken(p.tokens, nameIndex+1, end)
			if afterName >= end ||
				(p.tokens[afterName].Text != "<" &&
					p.tokens[afterName].Text != "." &&
					p.tokens[afterName].Text != "(") {
				return p.skipUnsupportedDartExtensionType(keyword, end)
			}
		}
	}
	if nameIndex < 0 {
		p.builder.MarkIncomplete()
		return keyword + 1
	}
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
	kind := p.policy.typeKinds[native]
	if dartExtensionType {
		native = "extension-type"
	} else if override := p.policy.typeNative[native]; override != "" {
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
	symbol, added := p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: p.tokens[nameIndex].Text, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: declarationEnd}, NameRange: OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: p.tokens[start].StartOffset, End: signatureEnd}, Body: body, Visibility: braceLanguageVisibility(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural})
	if added && open >= 0 {
		p.collectTypeRelations(symbol.QualifiedName, nameIndex+1, open, depth)
		if kind != SymbolKindEnum {
			child := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
			p.parseScope(open+1, close, child, true, symbol.Name)
		}
	}
	return terminator + 1
}

// An unsupported extension type must not expose its representation or body
// as unrelated top-level declarations. Stop at the nearest complete authored
// block, same-depth semicolon, or physical line boundary.
func (p *braceLanguageParser) skipUnsupportedDartExtensionType(keyword, end int) int {
	token := p.tokens[keyword]
	addDocumentDataHardwareDiagnostic(p.builder, "dart-unsupported-extension-type",
		"Unsupported or incomplete Dart extension type name or representation", token.StartOffset, token.EndOffset)
	for i := keyword + 1; i < end; i++ {
		current := p.tokens[i]
		if current.Kind == TokenNewline && current.Nesting == token.Nesting {
			return i + 1
		}
		if current.Text == ";" && current.Nesting == token.Nesting {
			return i + 1
		}
		if current.Text == "{" && current.Nesting == token.Nesting+1 {
			if close := p.pairs[i]; close > i && close < end {
				return close + 1
			}
			return end
		}
	}
	return end
}

func (p *braceLanguageParser) collectTypeRelations(source string, start, end, depth int) {
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

func (p *braceLanguageParser) addRelationParts(kind, source string, start, end, depth int) {
	for _, part := range splitCommaTokenRangeAt(p.tokens, start, end, depth) {
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

func (p *braceLanguageParser) parseExplicit(start, end int, parent *SymbolParent, members bool, owner, native string) int {
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
	case "error":
		// Solidity custom errors define a named revert type, not a callable method.
		kind = SymbolKindType
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

// parseCStyle reports whether it consumed a statement; consumption does not
// imply that a declaration was emitted. Unknown blocks remain traversable.
func (p *braceLanguageParser) parseCStyle(start, end int, parent *SymbolParent, members bool, owner string) (int, bool) {
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
	// Dart initializers can contain calls or function types; neither declares a callable.
	if p.policy.language == "dart" && p.tokens[terminator].Text == ";" {
		for i := start; i < terminator; i++ {
			if p.tokens[i].Text == "=" && p.tokens[i].Nesting == depth {
				return p.parseVariable(start, terminator, parent, members)
			}
		}
	}
	paren := -1
	assignment := false
	for i := start; i < terminator; i++ {
		if p.tokens[i].Text == "=" && p.tokens[i].Nesting == depth {
			assignment = true
		}
		if p.tokens[i].Text == "(" && p.tokens[i].Nesting == depth+1 {
			paren = i
			break
		}
	}
	if paren >= 0 {
		// Apex and Solidity allow field initializers containing calls;
		// those calls are not method declarations.
		if assignment && (p.policy.language == "apex" || p.policy.language == "solidity") {
			return p.parseVariable(start, terminator, parent, members)
		}
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
				// Dart named constructors use Owner.name(...), not a method name.
				if members && owner != "" && (p.eq(name, owner) ||
					(p.policy.language == "dart" && nameIndex >= start+2 &&
						p.tokens[nameIndex-1].Text == "." && p.eq(p.tokens[nameIndex-2].Text, owner))) {
					kind = SymbolKindConstructor
					native = "constructor"
				}
				if nameIndex > start && p.tokens[nameIndex-1].Text == "~" && (p.eq(name, owner) || p.eq(name, "this")) {
					kind = SymbolKindDestructor
					native = "destructor"
				}
				declarationEnd := p.tokens[terminator].EndOffset
				next := terminator + 1
				var body *OffsetRange
				if p.tokens[terminator].Text == "{" {
					bodyOpen := terminator
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
				modifiers := p.collectModifiers(start, paren)
				p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: name, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: declarationEnd}, NameRange: nameRange, Signature: &OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[terminator].StartOffset}, Body: body, Visibility: braceLanguageVisibility(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural, Disambiguator: p.parameterDisambiguator(paren)})
				return next, true
			}
		}
	}
	if p.tokens[terminator].Text == "{" {
		if p.policy.language == "apex" && members {
			if next, ok := p.parseApexAutoProperty(start, terminator, parent); ok {
				return next, true
			}
		}
		if close := p.pairs[terminator]; close > terminator {
			return close + 1, false
		}
		return terminator + 1, false
	}
	return p.parseVariable(start, terminator, parent, members)
}

// parseApexAutoProperty recognizes only property bodies consisting of
// semicolon-terminated get/set accessors. Arbitrary accessor implementations
// are not assumed to be simple fields or converted into phantom declarations.
func (p *braceLanguageParser) parseApexAutoProperty(start, open int, parent *SymbolParent) (int, bool) {
	close := p.pairs[open]
	if close <= open || parent == nil {
		return 0, false
	}
	nameIndex := previousIdentifierToken(p.tokens, open-1, start)
	if nameIndex <= start {
		return 0, false
	}
	accessors := 0
	for i := nextStructuralToken(p.tokens, open+1, close); i < close; {
		if p.eq(p.tokens[i].Text, "private") || p.eq(p.tokens[i].Text, "protected") ||
			p.eq(p.tokens[i].Text, "public") || p.eq(p.tokens[i].Text, "global") {
			i = nextStructuralToken(p.tokens, i+1, close)
			if i >= close {
				return 0, false
			}
		}
		if !p.eq(p.tokens[i].Text, "get") && !p.eq(p.tokens[i].Text, "set") {
			return 0, false
		}
		semi := nextStructuralToken(p.tokens, i+1, close)
		if semi >= close || p.tokens[semi].Text != ";" {
			return 0, false
		}
		accessors++
		i = nextStructuralToken(p.tokens, semi+1, close)
	}
	if accessors == 0 {
		return 0, false
	}
	mods := p.collectModifiers(start, nameIndex)
	p.add(SymbolSpec{Kind: SymbolKindProperty, NativeKind: "property",
		Name: p.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[close].EndOffset},
		NameRange:   OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset},
		Visibility:  braceLanguageVisibility(mods), Modifiers: mods, Evidence: SymbolEvidenceStructural})
	return close + 1, true
}

func (p *braceLanguageParser) parseVariable(start, semicolon int, parent *SymbolParent, members bool) (int, bool) {
	if start >= semicolon {
		return semicolon + 1, false
	}
	first := p.lower(p.tokens[start].Text)
	if first == "module" || first == "import" || first == "export" || first == "part" || first == "pragma" || first == "using" || first == "return" ||
		(first == "package" && p.policy.language == "groovy") {
		return semicolon + 1, true
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
	p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: p.tokens[nameIndex].Text, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[semicolon].EndOffset}, NameRange: OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset}, Visibility: braceLanguageVisibility(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural})
	return semicolon + 1, true
}

func (p *braceLanguageParser) collectModifiers(start, end int) []string {
	var result []string
	for i := start; i < end; i++ {
		text := p.lower(p.tokens[i].Text)
		if _, ok := p.policy.modifiers[text]; ok || text == "public" || text == "private" || text == "protected" || text == "static" || text == "external" || text == "internal" || text == "global" || text == "override" || text == "virtual" {
			result = append(result, text)
		}
	}
	return result
}

func braceLanguageVisibility(modifiers []string) Visibility {
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

func (p *braceLanguageParser) functionTerminator(start, end, depth int) (terminator, bodyOpen int) {
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

func (p *braceLanguageParser) parameterDisambiguator(paren int) string {
	if paren < 0 {
		return ""
	}
	if close := p.pairs[paren]; close > paren {
		return tokenRangeText(p.tokens, paren, close+1)
	}
	return ""
}

func (p *braceLanguageParser) findSameDepth(start, end int, text string, depth int) int {
	for i := start; i < end; i++ {
		if p.tokens[i].Text == text && p.tokens[i].Nesting == depth {
			return i
		}
	}
	return -1
}
func (p *braceLanguageParser) lineEnd(start int) int {
	for i := start; i < len(p.tokens); i++ {
		if p.tokens[i].Kind == TokenNewline || p.tokens[i].Kind == TokenEOF {
			return i
		}
	}
	return len(p.tokens)
}
func (p *braceLanguageParser) addDependency(kind StructuralDependencyKind, value string, start, end int) {
	rangeValue, err := p.document.RangeFromUTF8Offsets(start, end)
	if err == nil {
		p.dependencies = appendUniqueDependencies(p.dependencies, []StructuralDependency{{Kind: kind, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural}})
	}
}
func (p *braceLanguageParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
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
