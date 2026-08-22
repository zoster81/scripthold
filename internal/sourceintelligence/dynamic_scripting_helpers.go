package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

type phase8State struct {
	ctx          context.Context
	document     *SourceDocument
	builder      *SymbolBuilder
	dependencies []StructuralDependency
	relations    []StructuralRelation
	stopped      bool
}

func newPhase8State(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID) (*phase8State, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return nil, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, operation.Wrap(operation.KindCancelled, "analyze_phase8_source", document.Path, err)
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: language, Analyzer: string(analyzer), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return nil, err
	}
	return &phase8State{ctx: ctx, document: document, builder: builder}, nil
}

func (s *phase8State) scan(options AnalyzeOptions, profile ScannerProfile, masked string) (ScanResult, error) {
	if masked == "" && s.document.Text != "" {
		masked = s.document.Text
	}
	clone := *s.document
	clone.Text = masked
	clone.lineStarts = buildLineStarts(masked)
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(s.ctx, &clone, profile, ScannerLimits{MaxTokens: scannerTokenBudget(masked), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return ScanResult{}, err
	}
	for _, diagnostic := range scan.Diagnostics {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = s.builder.AddDiagnostic(DiagnosticSpec{Code: profile.Name + "-" + diagnostic.Code, Message: diagnostic.Message, Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
	}
	if !scan.Complete || scan.DiagnosticsTruncated {
		s.builder.MarkIncomplete()
	}
	return scan, nil
}

func (s *phase8State) add(spec SymbolSpec) (NormalizedSymbol, bool) {
	symbol, err := s.builder.Add(spec)
	if operation.KindOf(err) == operation.KindLimit {
		s.stopped = true
		return NormalizedSymbol{}, false
	}
	if err != nil {
		s.builder.MarkIncomplete()
		return NormalizedSymbol{}, false
	}
	return symbol, true
}

func (s *phase8State) addDependency(kind StructuralDependencyKind, value string, start, end int) {
	value = strings.TrimSpace(value)
	if value == "" || start < 0 || end <= start || end > len(s.document.Text) {
		return
	}
	rangeValue, err := s.document.RangeFromUTF8Offsets(start, end)
	if err != nil {
		return
	}
	s.dependencies = appendUniqueDependencies(s.dependencies, []StructuralDependency{{Kind: kind, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural}})
}

func (s *phase8State) addRelation(kind, source, target string, start, end int) {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(target) == "" || start < 0 || end <= start || end > len(s.document.Text) {
		return
	}
	rangeValue, err := s.document.RangeFromUTF8Offsets(start, end)
	if err != nil {
		return
	}
	s.relations = append(s.relations, StructuralRelation{Kind: kind, Source: source, Target: strings.TrimSpace(target), Range: rangeValue, Evidence: SymbolEvidenceStructural})
}

func (s *phase8State) result() (AnalyzerResult, error) {
	if err := s.ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_phase8_source", s.document.Path, err)
	}
	return AnalyzerResult{Analysis: s.builder.Result(), Dependencies: s.dependencies, Relations: s.relations}, nil
}

func phase8StringValue(token Token) string {
	if token.Kind != TokenString || len(token.Text) < 2 {
		return ""
	}
	if strings.HasPrefix(token.Text, "\"\"\"") && strings.HasSuffix(token.Text, "\"\"\"") && len(token.Text) >= 6 {
		return token.Text[3 : len(token.Text)-3]
	}
	if strings.HasPrefix(token.Text, "'''") && strings.HasSuffix(token.Text, "'''") && len(token.Text) >= 6 {
		return token.Text[3 : len(token.Text)-3]
	}
	quote := token.Text[0]
	if (quote == '\'' || quote == '"' || quote == '`') && token.Text[len(token.Text)-1] == quote {
		return token.Text[1 : len(token.Text)-1]
	}
	return ""
}

func phase8MaskRange(result []byte, start, end int) {
	if start < 0 {
		start = 0
	}
	if end > len(result) {
		end = len(result)
	}
	for i := start; i < end; i++ {
		if result[i] != '\r' && result[i] != '\n' {
			result[i] = ' '
		}
	}
}

func phase8LineBounds(text string, start int) (end, next int) {
	end = start
	for end < len(text) && text[end] != '\r' && text[end] != '\n' {
		end++
	}
	next = end
	if next < len(text) {
		if text[next] == '\r' && next+1 < len(text) && text[next+1] == '\n' {
			next += 2
		} else {
			next++
		}
	}
	return end, next
}

func maskPerlNonCode(text string) (string, bool) {
	result := []byte(text)
	complete := true
	inPOD := false
	for at := 0; at < len(text); {
		end, next := phase8LineBounds(text, at)
		line := text[at:end]
		trimmed := strings.TrimSpace(line)
		if !inPOD && (trimmed == "__DATA__" || trimmed == "__END__") {
			phase8MaskRange(result, at, len(text))
			break
		}
		podCommand, podDirective := phase8PerlPODCommand(line)
		if !inPOD && podDirective && podCommand != "cut" {
			inPOD = true
		}
		if inPOD {
			phase8MaskRange(result, at, end)
			if podDirective && podCommand == "cut" {
				inPOD = false
			}
		}
		at = next
	}

	for at := 0; at < len(text); {
		end, next := phase8LineBounds(text, at)
		delimiter, ok := phase8PerlHereDocDelimiter(text[at:end])
		if !ok {
			at = next
			continue
		}
		body := next
		for body < len(text) {
			bodyEnd, bodyNext := phase8LineBounds(text, body)
			phase8MaskRange(result, body, bodyEnd)
			if strings.TrimSpace(text[body:bodyEnd]) == delimiter {
				at = bodyNext
				break
			}
			body = bodyNext
		}
		if body >= len(text) {
			complete = false
			break
		}
	}
	maskPerlQuoteLikeOperators(result)
	return string(result), complete
}

func phase8PerlPODCommand(line string) (string, bool) {
	if len(line) < 2 || line[0] != '=' || !phase8PerlPODCommandLetter(line[1]) {
		return "", false
	}
	end := 2
	for end < len(line) && (phase8PerlPODCommandLetter(line[end]) || line[end] >= '0' && line[end] <= '9') {
		end++
	}
	return line[1:end], true
}

func phase8PerlPODCommandLetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func phase8PerlHereDocDelimiter(line string) (string, bool) {
	search := 0
	for search < len(line) {
		relative := strings.Index(line[search:], "<<")
		if relative < 0 {
			return "", false
		}
		operator := search + relative
		before := strings.TrimSpace(line[:operator])
		allowed, quotedOnly := phase8PerlHereDocContext(before)
		rest := strings.TrimSpace(line[operator+2:])
		if allowed {
			if len(rest) >= 3 && (rest[0] == '\'' || rest[0] == '"') {
				quote := rest[0]
				if close := strings.IndexByte(rest[1:], quote); close > 0 {
					delimiter := rest[1 : close+1]
					if delimiter != "" && !strings.ContainsAny(delimiter, " \t\r\n") {
						return delimiter, true
					}
				}
			} else if !quotedOnly {
				if delimiter, ok := phase8PerlBarewordHereDocDelimiter(rest); ok {
					return delimiter, true
				}
			}
		}
		search = operator + 2
	}
	return "", false
}

func phase8PerlHereDocContext(before string) (allowed, quotedOnly bool) {
	before = strings.TrimSpace(before)
	if strings.HasSuffix(before, "=") {
		return true, false
	}
	if phase8PerlEndsWithCallable(before, "print") || phase8PerlEndsWithCallable(before, "say") {
		return true, false
	}
	if strings.HasSuffix(before, "(") {
		callee := strings.TrimSpace(strings.TrimSuffix(before, "("))
		if phase8PerlEndsWithCallable(callee, "print") || phase8PerlEndsWithCallable(callee, "say") {
			return true, true
		}
	}
	return false, false
}

func phase8PerlEndsWithCallable(text, name string) bool {
	if !strings.HasSuffix(text, name) {
		return false
	}
	start := len(text) - len(name)
	return start == 0 || !phase8PerlIdentifierByte(text[start-1])
}

func phase8PerlBarewordHereDocDelimiter(rest string) (string, bool) {
	if rest == "" || !phase8PerlHereDocIdentifierStart(rest[0]) {
		return "", false
	}
	end := 1
	for end < len(rest) && phase8PerlHereDocIdentifierContinue(rest[end]) {
		end++
	}
	if end < len(rest) {
		switch rest[end] {
		case ';', ',', ' ', '\t':
		default:
			return "", false
		}
	}
	return rest[:end], true
}

func phase8PerlHereDocIdentifierStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func phase8PerlHereDocIdentifierContinue(value byte) bool {
	return phase8PerlHereDocIdentifierStart(value) || value >= '0' && value <= '9'
}

func maskPerlQuoteLikeOperators(result []byte) {
	for at := 0; at < len(result); {
		switch result[at] {
		case '#':
			at = phase8PerlLineEnd(result, at)
			continue
		case '\'', '"':
			at = phase8PerlOrdinaryStringEnd(result, at)
			continue
		case '/':
			if phase8PerlSlashRegexStart(result, at) {
				if end, ok := phase8PerlSlashRegexEnd(result, at); ok {
					phase8MaskRange(result, at, end)
					at = end
					continue
				}
			}
		}
		operatorLength, parts := phase8PerlQuoteLikeOperatorAt(result, at)
		if operatorLength == 0 {
			at++
			continue
		}
		delimiterAt := at + operatorLength
		for delimiterAt < len(result) && (result[delimiterAt] == ' ' || result[delimiterAt] == '\t') {
			delimiterAt++
		}
		if delimiterAt >= len(result) || !phase8PerlQuoteDelimiter(result[delimiterAt]) {
			at += operatorLength
			continue
		}
		firstEnd, paired, ok := phase8PerlDelimitedEnd(result, delimiterAt)
		if !ok {
			at += operatorLength
			continue
		}
		end := firstEnd
		if parts == 2 {
			if paired {
				secondAt := firstEnd
				for secondAt < len(result) && (result[secondAt] == ' ' || result[secondAt] == '\t') {
					secondAt++
				}
				if secondAt >= len(result) || !phase8PerlQuoteDelimiter(result[secondAt]) {
					at += operatorLength
					continue
				}
				secondEnd, _, secondOK := phase8PerlDelimitedEnd(result, secondAt)
				if !secondOK {
					at += operatorLength
					continue
				}
				end = secondEnd
			} else {
				secondEnd, secondOK := phase8PerlSimpleDelimitedContentEnd(result, firstEnd, result[delimiterAt])
				if !secondOK {
					at += operatorLength
					continue
				}
				end = secondEnd
			}
		}
		phase8MaskRange(result, at, end)
		at = end
	}
}

func phase8PerlSlashRegexStart(text []byte, at int) bool {
	if at < 0 || at >= len(text) || text[at] != '/' {
		return false
	}
	previous := at - 1
	for previous >= 0 && (text[previous] == ' ' || text[previous] == '\t') {
		previous--
	}
	if previous < 0 || text[previous] == '\r' || text[previous] == '\n' {
		return true
	}
	switch text[previous] {
	case '{', '(', '[', ',', ';', '!', '=', '?', ':':
		return true
	case '~':
		beforeTilde := previous - 1
		for beforeTilde >= 0 && (text[beforeTilde] == ' ' || text[beforeTilde] == '\t') {
			beforeTilde--
		}
		return beforeTilde >= 0 && (text[beforeTilde] == '=' || text[beforeTilde] == '!')
	case '&':
		return previous > 0 && text[previous-1] == '&'
	case '|':
		return previous > 0 && text[previous-1] == '|'
	default:
		return false
	}
}

func phase8PerlSlashRegexEnd(text []byte, delimiterAt int) (int, bool) {
	for at := delimiterAt + 1; at < len(text); at++ {
		if text[at] == '\\' {
			at++
			continue
		}
		if text[at] == '/' {
			return at + 1, true
		}
		if text[at] == '\r' || text[at] == '\n' {
			return 0, false
		}
	}
	return 0, false
}

func phase8PerlQuoteLikeOperatorAt(text []byte, at int) (length, parts int) {
	if at > 0 && (phase8PerlIdentifierByte(text[at-1]) || text[at-1] == '\\') {
		return 0, 0
	}
	for _, candidate := range []struct {
		operator string
		parts    int
	}{
		{operator: "tr", parts: 2},
		{operator: "qq", parts: 1},
		{operator: "qx", parts: 1},
		{operator: "qw", parts: 1},
		{operator: "qr", parts: 1},
		{operator: "q", parts: 1},
		{operator: "m", parts: 1},
		{operator: "s", parts: 2},
		{operator: "y", parts: 2},
	} {
		end := at + len(candidate.operator)
		if end > len(text) || string(text[at:end]) != candidate.operator {
			continue
		}
		if end < len(text) && phase8PerlIdentifierByte(text[end]) {
			continue
		}
		if phase8PerlBracedQuoteLikeKey(text, at, end) {
			return 0, 0
		}
		return len(candidate.operator), candidate.parts
	}
	return 0, 0
}

func phase8PerlBracedQuoteLikeKey(text []byte, at, end int) bool {
	previous := at - 1
	for previous >= 0 && (text[previous] == ' ' || text[previous] == '\t') {
		previous--
	}
	if previous < 0 || text[previous] != '{' || !phase8PerlHashSubscriptBeforeBrace(text, previous) {
		return false
	}
	next := end
	for next < len(text) && (text[next] == ' ' || text[next] == '\t') {
		next++
	}
	return next < len(text) && text[next] == '}'
}

func phase8PerlHashSubscriptBeforeBrace(text []byte, brace int) bool {
	if brace >= 2 && text[brace-2] == '-' && text[brace-1] == '>' {
		return true
	}
	at := brace - 1
	for at >= 0 && phase8PerlBareIdentifierByte(text[at]) {
		at--
	}
	return at >= 0 && (text[at] == '$' || text[at] == '@' || text[at] == '%')
}

func phase8PerlBareIdentifierByte(value byte) bool {
	return value == '_' || value == ':' || value == '\'' || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func phase8PerlIdentifierByte(value byte) bool {
	return value == '_' || value == '$' || value == '@' || value == '%' || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func phase8PerlQuoteDelimiter(value byte) bool {
	return value > ' ' && !phase8PerlIdentifierByte(value) && value != '\\'
}

func phase8PerlDelimitedEnd(text []byte, delimiterAt int) (end int, paired bool, ok bool) {
	open := text[delimiterAt]
	close := open
	switch open {
	case '(':
		close, paired = ')', true
	case '[':
		close, paired = ']', true
	case '{':
		close, paired = '}', true
	case '<':
		close, paired = '>', true
	}
	if !paired {
		end, ok := phase8PerlSimpleDelimitedContentEnd(text, delimiterAt+1, close)
		return end, false, ok
	}
	depth := 1
	for at := delimiterAt + 1; at < len(text); at++ {
		if text[at] == '\\' {
			at++
			continue
		}
		if text[at] == open {
			depth++
			continue
		}
		if text[at] == close {
			depth--
			if depth == 0 {
				return at + 1, true, true
			}
		}
	}
	return 0, true, false
}

func phase8PerlSimpleDelimitedContentEnd(text []byte, contentAt int, delimiter byte) (int, bool) {
	for at := contentAt; at < len(text); at++ {
		if text[at] == '\\' {
			at++
			continue
		}
		if text[at] == delimiter {
			return at + 1, true
		}
	}
	return 0, false
}

func phase8PerlLineEnd(text []byte, at int) int {
	for at < len(text) && text[at] != '\r' && text[at] != '\n' {
		at++
	}
	return at
}

func phase8PerlOrdinaryStringEnd(text []byte, start int) int {
	quote := text[start]
	for at := start + 1; at < len(text); at++ {
		if text[at] == '\\' {
			at++
			continue
		}
		if text[at] == quote {
			return at + 1
		}
		if text[at] == '\r' || text[at] == '\n' {
			return at
		}
	}
	return len(text)
}

func maskLuaLongBrackets(text string) (string, bool) {
	result := []byte(text)
	complete := true
	for at := 0; at < len(text); {
		start := at
		if strings.HasPrefix(text[at:], "--[") {
			start = at + 2
		}
		if start >= len(text) || text[start] != '[' {
			at++
			continue
		}
		eq := 0
		cursor := start + 1
		for cursor < len(text) && text[cursor] == '=' {
			eq++
			cursor++
		}
		if cursor >= len(text) || text[cursor] != '[' {
			at++
			continue
		}
		close := "]" + strings.Repeat("=", eq) + "]"
		endRelative := strings.Index(text[cursor+1:], close)
		if endRelative < 0 {
			phase8MaskRange(result, at, len(text))
			complete = false
			break
		}
		end := cursor + 1 + endRelative + len(close)
		phase8MaskRange(result, at, end)
		at = end
	}
	return string(result), complete
}

func maskGroovySlashyStrings(text string) (string, bool) {
	result := []byte(text)
	complete := true
	for at := 0; at < len(text); {
		switch {
		case strings.HasPrefix(text[at:], "//"):
			end, _ := phase8LineBounds(text, at)
			at = end
			continue
		case strings.HasPrefix(text[at:], "/*"):
			if relative := strings.Index(text[at+2:], "*/"); relative >= 0 {
				at += 2 + relative + 2
			} else {
				return string(result), false
			}
			continue
		case strings.HasPrefix(text[at:], "\"\"\"") || strings.HasPrefix(text[at:], "'''"):
			delimiter := text[at : at+3]
			at = phase8SkipQuoted(text, at, delimiter, true)
			continue
		case text[at] == '\'' || text[at] == '"':
			at = phase8SkipQuoted(text, at, text[at:at+1], true)
			continue
		case strings.HasPrefix(text[at:], "$/"):
			if relative := strings.Index(text[at+2:], "/$"); relative >= 0 {
				end := at + 2 + relative + 2
				phase8MaskRange(result, at, end)
				at = end
				continue
			}
			phase8MaskRange(result, at, len(text))
			return string(result), false
		case text[at] == '/' && phase8GroovySlashyStart(text, at):
			end := at + 1
			for end < len(text) {
				if text[end] == '\\' && end+1 < len(text) {
					end += 2
					continue
				}
				if text[end] == '/' {
					end++
					phase8MaskRange(result, at, end)
					at = end
					break
				}
				end++
			}
			if end >= len(text) {
				phase8MaskRange(result, at, len(text))
				complete = false
				at = end
			}
			continue
		}
		at++
	}
	return string(result), complete
}

func phase8GroovySlashyStart(text string, at int) bool {
	for i := at - 1; i >= 0; i-- {
		switch text[i] {
		case ' ', '\t':
			continue
		case '\r', '\n', '=', '(', '[', '{', ',', ':', ';', '!', '?':
			return true
		default:
			return false
		}
	}
	return true
}

func phase8SkipQuoted(text string, start int, delimiter string, backslashEscapes bool) int {
	at := start + len(delimiter)
	for at < len(text) {
		if backslashEscapes && text[at] == '\\' && at+1 < len(text) {
			at += 2
			continue
		}
		if strings.HasPrefix(text[at:], delimiter) {
			return at + len(delimiter)
		}
		at++
	}
	return len(text)
}

func maskAutoHotkeyCodeStrings(text string) (string, bool) {
	result := []byte(text)
	maskAutoHotkeyEscapedStructural(text, result)
	maskAutoHotkeyBracketHotkeys(text, result)
	maskAutoHotkeyContinuationSections(text, result)
	maskAutoHotkeyLegacyRawAssignments(text, result)

	complete := true
	for lineStart := 0; lineStart < len(text); {
		lineEnd, next := phase8LineBounds(text, lineStart)
		line := string(result[lineStart:lineEnd])
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			lineStart = next
			continue
		}
		for at := lineStart; at < lineEnd; at++ {
			if result[at] == ';' {
				break
			}
			quote := result[at]
			if quote != '"' && quote != '\'' {
				continue
			}
			if quote == '\'' && (!autoHotkeySingleQuoteStarts(result, lineStart, at) || autoHotkeyApostropheHotkey(result, lineStart, at, lineEnd)) {
				continue
			}
			end, closed := autoHotkeyQuoteEnd(result, at, lineEnd, quote)
			if !closed {
				if continuedEnd, ok := autoHotkeyContinuedQuoteEnd(text, result, next, quote); ok {
					end, closed = continuedEnd, true
				}
			}
			phase8MaskRange(result, at, end)
			if !closed {
				complete = false
			}
			at = end - 1
		}
		lineStart = next
	}
	maskAutoHotkeyLegacyCommandBraceTransitions(text, result)
	return string(result), complete
}

func maskAutoHotkeyLegacyCommandBraceTransitions(text string, result []byte) {
	for lineStart := 0; lineStart < len(text); {
		lineEnd, next := phase8LineBounds(text, lineStart)
		at := autoHotkeyHorizontalStart(result, lineStart, lineEnd)
		if at >= lineEnd || !autoHotkeyLegacyIdentifierStart(result[at]) {
			lineStart = next
			continue
		}
		at++
		for at < lineEnd && autoHotkeyLegacyIdentifierContinue(result[at]) {
			at++
		}
		for at < lineEnd && (result[at] == ' ' || result[at] == '\t') {
			at++
		}
		if at >= lineEnd || result[at] != ',' {
			lineStart = next
			continue
		}
		for cursor := at + 1; cursor+1 < lineEnd; cursor++ {
			if result[cursor] == ';' {
				break
			}
			if result[cursor] == '}' && result[cursor+1] == '{' {
				phase8MaskRange(result, cursor, cursor+2)
				cursor++
			}
		}
		lineStart = next
	}
}

func maskAutoHotkeyEscapedStructural(text string, result []byte) {
	for lineStart := 0; lineStart < len(text); {
		lineEnd, next := phase8LineBounds(text, lineStart)
		for at := lineStart; at+1 < lineEnd; at++ {
			if result[at] != '`' {
				continue
			}
			if autoHotkeyStructuralEscapeTarget(result[at+1]) {
				phase8MaskRange(result, at, at+2)
			}
			at++
		}
		lineStart = next
	}
}

func autoHotkeyStructuralEscapeTarget(value byte) bool {
	switch value {
	case '\'', '"', ';', '(', ')', '[', ']', '{', '}':
		return true
	default:
		return false
	}
}

func maskAutoHotkeyBracketHotkeys(text string, result []byte) {
	for lineStart := 0; lineStart < len(text); {
		lineEnd, next := phase8LineBounds(text, lineStart)
		at := autoHotkeyHorizontalStart(result, lineStart, lineEnd)
		for at < lineEnd {
			switch result[at] {
			case '<', '>', '!', '^', '+', '#', '*', '~', '$':
				at++
			default:
				goto key
			}
		}
	key:
		if at+2 < lineEnd && (result[at] == '[' || result[at] == ']') && result[at+1] == ':' && result[at+2] == ':' {
			result[at] = ' '
		}
		lineStart = next
	}
}

func maskAutoHotkeyLegacyRawAssignments(text string, result []byte) {
	for lineStart := 0; lineStart < len(text); {
		lineEnd, next := phase8LineBounds(text, lineStart)
		at := autoHotkeyHorizontalStart(result, lineStart, lineEnd)
		if at < lineEnd && result[at] != ';' && result[at] != '#' {
			if equal := autoHotkeyLegacyAssignmentEqual(result, at, lineEnd); equal >= 0 {
				rhs := autoHotkeyHorizontalStart(result, equal+1, lineEnd)
				if autoHotkeyLegacyRawCloser(result, rhs, lineEnd) || autoHotkeyLegacyRawQuoteColon(result, rhs, lineEnd) {
					phase8MaskRange(result, equal+1, lineEnd)
				}
			}
		}
		lineStart = next
	}
}

func autoHotkeyLegacyAssignmentEqual(data []byte, start, end int) int {
	if start >= end || !autoHotkeyLegacyIdentifierStart(data[start]) {
		return -1
	}
	at := start + 1
	for at < end && autoHotkeyLegacyIdentifierContinue(data[at]) {
		at++
	}
	for at < end && (data[at] == ' ' || data[at] == '\t') {
		at++
	}
	if at >= end || data[at] != '=' || at+1 >= end || data[at+1] == '=' || data[at+1] == '>' {
		return -1
	}
	return at
}

func autoHotkeyLegacyRawQuoteColon(data []byte, start, end int) bool {
	if start >= end || data[start] != '"' {
		return false
	}
	quoteEnd, closed := autoHotkeyQuoteEnd(data, start, end, '"')
	if !closed {
		return false
	}
	at := autoHotkeyHorizontalStart(data, quoteEnd, end)
	return at < end && data[at] == ':'
}

func autoHotkeyLegacyRawCloser(data []byte, start, end int) bool {
	stack := make([]byte, 0, 4)
	for at := start; at < end; at++ {
		if data[at] == '`' && at+1 < end {
			at++
			continue
		}
		if data[at] == ';' {
			break
		}
		if data[at] == '"' || data[at] == '\'' {
			quoteEnd, closed := autoHotkeyQuoteEnd(data, at, end, data[at])
			if closed {
				at = quoteEnd - 1
				continue
			}
		}
		switch data[at] {
		case '(', '[', '{':
			stack = append(stack, data[at])
		case ')', ']', '}':
			if len(stack) == 0 || !autoHotkeyDelimiterMatches(stack[len(stack)-1], data[at]) {
				return true
			}
			stack = stack[:len(stack)-1]
		}
	}
	return false
}

func autoHotkeyDelimiterMatches(open, close byte) bool {
	return open == '(' && close == ')' || open == '[' && close == ']' || open == '{' && close == '}'
}

func autoHotkeyLegacyIdentifierStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func autoHotkeyLegacyIdentifierContinue(value byte) bool {
	return autoHotkeyLegacyIdentifierStart(value) || value >= '0' && value <= '9'
}

func maskAutoHotkeyContinuationSections(text string, result []byte) {
	previousStart, previousEnd := -1, -1
	for lineStart := 0; lineStart < len(text); {
		lineEnd, next := phase8LineBounds(text, lineStart)
		trimmedStart := autoHotkeyHorizontalStart(result, lineStart, lineEnd)
		if autoHotkeyContinuationHeader(result, trimmedStart, lineEnd) {
			quoteStart, quote, quoted := -1, byte(0), false
			if previousStart >= 0 {
				quoteStart, quote, quoted = autoHotkeyUnclosedLineQuote(result, previousStart, previousEnd)
			}
			maskEnd, closeLineEnd, closeNext, ok := autoHotkeyContinuationClose(text, result, next, quote, quoted)
			if ok {
				maskStart := lineStart
				if quoted {
					maskStart = quoteStart
				}
				phase8MaskRange(result, maskStart, maskEnd)
				previousStart, previousEnd = maskEnd, closeLineEnd
				lineStart = closeNext
				continue
			}
		}
		previousStart, previousEnd = lineStart, lineEnd
		lineStart = next
	}
}

func autoHotkeyContinuationHeader(data []byte, start, end int) bool {
	if start >= end || data[start] != '(' {
		return false
	}
	rest := strings.TrimSpace(string(data[start+1 : end]))
	if comment := strings.IndexByte(rest, ';'); comment >= 0 {
		rest = strings.TrimSpace(rest[:comment])
	}
	if rest == "" {
		return true
	}
	for _, option := range strings.Fields(rest) {
		lower := strings.ToLower(option)
		switch {
		case strings.HasPrefix(lower, "join"):
		case lower == "ltrim", lower == "ltrim0", lower == "rtrim", lower == "rtrim0":
		case lower == "comments", lower == "comment", lower == "com", lower == "c":
		case lower == "quotes", lower == "q", lower == "%", lower == ",", lower == "`":
		default:
			return false
		}
	}
	return true
}

func autoHotkeyContinuationClose(text string, data []byte, start int, quote byte, quoted bool) (int, int, int, bool) {
	for lineStart := start; lineStart < len(text); {
		lineEnd, next := phase8LineBounds(text, lineStart)
		at := autoHotkeyHorizontalStart(data, lineStart, lineEnd)
		if at < lineEnd && data[at] == ')' {
			if !quoted {
				return lineEnd, lineEnd, next, true
			}
			for at++; at < lineEnd; at++ {
				if data[at] == '`' && at+1 < lineEnd {
					at++
					continue
				}
				if data[at] != quote {
					continue
				}
				if at+1 < lineEnd && data[at+1] == quote {
					at++
					continue
				}
				return at + 1, lineEnd, next, true
			}
		}
		lineStart = next
	}
	return 0, 0, 0, false
}

func autoHotkeyUnclosedLineQuote(data []byte, lineStart, lineEnd int) (int, byte, bool) {
	for at := lineStart; at < lineEnd; at++ {
		if data[at] == ';' {
			return -1, 0, false
		}
		quote := data[at]
		if quote != '"' && quote != '\'' {
			continue
		}
		if quote == '\'' && !autoHotkeySingleQuoteStarts(data, lineStart, at) {
			continue
		}
		end, closed := autoHotkeyQuoteEnd(data, at, lineEnd, quote)
		if !closed {
			return at, quote, true
		}
		at = end - 1
	}
	return -1, 0, false
}

func autoHotkeyQuoteEnd(data []byte, start, lineEnd int, quote byte) (int, bool) {
	at := start + 1
	for at < lineEnd {
		if data[at] == '`' && at+1 < lineEnd {
			at += 2
			continue
		}
		if data[at] == quote {
			if at+1 < lineEnd && data[at+1] == quote {
				at += 2
				continue
			}
			return at + 1, true
		}
		at++
	}
	return lineEnd, false
}

func autoHotkeyContinuedQuoteEnd(text string, data []byte, lineStart int, quote byte) (int, bool) {
	for lineStart < len(data) {
		lineEnd, next := phase8LineBounds(text, lineStart)
		at := autoHotkeyHorizontalStart(data, lineStart, lineEnd)
		if !autoHotkeyExpressionContinuationStart(data, at, lineEnd) {
			return 0, false
		}
		for at < lineEnd {
			if data[at] == '`' && at+1 < lineEnd {
				at += 2
				continue
			}
			if data[at] == quote {
				if at+1 < lineEnd && data[at+1] == quote {
					at += 2
					continue
				}
				return at + 1, true
			}
			at++
		}
		lineStart = next
	}
	return 0, false
}

func autoHotkeyExpressionContinuationStart(data []byte, start, end int) bool {
	if start >= end {
		return false
	}
	if start+1 < end && (data[start] == '+' && data[start+1] == '+' || data[start] == '-' && data[start+1] == '-') {
		return false
	}
	switch data[start] {
	case ',', '.', '+', '-', '*', '/', '%', '&', '|', '^', '!', '~', '?', ':', '=', '<', '>':
		return true
	}
	for _, word := range []string{"and", "or", "not"} {
		if end-start < len(word) || !strings.EqualFold(string(data[start:start+len(word)]), word) {
			continue
		}
		if start+len(word) == end || data[start+len(word)] == ' ' || data[start+len(word)] == '\t' {
			return true
		}
	}
	return false
}

func autoHotkeyApostropheHotkey(data []byte, lineStart, at, lineEnd int) bool {
	if at+2 >= lineEnd || data[at+1] != ':' || data[at+2] != ':' {
		return false
	}
	start := autoHotkeyHorizontalStart(data, lineStart, at)
	for i := start; i < at; i++ {
		switch data[i] {
		case '<', '>', '!', '^', '+', '#', '*', '~', '$':
		default:
			return false
		}
	}
	return true
}

func autoHotkeySingleQuoteStarts(data []byte, lineStart, at int) bool {
	if at <= lineStart {
		return true
	}
	if data[at-1] == '%' && autoHotkeyLegacyPercentDereferenceEndsAt(data, lineStart, at-1) {
		return false
	}
	switch data[at-1] {
	case ' ', '\t', '(', '[', '{', ',', '=', '>', '<', '!', '?', ':', '+', '-', '*', '/', '%', '&', '|', '^', '~':
		return true
	default:
		return false
	}
}

func autoHotkeyLegacyPercentDereferenceEndsAt(data []byte, lineStart, percent int) bool {
	if percent <= lineStart || percent >= len(data) || data[percent] != '%' {
		return false
	}
	start := percent - 1
	for start >= lineStart && autoHotkeyLegacyIdentifierContinue(data[start]) {
		start--
	}
	return start >= lineStart && data[start] == '%' && start+1 < percent && autoHotkeyLegacyIdentifierStart(data[start+1])
}

func autoHotkeyHorizontalStart(data []byte, start, end int) int {
	for start < end && (data[start] == ' ' || data[start] == '\t') {
		start++
	}
	return start
}

func phase8StaticDependencyTarget(text string, tokens []Token) (string, int, int, bool) {
	startIndex := 0
	for startIndex < len(tokens) && (tokens[startIndex].Kind == TokenNewline || tokens[startIndex].Kind == TokenDirective) {
		startIndex++
	}
	if startIndex >= len(tokens) || tokens[startIndex].Kind == TokenEOF {
		return "", 0, 0, false
	}
	first := tokens[startIndex]
	if value := phase8StringValue(first); value != "" {
		return value, first.StartOffset, first.EndOffset, true
	}
	start := first.StartOffset
	end := first.EndOffset
	for i := startIndex + 1; i < len(tokens); i++ {
		token := tokens[i]
		if token.Kind == TokenEOF || token.Kind == TokenNewline || token.StartOffset != end {
			break
		}
		end = token.EndOffset
	}
	if start < 0 || end <= start || end > len(text) {
		return "", 0, 0, false
	}
	value := text[start:end]
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '.', '/', '_', '-', '+', '@', ':':
			continue
		default:
			return "", 0, 0, false
		}
	}
	if value == "" {
		return "", 0, 0, false
	}
	return value, start, end, true
}

func phase8NextIdentifier(tokens []Token, start, end int) int {
	for i := start; i < end; i++ {
		if tokens[i].Kind == TokenIdentifier || tokens[i].Kind == TokenKeyword {
			return i
		}
	}
	return -1
}
