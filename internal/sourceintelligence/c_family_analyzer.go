package sourceintelligence

import (
	"context"
	"sort"
	"strconv"
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

var (
	cFamilyCScannerProfile   = CScannerProfile()
	cFamilyCPPScannerProfile = CPPScannerProfile()
)

func cFamilyScannerProfile(cpp bool) ScannerProfile {
	if cpp {
		return cFamilyCPPScannerProfile
	}
	return cFamilyCScannerProfile
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
	profile := cFamilyScannerProfile(cpp)
	lexicalText := document.Text
	if cpp {
		masked, _, err := maskCPPRawStrings(ctx, document.Text)
		if err != nil {
			return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_cpp_source", document.Path, err)
		}
		lexicalText = masked
	}
	lexicalText = maskCFamilyInitialBOM(lexicalText)
	variantBaseText := maskCFamilyInitialBOM(maskCFamilyDirectiveBlockComments(document.Text, lexicalText))
	planText := maskCFamilyDirectiveBlockComments(lexicalText, lexicalText)
	planDocument := document
	if planText != document.Text {
		clone := *document
		clone.Text = planText
		clone.lineStarts = buildLineStarts(planText)
		planDocument = &clone
	}
	planProfile := profile
	planProfile.DisableDelimiterTracking = true
	planScan, err := ScanSource(ctx, planDocument, planProfile, ScannerLimits{MaxTokens: scannerTokenBudget(planDocument.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: max(1, options.MaxNesting)})
	if err != nil {
		return AnalyzerResult{}, err
	}
	plan := cFamilyConditionalPlan(planScan.Tokens)
	if plan.issue != nil || len(plan.groups) == 0 {
		return analyzeCFamilySingle(ctx, document, options, cpp)
	}
	selections, ok := conditionalSelections(plan.groups)
	if !ok {
		result, singleErr := analyzeCFamilySingle(ctx, document, options, cpp)
		if singleErr != nil {
			return AnalyzerResult{}, singleErr
		}
		result.Analysis.CoverageComplete = false
		if len(result.Analysis.Diagnostics) < options.Limits.MaxDiagnostics {
			language := "c"
			if cpp {
				language = "cpp"
			}
			result.Analysis.Diagnostics = append(result.Analysis.Diagnostics, AnalysisDiagnostic{Code: language + "-conditional-variant-limit", Message: "conditional preprocessing exceeds the bounded structural variant limit", Severity: DiagnosticWarning})
		} else {
			result.Analysis.DiagnosticsTruncated = true
		}
		return result, nil
	}
	variants := make([]AnalyzerResult, 0, len(selections))
	for _, selection := range selections {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_c_family_source", document.Path, err)
		}
		masked := maskConditionalVariant(variantBaseText, plan, selection)
		clone := *document
		clone.Text = masked
		clone.lineStarts = buildLineStarts(masked)
		variant, variantErr := analyzeCFamilySingle(ctx, &clone, options, cpp)
		if variantErr != nil {
			return AnalyzerResult{}, variantErr
		}
		variants = append(variants, variant)
	}
	language := "c"
	if cpp {
		language = "cpp"
	}
	return mergeCFamilyConditionalVariants(language, options, variants), nil
}

func analyzeCFamilySingle(ctx context.Context, document *SourceDocument, options AnalyzeOptions, cpp bool) (AnalyzerResult, error) {
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
	profile := cFamilyScannerProfile(cpp)
	lexicalText := document.Text
	var rawDiagnostics []ScannerDiagnostic
	if cpp {
		language = "cpp"
		analyzer = AnalyzerCPP
		masked, diagnostics, err := maskCPPRawStrings(ctx, document.Text)
		if err != nil {
			return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_cpp_source", document.Path, err)
		}
		rawDiagnostics = diagnostics
		lexicalText = masked
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scanText := maskCFamilyInitialBOM(maskCFamilyDirectiveBlockComments(lexicalText, lexicalText))
	projectedText, projectionErr := maskCFamilyStructuralMacroEffects(ctx, document, scanText, profile, maxNesting)
	if projectionErr != nil {
		return AnalyzerResult{}, projectionErr
	}
	scanText = projectedText
	scanDocument := document
	if scanText != document.Text {
		clone := *document
		clone.Text = scanText
		clone.lineStarts = buildLineStarts(scanText)
		scanDocument = &clone
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: language, Analyzer: string(analyzer), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
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
	directives := collectCFamilyDirectives(document, scan.Tokens)
	if directives.hasConditionals {
		_ = builder.AddDiagnostic(DiagnosticSpec{
			Code: language + "-conditional-preprocessor", Message: "conditional preprocessing is not evaluated; structural analysis includes balanced source branches without selecting a compiled macro state",
			Severity: DiagnosticWarning, AffectsCoverage: false,
		})
	}
	if directives.issue != nil {
		value := OffsetRange{Start: directives.issue.startOffset, End: directives.issue.endOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{
			Code: language + "-malformed-conditional-preprocessor", Message: directives.issue.message,
			Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true,
		})
	}
	parser := &cFamilyParser{
		ctx: ctx, document: document, tokens: scan.Tokens, pairs: PairDelimiterTokens(scan.Tokens, nil), builder: builder,
		cpp: cpp, dependencies: directives.dependencies, types: make(map[string]SymbolParent),
	}
	parser.parseScope(0, len(scan.Tokens), nil, false, "")
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_c_family_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.takeResult(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

// A UTF-8 BOM is not a C/C++ token. Replace its bytes, rather than removing
// them, so source ranges remain aligned with the original document.
func maskCFamilyInitialBOM(text string) string {
	if strings.HasPrefix(text, "\ufeff") {
		return "   " + text[len("\ufeff"):]
	}
	return text
}

func maskCFamilyDirectiveBlockComments(text, lexicalText string) string {
	if len(lexicalText) != len(text) {
		lexicalText = text
	}
	var masked []byte
	mask := func(start, end int) {
		if start < 0 || end <= start || start >= len(text) {
			return
		}
		end = min(end, len(text))
		if masked == nil {
			masked = []byte(text)
		}
		for index := start; index < end; index++ {
			if masked[index] != '\r' && masked[index] != '\n' {
				masked[index] = ' '
			}
		}
	}
	maskCommentTail := func(start, end int) {
		if end > start && end <= len(lexicalText) && lexicalText[end-1] == '\\' {
			end--
		}
		mask(start, end)
	}
	inDirectiveComment := false
	directiveContinuation := false
	for start := 0; start < len(lexicalText); {
		contentEnd := start
		for contentEnd < len(lexicalText) && lexicalText[contentEnd] != '\r' && lexicalText[contentEnd] != '\n' {
			contentEnd++
		}
		line := lexicalText[start:contentEnd]
		if inDirectiveComment {
			if closeOffset := strings.Index(line, "*/"); closeOffset >= 0 {
				mask(start, start+closeOffset+2)
				inDirectiveComment = false
			} else {
				maskCommentTail(start, contentEnd)
			}
		} else {
			trimmed := strings.TrimLeft(line, " \t")
			directiveLine := directiveContinuation || strings.HasPrefix(trimmed, "#")
			directiveContinuation = false
			if directiveLine {
				openOffset := cFamilyDirectiveBlockCommentStart(line)
				if openOffset >= 0 {
					if closeRelative := strings.Index(line[openOffset+2:], "*/"); closeRelative >= 0 {
						mask(start+openOffset, start+openOffset+2+closeRelative+2)
					} else {
						maskCommentTail(start+openOffset, contentEnd)
						inDirectiveComment = true
					}
				}
				if !inDirectiveComment {
					directiveContinuation = strings.HasSuffix(strings.TrimSpace(line), "\\")
				}
			}
		}
		if contentEnd >= len(lexicalText) {
			break
		}
		if lexicalText[contentEnd] == '\r' && contentEnd+1 < len(lexicalText) && lexicalText[contentEnd+1] == '\n' {
			start = contentEnd + 2
		} else {
			start = contentEnd + 1
		}
	}
	if masked == nil {
		return text
	}
	return string(masked)
}

func cFamilyDirectiveBlockCommentStart(line string) int {
	var quote byte
	for index := 0; index < len(line); index++ {
		current := line[index]
		if quote != 0 {
			if current == '\\' && index+1 < len(line) {
				index++
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}
		if current == '\'' || current == '"' {
			quote = current
			continue
		}
		if current == '/' && index+1 < len(line) {
			switch line[index+1] {
			case '/':
				return -1
			case '*':
				return index
			}
		}
	}
	return -1
}

func cFamilyNextMacroToken(tokens []Token, start int) int {
	for start < len(tokens) && tokens[start].Kind == TokenNewline {
		start++
	}
	return start
}

type cFamilyMacroStructuralEffect struct {
	functionLike  bool
	prefixClosers []string
	suffixOpeners []string
}

type cFamilyProjectedDelimiter struct {
	text    string
	virtual bool
	opener  Token
}

func maskCFamilyStructuralMacroEffects(ctx context.Context, document *SourceDocument, text string, profile ScannerProfile, maxNesting int) (string, error) {
	if document == nil || !strings.Contains(text, "#") {
		return text, nil
	}
	probe := *document
	probe.Text = text
	probe.lineStarts = lineStartsForDerivedText(document, text)
	probeProfile := profile
	probeProfile.DisableDelimiterTracking = true
	scan, err := ScanSource(ctx, &probe, probeProfile, ScannerLimits{MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: max(1, maxNesting)})
	if err != nil {
		return "", err
	}
	if !scan.Complete {
		return text, nil
	}

	effects := make(map[string]cFamilyMacroStructuralEffect)
	stack := make([]cFamilyProjectedDelimiter, 0, 32)
	var masked []byte
	maskToken := func(token Token) {
		if masked == nil {
			masked = []byte(text)
		}
		for offset := token.StartOffset; offset < token.EndOffset && offset < len(masked); offset++ {
			if masked[offset] != '\r' && masked[offset] != '\n' {
				masked[offset] = ' '
			}
		}
	}
	applyEffect := func(effect cFamilyMacroStructuralEffect) bool {
		for _, close := range effect.prefixClosers {
			if len(stack) == 0 {
				return false
			}
			top := stack[len(stack)-1]
			if !cFamilyDelimiterMatches(top.text, close) {
				return false
			}
			if !top.virtual {
				maskToken(top.opener)
			}
			stack = stack[:len(stack)-1]
		}
		for _, open := range effect.suffixOpeners {
			if len(stack) >= 64 {
				return false
			}
			stack = append(stack, cFamilyProjectedDelimiter{text: open, virtual: true})
		}
		return true
	}

	for index := 0; index < len(scan.Tokens); index++ {
		if err := ctx.Err(); err != nil {
			return "", operation.Wrap(operation.KindCancelled, "project_c_family_macro_structure", document.Path, err)
		}
		token := scan.Tokens[index]
		if token.Kind == TokenDirective {
			keyword, rest := cFamilyDirectiveKeywordAndRest(token.Text)
			switch keyword {
			case "#define":
				name, effect, ok := cFamilyMacroStructuralDefinition(rest, effects)
				if name != "" {
					if ok && (len(effect.prefixClosers) > 0 || len(effect.suffixOpeners) > 0) {
						effects[name] = effect
					} else {
						delete(effects, name)
					}
				}
			case "#undef":
				if name := cFamilyDirectiveMacroName(rest); name != "" {
					delete(effects, name)
				}
			}
			continue
		}

		if (token.Kind == TokenIdentifier || token.Kind == TokenKeyword) && token.Text != "" {
			if effect, ok := effects[token.Text]; ok {
				if effect.functionLike {
					if _, ok := cFamilyStructuralMacroInvocationEnd(scan.Tokens, index); !ok {
						continue
					}
				}
				if !applyEffect(effect) {
					return text, nil
				}
			}
		}

		switch token.Text {
		case "(", "[", "{":
			stack = append(stack, cFamilyProjectedDelimiter{text: token.Text, opener: token})
		case ")", "]", "}":
			if len(stack) == 0 {
				return text, nil
			}
			top := stack[len(stack)-1]
			if !cFamilyDelimiterMatches(top.text, token.Text) {
				return text, nil
			}
			stack = stack[:len(stack)-1]
			if top.virtual {
				maskToken(token)
			}
		}
	}
	for _, delimiter := range stack {
		if delimiter.virtual {
			return text, nil
		}
	}
	if masked == nil {
		return text, nil
	}
	return string(masked), nil
}

func cFamilyMacroStructuralDefinition(rest string, effects map[string]cFamilyMacroStructuralEffect) (string, cFamilyMacroStructuralEffect, bool) {
	name := cFamilyDirectiveMacroName(rest)
	if name == "" {
		return "", cFamilyMacroStructuralEffect{}, false
	}
	effect := cFamilyMacroStructuralEffect{}
	bodyStart := len(name)
	if bodyStart < len(rest) && rest[bodyStart] == '(' {
		close := strings.IndexByte(rest[bodyStart+1:], ')')
		if close < 0 {
			return name, effect, false
		}
		close += bodyStart + 1
		if !cFamilyStructuralMacroParameters(rest[bodyStart+1 : close]) {
			return name, effect, false
		}
		effect.functionLike = true
		bodyStart = close + 1
	}
	prefix, suffix, ok := cFamilyReduceMacroDelimiterEffect(rest[bodyStart:], effects)
	if !ok {
		return name, effect, false
	}
	effect.prefixClosers = prefix
	effect.suffixOpeners = suffix
	return name, effect, true
}

func cFamilyStructuralMacroParameters(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" || part == "..." || cFamilyDirectiveMacroName(part) != part {
			return false
		}
	}
	return true
}

type cFamilyMacroDelimiterReducer struct {
	prefix  []string
	stack   []string
	effects map[string]cFamilyMacroStructuralEffect
}

func newCFamilyMacroDelimiterReducer(effects map[string]cFamilyMacroStructuralEffect) cFamilyMacroDelimiterReducer {
	return cFamilyMacroDelimiterReducer{
		prefix:  make([]string, 0, 4),
		stack:   make([]string, 0, 8),
		effects: effects,
	}
}

func (reducer *cFamilyMacroDelimiterReducer) applyEffect(effect cFamilyMacroStructuralEffect) bool {
	for _, close := range effect.prefixClosers {
		if len(reducer.stack) == 0 {
			reducer.prefix = append(reducer.prefix, close)
			continue
		}
		if !cFamilyDelimiterMatches(reducer.stack[len(reducer.stack)-1], close) {
			return false
		}
		reducer.stack = reducer.stack[:len(reducer.stack)-1]
	}
	for _, open := range effect.suffixOpeners {
		if len(reducer.stack) >= 64 {
			return false
		}
		reducer.stack = append(reducer.stack, open)
	}
	return true
}

func (reducer *cFamilyMacroDelimiterReducer) consumeIdentifier(body string, index int) (int, bool) {
	end := index + 1
	for end < len(body) && cFamilyMacroIdentifierContinue(body[end]) {
		end++
	}
	effect, ok := reducer.effects[body[index:end]]
	if !ok {
		return end - 1, true
	}
	if effect.functionLike {
		cursor := end
		for cursor < len(body) && (body[cursor] == ' ' || body[cursor] == '\t') {
			cursor++
		}
		return end - 1, cursor >= len(body) || body[cursor] != '('
	}
	return end - 1, reducer.applyEffect(effect)
}

func (reducer *cFamilyMacroDelimiterReducer) consumeDelimiter(current byte) bool {
	switch current {
	case '(', '[', '{':
		if len(reducer.stack) >= 64 {
			return false
		}
		reducer.stack = append(reducer.stack, string(current))
	case ')', ']', '}':
		close := string(current)
		if len(reducer.stack) == 0 {
			reducer.prefix = append(reducer.prefix, close)
			return true
		}
		if !cFamilyDelimiterMatches(reducer.stack[len(reducer.stack)-1], close) {
			return false
		}
		reducer.stack = reducer.stack[:len(reducer.stack)-1]
	}
	return true
}

func (reducer *cFamilyMacroDelimiterReducer) result() ([]string, []string, bool) {
	if len(reducer.prefix)+len(reducer.stack) > 64 {
		return nil, nil, false
	}
	return reducer.prefix, append([]string(nil), reducer.stack...), true
}

func cFamilyReduceMacroDelimiterEffect(body string, effects map[string]cFamilyMacroStructuralEffect) ([]string, []string, bool) {
	reducer := newCFamilyMacroDelimiterReducer(effects)
	var quote byte
	lineComment, blockComment := false, false
	for index := 0; index < len(body); index++ {
		current := body[index]
		if current == '\\' && index+1 < len(body) {
			if body[index+1] == '\n' {
				index++
				continue
			}
			if body[index+1] == '\r' && index+2 < len(body) && body[index+2] == '\n' {
				index += 2
				continue
			}
		}
		if lineComment {
			if current == '\r' || current == '\n' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			if current == '*' && index+1 < len(body) && body[index+1] == '/' {
				blockComment = false
				index++
			}
			continue
		}
		if quote != 0 {
			if current == '\\' && index+1 < len(body) {
				index++
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}
		if current == '/' && index+1 < len(body) {
			switch body[index+1] {
			case '/':
				lineComment = true
				index++
				continue
			case '*':
				blockComment = true
				index++
				continue
			}
		}
		if current == '\'' || current == '"' {
			quote = current
			continue
		}
		if current == '#' {
			return nil, nil, false
		}
		if cFamilyMacroIdentifierStart(current) {
			var ok bool
			index, ok = reducer.consumeIdentifier(body, index)
			if !ok {
				return nil, nil, false
			}
			continue
		}
		if !reducer.consumeDelimiter(current) {
			return nil, nil, false
		}
	}
	if quote != 0 || blockComment {
		return nil, nil, false
	}
	return reducer.result()
}

func cFamilyStructuralMacroInvocationEnd(tokens []Token, start int) (int, bool) {
	open := cFamilyNextMacroToken(tokens, start+1)
	if open >= len(tokens) || tokens[open].Text != "(" {
		return 0, false
	}
	depth := 1
	for index := open + 1; index < len(tokens); index++ {
		token := tokens[index]
		if token.Kind == TokenDirective || token.Kind == TokenEOF || token.Text == "{" || token.Text == "}" {
			return 0, false
		}
		switch token.Text {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return index, true
			}
		}
	}
	return 0, false
}

func cFamilyDelimiterMatches(open, close string) bool {
	switch open {
	case "(":
		return close == ")"
	case "[":
		return close == "]"
	case "{":
		return close == "}"
	default:
		return false
	}
}

func cFamilyMacroIdentifierStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func cFamilyMacroIdentifierContinue(value byte) bool {
	return cFamilyMacroIdentifierStart(value) || value >= '0' && value <= '9'
}

func cFamilyConditionalPlan(tokens []Token) conditionalPlan {
	plan := conditionalPlan{}
	stack := make([]conditionalFrame, 0, 8)
	macroVersions := make(map[string]int)
	includeEpoch := 0
	expressionEpoch := 0
	for _, token := range tokens {
		if token.Kind != TokenDirective {
			continue
		}
		keyword, rest := cFamilyDirectiveKeywordAndRest(token.Text)
		switch keyword {
		case "#define", "#undef":
			if name := cFamilyDirectiveMacroName(rest); name != "" {
				macroVersions[name]++
			}
			expressionEpoch++
		case "#include":
			includeEpoch++
			expressionEpoch++
		case "#if", "#ifdef", "#ifndef":
			parentGroup, parentBranch := -1, -1
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parentGroup, parentBranch = parent.group, parent.branch
			}
			conditionKey, conditionState := cFamilyConditionalState(keyword, rest, macroVersions, includeEpoch, expressionEpoch)
			group := conditionalGroup{parentGroup: parentGroup, parentBranch: parentBranch, branches: []conditionalBranch{{start: token.EndOffset}}, conditionKey: conditionKey}
			if conditionKey != "" {
				group.conditionState = []int8{conditionState}
			}
			plan.groups = append(plan.groups, group)
			stack = append(stack, conditionalFrame{group: len(plan.groups) - 1})
			plan.directives = append(plan.directives, OffsetRange{Start: token.StartOffset, End: token.EndOffset})
		case "#elif", "#else":
			plan.directives = append(plan.directives, OffsetRange{Start: token.StartOffset, End: token.EndOffset})
			if len(stack) == 0 {
				plan.issue = &OffsetRange{Start: token.StartOffset, End: token.EndOffset}
				plan.message = "conditional branch directive has no matching opener"
				return plan
			}
			frame := &stack[len(stack)-1]
			if frame.seenElse {
				plan.issue = &OffsetRange{Start: token.StartOffset, End: token.EndOffset}
				plan.message = "conditional branch appears after #else"
				return plan
			}
			group := &plan.groups[frame.group]
			group.branches[frame.branch].end = token.StartOffset
			group.branches = append(group.branches, conditionalBranch{start: token.EndOffset})
			if group.conditionKey != "" && len(group.conditionState) > 0 {
				group.conditionState = append(group.conditionState, -group.conditionState[0])
			}
			frame.branch = len(group.branches) - 1
			if keyword == "#else" {
				frame.seenElse = true
			}
		case "#endif":
			plan.directives = append(plan.directives, OffsetRange{Start: token.StartOffset, End: token.EndOffset})
			if len(stack) == 0 {
				plan.issue = &OffsetRange{Start: token.StartOffset, End: token.EndOffset}
				plan.message = "conditional #endif has no matching opener"
				return plan
			}
			frame := stack[len(stack)-1]
			plan.groups[frame.group].branches[frame.branch].end = token.StartOffset
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) > 0 {
		frame := stack[len(stack)-1]
		group := plan.groups[frame.group]
		plan.issue = &OffsetRange{Start: group.branches[0].start, End: group.branches[0].start}
		plan.message = "conditional block is not terminated by #endif"
		return plan
	}
	cFamilyFinalizeConditionalStates(&plan)
	return plan
}

func cFamilyConditionalState(keyword, rest string, macroVersions map[string]int, includeEpoch, expressionEpoch int) (string, int8) {
	switch keyword {
	case "#ifdef", "#ifndef":
		name := strings.TrimSpace(rest)
		if name == "" || cFamilyDirectiveMacroName(name) != name {
			return "", 0
		}
		key := "macro:" + strconv.Itoa(cFamilyConditionalIncludeEpoch(name, includeEpoch)) + ":" + strconv.Itoa(macroVersions[name]) + ":" + name
		if keyword == "#ifndef" {
			return key, -1
		}
		return key, 1
	case "#if":
		if name, positive := cFamilyExactDefinedCondition(rest); name != "" {
			key := "macro:" + strconv.Itoa(cFamilyConditionalIncludeEpoch(name, includeEpoch)) + ":" + strconv.Itoa(macroVersions[name]) + ":" + name
			if !positive {
				return key, -1
			}
			return key, 1
		}
		expression := strings.Join(strings.Fields(rest), " ")
		if expression == "" {
			return "", 0
		}
		return "expr:" + strconv.Itoa(expressionEpoch) + ":" + expression, 1
	default:
		return "", 0
	}
}

// Only a single exact defined(MACRO) expression has the same state as
// #ifdef MACRO. Composite expressions retain the conservative expression key.
func cFamilyExactDefinedCondition(expression string) (string, bool) {
	expression = strings.TrimSpace(expression)
	positive := true
	if strings.HasPrefix(expression, "!") {
		positive = false
		expression = strings.TrimSpace(expression[1:])
	}
	rest, ok := strings.CutPrefix(expression, "defined")
	if !ok || rest == "" {
		return "", false
	}
	if rest[0] != '(' && rest[0] != ' ' && rest[0] != '\t' {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "(") && strings.HasSuffix(rest, ")") {
		rest = strings.TrimSpace(rest[1 : len(rest)-1])
	}
	if rest == "" || cFamilyDirectiveMacroName(rest) != rest {
		return "", false
	}
	return rest, positive
}

func cFamilyConditionalIncludeEpoch(name string, includeEpoch int) int {
	if name == "__cplusplus" {
		return 0
	}
	return includeEpoch
}

func cFamilyDirectiveMacroName(rest string) string {
	value := strings.TrimSpace(rest)
	if value == "" || value[0] != '_' && (value[0] < 'A' || value[0] > 'Z') && (value[0] < 'a' || value[0] > 'z') {
		return ""
	}
	end := 1
	for end < len(value) {
		current := value[end]
		if current != '_' && (current < 'A' || current > 'Z') && (current < 'a' || current > 'z') && (current < '0' || current > '9') {
			break
		}
		end++
	}
	return value[:end]
}

func cFamilyFinalizeConditionalStates(plan *conditionalPlan) {
	if plan == nil || len(plan.groups) == 0 {
		return
	}
	counts := make(map[string]int)
	for _, group := range plan.groups {
		if group.conditionKey != "" {
			counts[group.conditionKey]++
		}
	}
	for index := range plan.groups {
		group := &plan.groups[index]
		if group.conditionKey == "" || counts[group.conditionKey] < 2 || len(group.branches) != 1 || len(group.conditionState) != 1 {
			continue
		}
		end := group.branches[0].end
		group.branches = append(group.branches, conditionalBranch{start: end, end: end})
		group.conditionState = append(group.conditionState, -group.conditionState[0])
	}
}

func mergeCFamilyConditionalVariants(language string, options AnalyzeOptions, variants []AnalyzerResult) AnalyzerResult {
	merged := AnalyzerResult{Analysis: AnalysisResult{CoverageComplete: true}}
	seenSymbols := make(map[string]struct{})
	seenRelations := make(map[StructuralRelation]struct{})
	for _, variant := range variants {
		if !variant.Analysis.CoverageComplete {
			merged.Analysis.CoverageComplete = false
		}
		if variant.Analysis.Truncated {
			merged.Analysis.Truncated = true
			merged.Analysis.CoverageComplete = false
		}
		if variant.Analysis.DiagnosticsTruncated {
			merged.Analysis.DiagnosticsTruncated = true
			merged.Analysis.CoverageComplete = false
		}
		for _, symbol := range variant.Analysis.Symbols {
			if _, exists := seenSymbols[symbol.ID]; exists {
				continue
			}
			if len(merged.Analysis.Symbols) >= options.Limits.MaxSymbols {
				merged.Analysis.Truncated = true
				merged.Analysis.CoverageComplete = false
				continue
			}
			seenSymbols[symbol.ID] = struct{}{}
			merged.Analysis.Symbols = append(merged.Analysis.Symbols, symbol)
		}
		merged.Dependencies = appendUniqueDependencies(merged.Dependencies, variant.Dependencies)
		for _, relation := range variant.Relations {
			if _, exists := seenRelations[relation]; exists {
				continue
			}
			seenRelations[relation] = struct{}{}
			merged.Relations = append(merged.Relations, relation)
		}
		for _, diagnostic := range variant.Analysis.Diagnostics {
			if len(merged.Analysis.Diagnostics) >= options.Limits.MaxDiagnostics {
				merged.Analysis.DiagnosticsTruncated = true
				break
			}
			merged.Analysis.Diagnostics = append(merged.Analysis.Diagnostics, diagnostic)
		}
	}
	sort.Slice(merged.Analysis.Symbols, func(i, j int) bool {
		left, right := merged.Analysis.Symbols[i], merged.Analysis.Symbols[j]
		if left.declarationOffsets.Start != right.declarationOffsets.Start {
			return left.declarationOffsets.Start < right.declarationOffsets.Start
		}
		if left.declarationOffsets.End != right.declarationOffsets.End {
			return left.declarationOffsets.End < right.declarationOffsets.End
		}
		return left.ID < right.ID
	})
	sort.SliceStable(merged.Dependencies, func(i, j int) bool {
		left, right := merged.Dependencies[i].Range, merged.Dependencies[j].Range
		if left.Start.Line != right.Start.Line {
			return left.Start.Line < right.Start.Line
		}
		if left.Start.Column != right.Start.Column {
			return left.Start.Column < right.Start.Column
		}
		if left.End.Line != right.End.Line {
			return left.End.Line < right.End.Line
		}
		return left.End.Column < right.End.Column
	})
	sort.SliceStable(merged.Relations, func(i, j int) bool {
		left, right := merged.Relations[i].Range, merged.Relations[j].Range
		if left.Start.Line != right.Start.Line {
			return left.Start.Line < right.Start.Line
		}
		if left.Start.Column != right.Start.Column {
			return left.Start.Column < right.Start.Column
		}
		if left.End.Line != right.End.Line {
			return left.End.Line < right.End.Line
		}
		return left.End.Column < right.End.Column
	})
	if len(merged.Analysis.Diagnostics) < options.Limits.MaxDiagnostics {
		merged.Analysis.Diagnostics = append(merged.Analysis.Diagnostics, AnalysisDiagnostic{
			Code: language + "-conditional-preprocessor", Message: "conditional preprocessing is analyzed as a bounded structural union across balanced source branches", Severity: DiagnosticWarning,
		})
	} else {
		merged.Analysis.DiagnosticsTruncated = true
	}
	return merged
}

type cFamilyDirectiveSummary struct {
	dependencies    []StructuralDependency
	hasConditionals bool
	issue           *cFamilyDirectiveIssue
}

type cFamilyDirectiveIssue struct {
	startOffset int
	endOffset   int
	message     string
}

type cFamilyConditionalFrame struct {
	startOffset int
	endOffset   int
	seenElse    bool
}

func collectCFamilyDirectives(document *SourceDocument, tokens []Token) cFamilyDirectiveSummary {
	var summary cFamilyDirectiveSummary
	var stack []cFamilyConditionalFrame
	for _, token := range tokens {
		if token.Kind != TokenDirective {
			continue
		}
		keyword, rest := cFamilyDirectiveKeywordAndRest(token.Text)
		switch keyword {
		case "#if", "#ifdef", "#ifndef":
			summary.hasConditionals = true
			stack = append(stack, cFamilyConditionalFrame{startOffset: token.StartOffset, endOffset: token.EndOffset})
		case "#elif":
			summary.hasConditionals = true
			if len(stack) == 0 {
				summary.setIssue(token, "conditional #elif has no matching opener")
			} else if stack[len(stack)-1].seenElse {
				summary.setIssue(token, "conditional #elif appears after #else")
			}
		case "#else":
			summary.hasConditionals = true
			if len(stack) == 0 {
				summary.setIssue(token, "conditional #else has no matching opener")
			} else if stack[len(stack)-1].seenElse {
				summary.setIssue(token, "conditional block contains more than one #else")
			} else {
				stack[len(stack)-1].seenElse = true
			}
		case "#endif":
			summary.hasConditionals = true
			if len(stack) == 0 {
				summary.setIssue(token, "conditional #endif has no matching opener")
			} else {
				stack = stack[:len(stack)-1]
			}
		case "#include":
			if value := cFamilyIncludeValue(rest); value != "" {
				rangeValue, err := document.RangeFromUTF8Offsets(token.StartOffset, token.EndOffset)
				if err == nil {
					summary.dependencies = append(summary.dependencies, StructuralDependency{
						Kind: StructuralDependencyInclude, Value: value, Range: rangeValue,
						Evidence: SymbolEvidenceStructural, cFamilyAngleInclude: strings.HasPrefix(rest, "<"),
					})
				}
			}
		}
	}
	if summary.issue == nil && len(stack) > 0 {
		frame := stack[len(stack)-1]
		summary.issue = &cFamilyDirectiveIssue{startOffset: frame.startOffset, endOffset: frame.endOffset, message: "conditional block is not terminated by #endif"}
	}
	return summary
}

func (summary *cFamilyDirectiveSummary) setIssue(token Token, message string) {
	if summary.issue != nil {
		return
	}
	summary.issue = &cFamilyDirectiveIssue{startOffset: token.StartOffset, endOffset: token.EndOffset, message: message}
}

func cFamilyDirectiveKeywordAndRest(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || trimmed[0] != '#' {
		return "", ""
	}
	rest := strings.TrimSpace(trimmed[1:])
	end := 0
	for end < len(rest) && (rest[end] == '_' || rest[end] >= 'A' && rest[end] <= 'Z' || rest[end] >= 'a' && rest[end] <= 'z') {
		end++
	}
	if end == 0 {
		return "", ""
	}
	keyword := "#" + strings.ToLower(rest[:end])
	return keyword, strings.TrimSpace(rest[end:])
}

func cFamilyIncludeValue(rest string) string {
	if len(rest) >= 2 && rest[0] == '<' {
		if end := strings.IndexByte(rest[1:], '>'); end >= 0 {
			return rest[1 : end+1]
		}
	}
	if len(rest) >= 2 && rest[0] == '"' {
		if end := strings.IndexByte(rest[1:], '"'); end >= 0 {
			return rest[1 : end+1]
		}
	}
	return ""
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
		// A conditional C++ linkage wrapper in a C header is a transparent
		// scope, not the return-type prefix of the first wrapped declaration.
		// Conditional variants are scanned structurally without macro expansion.
		if parser.token(index, "extern") {
			linkage := index + 1
			for linkage < end && (parser.tokens[linkage].Kind == TokenNewline || parser.tokens[linkage].Kind == TokenDirective) {
				linkage++
			}
			if linkage < end && parser.tokens[linkage].Kind == TokenString &&
				(parser.token(linkage, "\"C\"") || parser.token(linkage, "\"C++\"")) {
				open := nextStructuralToken(parser.tokens, linkage+1, end)
				if open < end && parser.token(open, "{") {
					close := parser.pairs[open]
					if close <= open || close >= end {
						parser.builder.MarkIncomplete()
						return
					}
					parser.parseScope(open+1, close, parent, members, owner)
					index = close + 1
					continue
				}
			}
		}
		if parser.cpp {
			if next, ok := parser.cppExplicitClassInstantiation(index, end); ok {
				index = next
				continue
			}
		}
		if parser.cpp {
			if next, ok := parser.cppStandaloneUnresolvedMacro(index, end, members, owner); ok {
				index = next
				continue
			}
			if next, ok := parser.cppUnsupportedConceptDeclaration(index, end); ok {
				index = next
				continue
			}
			if using, ok := parser.cppTemplateUsingAlias(index, end); ok {
				if next, parsed := parser.parseUsingAlias(index, using, end, parent); parsed {
					index = next
					continue
				}
			}
		}
		if parser.cpp && parser.token(index, "namespace") {
			index = parser.parseNamespace(index, end, parent)
			continue
		}
		if parser.cpp && parser.token(index, "static_assert") {
			open := nextStructuralToken(parser.tokens, index+1, end)
			if open >= end || !parser.token(open, "(") {
				parser.builder.MarkIncomplete()
				return
			}
			close := parser.pairs[open]
			if close <= open || close >= end {
				parser.builder.MarkIncomplete()
				return
			}
			terminator := nextStructuralToken(parser.tokens, close+1, end)
			if terminator >= end || !parser.token(terminator, ";") {
				parser.builder.MarkIncomplete()
				return
			}
			index = terminator + 1
			continue
		}
		if parser.token(index, "typedef") {
			if next, ok := parser.parseAnonymousAggregateTypedef(index, end, parent); ok {
				index = next
				continue
			}
		}
		if keyword, declarationStart, ok := parser.typeDeclarationAt(index, end); ok {
			index = parser.parseType(declarationStart, keyword, end, parent)
			continue
		}
		if parser.cpp && parser.token(index, "using") {
			if next, ok := parser.parseUsingAlias(index, index, end, parent); ok {
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
		} else if parser.cpp && next > index+1 && next <= end && parser.tokens[next-1].Text == ";" {
			// An unsupported complete declaration (such as a deleted operator)
			// must not be reinterpreted from its interior as a new declaration.
			index = next
			continue
		}
		index++
	}
}

// An explicit template instantiation such as "template class Box<int>;"
// refers to an existing C++ class. Neither its template arguments nor its
// specialization name introduce a source declaration in this file. Match
// this exact semicolon-terminated form only: template declarations beginning
// with '<' and class definitions retain their normal parser path.
func (parser *cFamilyParser) cppExplicitClassInstantiation(start, end int) (int, bool) {
	cursor := start
	if parser.token(cursor, "extern") {
		cursor = nextStructuralToken(parser.tokens, cursor+1, end)
	}
	if !parser.token(cursor, "template") {
		return start, false
	}
	keyword := nextStructuralToken(parser.tokens, cursor+1, end)
	if !parser.token(keyword, "class") && !parser.token(keyword, "struct") && !parser.token(keyword, "union") {
		return start, false
	}
	depth := parser.tokens[start].Nesting
	seenArguments := false
	for i := keyword + 1; i < end; i++ {
		token := parser.tokens[i]
		if token.Kind == TokenEOF || token.Text == "{" || token.Text == "}" {
			return start, false
		}
		if token.Text == "<" {
			seenArguments = true
		}
		if token.Nesting != depth {
			continue
		}
		if token.Text == "=" {
			return start, false
		}
		if token.Text == ";" {
			if seenArguments {
				return i + 1, true
			}
			return start, false
		}
	}
	return start, false
}

// A macro call occupying an entire C++ source line is not a function
// declaration. Its expansion might introduce scopes, so preserve following
// authored declarations but explicitly report incomplete structural coverage.
// Do not assume any specific project macro expands to namespace braces.
func (parser *cFamilyParser) cppStandaloneUnresolvedMacro(start, end int, members bool, owner string) (int, bool) {
	token := parser.tokens[start]
	name := token.Text
	if token.Kind != TokenIdentifier || len(name) < 3 ||
		name != strings.ToUpper(name) || !strings.Contains(name, "_") ||
		(members && name == owner) {
		return start, false
	}
	open := nextStructuralToken(parser.tokens, start+1, end)
	if !parser.token(open, "(") || parser.tokens[open].Nesting != token.Nesting+1 {
		return start, false
	}
	if strings.ContainsAny(parser.document.Text[token.EndOffset:parser.tokens[open].StartOffset], "\r\n") {
		return start, false
	}
	close := parser.pairs[open]
	if close <= open || close >= end {
		return start, false
	}
	after := close + 1
	if after >= end {
		return start, false
	}
	next := after + 1
	rangeEnd := parser.tokens[close].EndOffset
	if parser.token(after, "{") {
		blockClose := parser.pairs[after]
		if blockClose <= after || blockClose >= end {
			parser.builder.MarkIncomplete()
			return start, false
		}
		rangeEnd = parser.tokens[blockClose].EndOffset
		next = blockClose + 1
	} else if parser.tokens[after].Kind != TokenNewline && parser.tokens[after].Kind != TokenEOF {
		return start, false
	}
	rangeValue := OffsetRange{Start: token.StartOffset, End: rangeEnd}
	_ = parser.builder.AddDiagnostic(DiagnosticSpec{
		Code:     "cpp-unresolved-macro-invocation",
		Message:  "standalone function-like macro expansion is unknown; following C++ declarations are preserved without inferring macro-generated scope",
		Severity: DiagnosticWarning, Range: &rangeValue, AffectsCoverage: true,
	})
	return next, true
}

// A complete bounded template head is shared by template aliases and
// unsupported concept declarations. Leave other templates to their parsers.
func (parser *cFamilyParser) cppTemplateDeclarationKeyword(start, end int) (int, bool) {
	if !parser.token(start, "template") {
		return start, false
	}
	open := nextStructuralToken(parser.tokens, start+1, end)
	if !parser.token(open, "<") {
		return start, false
	}
	angle := 0
	for index := open; index < end; index++ {
		token := parser.tokens[index]
		switch token.Text {
		case "<":
			angle++
		case ">":
			angle--
		case ">>":
			angle -= 2
		case "{", ";":
			return start, false
		}
		if angle < 0 || angle > 64 || token.Kind == TokenEOF {
			return start, false
		}
		if angle == 0 {
			return nextStructuralToken(parser.tokens, index+1, end), true
		}
	}
	return start, false
}

func (parser *cFamilyParser) cppTemplateUsingAlias(start, end int) (int, bool) {
	keyword, ok := parser.cppTemplateDeclarationKeyword(start, end)
	return keyword, ok && parser.token(keyword, "using")
}

// A concept is a constraint, not a variable. Until a normalized concept kind
// exists, consume only a proven complete declaration and report partial coverage.
func (parser *cFamilyParser) cppUnsupportedConceptDeclaration(start, end int) (int, bool) {
	keyword, ok := parser.cppTemplateDeclarationKeyword(start, end)
	if !ok || !parser.token(keyword, "concept") {
		return start, false
	}
	name := nextStructuralToken(parser.tokens, keyword+1, end)
	if name >= end || parser.tokens[name].Kind != TokenIdentifier {
		return start, false
	}
	assignment := nextStructuralToken(parser.tokens, name+1, end)
	if !parser.token(assignment, "=") {
		return start, false
	}
	depth := parser.tokens[start].Nesting
	for index := assignment + 1; index < end; index++ {
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == depth {
			value := OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[index].EndOffset}
			_ = parser.builder.AddDiagnostic(DiagnosticSpec{
				Code:     "cpp-unsupported-concept",
				Message:  "C++20 concept declarations are not yet represented; no variable was inferred",
				Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true,
			})
			return index + 1, true
		}
		if parser.tokens[index].Kind == TokenEOF || parser.tokens[index].Nesting < depth {
			break
		}
	}
	parser.builder.MarkIncomplete()
	return start, false
}

func (parser *cFamilyParser) token(index int, value string) bool {
	return index >= 0 && index < len(parser.tokens) && parser.tokens[index].Text == value
}

// parseAnonymousAggregateTypedef accepts a single balanced anonymous
// struct, union, or enum typedef. It owns its fields and does not reinterpret
// the alias or members as top-level variables.
func (parser *cFamilyParser) parseAnonymousAggregateTypedef(start, end int, parent *SymbolParent) (int, bool) {
	keyword := nextStructuralToken(parser.tokens, start+1, end)
	if keyword >= end {
		return start, false
	}
	nativeKind := parser.tokens[keyword].Text
	if nativeKind != "enum" && nativeKind != "struct" && nativeKind != "union" {
		return start, false
	}
	open := nextStructuralToken(parser.tokens, keyword+1, end)
	if !parser.token(open, "{") || parser.tokens[open].Nesting != parser.tokens[start].Nesting+1 {
		return start, false
	}
	close := parser.pairs[open]
	if close <= open || close >= end {
		return start, false
	}
	nameIndex := nextStructuralToken(parser.tokens, close+1, end)
	semicolon := nextStructuralToken(parser.tokens, nameIndex+1, end)
	if nameIndex >= end || parser.tokens[nameIndex].Kind != TokenIdentifier ||
		parser.tokens[nameIndex].Nesting != parser.tokens[start].Nesting ||
		!parser.token(semicolon, ";") || parser.tokens[semicolon].Nesting != parser.tokens[start].Nesting {
		return start, false
	}
	symbol, added := parser.add(SymbolSpec{
		Kind: SymbolKindAlias, NativeKind: "typedef-" + nativeKind, Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[semicolon].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Evidence:    SymbolEvidenceStructural,
	})
	if added && nativeKind != "enum" {
		typeParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		parser.types[symbol.QualifiedName] = *typeParent
		parser.parseScope(open+1, close, typeParent, true, symbol.Name)
	}
	return semicolon + 1, true
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
		if _, modifier := cFamilyModifiers[text]; modifier {
			cursor++
			continue
		}
		if text == "struct" || text == "class" || text == "union" || text == "enum" {
			// C++ keywords are case-sensitive; Objective-C uses Class as a type name.
			if text == "class" && (!parser.cpp || parser.tokens[cursor].Text != "class") {
				return 0, start, false
			}
			if !parser.typeHeadIsDefinitionOrForwardDeclaration(cursor, end) {
				return 0, start, false
			}
			return cursor, declarationStart, true
		}
		return 0, start, false
	}
	return 0, start, false
}

// cppTypeNameIndex skips the common unexpanded DLL export decoration in
// class/struct/union heads, but never treats a bare macro-shaped name as a decoration.
func (parser *cFamilyParser) cppTypeNameIndex(keyword, end int) int {
	cursor := keyword + 1
	kind := parser.tokens[keyword].Text
	if kind == "enum" && parser.cpp && cursor < end && (parser.token(cursor, "class") || parser.token(cursor, "struct")) {
		cursor++
	}
	nameIndex := nextIdentifierToken(parser.tokens, cursor, end)
	if !parser.cpp || nameIndex < 0 || (kind != "class" && kind != "struct" && kind != "union") {
		return nameIndex
	}
	macro := parser.tokens[nameIndex].Text
	if macro != strings.ToUpper(macro) || (!strings.HasSuffix(macro, "_API") && !strings.HasSuffix(macro, "_EXPORT")) {
		return nameIndex
	}
	next := nextStructuralToken(parser.tokens, nameIndex+1, end)
	if next >= end || parser.tokens[next].Nesting != parser.tokens[keyword].Nesting ||
		parser.tokens[next].Kind != TokenIdentifier || parser.token(next, "final") || cFamilyTypeAttribute(parser.tokens[next].Text) {
		return nameIndex
	}
	return next
}

func (parser *cFamilyParser) typeHeadIsDefinitionOrForwardDeclaration(keyword, end int) bool {
	depth := parser.tokens[keyword].Nesting
	nameIndex := parser.cppTypeNameIndex(keyword, end)
	if nameIndex < 0 {
		return true
	}
	inheritance := false
	for index := nameIndex + 1; index < end; index++ {
		token := parser.tokens[index]
		if token.Kind == TokenEOF {
			return true
		}
		if token.Text == "{" && token.Nesting == depth+1 {
			return true
		}
		if token.Text == ";" && token.Nesting == depth {
			return true
		}
		if token.Nesting != depth {
			continue
		}
		if parser.cpp && token.Text == ":" {
			inheritance = true
			continue
		}
		if inheritance {
			continue
		}
		if parser.cpp && strings.EqualFold(token.Text, "final") {
			continue
		}
		if cFamilyTypeAttribute(token.Text) {
			next := nextStructuralToken(parser.tokens, index+1, end)
			if next < end && parser.tokens[next].Text == "(" {
				if close := parser.pairs[next]; close > next && close < end {
					index = close
				}
			}
			continue
		}
		if token.Kind == TokenIdentifier || token.Kind == TokenKeyword || token.Text == "*" || token.Text == "&" || token.Text == "=" || token.Text == "," {
			return false
		}
	}
	return true
}

func cFamilyTypeAttribute(value string) bool {
	switch strings.ToLower(value) {
	case "__attribute__", "__attribute", "__declspec", "alignas":
		return true
	default:
		return false
	}
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
			parser.parseNamespaceAlias(start, index, parent)
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

// A namespace alias declares a name. In contrast, "using namespace X"
// imports a namespace and does not declare a new symbol.
func (parser *cFamilyParser) parseNamespaceAlias(start, semicolon int, parent *SymbolParent) {
	name := nextStructuralToken(parser.tokens, start+1, semicolon)
	if name >= semicolon || parser.tokens[name].Kind != TokenIdentifier {
		return
	}
	equals := nextStructuralToken(parser.tokens, name+1, semicolon)
	if equals >= semicolon || !parser.token(equals, "=") {
		return
	}
	cursor := nextStructuralToken(parser.tokens, equals+1, semicolon)
	if cursor >= semicolon {
		return
	}
	if next, ok := parser.namespaceAliasSeparator(cursor, semicolon); ok {
		cursor = next
	}
	for cursor < semicolon {
		if parser.tokens[cursor].Kind != TokenIdentifier && parser.tokens[cursor].Kind != TokenKeyword {
			return
		}
		next := nextStructuralToken(parser.tokens, cursor+1, semicolon)
		if next == semicolon {
			break
		}
		afterSeparator, ok := parser.namespaceAliasSeparator(next, semicolon)
		if !ok {
			return
		}
		cursor = afterSeparator
		if cursor >= semicolon {
			return
		}
	}
	_, _ = parser.add(SymbolSpec{
		Kind: SymbolKindAlias, NativeKind: "namespace-alias", Name: parser.tokens[name].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[semicolon].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[name].StartOffset, End: parser.tokens[name].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[semicolon].EndOffset},
		Evidence:    SymbolEvidenceStructural,
	})
}

// The structural scanner may represent C++ "::" as one token or two
// adjacent ":" tokens. Do not accept a single colon as a namespace path.
func (parser *cFamilyParser) namespaceAliasSeparator(index, end int) (int, bool) {
	if parser.token(index, "::") {
		return nextStructuralToken(parser.tokens, index+1, end), true
	}
	if parser.token(index, ":") {
		second := nextStructuralToken(parser.tokens, index+1, end)
		if parser.token(second, ":") {
			return nextStructuralToken(parser.tokens, second+1, end), true
		}
	}
	return index, false
}

func (parser *cFamilyParser) parseType(start, keyword, end int, parent *SymbolParent) int {
	depth := parser.tokens[keyword].Nesting
	nativeKind := strings.ToLower(parser.tokens[keyword].Text)
	nameIndex := parser.cppTypeNameIndex(keyword, end)
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
	lastNameIndex := nameIndex
	qualifiedName := ""
	if parser.cpp && open >= 0 {
		// A definition such as "struct Outer::Inner { ... }" declares Inner,
		// not Outer. Its owner may only exist in a separate header: retain
		// the written qualifier without manufacturing an outer class ID.
		parts := []string{parser.tokens[nameIndex].Text}
		for cursor := nextStructuralToken(parser.tokens, lastNameIndex+1, open); cursor < open; {
			next, ok := parser.namespaceAliasSeparator(cursor, open)
			if !ok || next >= open || parser.tokens[next].Kind != TokenIdentifier ||
				parser.tokens[next].Nesting != depth {
				break
			}
			lastNameIndex = next
			parts = append(parts, parser.tokens[next].Text)
			cursor = nextStructuralToken(parser.tokens, next+1, open)
		}
		if len(parts) > 1 {
			qualifiedName = strings.Join(parts, ".")
			if parent != nil && parent.QualifiedName != "" &&
				!strings.HasPrefix(qualifiedName, parent.QualifiedName+".") {
				qualifiedName = parent.QualifiedName + "." + qualifiedName
			}
		}
	}
	symbol, added := parser.add(SymbolSpec{
		Kind: kind, NativeKind: nativeKind, Name: parser.tokens[lastNameIndex].Text, QualifiedName: qualifiedName, Parent: parent,
		Declaration: OffsetRange{Start: declarationStart, End: declarationEnd},
		NameRange:   OffsetRange{Start: parser.tokens[lastNameIndex].StartOffset, End: parser.tokens[lastNameIndex].EndOffset},
		Signature:   &OffsetRange{Start: declarationStart, End: signatureEnd}, Body: body,
		Visibility: visibilityFromModifiers(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
	})
	if !added {
		return terminator + 1
	}
	parser.types[symbol.QualifiedName] = SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
	if parser.cpp && open >= 0 {
		parser.collectCPPBaseRelations(symbol.QualifiedName, lastNameIndex+1, open, depth)
	}
	if open >= 0 && nativeKind != "enum" {
		typeParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		parser.parseScope(open+1, close, typeParent, true, symbol.Name)
	}
	if open >= 0 && parser.token(start, "typedef") {
		// A tagged aggregate and its trailing typedef name are two distinct
		// declarations. Accept only one directly authored alias and semicolon;
		// ordinary trailing objects and compound declarators retain their path.
		aliasIndex := nextStructuralToken(parser.tokens, close+1, end)
		if aliasIndex < end && parser.tokens[aliasIndex].Kind == TokenIdentifier &&
			parser.tokens[aliasIndex].Nesting == depth {
			semicolonIndex := nextStructuralToken(parser.tokens, aliasIndex+1, end)
			if parser.token(semicolonIndex, ";") && parser.tokens[semicolonIndex].Nesting == depth {
				_, _ = parser.add(SymbolSpec{
					Kind: SymbolKindAlias, NativeKind: "typedef", Name: parser.tokens[aliasIndex].Text, Parent: parent,
					Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[semicolonIndex].EndOffset},
					NameRange:   OffsetRange{Start: parser.tokens[aliasIndex].StartOffset, End: parser.tokens[aliasIndex].EndOffset},
					Evidence:    SymbolEvidenceStructural,
				})
				return semicolonIndex + 1
			}
		}
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
	for _, part := range splitTypeTokenRange(parser.tokens, colon+1, end, nesting) {
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

func (parser *cFamilyParser) parseUsingAlias(start, using, end int, parent *SymbolParent) (int, bool) {
	depth := parser.tokens[using].Nesting
	semicolon := -1
	for index := using + 1; index < end; index++ {
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == depth {
			semicolon = index
			break
		}
	}
	if semicolon < 0 {
		return start + 1, false
	}
	nameIndex := nextIdentifierToken(parser.tokens, using+1, semicolon)
	if nameIndex < 0 || !parser.hasToken(nameIndex+1, semicolon, "=") {
		// A using-directive or using-declaration imports a name; it does
		// not introduce a variable. Consume it instead of rescanning tokens.
		return semicolon + 1, true
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
	assignmentSeen := false
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == "=" && parser.tokens[index].Nesting == depth {
			// operator=(...) names an overload, not a variable initializer.
			if !parser.operatorTokenBefore(index, start) {
				assignmentSeen = true
			}
			continue
		}
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == depth {
			terminator = index
			break
		}
		if parser.tokens[index].Text == "{" && parser.tokens[index].Nesting == depth+1 {
			if assignmentSeen {
				close := parser.pairs[index]
				if close <= index || close >= end {
					parser.builder.MarkIncomplete()
					return end, false
				}
				// An '= { ... }' initializer must be followed by a
				// declaration delimiter. If the semicolon is missing,
				// do not swallow a following function or invent a variable
				// from its return statement. This does not affect braced
				// expressions such as lambdas or typed temporaries.
				previous := previousStructuralToken(parser.tokens, index-1, start)
				if previous >= start && parser.token(previous, "=") {
					following := nextStructuralToken(parser.tokens, close+1, end)
					if following >= end || (parser.tokens[following].Nesting == depth &&
						!parser.token(following, ";") && !parser.token(following, ",")) {
						parser.builder.MarkIncomplete()
						return close + 1, true
					}
				}
				index = close
				continue
			}
			if parser.cpp && parser.firstFunctionParen(start, index, depth) < 0 {
				// A C++ declarator such as int count{1}; is a variable,
				// not a function body. Consume its full initializer before
				// scanning the next declaration, even inside a class.
				close := parser.pairs[index]
				if close > index && close < end {
					semicolon := nextStructuralToken(parser.tokens, close+1, end)
					previous := previousStructuralToken(parser.tokens, index-1, start)
					if previous >= start && parser.tokens[previous].Kind == TokenIdentifier &&
						semicolon < end && parser.tokens[semicolon].Text == ";" && parser.tokens[semicolon].Nesting == depth {
						return parser.parseVariable(start, semicolon, index, parent, members)
					}
				}
			}
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
	// An unsupported return-type macro followed by a function declarator
	// is neither a real function named after the macro nor a variable.
	if parser.invalidWrappedReturnPrefix(start, terminator, depth) &&
		parser.wrappedFunctionParen(start, terminator, depth) < 0 {
		if parser.tokens[terminator].Text == "{" {
			if close := parser.pairs[terminator]; close > terminator {
				return close + 1, true
			}
		}
		// The unsupported declaration is consumed. Returning false would
		// rescan its suffix and invent a function or variable from it.
		return terminator + 1, true
	}
	if _, functionPointer := parser.functionPointerDeclarator(start, terminator); !functionPointer {
		paren := parser.firstFunctionParen(start, terminator, depth)
		if paren >= 0 {
			if parser.cpp && parser.tokens[terminator].Text == ";" {
				// Literal and 'new' arguments cannot begin a function parameter declaration.
				if close := parser.pairs[paren]; close > paren+1 && (parser.tokens[paren+1].Kind == TokenNumber || parser.tokens[paren+1].Kind == TokenString || parser.tokens[paren+1].Text == "new") {
					return parser.parseVariable(start, terminator, paren, parent, members)
				}
				if parser.cppDirectInitializerExpression(paren) {
					return parser.parseVariable(start, terminator, paren, parent, members)
				}
			}
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
	return parser.parseVariable(start, terminator, terminator, parent, members)
}

// cppDirectInitializerExpression detects a source-written '+' expression in
// constructor arguments. A '+' inside a default parameter, template argument,
// or nested declarator is not proof that the outer declaration is a variable.
// Unproven or oversized lists retain the conservative existing classification.
func (parser *cFamilyParser) cppDirectInitializerExpression(paren int) bool {
	close := parser.pairs[paren]
	const maxTokens = 256
	if close <= paren || close >= len(parser.tokens) || close-paren > maxTokens {
		return false
	}
	// The scanner records the opening delimiter after pushing it; direct
	// argument tokens have the same nesting, while bracketed extents do not.
	depth := parser.tokens[paren].Nesting
	angleDepth := 0
	defaultArgument := false
	for index := paren + 1; index < close; index++ {
		token := parser.tokens[index]
		if token.Nesting != depth {
			continue
		}
		switch token.Text {
		case "<":
			angleDepth++
		case ">":
			if angleDepth > 0 {
				angleDepth--
			}
		case ">>":
			angleDepth = max(0, angleDepth-2)
		case ",":
			if angleDepth == 0 {
				defaultArgument = false
			}
		case "=":
			if angleDepth == 0 {
				defaultArgument = true
			}
		case "+":
			if angleDepth == 0 && !defaultArgument {
				return true
			}
		}
	}
	return false
}

func (parser *cFamilyParser) firstFunctionParen(start, end, depth int) int {
	if paren := parser.wrappedFunctionParen(start, end, depth); paren >= 0 {
		return paren
	}
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == "=" && parser.tokens[index].Nesting == depth && !parser.operatorTokenBefore(index, start) {
			return -1
		}
		if parser.tokens[index].Text != "(" || parser.tokens[index].Nesting != depth+1 {
			continue
		}
		previous := previousStructuralToken(parser.tokens, index-1, start)
		if previous < 0 {
			continue
		}
		if parser.tokens[previous].Kind == TokenIdentifier || parser.tokens[previous].Text == "]" || parser.tokens[previous].Text == ")" || parser.operatorTokenBefore(previous, start) ||
			(parser.cpp && parser.cppExplicitSpecializationName(start, index) >= 0) {
			return index
		}
	}
	return -1
}

// Only punctuation immediately following the operator keyword belongs to
// the overloaded function name; other '=' tokens still start initializers.
func (parser *cFamilyParser) operatorTokenBefore(index, start int) bool {
	if !parser.cpp || index <= start {
		return false
	}
	previous := previousStructuralToken(parser.tokens, index-1, start)
	return previous >= start && parser.token(previous, "operator")
}

// Detect malformed/unsupported macro-return syntax without inventing a
// function whose name is the unexpanded return-type macro.
func (parser *cFamilyParser) invalidWrappedReturnPrefix(start, end, depth int) bool {
	if !cFamilyReturnMacroName(parser.tokens[start].Text) {
		return false
	}
	open := nextStructuralToken(parser.tokens, start+1, end)
	if open >= end || parser.tokens[open].Text != "(" || parser.tokens[open].Nesting != depth+1 {
		return false
	}
	close := parser.pairs[open]
	if close <= open || close >= end {
		return false
	}
	name := nextStructuralToken(parser.tokens, close+1, end)
	if name >= end || parser.tokens[name].Nesting != depth || parser.tokens[name].Kind != TokenIdentifier {
		return false
	}
	paren := nextStructuralToken(parser.tokens, name+1, end)
	return paren < end && parser.tokens[paren].Text == "(" && parser.tokens[paren].Nesting == depth+1
}

func cFamilyReturnMacroName(name string) bool {
	upper := strings.ToUpper(name)
	unseparatedAPI := name == upper && len(upper) > len("API") && strings.HasSuffix(upper, "API")
	return strings.HasSuffix(upper, "_PUBLIC") || strings.HasSuffix(upper, "_EXPORT") ||
		strings.HasSuffix(upper, "_API") || unseparatedAPI
}

// Accept the common unexpanded EXPORT(type) function-return decoration only
// when it precedes a distinct named function declarator. Macro expansion is
// not inferred; this identifies the declaration written in the source.
func (parser *cFamilyParser) wrappedFunctionParen(start, end, depth int) int {
	macro := parser.tokens[start]
	if macro.Kind != TokenIdentifier && macro.Kind != TokenKeyword {
		return -1
	}
	if !cFamilyReturnMacroName(macro.Text) {
		return -1
	}
	open := nextStructuralToken(parser.tokens, start+1, end)
	if open >= end || parser.tokens[open].Text != "(" || parser.tokens[open].Nesting != depth+1 {
		return -1
	}
	close := parser.pairs[open]
	if close <= open+1 || close >= end {
		return -1
	}
	hasType := false
	for i := open + 1; i < close; i++ {
		token := parser.tokens[i]
		if token.Kind == TokenIdentifier || token.Kind == TokenKeyword {
			hasType = true
			continue
		}
		if token.Text != "*" && token.Text != "&" {
			return -1 // Not a simple type expression: do not guess macro semantics.
		}
	}
	if !hasType {
		return -1
	}
	name := nextStructuralToken(parser.tokens, close+1, end)
	if name >= end || parser.tokens[name].Nesting != depth || parser.tokens[name].Kind != TokenIdentifier {
		return -1
	}
	paren := nextStructuralToken(parser.tokens, name+1, end)
	if paren >= end || parser.tokens[paren].Text != "(" || parser.tokens[paren].Nesting != depth+1 {
		return -1
	}
	return paren
}

// cppLexicalDeclaratorKey keeps only a bounded private C++ token spelling.
// It does not erase parameter names, resolve types, or claim semantic overload
// identity. Token separators prevent distinct spellings from colliding.
func cppLexicalDeclaratorKey(tokens []Token, start, closeParen, end int) string {
	const maxBytes = 512
	if start < 0 || end > len(tokens) || start >= end {
		return ""
	}
	var result strings.Builder
	for i := start; i < end; i++ {
		token := tokens[i]
		switch token.Kind {
		case TokenNewline:
			continue
		case TokenString, TokenDirective, TokenEOF:
			return ""
		}
		if token.Text == "" {
			continue
		}
		// override/final are in-class virt-specifiers, not part of a
		// corresponding out-of-class function definition's declarator.
		if i > closeParen && token.Nesting == tokens[start].Nesting-1 &&
			(token.Text == "override" || token.Text == "final") {
			continue
		}
		if result.Len()+1+len(token.Text) > maxBytes {
			return ""
		}
		result.WriteByte(0)
		result.WriteString(token.Text)
	}
	return result.String()
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
	if parser.cpp {
		if specializationName := parser.cppExplicitSpecializationName(start, paren); specializationName >= 0 {
			nameIndex = specializationName
		}
	}
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
	unresolvedQualifier := ""
	if parser.cpp {
		if qualifiedParent, qualifiedOwner, ok := parser.qualifiedFunctionOwner(start, nameIndex, parent); ok {
			effectiveParent = qualifiedParent
			effectiveMembers = true
			effectiveOwner = qualifiedOwner
		} else {
			unresolvedQualifier = qualifiedOwner
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
	defaulted := false
	if parser.cpp && body == nil && parser.token(terminator, ";") {
		last := previousStructuralToken(parser.tokens, terminator-1, closeParen)
		equals := previousStructuralToken(parser.tokens, last-1, closeParen)
		defaulted = equals >= 0 && parser.token(last, "default") && parser.token(equals, "=") &&
			parser.tokens[equals].Nesting == parser.tokens[start].Nesting
		if defaulted {
			switch kind {
			case SymbolKindConstructor:
				nativeKind = "constructor-defaulted-definition"
			case SymbolKindDestructor:
				nativeKind = "destructor-defaulted-definition"
			case SymbolKindOperator:
				nativeKind = "operator-defaulted-definition"
			}
		}
	}
	if parser.cpp && unresolvedQualifier != "" && (body != nil || defaulted) && kind == SymbolKindFunction {
		// The written spelling distinguishes lifecycle definitions even when the
		// qualifier's class/namespace identity is unknown across files. Preserve
		// the conservative function kind and unowned parent. '= default' has no
		// body, but is a C++ definition rather than a mere declaration.
		owner := unresolvedQualifier
		if dot := strings.LastIndexByte(owner, '.'); dot >= 0 {
			owner = owner[dot+1:]
		}
		if name == owner {
			prefix := tokenRangeText(parser.tokens, start, nameIndex)
			qualifier := strings.ReplaceAll(unresolvedQualifier, ".", "::") + "::"
			switch prefix {
			case qualifier:
				if defaulted {
					nativeKind = "qualified-constructor-defaulted-definition"
				} else {
					nativeKind = "qualified-constructor-definition"
				}
			case qualifier + "~":
				if defaulted {
					nativeKind = "qualified-destructor-defaulted-definition"
				} else {
					nativeKind = "qualified-destructor-definition"
				}
			}
		}
	}
	modifiers := collectKnownModifiers(parser.tokens, start, paren, cFamilyModifiers)
	nameStart := parser.tokens[nameIndex].StartOffset
	if parser.cpp && nameIndex > start && parser.token(nameIndex-1, "~") &&
		(kind == SymbolKindDestructor || strings.HasPrefix(nativeKind, "qualified-destructor-")) {
		name = "~" + name
		nameStart = parser.tokens[nameIndex-1].StartOffset
	}
	qualifiedName := ""
	if unresolvedQualifier != "" {
		// Preserve an explicit single-level C++ qualifier without inventing
		// class membership: it might name a namespace from another file.
		qualifiedName = unresolvedQualifier + "." + name
		if parent != nil && parent.QualifiedName != "" {
			qualifiedName = parent.QualifiedName + "." + qualifiedName
		}
	}
	nameEnd := parser.tokens[nameIndex].EndOffset
	if kind == SymbolKindOperator {
		last := previousStructuralToken(parser.tokens, paren-1, nameIndex)
		if last >= nameIndex {
			nameEnd = parser.tokens[last].EndOffset
		}
	}
	cppDeclaratorKey := ""
	if parser.cpp {
		keyEnd := terminator
		depth := parser.tokens[start].Nesting
		for index := closeParen + 1; index < terminator; index++ {
			if parser.tokens[index].Nesting == depth &&
				(parser.token(index, ":") || parser.token(index, "=")) {
				keyEnd = index // Constructor initializers and =default/delete are not qualifiers.
				break
			}
		}
		cppDeclaratorKey = cppLexicalDeclaratorKey(parser.tokens, paren, closeParen, keyEnd)
		last := previousStructuralToken(parser.tokens, terminator-1, closeParen)
		before := previousStructuralToken(parser.tokens, last-1, closeParen)
		if parser.token(last, "delete") && parser.token(before, "=") {
			cppDeclaratorKey = "" // A deleted member cannot have an out-of-class definition.
		}
	}
	_, added := parser.add(SymbolSpec{
		Kind: kind, NativeKind: nativeKind, Name: name, QualifiedName: qualifiedName, Parent: effectiveParent,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: nameStart, End: nameEnd},
		Signature:   &OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[bodyStartOrTerminator(bodyOpen, terminator)].StartOffset}, Body: body,
		Visibility: visibilityFromModifiers(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
		Disambiguator:    tokenRangeText(parser.tokens, paren, closeParen+1),
		cppDeclaratorKey: cppDeclaratorKey,
	})
	return next, added
}

// cppExplicitSpecializationName finds the name before a balanced template
// argument list only for a written "template<> ... Name<...>(...)" function.
// Ordinary calls, initializers and unproven templates keep existing parsing.
func (parser *cFamilyParser) cppExplicitSpecializationName(start, paren int) int {
	if !parser.cpp || !parser.token(start, "template") {
		return -1
	}
	open := nextStructuralToken(parser.tokens, start+1, paren)
	close := open
	if !parser.token(open, "<>") {
		close = nextStructuralToken(parser.tokens, open+1, paren)
		if !parser.token(open, "<") || !parser.token(close, ">") {
			return -1
		}
	}
	cursor := previousStructuralToken(parser.tokens, paren-1, start)
	if !parser.token(cursor, ">") && !parser.token(cursor, ">>") {
		return -1
	}
	angle := 0
	for index := cursor; index > close; index-- {
		switch parser.tokens[index].Text {
		case ">":
			angle++
		case ">>":
			angle += 2
		case "<":
			angle--
		}
		if angle == 0 {
			name := previousStructuralToken(parser.tokens, index-1, start)
			if name > close && parser.tokens[name].Kind == TokenIdentifier {
				return name
			}
			return -1
		}
		if angle < 0 || angle > 64 {
			return -1
		}
	}
	return -1
}

// Only a written scope adjacent to the function name is collected. Return
// types can carry unrelated qualifiers. Unknown multi-part qualifiers remain
// qualified names, never invented parent symbols.
func (parser *cFamilyParser) qualifiedFunctionOwner(start, nameIndex int, lexicalParent *SymbolParent) (*SymbolParent, string, bool) {
	cursor := previousStructuralToken(parser.tokens, nameIndex-1, start)
	if parser.token(cursor, "~") {
		cursor = previousStructuralToken(parser.tokens, cursor-1, start)
	}
	parts := make([]string, 0, 3)
	for {
		separator := -1
		if parser.token(cursor, "::") {
			separator = cursor
		} else if parser.token(cursor, ":") {
			previous := previousStructuralToken(parser.tokens, cursor-1, start)
			if parser.token(previous, ":") {
				separator = previous
			}
		}
		if separator < 0 {
			break
		}
		cursor = previousStructuralToken(parser.tokens, separator-1, start)
		if parser.token(cursor, ">") || parser.token(cursor, ">>") {
			angle := 0
			for cursor >= start {
				switch parser.tokens[cursor].Text {
				case ">":
					angle++
				case ">>":
					angle += 2
				case "<":
					angle--
				}
				cursor = previousStructuralToken(parser.tokens, cursor-1, start)
				if angle == 0 {
					break
				}
				if angle < 0 || angle > 64 {
					return nil, "", false
				}
			}
		}
		if cursor < start || parser.tokens[cursor].Kind != TokenIdentifier {
			return nil, "", false
		}
		parts = append(parts, parser.tokens[cursor].Text)
		cursor = previousStructuralToken(parser.tokens, cursor-1, start)
	}
	if len(parts) == 0 {
		return nil, "", false
	}
	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
	}
	qualifier := strings.Join(parts, ".")
	ownerName := parts[len(parts)-1]
	candidate := qualifier
	if lexicalParent != nil && lexicalParent.QualifiedName != "" {
		candidate = lexicalParent.QualifiedName + "." + qualifier
	}
	if parent, ok := parser.types[candidate]; ok {
		value := parent
		return &value, ownerName, true
	}
	var match *SymbolParent
	for qualified, parent := range parser.types {
		if qualified != qualifier && !strings.HasSuffix(qualified, "."+qualifier) {
			continue
		}
		if match != nil {
			return nil, qualifier, false
		}
		value := parent
		match = &value
	}
	if match != nil {
		return match, ownerName, true
	}
	return nil, qualifier, false
}
func bodyStartOrTerminator(bodyOpen, terminator int) int {
	if bodyOpen >= 0 {
		return bodyOpen
	}
	return terminator
}

func (parser *cFamilyParser) parseVariable(start, semicolon, nameLimit int, parent *SymbolParent, members bool) (int, bool) {
	if parser.hasToken(start, semicolon, "typedef") {
		nameIndex, functionPointer := parser.functionPointerDeclarator(start, semicolon)
		if !functionPointer {
			nameIndex = previousIdentifierToken(parser.tokens, semicolon-1, start)
		}
		if nameIndex >= 0 {
			_, added := parser.add(SymbolSpec{Kind: SymbolKindAlias, NativeKind: "typedef", Name: parser.tokens[nameIndex].Text, Parent: parent,
				Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[semicolon].EndOffset},
				NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset}, Evidence: SymbolEvidenceStructural})
			return semicolon + 1, added
		}
	}
	limit := min(semicolon, nameLimit)
	for index := start; index < limit; index++ {
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
	nameIndices := []int{nameIndex}
	if nameLimit == semicolon {
		// A declaration may introduce several names under one type. Split only
		// at top-level commas; template, call and initializer commas are not
		// independent declarators.
		parts := splitTypeTokenRange(parser.tokens, start, semicolon, parser.tokens[start].Nesting)
		if len(parts) == 1 {
			// In an initializer, '<' can be comparison rather than a template
			// opener. A type before '=' may still contain template commas.
			firstEqual, firstAngle := -1, -1
			for index := start; index < semicolon; index++ {
				if parser.tokens[index].Nesting != parser.tokens[start].Nesting {
					continue
				}
				if firstEqual < 0 && parser.tokens[index].Text == "=" {
					firstEqual = index
				}
				if firstAngle < 0 && parser.tokens[index].Text == "<" {
					firstAngle = index
				}
			}
			if firstEqual >= 0 && (firstAngle < 0 || firstEqual < firstAngle) {
				parts = splitCommaTokenRangeAt(parser.tokens, start, semicolon, parser.tokens[start].Nesting)
			}
		}
		if len(parts) > 1 {
			candidates := make([]int, 0, len(parts))
			for _, part := range parts {
				partLimit := part[1]
				for index := part[0]; index < partLimit; index++ {
					if parser.tokens[index].Text == "=" && parser.tokens[index].Nesting == parser.tokens[start].Nesting {
						partLimit = index
						break
					}
				}
				candidate := previousIdentifierToken(parser.tokens, partLimit-1, part[0])
				// In a fixed-size array declarator, the identifier before
				// the bracket owns the declaration, not a dimension name.
				for index := part[0]; index < partLimit; index++ {
					if parser.tokens[index].Text == "[" && parser.tokens[index].Nesting == parser.tokens[start].Nesting+1 {
						candidate = previousIdentifierToken(parser.tokens, index-1, part[0])
						break
					}
				}
				if candidate < 0 {
					candidates = nil
					break
				}
				if _, modifier := cFamilyModifiers[strings.ToLower(parser.tokens[candidate].Text)]; modifier {
					candidates = nil
					break
				}
				candidates = append(candidates, candidate)
			}
			if len(candidates) == len(parts) {
				nameIndices = candidates
			}
		}
	}
	lowerName := strings.ToLower(parser.tokens[nameIndices[0]].Text)
	if _, isModifier := cFamilyModifiers[lowerName]; isModifier {
		return semicolon + 1, false
	}
	kind := SymbolKindVariable
	nativeKind := "variable"
	if members {
		kind = SymbolKindField
		nativeKind = "field"
	}
	if parser.hasToken(start, nameIndices[0], "const") || parser.hasToken(start, nameIndices[0], "constexpr") {
		if !members {
			kind = SymbolKindConstant
			nativeKind = "constant"
		}
	}
	modifiers := collectKnownModifiers(parser.tokens, start, nameIndices[0], cFamilyModifiers)
	added := false
	for _, name := range nameIndices {
		_, ok := parser.add(SymbolSpec{
			Kind: kind, NativeKind: nativeKind, Name: parser.tokens[name].Text, Parent: parent,
			Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[semicolon].EndOffset},
			NameRange:   OffsetRange{Start: parser.tokens[name].StartOffset, End: parser.tokens[name].EndOffset},
			Visibility:  visibilityFromModifiers(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
		})
		added = added || ok
		if parser.stopped {
			break
		}
	}
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
	var masked []byte
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
			if masked == nil {
				masked = []byte(text)
			}
			for cursor := index; cursor < end; cursor++ {
				if masked[cursor] != '\r' && masked[cursor] != '\n' {
					masked[cursor] = ' '
				}
			}
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
	if masked == nil {
		return text, diagnostics, nil
	}
	return string(masked), diagnostics, nil
}

func cppRawStringStart(text string, index int) (prefixBytes, delimiterStart int, ok bool) {
	if index == len(text) {
		return 0, 0, false
	}
	remaining := len(text) - index
	switch text[index] {
	case 'R':
		if remaining >= 2 && text[index+1] == '"' {
			prefixBytes = 2
		}
	case 'L', 'U':
		if remaining >= 3 && text[index+1] == 'R' && text[index+2] == '"' {
			prefixBytes = 3
		}
	case 'u':
		if remaining >= 3 && text[index+1] == 'R' && text[index+2] == '"' {
			prefixBytes = 3
		} else if remaining >= 4 && text[index+1] == '8' && text[index+2] == 'R' && text[index+3] == '"' {
			prefixBytes = 4
		}
	}
	if prefixBytes == 0 {
		return 0, 0, false
	}
	if index > 0 {
		previous := text[index-1]
		if previous == '_' || previous >= '0' && previous <= '9' || previous >= 'A' && previous <= 'Z' || previous >= 'a' && previous <= 'z' {
			return 0, 0, false
		}
	}
	return prefixBytes, index + prefixBytes, true
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
