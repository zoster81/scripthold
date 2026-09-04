package sourceintelligence

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

const (
	alConditionalVariantLimit    = 32
	alConditionalSolverNodeLimit = 8192
)

type alConditionalExprKind uint8

const (
	alConditionalExprFalse alConditionalExprKind = iota
	alConditionalExprTrue
	alConditionalExprSymbol
	alConditionalExprNot
	alConditionalExprAnd
	alConditionalExprOr
)

type alConditionalExpr struct {
	kind   alConditionalExprKind
	symbol string
	left   *alConditionalExpr
	right  *alConditionalExpr
}

type alConditionalBranch struct {
	start      int
	end        int
	condition  *alConditionalExpr
	elseBranch bool
}

type alConditionalGroup struct {
	openOffset   int
	parentGroup  int
	parentBranch int
	branches     []alConditionalBranch
	seenElse     bool
}

type alConditionalFrame struct {
	group  int
	branch int
}

type alConditionalPlan struct {
	directives []OffsetRange
	groups     []alConditionalGroup
	variants   []map[int]int
}

type alConditionalIssue struct {
	offset  int
	message string
}

func analyzeALSource(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	builderOptions := SymbolBuilderOptions{
		Context: ctx, Language: "al", Analyzer: string(AnalyzerAL), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	}
	if err := validateSymbolBuilderOptions(document, builderOptions); err != nil {
		return AnalyzerResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_al_source", document.Path, err)
	}
	plan, issue, err := planALConditionalVariants(ctx, document, options)
	if err != nil {
		return AnalyzerResult{}, err
	}
	if issue != nil {
		return alConditionalIssueResult(ctx, document, options, issue), nil
	}
	if len(plan.directives) == 0 {
		return analyzeALVariant(ctx, document, options)
	}

	variants := plan.variants
	if len(variants) == 0 {
		variants = []map[int]int{{}}
	}
	merged := newALConditionalMerge(options.Limits)
	for _, selection := range variants {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_al_source", document.Path, err)
		}
		variant := alConditionalVariant(document, plan, selection)
		result, err := analyzeALVariant(ctx, variant, options)
		if err != nil {
			return AnalyzerResult{}, err
		}
		merged.add(result)
	}
	return merged.finish(), nil
}

func planALConditionalVariants(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (alConditionalPlan, *alConditionalIssue, error) {
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	profile := ALScannerProfile()
	profile.Directives = true
	profile.DisableDelimiterTracking = true
	scan, err := ScanSource(ctx, document, profile, ScannerLimits{
		MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting,
	})
	if err != nil {
		return alConditionalPlan{}, nil, err
	}

	plan := alConditionalPlan{}
	stack := make([]alConditionalFrame, 0, 8)
	for _, token := range scan.Tokens {
		if token.Kind != TokenDirective {
			continue
		}
		plan.directives = append(plan.directives, OffsetRange{Start: token.StartOffset, End: token.EndOffset})
		directive := alDirectiveKeyword(token.Text)
		switch directive {
		case "#if":
			condition, parseErr := alConditionalDirectiveExpression(token.Text, directive)
			if parseErr != nil {
				return plan, &alConditionalIssue{offset: token.StartOffset, message: parseErr.Error()}, nil
			}
			group := alConditionalGroup{openOffset: token.StartOffset, parentGroup: -1, parentBranch: -1}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				group.parentGroup = parent.group
				group.parentBranch = parent.branch
			}
			group.branches = append(group.branches, alConditionalBranch{start: token.EndOffset, condition: condition})
			plan.groups = append(plan.groups, group)
			stack = append(stack, alConditionalFrame{group: len(plan.groups) - 1})
		case "#elif", "#else":
			if len(stack) == 0 {
				return plan, &alConditionalIssue{offset: token.StartOffset, message: "conditional branch directive has no matching #if"}, nil
			}
			frame := &stack[len(stack)-1]
			group := &plan.groups[frame.group]
			if group.seenElse {
				return plan, &alConditionalIssue{offset: token.StartOffset, message: "conditional branch appears after #else"}, nil
			}
			group.branches[frame.branch].end = token.StartOffset
			branch := alConditionalBranch{start: token.EndOffset}
			if directive == "#else" {
				group.seenElse = true
				branch.elseBranch = true
			} else {
				condition, parseErr := alConditionalDirectiveExpression(token.Text, directive)
				if parseErr != nil {
					return plan, &alConditionalIssue{offset: token.StartOffset, message: parseErr.Error()}, nil
				}
				branch.condition = condition
			}
			group.branches = append(group.branches, branch)
			frame.branch = len(group.branches) - 1
		case "#endif":
			if len(stack) == 0 {
				return plan, &alConditionalIssue{offset: token.StartOffset, message: "#endif has no matching #if"}, nil
			}
			frame := stack[len(stack)-1]
			group := &plan.groups[frame.group]
			group.branches[frame.branch].end = token.StartOffset
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) > 0 {
		group := plan.groups[stack[len(stack)-1].group]
		return plan, &alConditionalIssue{offset: group.openOffset, message: "#if has no matching #endif"}, nil
	}

	variants, issue := alConditionalVariantSelections(plan.groups)
	if issue != nil {
		return plan, issue, nil
	}
	plan.variants = variants
	return plan, nil, nil
}

func alDirectiveKeyword(text string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(fields) == 0 {
		return ""
	}
	switch fields[0] {
	case "#if", "#elif", "#else", "#endif":
		return fields[0]
	default:
		return ""
	}
}

type alConditionalExpressionTokenKind uint8

const (
	alConditionalExpressionSymbol alConditionalExpressionTokenKind = iota
	alConditionalExpressionNot
	alConditionalExpressionAnd
	alConditionalExpressionOr
	alConditionalExpressionLeftParen
	alConditionalExpressionRightParen
)

type alConditionalExpressionToken struct {
	kind  alConditionalExpressionTokenKind
	value string
}

type alConditionalExpressionParser struct {
	tokens []alConditionalExpressionToken
	at     int
}

type alConditionalTruth uint8

const (
	alConditionalUnknown alConditionalTruth = iota
	alConditionalFalse
	alConditionalTrue
)

func alConditionalDirectiveExpression(text, keyword string) (*alConditionalExpr, error) {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < len(keyword) || !strings.EqualFold(trimmed[:len(keyword)], keyword) {
		return nil, fmt.Errorf("invalid conditional directive")
	}
	expression := strings.TrimSpace(trimmed[len(keyword):])
	if comment := strings.Index(expression, "//"); comment >= 0 {
		expression = strings.TrimSpace(expression[:comment])
	}
	if expression == "" {
		return nil, fmt.Errorf("conditional directive has no expression")
	}
	return parseALConditionalExpression(expression)
}

func parseALConditionalExpression(text string) (*alConditionalExpr, error) {
	tokens, err := tokenizeALConditionalExpression(text)
	if err != nil {
		return nil, err
	}
	parser := alConditionalExpressionParser{tokens: tokens}
	expression, err := parser.parseOr()
	if err != nil {
		return nil, err
	}
	if parser.at != len(parser.tokens) {
		return nil, fmt.Errorf("unexpected token %q in conditional expression", parser.tokens[parser.at].value)
	}
	return expression, nil
}

func tokenizeALConditionalExpression(text string) ([]alConditionalExpressionToken, error) {
	tokens := make([]alConditionalExpressionToken, 0, 8)
	for at := 0; at < len(text); {
		value := text[at]
		if value == ' ' || value == '\t' || value == '\r' || value == '\n' {
			at++
			continue
		}
		switch value {
		case '(':
			tokens = append(tokens, alConditionalExpressionToken{kind: alConditionalExpressionLeftParen, value: "("})
			at++
			continue
		case ')':
			tokens = append(tokens, alConditionalExpressionToken{kind: alConditionalExpressionRightParen, value: ")"})
			at++
			continue
		}
		if !isALConditionalSymbolStart(value) {
			return nil, fmt.Errorf("unsupported character %q in conditional expression", value)
		}
		start := at
		at++
		for at < len(text) && isALConditionalSymbolPart(text[at]) {
			at++
		}
		raw := text[start:at]
		normalized := strings.ToLower(raw)
		switch normalized {
		case "not":
			tokens = append(tokens, alConditionalExpressionToken{kind: alConditionalExpressionNot, value: raw})
		case "and":
			tokens = append(tokens, alConditionalExpressionToken{kind: alConditionalExpressionAnd, value: raw})
		case "or":
			tokens = append(tokens, alConditionalExpressionToken{kind: alConditionalExpressionOr, value: raw})
		default:
			tokens = append(tokens, alConditionalExpressionToken{kind: alConditionalExpressionSymbol, value: normalized})
		}
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("conditional expression is empty")
	}
	return tokens, nil
}

func isALConditionalSymbolStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func isALConditionalSymbolPart(value byte) bool {
	return isALConditionalSymbolStart(value) || value >= '0' && value <= '9'
}

func (parser *alConditionalExpressionParser) parseOr() (*alConditionalExpr, error) {
	left, err := parser.parseAnd()
	if err != nil {
		return nil, err
	}
	for parser.at < len(parser.tokens) && parser.tokens[parser.at].kind == alConditionalExpressionOr {
		parser.at++
		right, parseErr := parser.parseAnd()
		if parseErr != nil {
			return nil, parseErr
		}
		left = alConditionalOr(left, right)
	}
	return left, nil
}

func (parser *alConditionalExpressionParser) parseAnd() (*alConditionalExpr, error) {
	left, err := parser.parseUnary()
	if err != nil {
		return nil, err
	}
	for parser.at < len(parser.tokens) && parser.tokens[parser.at].kind == alConditionalExpressionAnd {
		parser.at++
		right, parseErr := parser.parseUnary()
		if parseErr != nil {
			return nil, parseErr
		}
		left = alConditionalAnd(left, right)
	}
	return left, nil
}

func (parser *alConditionalExpressionParser) parseUnary() (*alConditionalExpr, error) {
	if parser.at < len(parser.tokens) && parser.tokens[parser.at].kind == alConditionalExpressionNot {
		parser.at++
		value, err := parser.parseUnary()
		if err != nil {
			return nil, err
		}
		return alConditionalNot(value), nil
	}
	return parser.parsePrimary()
}

func (parser *alConditionalExpressionParser) parsePrimary() (*alConditionalExpr, error) {
	if parser.at >= len(parser.tokens) {
		return nil, fmt.Errorf("conditional expression ended unexpectedly")
	}
	token := parser.tokens[parser.at]
	parser.at++
	switch token.kind {
	case alConditionalExpressionSymbol:
		return &alConditionalExpr{kind: alConditionalExprSymbol, symbol: token.value}, nil
	case alConditionalExpressionLeftParen:
		value, err := parser.parseOr()
		if err != nil {
			return nil, err
		}
		if parser.at >= len(parser.tokens) || parser.tokens[parser.at].kind != alConditionalExpressionRightParen {
			return nil, fmt.Errorf("conditional expression has an unmatched parenthesis")
		}
		parser.at++
		return value, nil
	default:
		return nil, fmt.Errorf("unexpected token %q in conditional expression", token.value)
	}
}

func alConditionalNot(value *alConditionalExpr) *alConditionalExpr {
	if value == nil {
		return &alConditionalExpr{kind: alConditionalExprFalse}
	}
	switch value.kind {
	case alConditionalExprTrue:
		return &alConditionalExpr{kind: alConditionalExprFalse}
	case alConditionalExprFalse:
		return &alConditionalExpr{kind: alConditionalExprTrue}
	case alConditionalExprNot:
		return value.left
	default:
		return &alConditionalExpr{kind: alConditionalExprNot, left: value}
	}
}

func alConditionalAnd(left, right *alConditionalExpr) *alConditionalExpr {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if left.kind == alConditionalExprFalse || right.kind == alConditionalExprFalse {
		return &alConditionalExpr{kind: alConditionalExprFalse}
	}
	if left.kind == alConditionalExprTrue {
		return right
	}
	if right.kind == alConditionalExprTrue {
		return left
	}
	return &alConditionalExpr{kind: alConditionalExprAnd, left: left, right: right}
}

func alConditionalOr(left, right *alConditionalExpr) *alConditionalExpr {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if left.kind == alConditionalExprTrue || right.kind == alConditionalExprTrue {
		return &alConditionalExpr{kind: alConditionalExprTrue}
	}
	if left.kind == alConditionalExprFalse {
		return right
	}
	if right.kind == alConditionalExprFalse {
		return left
	}
	return &alConditionalExpr{kind: alConditionalExprOr, left: left, right: right}
}

func alConditionalVariantSelections(groups []alConditionalGroup) ([]map[int]int, *alConditionalIssue) {
	if len(groups) == 0 {
		return []map[int]int{{}}, nil
	}
	variants := make([][]*alConditionalExpr, 0, 4)
	for groupID, group := range groups {
		for branchID := range group.branches {
			requirement := alConditionalBranchRequirementExpr(groups, groupID, branchID)
			_, satisfiable, solveErr := alConditionalSolve([]*alConditionalExpr{requirement})
			if solveErr != nil {
				return nil, &alConditionalIssue{offset: group.openOffset, message: solveErr.Error()}
			}
			if !satisfiable {
				continue
			}
			placed := false
			for index := range variants {
				combined := append(append([]*alConditionalExpr(nil), variants[index]...), requirement)
				_, compatible, err := alConditionalSolve(combined)
				if err != nil {
					return nil, &alConditionalIssue{offset: group.openOffset, message: err.Error()}
				}
				if !compatible {
					continue
				}
				variants[index] = combined
				placed = true
				break
			}
			if placed {
				continue
			}
			if len(variants) >= alConditionalVariantLimit {
				return nil, &alConditionalIssue{
					offset:  group.openOffset,
					message: fmt.Sprintf("conditional branch coverage requires more than %d structural variants", alConditionalVariantLimit),
				}
			}
			variants = append(variants, []*alConditionalExpr{requirement})
		}
	}
	if len(variants) == 0 {
		return []map[int]int{{}}, nil
	}

	selections := make([]map[int]int, 0, len(variants))
	seen := make(map[string]struct{}, len(variants))
	for _, constraints := range variants {
		assignment, satisfiable, solveErr := alConditionalSolve(constraints)
		if solveErr != nil {
			return nil, &alConditionalIssue{message: solveErr.Error()}
		}
		if !satisfiable {
			continue
		}
		alConditionalFillUnassignedSymbols(groups, assignment)
		selection := alConditionalSelectionForAssignment(groups, assignment)
		key := alConditionalSelectionKey(groups, selection)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		selections = append(selections, selection)
	}
	if len(selections) == 0 {
		return []map[int]int{{}}, nil
	}
	return selections, nil
}

func alConditionalBranchRequirementExpr(groups []alConditionalGroup, groupID, branchID int) *alConditionalExpr {
	requirement := alConditionalBranchPredicate(groups[groupID], branchID)
	for groupID >= 0 {
		group := groups[groupID]
		if group.parentGroup < 0 {
			break
		}
		requirement = alConditionalAnd(requirement, alConditionalBranchPredicate(groups[group.parentGroup], group.parentBranch))
		groupID = group.parentGroup
	}
	return requirement
}

func alConditionalBranchPredicate(group alConditionalGroup, branchID int) *alConditionalExpr {
	if branchID < 0 || branchID >= len(group.branches) {
		return &alConditionalExpr{kind: alConditionalExprFalse}
	}
	predicate := &alConditionalExpr{kind: alConditionalExprTrue}
	for index := 0; index < branchID; index++ {
		branch := group.branches[index]
		if branch.elseBranch || branch.condition == nil {
			return &alConditionalExpr{kind: alConditionalExprFalse}
		}
		predicate = alConditionalAnd(predicate, alConditionalNot(branch.condition))
	}
	branch := group.branches[branchID]
	if branch.elseBranch {
		return predicate
	}
	if branch.condition == nil {
		return &alConditionalExpr{kind: alConditionalExprFalse}
	}
	return alConditionalAnd(predicate, branch.condition)
}

func alConditionalSolve(constraints []*alConditionalExpr) (map[string]bool, bool, error) {
	symbolSet := make(map[string]struct{})
	for _, constraint := range constraints {
		alConditionalCollectSymbols(constraint, symbolSet)
	}
	symbols := make([]string, 0, len(symbolSet))
	for symbol := range symbolSet {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	assignment := make(map[string]bool, len(symbols))
	nodes := 0
	var solve func() bool
	solve = func() bool {
		nodes++
		if nodes > alConditionalSolverNodeLimit {
			return false
		}
		state := alConditionalConstraintTruth(constraints, assignment)
		if state == alConditionalFalse {
			return false
		}
		if state == alConditionalTrue {
			return true
		}
		symbol := ""
		for _, candidate := range symbols {
			if _, assigned := assignment[candidate]; !assigned {
				symbol = candidate
				break
			}
		}
		if symbol == "" {
			return false
		}
		assignment[symbol] = false
		if solve() {
			return true
		}
		assignment[symbol] = true
		if solve() {
			return true
		}
		delete(assignment, symbol)
		return false
	}
	satisfiable := solve()
	if nodes > alConditionalSolverNodeLimit {
		return nil, false, fmt.Errorf("conditional expression solving exceeded the %d-node limit", alConditionalSolverNodeLimit)
	}
	if !satisfiable {
		return nil, false, nil
	}
	for _, symbol := range symbols {
		if _, assigned := assignment[symbol]; !assigned {
			assignment[symbol] = false
		}
	}
	return assignment, true, nil
}

func alConditionalConstraintTruth(constraints []*alConditionalExpr, assignment map[string]bool) alConditionalTruth {
	unknown := false
	for _, constraint := range constraints {
		switch alConditionalEvaluate(constraint, assignment) {
		case alConditionalFalse:
			return alConditionalFalse
		case alConditionalUnknown:
			unknown = true
		}
	}
	if unknown {
		return alConditionalUnknown
	}
	return alConditionalTrue
}

func alConditionalEvaluate(expression *alConditionalExpr, assignment map[string]bool) alConditionalTruth {
	if expression == nil {
		return alConditionalFalse
	}
	switch expression.kind {
	case alConditionalExprFalse:
		return alConditionalFalse
	case alConditionalExprTrue:
		return alConditionalTrue
	case alConditionalExprSymbol:
		value, ok := assignment[expression.symbol]
		if !ok {
			return alConditionalUnknown
		}
		if value {
			return alConditionalTrue
		}
		return alConditionalFalse
	case alConditionalExprNot:
		switch alConditionalEvaluate(expression.left, assignment) {
		case alConditionalTrue:
			return alConditionalFalse
		case alConditionalFalse:
			return alConditionalTrue
		default:
			return alConditionalUnknown
		}
	case alConditionalExprAnd:
		left := alConditionalEvaluate(expression.left, assignment)
		right := alConditionalEvaluate(expression.right, assignment)
		if left == alConditionalFalse || right == alConditionalFalse {
			return alConditionalFalse
		}
		if left == alConditionalTrue && right == alConditionalTrue {
			return alConditionalTrue
		}
		return alConditionalUnknown
	case alConditionalExprOr:
		left := alConditionalEvaluate(expression.left, assignment)
		right := alConditionalEvaluate(expression.right, assignment)
		if left == alConditionalTrue || right == alConditionalTrue {
			return alConditionalTrue
		}
		if left == alConditionalFalse && right == alConditionalFalse {
			return alConditionalFalse
		}
		return alConditionalUnknown
	default:
		return alConditionalFalse
	}
}

func alConditionalCollectSymbols(expression *alConditionalExpr, result map[string]struct{}) {
	if expression == nil {
		return
	}
	if expression.kind == alConditionalExprSymbol {
		result[expression.symbol] = struct{}{}
		return
	}
	alConditionalCollectSymbols(expression.left, result)
	alConditionalCollectSymbols(expression.right, result)
}

func alConditionalFillUnassignedSymbols(groups []alConditionalGroup, assignment map[string]bool) {
	symbols := make(map[string]struct{})
	for _, group := range groups {
		for _, branch := range group.branches {
			alConditionalCollectSymbols(branch.condition, symbols)
		}
	}
	for symbol := range symbols {
		if _, assigned := assignment[symbol]; !assigned {
			assignment[symbol] = false
		}
	}
}

func alConditionalSelectionForAssignment(groups []alConditionalGroup, assignment map[string]bool) map[int]int {
	selection := make(map[int]int, len(groups))
	for groupID, group := range groups {
		if group.parentGroup >= 0 {
			parentBranch, active := selection[group.parentGroup]
			if !active || parentBranch != group.parentBranch {
				continue
			}
		}
		for branchID, branch := range group.branches {
			if branch.elseBranch {
				selection[groupID] = branchID
				break
			}
			if alConditionalEvaluate(branch.condition, assignment) == alConditionalTrue {
				selection[groupID] = branchID
				break
			}
		}
	}
	return selection
}

func alConditionalSelectionKey(groups []alConditionalGroup, selection map[int]int) string {
	var builder strings.Builder
	for groupID := range groups {
		if branch, ok := selection[groupID]; ok {
			fmt.Fprintf(&builder, "%d:%d;", groupID, branch)
		} else {
			fmt.Fprintf(&builder, "%d:-;", groupID)
		}
	}
	return builder.String()
}

func alConditionalVariant(document *SourceDocument, plan alConditionalPlan, selection map[int]int) *SourceDocument {
	masked := []byte(document.Text)
	for _, directive := range plan.directives {
		alMaskSourceRange(masked, directive.Start, directive.End)
	}
	for groupID, group := range plan.groups {
		selected, active := selection[groupID]
		for branchID, branch := range group.branches {
			if active && branchID == selected {
				continue
			}
			alMaskSourceRange(masked, branch.start, branch.end)
		}
	}
	clone := *document
	clone.Text = string(masked)
	clone.lineStarts = document.lineStarts
	return &clone
}

func alMaskSourceRange(text []byte, start, end int) {
	start = max(0, start)
	end = min(len(text), end)
	for index := start; index < end; index++ {
		if text[index] != '\r' && text[index] != '\n' {
			text[index] = ' '
		}
	}
}

func alConditionalIssueResult(ctx context.Context, document *SourceDocument, options AnalyzeOptions, issue *alConditionalIssue) AnalyzerResult {
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: "al", Analyzer: string(AnalyzerAL), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if builder.validationErr != nil {
		return AnalyzerResult{Analysis: builder.Result()}
	}
	start := max(0, min(issue.offset, len(document.Text)))
	end := start
	if end < len(document.Text) {
		end++
	}
	var value *OffsetRange
	if end > start {
		rangeValue := OffsetRange{Start: start, End: end}
		value = &rangeValue
	}
	_ = builder.AddDiagnostic(DiagnosticSpec{
		Code: "al-conditional-directive", Message: issue.message, Severity: DiagnosticWarning,
		Range: value, AffectsCoverage: true,
	})
	return AnalyzerResult{Analysis: builder.Result()}
}

type alConditionalMerge struct {
	limits        SymbolBuilderLimits
	result        AnalyzerResult
	symbols       map[string]NormalizedSymbol
	dependencies  map[string]struct{}
	relations     map[string]struct{}
	diagnostics   map[string]struct{}
	symbolLimit   bool
	depLimit      bool
	relationLimit bool
}

func newALConditionalMerge(limits SymbolBuilderLimits) *alConditionalMerge {
	return &alConditionalMerge{
		limits:       limits,
		result:       AnalyzerResult{Analysis: AnalysisResult{CoverageComplete: true}},
		symbols:      make(map[string]NormalizedSymbol),
		dependencies: make(map[string]struct{}),
		relations:    make(map[string]struct{}),
		diagnostics:  make(map[string]struct{}),
	}
}

type alConditionalSymbolValue struct {
	ID                  string
	Path                string
	Language            string
	Kind                SymbolKind
	NativeKind          string
	Name                string
	QualifiedName       string
	ParentID            string
	ParentQualifiedName string
	RegionID            string
	DeclarationRange    Range
	NameRange           Range
	Signature           string
	Visibility          Visibility
	Evidence            SymbolEvidence
	Analyzer            string
	SignatureTruncated  bool
}

func alConditionalSymbolValueOf(symbol NormalizedSymbol) alConditionalSymbolValue {
	return alConditionalSymbolValue{
		ID: symbol.ID, Path: symbol.Path, Language: symbol.Language, Kind: symbol.Kind, NativeKind: symbol.NativeKind,
		Name: symbol.Name, QualifiedName: symbol.QualifiedName, ParentID: symbol.ParentID, ParentQualifiedName: symbol.ParentQualifiedName,
		RegionID: symbol.RegionID, DeclarationRange: symbol.DeclarationRange, NameRange: symbol.NameRange, Signature: symbol.Signature,
		Visibility: symbol.Visibility, Evidence: symbol.Evidence, Analyzer: symbol.Analyzer, SignatureTruncated: symbol.signatureTruncated,
	}
}

func alConditionalSymbolsEqual(left, right NormalizedSymbol) bool {
	if alConditionalSymbolValueOf(left) != alConditionalSymbolValueOf(right) || !left.sameSourceOffsets(right) {
		return false
	}
	if !alConditionalRangePointersEqual(left.SignatureRange, right.SignatureRange) ||
		!alConditionalRangePointersEqual(left.BodyRange, right.BodyRange) {
		return false
	}
	return alConditionalStringSlicesEqual(left.Modifiers, right.Modifiers)
}

func alConditionalStringSlicesEqual(left, right []string) bool {
	if (left == nil) != (right == nil) || len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func alConditionalRangePointersEqual(left, right *Range) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func (merge *alConditionalMerge) add(variant AnalyzerResult) {
	if !variant.Analysis.CoverageComplete {
		merge.result.Analysis.CoverageComplete = false
	}
	if variant.Analysis.Truncated {
		merge.result.Analysis.Truncated = true
		merge.result.Analysis.CoverageComplete = false
	}
	if variant.Analysis.DiagnosticsTruncated {
		merge.result.Analysis.DiagnosticsTruncated = true
		merge.result.Analysis.CoverageComplete = false
	}

	symbolLimit := max(1, merge.limits.MaxSymbols)
	for _, symbol := range variant.Analysis.Symbols {
		if existing, ok := merge.symbols[symbol.ID]; ok {
			if !alConditionalSymbolsEqual(existing, symbol) {
				merge.result.Analysis.CoverageComplete = false
				merge.addDiagnostic(AnalysisDiagnostic{Code: "al-conditional-symbol-conflict", Message: "conditional variants produced conflicting data for one symbol identity", Severity: DiagnosticWarning})
			}
			continue
		}
		if len(merge.result.Analysis.Symbols) >= symbolLimit {
			if !merge.symbolLimit {
				merge.symbolLimit = true
				merge.markRetentionLimit("symbol-limit", "symbol retention limit reached")
			}
			continue
		}
		copySymbol := cloneNormalizedSymbol(symbol)
		merge.symbols[symbol.ID] = copySymbol
		merge.result.Analysis.Symbols = append(merge.result.Analysis.Symbols, copySymbol)
	}

	for _, diagnostic := range variant.Analysis.Diagnostics {
		merge.addDiagnostic(diagnostic)
	}
	for _, dependency := range variant.Dependencies {
		key := alDependencyKey(dependency)
		if _, ok := merge.dependencies[key]; ok {
			continue
		}
		if len(merge.result.Dependencies) >= symbolLimit {
			if !merge.depLimit {
				merge.depLimit = true
				merge.markRetentionLimit("dependency-limit", "dependency retention limit reached")
			}
			continue
		}
		merge.dependencies[key] = struct{}{}
		merge.result.Dependencies = append(merge.result.Dependencies, dependency)
	}
	for _, relation := range variant.Relations {
		key := alRelationKey(relation)
		if _, ok := merge.relations[key]; ok {
			continue
		}
		if len(merge.result.Relations) >= symbolLimit {
			if !merge.relationLimit {
				merge.relationLimit = true
				merge.markRetentionLimit("relation-limit", "relation retention limit reached")
			}
			continue
		}
		merge.relations[key] = struct{}{}
		merge.result.Relations = append(merge.result.Relations, relation)
	}
}

func (merge *alConditionalMerge) markRetentionLimit(code, message string) {
	merge.result.Analysis.Truncated = true
	merge.result.Analysis.CoverageComplete = false
	merge.addDiagnostic(AnalysisDiagnostic{Code: code, Message: message, Severity: DiagnosticWarning})
}

func (merge *alConditionalMerge) addDiagnostic(diagnostic AnalysisDiagnostic) {
	key := alDiagnosticKey(diagnostic)
	if _, ok := merge.diagnostics[key]; ok {
		return
	}
	merge.diagnostics[key] = struct{}{}
	if len(merge.result.Analysis.Diagnostics) >= max(1, merge.limits.MaxDiagnostics) {
		merge.result.Analysis.DiagnosticsTruncated = true
		merge.result.Analysis.CoverageComplete = false
		return
	}
	if diagnostic.Range != nil {
		copyRange := *diagnostic.Range
		diagnostic.Range = &copyRange
	}
	merge.result.Analysis.Diagnostics = append(merge.result.Analysis.Diagnostics, diagnostic)
}

func (merge *alConditionalMerge) finish() AnalyzerResult {
	sort.Slice(merge.result.Analysis.Symbols, func(i, j int) bool {
		left := merge.result.Analysis.Symbols[i]
		right := merge.result.Analysis.Symbols[j]
		if left.declarationOffsets.Start != right.declarationOffsets.Start {
			return left.declarationOffsets.Start < right.declarationOffsets.Start
		}
		if left.declarationOffsets.End != right.declarationOffsets.End {
			return left.declarationOffsets.End < right.declarationOffsets.End
		}
		return left.ID < right.ID
	})
	sort.Slice(merge.result.Dependencies, func(i, j int) bool {
		left := merge.result.Dependencies[i]
		right := merge.result.Dependencies[j]
		if left.Range.Start.Line != right.Range.Start.Line {
			return left.Range.Start.Line < right.Range.Start.Line
		}
		if left.Range.Start.Column != right.Range.Start.Column {
			return left.Range.Start.Column < right.Range.Start.Column
		}
		return alDependencyKey(left) < alDependencyKey(right)
	})
	sort.Slice(merge.result.Relations, func(i, j int) bool {
		left := merge.result.Relations[i]
		right := merge.result.Relations[j]
		if left.Range.Start.Line != right.Range.Start.Line {
			return left.Range.Start.Line < right.Range.Start.Line
		}
		if left.Range.Start.Column != right.Range.Start.Column {
			return left.Range.Start.Column < right.Range.Start.Column
		}
		return alRelationKey(left) < alRelationKey(right)
	})
	sort.Slice(merge.result.Analysis.Diagnostics, func(i, j int) bool {
		return alDiagnosticKey(merge.result.Analysis.Diagnostics[i]) < alDiagnosticKey(merge.result.Analysis.Diagnostics[j])
	})
	return merge.result
}

func alDiagnosticKey(diagnostic AnalysisDiagnostic) string {
	key := diagnostic.Code + "\x00" + diagnostic.Message
	if diagnostic.Range != nil {
		key += fmt.Sprintf("\x00%d:%d:%d:%d", diagnostic.Range.Start.Line, diagnostic.Range.Start.Column, diagnostic.Range.End.Line, diagnostic.Range.End.Column)
	}
	return key
}

func alDependencyKey(value StructuralDependency) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%d:%d:%d:%d\x00%s", value.Kind, value.Value, value.Alias, value.Range.Start.Line, value.Range.Start.Column, value.Range.End.Line, value.Range.End.Column, value.Evidence)
}

func alRelationKey(value StructuralRelation) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%d:%d:%d:%d\x00%s", value.Kind, value.Source, value.Target, value.Range.Start.Line, value.Range.Start.Column, value.Range.End.Line, value.Range.End.Column, value.Evidence)
}
