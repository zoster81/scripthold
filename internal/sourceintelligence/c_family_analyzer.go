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
	variantBaseText := maskCFamilyDirectiveBlockComments(document.Text, lexicalText)
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
	scanText := maskCFamilyDirectiveBlockComments(lexicalText, lexicalText)
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
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
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
	probe.lineStarts = buildLineStarts(text)
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

func cFamilyReduceMacroDelimiterEffect(body string, effects map[string]cFamilyMacroStructuralEffect) ([]string, []string, bool) {
	prefix := make([]string, 0, 4)
	stack := make([]string, 0, 8)
	apply := func(effect cFamilyMacroStructuralEffect) bool {
		for _, close := range effect.prefixClosers {
			if len(stack) == 0 {
				prefix = append(prefix, close)
				continue
			}
			if !cFamilyDelimiterMatches(stack[len(stack)-1], close) {
				return false
			}
			stack = stack[:len(stack)-1]
		}
		for _, open := range effect.suffixOpeners {
			if len(stack) >= 64 {
				return false
			}
			stack = append(stack, open)
		}
		return true
	}

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
			end := index + 1
			for end < len(body) && cFamilyMacroIdentifierContinue(body[end]) {
				end++
			}
			name := body[index:end]
			if effect, ok := effects[name]; ok {
				if effect.functionLike {
					cursor := end
					for cursor < len(body) && (body[cursor] == ' ' || body[cursor] == '\t') {
						cursor++
					}
					if cursor < len(body) && body[cursor] == '(' {
						return nil, nil, false
					}
				} else if !apply(effect) {
					return nil, nil, false
				}
			}
			index = end - 1
			continue
		}
		switch current {
		case '(', '[', '{':
			if len(stack) >= 64 {
				return nil, nil, false
			}
			stack = append(stack, string(current))
		case ')', ']', '}':
			close := string(current)
			if len(stack) == 0 {
				prefix = append(prefix, close)
				continue
			}
			if !cFamilyDelimiterMatches(stack[len(stack)-1], close) {
				return nil, nil, false
			}
			stack = stack[:len(stack)-1]
		}
	}
	if quote != 0 || blockComment || len(prefix)+len(stack) > 64 {
		return nil, nil, false
	}
	return prefix, append([]string(nil), stack...), true
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
		expression := strings.Join(strings.Fields(rest), " ")
		if expression == "" {
			return "", 0
		}
		return "expr:" + strconv.Itoa(expressionEpoch) + ":" + expression, 1
	default:
		return "", 0
	}
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
					summary.dependencies = append(summary.dependencies, StructuralDependency{Kind: StructuralDependencyInclude, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural})
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
			if !parser.typeHeadIsDefinitionOrForwardDeclaration(cursor, end) {
				return 0, start, false
			}
			return cursor, declarationStart, true
		}
		return 0, start, false
	}
	return 0, start, false
}

func (parser *cFamilyParser) typeHeadIsDefinitionOrForwardDeclaration(keyword, end int) bool {
	depth := parser.tokens[keyword].Nesting
	nameIndex := nextIdentifierToken(parser.tokens, keyword+1, end)
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
	assignmentSeen := false
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == "=" && parser.tokens[index].Nesting == depth {
			assignmentSeen = true
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
				index = close
				continue
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
