package sourceintelligence

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type FortranAnalyzer struct{}
type COBOLAnalyzer struct{}
type AdaAnalyzer struct{}

func (FortranAnalyzer) ID() AnalyzerID   { return AnalyzerFortran }
func (FortranAnalyzer) Language() string { return "fortran" }
func (COBOLAnalyzer) ID() AnalyzerID     { return AnalyzerCOBOL }
func (COBOLAnalyzer) Language() string   { return "cobol" }
func (AdaAnalyzer) ID() AnalyzerID       { return AnalyzerAda }
func (AdaAnalyzer) Language() string     { return "ada" }

func fortranLooksFreeForm(text string) bool {
	const maxCodeLines = 256
	codeLines := 0
	for start := 0; start < len(text) && codeLines < maxCodeLines; {
		end := strings.IndexByte(text[start:], '\n')
		if end < 0 {
			end = len(text)
		} else {
			end += start
		}
		line := strings.TrimSuffix(text[start:end], "\r")
		first := 0
		for first < len(line) && line[first] == ' ' {
			first++
		}
		if first < len(line) {
			trimmed := line[first:]
			if trimmed[0] != '!' && trimmed[0] != '#' && trimmed[0] != '*' {
				codeLines++
				if first < 5 {
					wordEnd := 0
					for wordEnd < len(trimmed) && ((trimmed[wordEnd] >= 'A' && trimmed[wordEnd] <= 'Z') || (trimmed[wordEnd] >= 'a' && trimmed[wordEnd] <= 'z') || trimmed[wordEnd] == '_') {
						wordEnd++
					}
					switch strings.ToLower(trimmed[:wordEnd]) {
					case "module", "submodule", "program", "use", "implicit", "private", "public", "contains", "subroutine", "function", "type", "interface", "block", "select":
						return true
					}
				}
			}
		}
		if end >= len(text) {
			break
		}
		start = end + 1
	}
	return false
}

func (FortranAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if document == nil || ctx != nil && ctx.Err() != nil {
		return analyzeFortranSingle(ctx, document, options)
	}
	plan := planFortranConditionals(document.Text)
	if plan.issue != nil {
		result, err := analyzeFortranSingle(ctx, document, options)
		if err != nil {
			return AnalyzerResult{}, err
		}
		result.Analysis.CoverageComplete = false
		rangeValue, rangeErr := document.RangeFromUTF8Offsets(plan.issue.Start, plan.issue.End)
		var normalized *Range
		if rangeErr == nil {
			normalized = &rangeValue
		}
		result.Analysis.Diagnostics = append(result.Analysis.Diagnostics, AnalysisDiagnostic{
			Code: "fortran-malformed-conditional-preprocessor", Message: plan.message, Severity: DiagnosticWarning, Range: normalized,
		})
		return result, nil
	}
	if len(plan.groups) == 0 {
		return analyzeFortranSingle(ctx, document, options)
	}
	selections, ok := conditionalSelections(plan.groups)
	if !ok {
		result, err := analyzeFortranSingle(ctx, document, options)
		if err != nil {
			return AnalyzerResult{}, err
		}
		result.Analysis.CoverageComplete = false
		result.Analysis.Diagnostics = append(result.Analysis.Diagnostics, AnalysisDiagnostic{
			Code: "fortran-conditional-variant-limit", Message: "conditional preprocessing requires more than 32 bounded structural variants", Severity: DiagnosticWarning,
		})
		return result, nil
	}
	variants := make([]AnalyzerResult, 0, len(selections))
	for _, selection := range selections {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		masked := maskConditionalVariant(document.Text, plan, selection)
		clone := *document
		clone.Text = masked
		clone.lineStarts = buildLineStarts(masked)
		variant, err := analyzeFortranSingle(ctx, &clone, options)
		if err != nil {
			return AnalyzerResult{}, err
		}
		variants = append(variants, variant)
	}
	return mergeFortranConditionalVariants(options, variants), nil
}

func analyzeFortranSingle(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "fortran", AnalyzerFortran)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scanDocument := document
	if masked := maskFortranFyppDirectives(document.Text); masked != document.Text {
		clone := *document
		clone.Text = masked
		clone.lineStarts = buildLineStarts(masked)
		scanDocument = &clone
	}
	if strings.EqualFold(filepath.Ext(document.Path), ".f") && !fortranLooksFreeForm(scanDocument.Text) {
		fixedDocument := scanDocument
		if masked := maskFortranFixedVendorDirectiveContinuations(scanDocument.Text); masked != scanDocument.Text {
			clone := *scanDocument
			clone.Text = masked
			clone.lineStarts = buildLineStarts(masked)
			fixedDocument = &clone
		}
		return analyzeFortranFixed(ctx, fixedDocument, options, builder)
	}
	scan, lines, err := scanAnalyzerLogicalLines(ctx, scanDocument, FortranScannerProfile(), options.MaxNesting)
	if err != nil {
		return AnalyzerResult{}, err
	}
	applyStructuralScanDiagnostics(builder, scan, "fortran")
	dependencies := []StructuralDependency{}
	var scopes []structuralAnalyzerScope
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		parseFortranTokens(document, builder, line.Tokens, line.StartOffset, line.EndOffset, &scopes, &dependencies)
	}
	markUnclosedStructuralScopes(builder, "fortran", scopes)
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func maskFortranFyppDirectives(text string) string {
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
	continuation := false
	for start := 0; start < len(text); {
		contentEnd := start
		for contentEnd < len(text) && text[contentEnd] != '\r' && text[contentEnd] != '\n' {
			contentEnd++
		}
		trimmed := strings.TrimLeft(text[start:contentEnd], " \t")
		fypp := strings.HasPrefix(trimmed, "#:")
		if fypp || continuation {
			mask(start, contentEnd)
			continuation = strings.HasSuffix(strings.TrimSpace(text[start:contentEnd]), "&")
		} else {
			continuation = false
		}
		if contentEnd >= len(text) {
			break
		}
		if text[contentEnd] == '\r' && contentEnd+1 < len(text) && text[contentEnd+1] == '\n' {
			start = contentEnd + 2
		} else {
			start = contentEnd + 1
		}
	}
	for search := 0; search < len(text); {
		openRelative := strings.Index(text[search:], "#{")
		if openRelative < 0 {
			break
		}
		open := search + openRelative
		closeRelative := strings.Index(text[open+2:], "}#")
		if closeRelative < 0 {
			break
		}
		end := open + 2 + closeRelative + 2
		mask(open, end)
		search = end
	}
	if masked == nil {
		return text
	}
	return string(masked)
}

func maskFortranFixedVendorDirectiveContinuations(text string) string {
	var masked []byte
	maskLine := func(start, end int) {
		if masked == nil {
			masked = []byte(text)
		}
		for index := start; index < end; index++ {
			masked[index] = ' '
		}
	}
	vendorDirective := false
	for start := 0; start < len(text); {
		contentEnd := start
		for contentEnd < len(text) && text[contentEnd] != '\r' && text[contentEnd] != '\n' {
			contentEnd++
		}
		trimmed := strings.TrimLeft(text[start:contentEnd], " \t")
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "!dir$"):
			vendorDirective = true
		case vendorDirective && strings.HasPrefix(trimmed, "&"):
			maskLine(start, contentEnd)
			vendorDirective = true
		default:
			vendorDirective = false
		}
		if contentEnd >= len(text) {
			break
		}
		if text[contentEnd] == '\r' && contentEnd+1 < len(text) && text[contentEnd+1] == '\n' {
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

func planFortranConditionals(text string) conditionalPlan {
	plan := conditionalPlan{}
	stack := make([]conditionalFrame, 0, 8)
	for start := 0; start < len(text); {
		contentEnd := start
		for contentEnd < len(text) && text[contentEnd] != '\r' && text[contentEnd] != '\n' {
			contentEnd++
		}
		next := contentEnd
		if next < len(text) && text[next] == '\r' && next+1 < len(text) && text[next+1] == '\n' {
			next += 2
		} else if next < len(text) {
			next++
		}
		trimmed := strings.TrimLeft(text[start:contentEnd], " \t")
		keyword := ""
		if strings.HasPrefix(trimmed, "#") {
			rest := strings.TrimLeft(trimmed[1:], " \t")
			end := 0
			for end < len(rest) && (rest[end] == '_' || rest[end] >= 'A' && rest[end] <= 'Z' || rest[end] >= 'a' && rest[end] <= 'z') {
				end++
			}
			if end > 0 {
				keyword = "#" + strings.ToLower(rest[:end])
			}
		}
		switch keyword {
		case "#if", "#ifdef", "#ifndef":
			parentGroup, parentBranch := -1, -1
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parentGroup, parentBranch = parent.group, parent.branch
			}
			group := conditionalGroup{parentGroup: parentGroup, parentBranch: parentBranch, branches: []conditionalBranch{{start: next}}}
			plan.groups = append(plan.groups, group)
			stack = append(stack, conditionalFrame{group: len(plan.groups) - 1})
			plan.directives = append(plan.directives, OffsetRange{Start: start, End: contentEnd})
		case "#elif", "#else":
			plan.directives = append(plan.directives, OffsetRange{Start: start, End: contentEnd})
			if len(stack) == 0 {
				plan.issue = &OffsetRange{Start: start, End: contentEnd}
				plan.message = "conditional branch directive has no matching opener"
				return plan
			}
			frame := &stack[len(stack)-1]
			if frame.seenElse {
				plan.issue = &OffsetRange{Start: start, End: contentEnd}
				plan.message = "conditional branch appears after #else"
				return plan
			}
			group := &plan.groups[frame.group]
			group.branches[frame.branch].end = start
			group.branches = append(group.branches, conditionalBranch{start: next})
			frame.branch = len(group.branches) - 1
			if keyword == "#else" {
				frame.seenElse = true
			}
		case "#endif":
			plan.directives = append(plan.directives, OffsetRange{Start: start, End: contentEnd})
			if len(stack) == 0 {
				plan.issue = &OffsetRange{Start: start, End: contentEnd}
				plan.message = "conditional #endif has no matching opener"
				return plan
			}
			frame := stack[len(stack)-1]
			plan.groups[frame.group].branches[frame.branch].end = start
			stack = stack[:len(stack)-1]
		}
		start = next
	}
	if len(stack) > 0 {
		frame := stack[len(stack)-1]
		group := plan.groups[frame.group]
		plan.issue = &OffsetRange{Start: group.branches[0].start, End: group.branches[0].start}
		plan.message = "conditional block is not terminated by #endif"
	}
	return plan
}

type fortranLogicalSymbolKey struct {
	kind          SymbolKind
	qualifiedName string
}

func mergeFortranConditionalVariants(options AnalyzeOptions, variants []AnalyzerResult) AnalyzerResult {
	merged := AnalyzerResult{Analysis: AnalysisResult{CoverageComplete: true}}
	seenIDs := make(map[string]struct{})
	seenLogical := make(map[fortranLogicalSymbolKey]struct{})
	var legacySeenLogical map[string]struct{}
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
			if _, exists := seenIDs[symbol.ID]; exists {
				continue
			}
			if symbol.QualifiedName != "" {
				kind := string(symbol.Kind)
				if strings.IndexByte(kind, 0) >= 0 || strings.IndexByte(symbol.QualifiedName, 0) >= 0 {
					if legacySeenLogical == nil {
						legacySeenLogical = make(map[string]struct{})
					}
					logical := kind + "\x00" + symbol.QualifiedName
					if _, exists := legacySeenLogical[logical]; exists {
						continue
					}
					legacySeenLogical[logical] = struct{}{}
				} else {
					logical := fortranLogicalSymbolKey{kind: symbol.Kind, qualifiedName: symbol.QualifiedName}
					if _, exists := seenLogical[logical]; exists {
						continue
					}
					seenLogical[logical] = struct{}{}
				}
			}
			if len(merged.Analysis.Symbols) >= options.Limits.MaxSymbols {
				merged.Analysis.Truncated = true
				merged.Analysis.CoverageComplete = false
				continue
			}
			seenIDs[symbol.ID] = struct{}{}
			merged.Analysis.Symbols = append(merged.Analysis.Symbols, symbol)
		}
		merged.Dependencies = appendUniqueDependencies(merged.Dependencies, variant.Dependencies)
		for _, diagnostic := range variant.Analysis.Diagnostics {
			if len(merged.Analysis.Diagnostics) >= options.Limits.MaxDiagnostics {
				merged.Analysis.DiagnosticsTruncated = true
				merged.Analysis.CoverageComplete = false
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
	return merged
}

func normalizeFortranFixedLines(text string, lines []SourceLine) {
	for index := range lines {
		line := &lines[index]
		if line.Physical.End <= line.Physical.Start {
			continue
		}
		raw := text[line.Physical.Start:line.Physical.End]
		labelEnd := min(len(raw), scalarColumnOffset(raw, 7))
		if strings.HasPrefix(strings.TrimLeft(raw[:labelEnd], " \t"), "!") {
			line.Comment = true
			line.Code = OffsetRange{Start: line.Physical.End, End: line.Physical.End}
			continue
		}
		if line.Comment {
			continue
		}
		if line.Code.End > line.Code.Start && strings.HasPrefix(strings.TrimLeft(text[line.Code.Start:line.Code.End], " \t"), "!") {
			line.Comment = true
			line.Code = OffsetRange{Start: line.Physical.End, End: line.Physical.End}
			continue
		}
		if tab := strings.IndexByte(raw, '\t'); tab >= 0 && tab < 6 && strings.Trim(raw[:tab], " ") == "" {
			cursor := tab + 1
			continuation := false
			if cursor < len(raw) && raw[cursor] >= '1' && raw[cursor] <= '9' {
				continuation = true
				cursor++
			}
			for cursor < len(raw) && (raw[cursor] == ' ' || raw[cursor] == '\t') {
				cursor++
			}
			line.Label = OffsetRange{}
			line.Continuation = continuation
			line.Code = trimHorizontalRange(text, OffsetRange{Start: line.Physical.Start + cursor, End: line.Physical.End})
			if line.Code.End > line.Code.Start && text[line.Code.Start] == '!' {
				line.Comment = true
				line.Code = OffsetRange{Start: line.Physical.End, End: line.Physical.End}
			}
			continue
		}

		standardEnd := line.Physical.Start + scalarColumnOffset(raw, 73)
		extendedEnd := line.Physical.Start + scalarColumnOffset(raw, 133)
		if extendedEnd <= standardEnd || standardEnd >= line.Physical.End {
			continue
		}
		tail := strings.TrimSpace(text[standardEnd:extendedEnd])
		if tail == "" || fortranSequenceField(tail) {
			continue
		}
		line.Code = trimHorizontalRange(text, OffsetRange{Start: line.Code.Start, End: extendedEnd})
	}
}

func fortranSequenceField(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

func analyzeFortranFixed(ctx context.Context, document *SourceDocument, options AnalyzeOptions, builder *SymbolBuilder) (AnalyzerResult, error) {
	lines, err := BuildSourceLines(ctx, document, LineModelProfile{Kind: LineModelFixed, Fixed: FixedLineProfile{
		CommentColumnOne: []string{"C", "c", "*", "!"}, LabelStartColumn: 1, LabelEndColumn: 6,
		ContinuationColumn: 6, CodeStartColumn: 7, CodeEndColumn: 73,
	}}, LineModelLimits{MaxLines: max(4096, len(document.Text)+1), MaxLineBytes: 1024 * 1024})
	if err != nil {
		return AnalyzerResult{}, err
	}
	normalizeFortranFixedLines(document.Text, lines)
	dependencies := []StructuralDependency{}
	var scopes []structuralAnalyzerScope
	var statementRanges []OffsetRange
	statementStart := -1
	statementEnd := -1
	flush := func() error {
		if statementStart < 0 || len(statementRanges) == 0 {
			statementRanges = statementRanges[:0]
			statementStart = -1
			statementEnd = -1
			return nil
		}
		text := maskedFortranFixedStatement(document.Text, statementStart, statementEnd, statementRanges)
		fake := &SourceDocument{Path: document.Path, Text: text, Encoding: "utf-8", lineStarts: buildLineStarts(text)}
		scan, logical, scanErr := scanAnalyzerLogicalLines(ctx, fake, FortranScannerProfile(), options.MaxNesting)
		if scanErr != nil {
			return scanErr
		}
		if !scan.Complete {
			builder.MarkIncomplete()
		}
		if len(logical) > 0 {
			tokens := append([]Token(nil), logical[0].Tokens...)
			for index := range tokens {
				tokens[index].StartOffset += statementStart
				tokens[index].EndOffset += statementStart
			}
			parseFortranTokens(document, builder, tokens, statementStart, statementEnd, &scopes, &dependencies)
		}
		statementRanges = statementRanges[:0]
		statementStart = -1
		statementEnd = -1
		return nil
	}
	for _, line := range lines {
		if line.Comment || line.Code.End <= line.Code.Start {
			continue
		}
		rawPhysical := strings.TrimLeft(document.Text[line.Physical.Start:line.Physical.End], " \t")
		if strings.HasPrefix(rawPhysical, "#") {
			continue
		}
		if !line.Continuation {
			if err := flush(); err != nil {
				return AnalyzerResult{}, err
			}
			statementStart = line.Code.Start
		} else if statementStart < 0 {
			builder.MarkIncomplete()
			statementStart = line.Code.Start
		}
		statementRanges = append(statementRanges, line.Code)
		statementEnd = line.Code.End
	}
	if err := flush(); err != nil {
		return AnalyzerResult{}, err
	}
	markUnclosedStructuralScopes(builder, "fortran", scopes)
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func maskedFortranFixedStatement(text string, start, end int, ranges []OffsetRange) string {
	if start < 0 || end < start || end > len(text) {
		return ""
	}
	result := make([]byte, end-start)
	for index := range result {
		result[index] = ' '
	}
	var quote byte
	for _, value := range ranges {
		if value.Start < start || value.End > end || value.End <= value.Start {
			continue
		}
		for index := value.Start; index < value.End; index++ {
			current := text[index]
			if quote == 0 {
				if hollerithEnd, ok := fortranFixedHollerithEnd(text, value.Start, index, value.End); ok {
					index = hollerithEnd - 1
					continue
				}
			}
			if quote != 0 {
				result[index-start] = current
				if current == quote {
					if index+1 < value.End && text[index+1] == quote {
						index++
						result[index-start] = text[index]
						continue
					}
					quote = 0
				}
				continue
			}
			if current == '\'' || current == '"' {
				quote = current
				result[index-start] = current
				continue
			}
			if current == '!' {
				break
			}
			result[index-start] = current
		}
	}
	return string(result)
}

func fortranFixedHollerithEnd(text string, rangeStart, start, end int) (int, bool) {
	if start < rangeStart || start >= end || end > len(text) || text[start] < '0' || text[start] > '9' {
		return 0, false
	}
	if start > rangeStart {
		previous := text[start-1]
		if previous == '_' || previous >= '0' && previous <= '9' || previous >= 'A' && previous <= 'Z' || previous >= 'a' && previous <= 'z' {
			return 0, false
		}
	}
	count := 0
	marker := start
	for marker < end && text[marker] >= '0' && text[marker] <= '9' {
		digit := int(text[marker] - '0')
		if count > (end-start-digit)/10 {
			return 0, false
		}
		count = count*10 + digit
		marker++
	}
	if count <= 0 || marker >= end || text[marker] != 'H' && text[marker] != 'h' {
		return 0, false
	}
	payloadEnd := marker + 1
	for remaining := count; remaining > 0; remaining-- {
		if payloadEnd >= end {
			return 0, false
		}
		_, size := utf8.DecodeRuneInString(text[payloadEnd:end])
		if size <= 0 {
			return 0, false
		}
		payloadEnd += size
	}
	return payloadEnd, true
}

func parseFortranTokens(document *SourceDocument, builder *SymbolBuilder, tokens []Token, start, end int, scopes *[]structuralAnalyzerScope, dependencies *[]StructuralDependency) {
	if len(tokens) == 0 {
		return
	}
	first := fortranStatementKeyword(tokens[0].Text)
	if first == "end" {
		if len(tokens) == 1 {
			*scopes = popStructuralScope(*scopes, "", true)
			return
		}
		label := fortranStatementKeyword(tokens[1].Text)
		if label == "module" || label == "program" || label == "submodule" || label == "type" || label == "subroutine" || label == "function" {
			*scopes = popStructuralScope(*scopes, label, true)
		}
		return
	}
	if label, ok := fortranCompactEndLabel(first); ok {
		*scopes = popStructuralScope(*scopes, label, true)
		return
	}
	if first == "use" {
		name := -1
		for index := 1; index < len(tokens); index++ {
			if strings.EqualFold(tokens[index].Text, "intrinsic") || strings.EqualFold(tokens[index].Text, "non_intrinsic") {
				continue
			}
			if tokens[index].Kind == TokenIdentifier {
				name = index
				break
			}
		}
		if name >= 0 {
			addStructuralDependency(document, dependencies, StructuralDependencyImport, tokens[name].Text, tokens[name].StartOffset, tokens[name].EndOffset)
		}
		return
	}
	parent := parentFromStructuralScopes(*scopes)
	if first == "module" && len(tokens) > 1 {
		second := fortranStatementKeyword(tokens[1].Text)
		if second == "procedure" {
			return
		}
		if second == "subroutine" || second == "function" {
			parseFortranProcedure(builder, tokens, start, end, 1, parent, scopes)
			return
		}
	}
	switch first {
	case "module", "submodule", "program":
		nameIndex := firstIdentifierToken(tokens, 1)
		if first == "submodule" {
			for index := 1; index < len(tokens); index++ {
				if tokens[index].Text != ")" {
					continue
				}
				nameIndex = firstIdentifierToken(tokens, index+1)
				break
			}
		}
		if nameIndex < 0 {
			return
		}
		kind := SymbolKindModule
		symbol, ok := addStructuralSymbol(builder, SymbolSpec{Kind: kind, NativeKind: first, Name: tokens[nameIndex].Text, Parent: parent,
			Declaration: OffsetRange{Start: start, End: end}, NameRange: OffsetRange{Start: tokens[nameIndex].StartOffset, End: tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: start, End: end}, Evidence: SymbolEvidenceStructural})
		if ok {
			*scopes = append(*scopes, structuralAnalyzerScope{label: first, parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
		}
	case "type":
		if len(tokens) > 1 && (tokens[1].Text == "(" || tokens[1].Text == "=" || tokens[1].Text == "=>" || strings.EqualFold(tokens[1].Text, "is")) {
			return
		}
		nameIndex := -1
		for index := 1; index < len(tokens); index++ {
			if tokens[index].Kind == TokenIdentifier && !strings.EqualFold(tokens[index].Text, "is") {
				nameIndex = index
			}
		}
		if nameIndex < 0 {
			return
		}
		symbol, ok := addStructuralSymbol(builder, SymbolSpec{Kind: SymbolKindType, NativeKind: "derived-type", Name: tokens[nameIndex].Text, Parent: parent,
			Declaration: OffsetRange{Start: start, End: end}, NameRange: OffsetRange{Start: tokens[nameIndex].StartOffset, End: tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: start, End: end}, Evidence: SymbolEvidenceStructural})
		if ok {
			*scopes = append(*scopes, structuralAnalyzerScope{label: "type", parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
		}
	default:
		parseFortranProcedure(builder, tokens, start, end, -1, parent, scopes)
	}
}

func fortranStatementKeyword(value string) string {
	if value == "" {
		return ""
	}
	first := value[0]
	if first >= 'A' && first <= 'Z' {
		first += 'a' - 'A'
	}
	if first >= utf8.RuneSelf {
		return fortranNonASCIIStatementKeyword(value)
	}
	return fortranASCIIStatementKeyword(value, first)
}

func fortranASCIIStatementKeyword(value string, first byte) string {
	switch first {
	case 'e':
		return fortranEndStatementKeyword(value)
	case 'f':
		return fortranStatementKeywordMatch(value, "function")
	case 'm':
		return fortranStatementKeywordMatch(value, "module")
	case 'p':
		if matched := fortranStatementKeywordMatch(value, "program"); matched != "" {
			return matched
		}
		return fortranStatementKeywordMatch(value, "procedure")
	case 's':
		if matched := fortranStatementKeywordMatch(value, "submodule"); matched != "" {
			return matched
		}
		return fortranStatementKeywordMatch(value, "subroutine")
	case 't':
		return fortranStatementKeywordMatch(value, "type")
	case 'u':
		return fortranStatementKeywordMatch(value, "use")
	default:
		return ""
	}
}

func fortranEndStatementKeyword(value string) string {
	for _, keyword := range [...]string{"end", "endmodule", "endprogram", "endsubmodule", "endtype", "endsubroutine", "endfunction"} {
		if matched := fortranStatementKeywordMatch(value, keyword); matched != "" {
			return matched
		}
	}
	return ""
}

func fortranNonASCIIStatementKeyword(value string) string {
	for _, keyword := range [...]string{"end", "endmodule", "endprogram", "endsubmodule", "endtype", "endsubroutine", "endfunction", "function", "module", "program", "procedure", "submodule", "subroutine", "type", "use"} {
		if matched := fortranStatementKeywordMatch(value, keyword); matched != "" {
			return matched
		}
	}
	return ""
}

func fortranStatementKeywordMatch(value, keyword string) string {
	if value == keyword || caseInsensitiveKeywordEqual(value, keyword) {
		return keyword
	}
	return ""
}

func fortranCompactEndLabel(token string) (string, bool) {
	switch token {
	case "endmodule":
		return "module", true
	case "endprogram":
		return "program", true
	case "endsubmodule":
		return "submodule", true
	case "endtype":
		return "type", true
	case "endsubroutine":
		return "subroutine", true
	case "endfunction":
		return "function", true
	default:
		return "", false
	}
}

func parseFortranProcedure(builder *SymbolBuilder, tokens []Token, start, end, keyword int, parent *SymbolParent, scopes *[]structuralAnalyzerScope) {
	native := ""
	if keyword < 0 {
		for index := 0; index < len(tokens); index++ {
			native = fortranProcedureKeyword(tokens[index].Text)
			if native == "" {
				continue
			}
			if index > 0 && tokens[index-1].Text == "%" {
				native = ""
				continue
			}
			keyword = index
			break
		}
	}
	if keyword < 0 || keyword >= len(tokens) {
		return
	}
	if native == "" {
		native = fortranProcedureKeyword(tokens[keyword].Text)
	}
	if native == "" {
		return
	}
	nameIndex := firstIdentifierToken(tokens, keyword+1)
	if nameIndex < 0 {
		return
	}
	symbol, ok := addStructuralSymbol(builder, SymbolSpec{Kind: SymbolKindFunction, NativeKind: native, Name: tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: start, End: end}, NameRange: OffsetRange{Start: tokens[nameIndex].StartOffset, End: tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: start, End: end}, Evidence: SymbolEvidenceStructural})
	if ok {
		*scopes = append(*scopes, structuralAnalyzerScope{label: native, parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
	}
}

func fortranProcedureKeyword(value string) string {
	if caseInsensitiveKeywordEqual(value, "subroutine") {
		return "subroutine"
	}
	if caseInsensitiveKeywordEqual(value, "function") {
		return "function"
	}
	return ""
}

func (COBOLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "cobol", AnalyzerCOBOL)
	if err != nil {
		return AnalyzerResult{}, err
	}
	fixedFormat := !cobolLooksFreeForm(document.Text)
	lineProfile := LineModelProfile{Kind: LineModelFree}
	if fixedFormat {
		lineProfile = LineModelProfile{Kind: LineModelFixed, Fixed: FixedLineProfile{
			LabelStartColumn: 1, LabelEndColumn: 7, ContinuationColumn: 7, CodeStartColumn: 8, CodeEndColumn: 73,
		}}
	}
	lines, err := BuildSourceLines(ctx, document, lineProfile, LineModelLimits{MaxLines: max(4096, len(document.Text)+1), MaxLineBytes: 1024 * 1024})
	if err != nil {
		return AnalyzerResult{}, err
	}
	var program *SymbolParent
	dependencies := []StructuralDependency{}
	continuedQuote := byte(0)
	continuedRange := OffsetRange{}
	addUnterminatedLiteral := func(value OffsetRange) {
		builder.MarkIncomplete()
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: "cobol-unterminated-string", Message: "COBOL source contains an unterminated quoted literal", Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
	}
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		if fixedFormat && cobolFixedComment(document.Text[line.Physical.Start:line.Physical.End]) || line.Code.End <= line.Code.Start {
			continue
		}
		rawCode := document.Text[line.Code.Start:line.Code.End]
		code := ""
		if fixedFormat {
			if continuedQuote != 0 && !line.Continuation {
				addUnterminatedLiteral(continuedRange)
				continuedQuote = 0
				continuedRange = OffsetRange{}
			}
			previousQuote := continuedQuote
			var validContinuation bool
			code, continuedQuote, validContinuation = cobolStripInlineCommentState(rawCode, continuedQuote, line.Continuation)
			if previousQuote != 0 && !validContinuation {
				addUnterminatedLiteral(continuedRange)
				continuedRange = OffsetRange{}
				code, continuedQuote, _ = cobolStripInlineCommentState(rawCode, 0, false)
			}
			if continuedQuote != 0 && continuedRange == (OffsetRange{}) {
				continuedRange = line.Code
			} else if continuedQuote == 0 {
				continuedRange = OffsetRange{}
			}
		} else {
			var complete bool
			code, complete = cobolStripInlineComment(rawCode)
			if !complete {
				addUnterminatedLiteral(line.Code)
			}
		}
		if code == "" {
			continue
		}
		upper := strings.ToUpper(code)
		switch {
		case strings.HasPrefix(upper, "PROGRAM-ID."):
			name := cobolWordAfter(code, "PROGRAM-ID.")
			if name == "" {
				continue
			}
			nameStart := findEqualFoldRange(document.Text, line.Code, name)
			symbol, ok := addStructuralSymbol(builder, SymbolSpec{Kind: SymbolKindModule, NativeKind: "program-id", Name: name,
				Declaration: line.Code, NameRange: nameStart, Signature: &line.Code, Evidence: SymbolEvidenceStructural})
			if ok {
				value := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
				program = &value
			}
		case strings.HasSuffix(upper, " SECTION."):
			name := strings.TrimSpace(code[:len(code)-len(" SECTION.")])
			if name == "" || strings.ContainsAny(name, " \t") {
				continue
			}
			nameRange := findEqualFoldRange(document.Text, line.Code, name)
			addStructuralSymbol(builder, SymbolSpec{Kind: SymbolKindFunction, NativeKind: "section", Name: name, Parent: program,
				Declaration: line.Code, NameRange: nameRange, Signature: &line.Code, Evidence: SymbolEvidenceStructural})
		}
		if index := strings.Index(upper, "COPY "); index >= 0 {
			rest := strings.TrimSpace(code[index+len("COPY "):])
			name := cobolLeadingWord(rest)
			if name != "" {
				absolute := line.Code.Start + index + len("COPY ") + strings.Index(code[index+len("COPY "):], name)
				addStructuralDependency(document, &dependencies, StructuralDependencyInclude, name, absolute, absolute+len(name))
			}
		}
	}
	if fixedFormat && continuedQuote != 0 {
		addUnterminatedLiteral(continuedRange)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func cobolStripInlineComment(text string) (string, bool) {
	code, quote, _ := cobolStripInlineCommentState(text, 0, false)
	return code, quote == 0
}

func cobolStripInlineCommentState(text string, quote byte, continuation bool) (string, byte, bool) {
	start := 0
	if quote != 0 {
		if !continuation {
			return strings.TrimSpace(text), quote, false
		}
		for start < len(text) && (text[start] == ' ' || text[start] == '\t') {
			start++
		}
		if start >= len(text) || text[start] != quote {
			return strings.TrimSpace(text), quote, false
		}
		start++
	}
	for index := start; index < len(text); index++ {
		value := text[index]
		if quote != 0 {
			if value == quote {
				if index+1 < len(text) && text[index+1] == quote {
					index++
					continue
				}
				quote = 0
			}
			continue
		}
		if value == '\'' || value == '"' {
			quote = value
			continue
		}
		if value == '*' && index+1 < len(text) && text[index+1] == '>' {
			return strings.TrimSpace(text[:index]), quote, true
		}
	}
	return strings.TrimSpace(text), quote, true
}

func cobolLooksFreeForm(text string) bool {
	for start := 0; start < len(text); {
		end := start
		for end < len(text) && text[end] != '\r' && text[end] != '\n' {
			end++
		}
		line := text[start:end]
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "*>") && cobolFreeFormLineEvidence(line) {
			return true
		}
		if end >= len(text) {
			break
		}
		if text[end] == '\r' && end+1 < len(text) && text[end+1] == '\n' {
			start = end + 2
		} else {
			start = end + 1
		}
	}
	return false
}

func cobolFreeFormLineEvidence(line string) bool {
	if !cobolFixedPrefixCompatible(line) {
		first := strings.TrimLeft(line, " \t")
		leading := len(line) - len(first)
		if leading < 7 {
			return true
		}
	}
	if utf8.RuneCountInString(line) <= 72 {
		return false
	}
	codeStart := scalarColumnOffset(line, 8)
	codeEnd := scalarColumnOffset(line, 73)
	if codeStart >= codeEnd {
		return false
	}
	_, fullComplete := cobolStripInlineComment(line)
	_, fixedComplete := cobolStripInlineComment(line[codeStart:codeEnd])
	return fullComplete && !fixedComplete
}

func cobolFixedPrefixCompatible(line string) bool {
	if utf8.RuneCountInString(line) < 7 {
		return false
	}
	for column := 1; column <= 6; column++ {
		offset := scalarColumnOffset(line, column)
		if offset >= len(line) {
			return false
		}
		r, _ := utf8.DecodeRuneInString(line[offset:])
		if r != ' ' && (r < '0' || r > '9') {
			return false
		}
	}
	indicatorOffset := scalarColumnOffset(line, 7)
	if indicatorOffset >= len(line) {
		return false
	}
	indicator, _ := utf8.DecodeRuneInString(line[indicatorOffset:])
	switch indicator {
	case ' ', '*', '/', 'D', 'd', '-':
		return true
	default:
		return false
	}
}

func cobolFixedComment(line string) bool {
	if utf8.RuneCountInString(line) < 7 {
		return false
	}
	offset := scalarColumnOffset(line, 7)
	if offset >= len(line) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(line[offset:])
	return r == '*' || r == '/' || r == 'D' || r == 'd'
}

func cobolWordAfter(text, prefix string) string {
	if len(text) < len(prefix) {
		return ""
	}
	return cobolLeadingWord(strings.TrimSpace(text[len(prefix):]))
}

func cobolLeadingWord(text string) string {
	end := 0
	for end < len(text) {
		value := text[end]
		if !(value == '-' || value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9') {
			break
		}
		end++
	}
	if end == 0 {
		return ""
	}
	return text[:end]
}

func findEqualFoldRange(text string, scope OffsetRange, value string) OffsetRange {
	if scope.Start < 0 || scope.End > len(text) || scope.End <= scope.Start || value == "" {
		return OffsetRange{}
	}
	index := strings.Index(strings.ToLower(text[scope.Start:scope.End]), strings.ToLower(value))
	if index < 0 {
		return OffsetRange{}
	}
	start := scope.Start + index
	return OffsetRange{Start: start, End: start + len(value)}
}

func (AdaAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "ada", AnalyzerAda)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scanDocument, err := maskAdaCharacterLiterals(ctx, document)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, lines, err := scanAnalyzerLogicalLines(ctx, scanDocument, AdaScannerProfile(), options.MaxNesting)
	if err != nil {
		return AnalyzerResult{}, err
	}
	applyStructuralScanDiagnostics(builder, scan, "ada")
	dependencies := []StructuralDependency{}
	var scopes []structuralAnalyzerScope
	for lineIndex := range lines {
		line := lines[lineIndex]
		if len(line.Tokens) == 0 {
			continue
		}
		first := strings.ToLower(line.Tokens[0].Text)
		if first == "with" {
			end := logicalLineTokenEnd(line.Tokens)
			for _, part := range splitCommaTokenRangeAt(line.Tokens, 1, end, line.Tokens[0].Nesting) {
				partEnd := part[1]
				for partEnd > part[0] && line.Tokens[partEnd-1].Text == ";" {
					partEnd--
				}
				if partEnd <= part[0] {
					continue
				}
				value := tokenRangeText(line.Tokens, part[0], partEnd)
				if value != "" {
					addStructuralDependency(document, &dependencies, StructuralDependencyImport, value, line.Tokens[part[0]].StartOffset, line.Tokens[partEnd-1].EndOffset)
				}
			}
			continue
		}
		if first == "end" {
			if len(scopes) > 0 && len(line.Tokens) > 1 {
				closing := line.Tokens[1].Text
				if name, _, _, ok := adaSelectedName(line.Tokens, 1); ok {
					closing = name
				}
				current := scopes[len(scopes)-1]
				if strings.EqualFold(closing, "package") || strings.EqualFold(closing, current.parent.QualifiedName) || strings.EqualFold(closing, qualifiedNameTail(current.parent.QualifiedName)) {
					scopes = scopes[:len(scopes)-1]
				}
			}
			continue
		}
		parent := parentFromStructuralScopes(scopes)
		if first == "package" {
			nameStart := 1
			if len(line.Tokens) > 1 && strings.EqualFold(line.Tokens[1].Text, "body") {
				nameStart = 2
			}
			name, nameIndex, nameEnd, ok := adaSelectedName(line.Tokens, nameStart)
			if !ok {
				continue
			}
			symbol, added := addStructuralSymbol(builder, SymbolSpec{Kind: SymbolKindPackage, NativeKind: "package", Name: name, Parent: parent,
				Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameEnd-1].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			if added && adaPackageOpensScope(line.Tokens, nameEnd-1, adaNextTokens(lines, lineIndex+1)) {
				scopes = append(scopes, structuralAnalyzerScope{label: "package", parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
			}
			continue
		}
		if first == "type" || first == "subtype" || first == "task" || first == "protected" {
			nameIndex := firstIdentifierToken(line.Tokens, 1)
			if nameIndex >= 0 {
				addStructuralSymbol(builder, SymbolSpec{Kind: SymbolKindType, NativeKind: first, Name: line.Tokens[nameIndex].Text, Parent: parent,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			}
			continue
		}
		if first == "procedure" || first == "function" {
			nameIndex := firstIdentifierToken(line.Tokens, 1)
			if nameIndex >= 0 {
				addStructuralSymbol(builder, SymbolSpec{Kind: SymbolKindFunction, NativeKind: first, Name: line.Tokens[nameIndex].Text, Parent: parent,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
			}
		}
	}
	markUnclosedStructuralScopes(builder, "ada", scopes)
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func maskAdaCharacterLiterals(ctx context.Context, document *SourceDocument) (*SourceDocument, error) {
	if document == nil || !strings.Contains(document.Text, "'") {
		return document, nil
	}
	masked := []byte(document.Text)
	changed := false
	for index := 0; index < len(document.Text); {
		if index&0x3fff == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if document.Text[index] != '\'' || index+1 >= len(document.Text) {
			_, size := utf8.DecodeRuneInString(document.Text[index:])
			if size <= 0 {
				size = 1
			}
			index += size
			continue
		}
		value, size := utf8.DecodeRuneInString(document.Text[index+1:])
		if value == utf8.RuneError && size == 0 {
			index++
			continue
		}
		closeAt := index + 1 + size
		if value == '\n' || value == '\r' || closeAt >= len(document.Text) || document.Text[closeAt] != '\'' {
			index++
			continue
		}
		for offset := index; offset <= closeAt; offset++ {
			masked[offset] = ' '
		}
		changed = true
		index = closeAt + 1
	}
	if !changed {
		return document, nil
	}
	clone := *document
	clone.Text = string(masked)
	clone.lineStarts = buildLineStarts(clone.Text)
	return &clone, nil
}

func adaSelectedName(tokens []Token, start int) (string, int, int, bool) {
	nameStart := firstIdentifierToken(tokens, start)
	if nameStart < 0 {
		return "", 0, 0, false
	}
	nameEnd := nameStart + 1
	for nameEnd+1 < len(tokens) && tokens[nameEnd].Text == "." && tokens[nameEnd+1].Kind == TokenIdentifier {
		nameEnd += 2
	}
	name := tokenRangeText(tokens, nameStart, nameEnd)
	return name, nameStart, nameEnd, name != ""
}

func adaNextTokens(lines []LogicalLine, start int) []Token {
	for index := max(start, 0); index < len(lines); index++ {
		if logicalLineTokenEnd(lines[index].Tokens) > 0 {
			return lines[index].Tokens
		}
	}
	return nil
}

func adaPackageOpensScope(tokens []Token, nameIndex int, nextTokens []Token) bool {
	end := logicalLineTokenEnd(tokens)
	hasIs := false
	for index := nameIndex + 1; index < end; index++ {
		switch strings.ToLower(tokens[index].Text) {
		case "renames":
			return false
		case "is":
			hasIs = true
		case "new", "separate":
			if hasIs {
				return false
			}
		case ";":
			return true
		}
	}
	nextEnd := logicalLineTokenEnd(nextTokens)
	if nextEnd == 0 {
		return true
	}
	if hasIs && (strings.EqualFold(nextTokens[0].Text, "new") || strings.EqualFold(nextTokens[0].Text, "separate")) {
		return false
	}
	if !hasIs && strings.EqualFold(nextTokens[0].Text, "is") && nextEnd > 1 &&
		(strings.EqualFold(nextTokens[1].Text, "new") || strings.EqualFold(nextTokens[1].Text, "separate")) {
		return false
	}
	return true
}
