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
			value := quotedStringValue(p.tokens[j].Text)
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
	if open < 0 && semicolon < 0 {
		p.builder.MarkIncomplete()
		return closeParen + 1
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
	symbol, added := p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: p.tokens[nameIndex].Text, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: declarationEnd}, NameRange: OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[terminator].StartOffset}, Body: body, Evidence: SymbolEvidenceStructural, Disambiguator: tokenRangeText(p.tokens, paren, closeParen+1)})
	if added && open >= 0 {
		// Only type factories returning a direct anonymous struct expose
		// its declarations as members of the factory's result.
		for i := closeParen + 1; i < open; i++ {
			if p.tokens[i].Text == "type" {
				p.parseReturnedStructMembers(open+1, p.pairs[open], &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName})
				break
			}
		}
	}
	return next
}

func (p *zigParser) parseReturnedStructMembers(start, end int, parent *SymbolParent) {
	for i := start; i < end && !p.stopped; i++ {
		if p.ctx.Err() != nil {
			return
		}
		// Nested blocks can contain local types, not declarations of the
		// returned type; never index their members as the factory's API.
		if p.tokens[i].Text == "{" {
			if close := p.pairs[i]; close > i && close < end {
				i = close
			}
			continue
		}
		if p.tokens[i].Text != "return" {
			continue
		}
		kind := nextStructuralToken(p.tokens, i+1, end)
		if kind >= end || p.tokens[kind].Text != "struct" {
			continue
		}
		open := nextStructuralToken(p.tokens, kind+1, end)
		if open >= end || p.tokens[open].Text != "{" {
			continue
		}
		close := p.pairs[open]
		if close <= open || close >= end {
			p.builder.MarkIncomplete()
			return
		}
		p.parseScope(open+1, close, parent, true)
		i = close
	}
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
	scanDocument := document
	if masked := maskNimApostropheSuffixes(document.Text); masked != document.Text {
		clone := *document
		clone.Text = masked
		scanDocument = &clone
	}
	scan, err := ScanSource(ctx, scanDocument, NimScannerProfile(), ScannerLimits{MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
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

func maskNimApostropheSuffixes(text string) string {
	masked := []byte(text)
	for at := 0; at < len(text); at++ {
		if text[at] != '\'' || at+1 >= len(text) {
			continue
		}
		if at > 0 && text[at-1] == '`' && nimSuffixByte(text[at+1]) {
			masked[at] = ' '
			continue
		}
		if at == 0 || !nimNumericSuffixLeftByte(text[at-1]) || !nimSuffixByte(text[at+1]) {
			continue
		}
		end := at + 2
		for end < len(text) && nimSuffixByte(text[end]) {
			end++
		}
		maskRangePreservingLines(masked, at, end)
		at = end - 1
	}
	return string(masked)
}

func nimNumericSuffixLeftByte(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'A' && value <= 'F' || value >= 'a' && value <= 'f'
}

func nimSuffixByte(value byte) bool {
	return value == '_' || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
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
	callableScopes    []nimCallableScope
	stopped           bool
}

type nimCallableScope struct {
	indent int
	parent SymbolParent
}

func (p *nimParser) parse(lines []LogicalLine) {
	for _, line := range lines {
		if p.stopped || p.ctx.Err() != nil || len(line.Tokens) == 0 {
			continue
		}
		if p.currentType != nil && line.Indent <= p.currentTypeIndent {
			p.currentType = nil
		}
		for len(p.callableScopes) > 0 && line.Indent <= p.callableScopes[len(p.callableScopes)-1].indent {
			p.callableScopes = p.callableScopes[:len(p.callableScopes)-1]
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
		if first == "from" {
			p.fromImportLine(line)
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
	tokens := line.Tokens
	depth := tokens[0].Nesting
	for i := 1; i < len(tokens); i++ {
		if tokens[i].Text != "except" || tokens[i].Nesting != depth {
			continue
		}
		// Preserve the existing grouped-import unsupported diagnostic for
		// syntactically distinct bracketed source module expressions.
		grouped := false
		for _, token := range tokens[1:i] {
			if token.Text == "[" || token.Text == "]" {
				grouped = true
				break
			}
		}
		if !grouped {
			p.exceptImportLine(tokens, i)
			return
		}
		break
	}
	for _, part := range splitCommaTokenRangeAt(tokens, 1, len(tokens), depth) {
		start, end := part[0], part[1]
		if start >= end {
			continue
		}
		// Keep the existing unsupported-alias diagnostic for a bracketed
		// expression followed by a top-level "as" suffix.
		aliasSuffix := end-start >= 2 && tokens[end-2].Text == "as" &&
			tokens[end-2].Nesting == depth
		if tokens[0].Text == "import" && !aliasSuffix {
			if grouped, handled := p.nimGroupedImportPart(tokens, start, end); handled {
				if grouped == nil {
					span := OffsetRange{Start: tokens[start].StartOffset, End: tokens[end-1].EndOffset}
					_ = p.builder.AddDiagnostic(DiagnosticSpec{
						Code:     "nim-unsupported-grouped-import",
						Message:  "Nim grouped import has unsupported syntax; inspect the authored directive",
						Severity: DiagnosticWarning, Range: &span, AffectsCoverage: true,
					})
					p.builder.MarkIncomplete()
				} else {
					p.dependencies = appendUniqueDependencies(p.dependencies, grouped)
				}
				continue
			}
		}
		moduleEnd := end
		alias := ""
		invalid := false
		for i := start; i < end; i++ {
			if tokens[i].Text != "as" || tokens[i].Nesting != depth {
				continue
			}
			if i != end-2 || tokens[i+1].Kind != TokenIdentifier ||
				tokens[i+1].Nesting != depth {
				invalid = true
			} else {
				moduleEnd = i
				alias = tokens[i+1].Text
			}
			break
		}
		module := tokenRangeText(tokens, start, moduleEnd)
		if module == "" {
			invalid = true
		}
		if alias != "" {
			begin, finish := tokens[start].StartOffset, tokens[moduleEnd-1].EndOffset
			if !nimSimpleImportModule(module) || begin < 0 ||
				finish > len(p.document.Text) || begin >= finish ||
				p.document.Text[begin:finish] != module {
				invalid = true
			}
		}
		if invalid {
			p.builder.MarkIncomplete()
			span := OffsetRange{Start: tokens[start].StartOffset, End: tokens[end-1].EndOffset}
			_ = p.builder.AddDiagnostic(DiagnosticSpec{
				Code:     "nim-unsupported-import-alias",
				Message:  "Nim import alias has unsupported syntax; inspect the authored directive",
				Severity: DiagnosticWarning, Range: &span, AffectsCoverage: true,
			})
			continue
		}
		r, err := p.document.RangeFromUTF8Offsets(tokens[start].StartOffset, tokens[moduleEnd-1].EndOffset)
		if err != nil {
			p.builder.MarkIncomplete()
			continue
		}
		p.dependencies = appendUniqueDependencies(p.dependencies, []StructuralDependency{{
			Kind: StructuralDependencyImport, Value: module, Alias: alias, Range: r,
			Evidence: SymbolEvidenceStructural,
		}})
	}
}

// Except selectors filter member names imported from one source module.
// Only the module itself forms a structural dependency. Unsupported directives
// must not invent one module per selector or silently claim complete coverage.
func (p *nimParser) exceptImportLine(tokens []Token, delimiter int) {
	depth := tokens[0].Nesting
	valid := delimiter > 1 && delimiter+1 < len(tokens)
	module := ""
	if valid {
		start, end := tokens[1].StartOffset, tokens[delimiter-1].EndOffset
		valid = start >= 0 && end <= len(p.document.Text) && start < end
		if valid && delimiter == 2 && tokens[1].Kind == TokenString {
			quoted := tokens[1].Text
			module = quotedTokenStringValue(tokens[1])
			valid = len(quoted) >= 2 && quoted[0] == '"' &&
				quoted[len(quoted)-1] == '"' && quoted == "\""+module+"\"" &&
				nimSimpleImportModule(module)
		} else if valid {
			module = tokenRangeText(tokens, 1, delimiter)
			valid = nimSimpleImportModule(module) && p.document.Text[start:end] == module
		}
	}
	for i := delimiter + 1; valid && i < len(tokens); i++ {
		token := tokens[i]
		if token.Nesting != depth {
			valid = false
		} else if (i-delimiter)%2 == 1 {
			if token.Kind != TokenIdentifier || !nimSimpleImportModule(token.Text) ||
				strings.ContainsAny(token.Text, "./") {
				valid = false
			}
		} else if token.Text != "," {
			valid = false
		}
	}
	if (len(tokens)-delimiter)%2 != 0 {
		valid = false
	}
	if !valid {
		p.builder.MarkIncomplete()
		if len(tokens) > 1 {
			span := OffsetRange{Start: tokens[1].StartOffset, End: tokens[len(tokens)-1].EndOffset}
			_ = p.builder.AddDiagnostic(DiagnosticSpec{
				Code:     "nim-unsupported-import-except",
				Message:  "Nim import except has unsupported syntax; inspect the authored directive",
				Severity: DiagnosticWarning, Range: &span, AffectsCoverage: true,
			})
		}
		return
	}
	r, err := p.document.RangeFromUTF8Offsets(tokens[1].StartOffset, tokens[delimiter-1].EndOffset)
	if err != nil {
		p.builder.MarkIncomplete()
		return
	}
	p.dependencies = appendUniqueDependencies(p.dependencies, []StructuralDependency{{
		Kind: StructuralDependencyImport, Value: module, Range: r,
		Evidence: SymbolEvidenceStructural,
	}})
}

// Grouped imports expand only simple member identifiers. The source range of
// each logical module points to its authored member (the shared prefix is
// outside that contiguous range), so navigation does not fabricate a span.
func (p *nimParser) nimGroupedImportPart(tokens []Token, start, end int) ([]StructuralDependency, bool) {
	open := -1
	for i := start; i < end; i++ {
		if tokens[i].Text == "[" || tokens[i].Text == "]" {
			if open >= 0 {
				if tokens[i].Text == "[" || i != end-1 {
					return nil, true
				}
			} else if tokens[i].Text != "[" {
				return nil, true
			} else {
				open = i
			}
		}
	}
	if open < 0 {
		return nil, false
	}
	if open < start+2 || end < open+3 ||
		tokens[open-1].Text != "/" || tokens[end-1].Text != "]" {
		return nil, true
	}
	prefix := tokenRangeText(tokens, start, open-1)
	if !nimSimpleImportModule(prefix) {
		return nil, true
	}
	// Whitespace between two module-name tokens must not be collapsed into
	// an invented prefix; spaces around slash separators are permitted.
	for i := start + 1; i < open; i++ {
		if tokens[i-1].Text == "/" || tokens[i].Text == "/" {
			continue
		}
		if tokens[i].StartOffset != tokens[i-1].EndOffset {
			return nil, true
		}
	}
	members := make([]StructuralDependency, 0, (end-open)/2)
	for i := open + 1; i < end-1; i++ {
		token := tokens[i]
		if (i-open)%2 == 0 {
			if token.Text != "," {
				return nil, true
			}
			continue
		}
		if token.Kind != TokenIdentifier || !nimSimpleImportModule(token.Text) ||
			strings.ContainsAny(token.Text, "/.") {
			return nil, true
		}
		r, err := p.document.RangeFromUTF8Offsets(token.StartOffset, token.EndOffset)
		if err != nil {
			return nil, true
		}
		members = append(members, StructuralDependency{
			Kind: StructuralDependencyImport, Value: prefix + "/" + token.Text,
			Range: r, Evidence: SymbolEvidenceStructural,
		})
	}
	if len(members) == 0 || tokens[end-2].Text == "," {
		return nil, true
	}
	return members, true
}

// Aliased imports are indexed only when the module's lexical spelling is a
// simple path; grouped and computed module expressions need separate parsing.
func nimSimpleImportModule(module string) bool {
	module = strings.TrimPrefix(strings.TrimPrefix(module, "../"), "./")
	if module == "" {
		return false
	}
	for _, segment := range strings.Split(module, "/") {
		if segment == "" || segment[0] == '.' || segment[len(segment)-1] == '.' ||
			strings.Contains(segment, "..") {
			return false
		}
		for i := 0; i < len(segment); i++ {
			c := segment[i]
			letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
			digit := i > 0 && c >= '0' && c <= '9'
			if c != '_' && c != '.' && !letter && !digit {
				return false
			}
		}
	}
	return true
}

// Nim selective imports name one source module before the "import" keyword.
// Imported member names do not form separate module dependencies.
func (p *nimParser) fromImportLine(line LogicalLine) {
	tokens := line.Tokens
	delimiter := -1
	for i := 2; i < len(tokens); i++ {
		if tokens[i].Text == "import" && tokens[i].Nesting == tokens[0].Nesting {
			delimiter = i
			break
		}
	}
	valid := delimiter > 1 && delimiter+1 < len(tokens)
	if valid {
		for i := 1; i < delimiter; i++ {
			if tokens[i].Text == "as" {
				valid = false
				break
			}
		}
	}
	module := ""
	if valid {
		if tokens[1].Kind == TokenString {
			// Nim also permits "quoted-root" / module/path. Preserve the
			// authored full range, but never resolve the symbolic "$nim" root.
			module, valid = p.nimComposedFromModule(tokens, delimiter)
			if valid {
				for i := delimiter + 1; i < len(tokens); i++ {
					if (i-delimiter)%2 == 1 {
						if tokens[i].Kind != TokenIdentifier {
							valid = false
						}
					} else if tokens[i].Text != "," {
						valid = false
					}
				}
				if (len(tokens)-delimiter)%2 != 0 {
					valid = false
				}
			}
		} else {
			module = tokenRangeText(tokens, 1, delimiter)
			// Never merge whitespace-separated names or opaque syntax into a
			// fabricated source-unit path.
			start, end := tokens[1].StartOffset, tokens[delimiter-1].EndOffset
			if module == "" || start < 0 || end > len(p.document.Text) ||
				start >= end || p.document.Text[start:end] != module {
				valid = false
			}
			for i := 0; i < len(module); i++ {
				c := module[i]
				if c != '_' && c != '.' && c != '/' &&
					(c < 'a' || c > 'z') && (c < 'A' || c > 'Z') &&
					(c < '0' || c > '9') {
					valid = false
					break
				}
			}
		}
	}
	if !valid {
		span := OffsetRange{Start: tokens[0].StartOffset, End: tokens[len(tokens)-1].EndOffset}
		_ = p.builder.AddDiagnostic(DiagnosticSpec{
			Code:     "nim-unsupported-from-import",
			Message:  "Nim selective import has unsupported module syntax; inspect the authored directive",
			Severity: DiagnosticWarning, Range: &span, AffectsCoverage: true,
		})
		p.builder.MarkIncomplete()
		return
	}
	r, err := p.document.RangeFromUTF8Offsets(tokens[1].StartOffset, tokens[delimiter-1].EndOffset)
	if err != nil {
		p.builder.MarkIncomplete()
		return
	}
	p.dependencies = appendUniqueDependencies(p.dependencies, []StructuralDependency{{
		Kind: StructuralDependencyImport, Value: module, Range: r, Evidence: SymbolEvidenceStructural,
	}})
}

// The module path has an authored quoted root, slash separator and one
// contiguous plain module suffix. $nim is an unresolved lexical root token,
// not a filesystem path inferred from the user's environment.
func (p *nimParser) nimComposedFromModule(tokens []Token, delimiter int) (string, bool) {
	if delimiter < 4 || tokens[2].Text != "/" ||
		tokens[2].Nesting != tokens[0].Nesting {
		return "", false
	}
	quoted := tokens[1].Text
	root := quotedTokenStringValue(tokens[1])
	if len(quoted) < 2 || quoted != "\""+root+"\"" ||
		root != "$nim" && !nimSimpleImportModule(root) {
		return "", false
	}
	source := p.document.Text
	if tokens[1].EndOffset > tokens[2].StartOffset ||
		tokens[2].EndOffset > tokens[3].StartOffset ||
		tokens[1].EndOffset < 0 || tokens[3].StartOffset > len(source) {
		return "", false
	}
	if strings.TrimSpace(source[tokens[1].EndOffset:tokens[2].StartOffset]) != "" ||
		strings.TrimSpace(source[tokens[2].EndOffset:tokens[3].StartOffset]) != "" {
		return "", false
	}
	suffix := tokenRangeText(tokens, 3, delimiter)
	start, end := tokens[3].StartOffset, tokens[delimiter-1].EndOffset
	if start < 0 || end > len(source) || start >= end ||
		!nimSimpleImportModule(suffix) || source[start:end] != suffix {
		return "", false
	}
	return root + "/" + suffix, true
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
	// Variant-object and conditional branch labels are not field names.
	// Quoted/backtick identifiers are handled separately by the scanner.
	switch strings.ToLower(line.Tokens[0].Text) {
	case "else", "elif", "of", "case", "when", "if":
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
	// The callable name must immediately follow the declaration keyword.
	// Symbolic/backtick names are outside this identifier-only subset;
	// searching ahead could mistake their first parameter for the function.
	if len(line.Tokens) < 2 || line.Tokens[1].Kind != TokenIdentifier {
		return
	}
	name, nr := nimNameToken(line.Tokens[1])
	if name == "" {
		return
	}
	var parent *SymbolParent
	if len(p.callableScopes) > 0 {
		parent = &p.callableScopes[len(p.callableScopes)-1].parent
	}
	if symbol, ok := p.add(SymbolSpec{Kind: SymbolKindFunction, NativeKind: native, Name: name, Parent: parent,
		Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: nr,
		Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural}); ok {
		p.callableScopes = append(p.callableScopes, nimCallableScope{indent: line.Indent,
			parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
	}
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
	var parent *SymbolParent
	if len(p.callableScopes) > 0 {
		parent = &p.callableScopes[len(p.callableScopes)-1].parent
	}
	if symbol, ok := p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: name, Parent: parent,
		Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: nr, Evidence: SymbolEvidenceStructural}); ok && nimBlockInitializer(line.Tokens) {
		p.callableScopes = append(p.callableScopes, nimCallableScope{indent: line.Indent,
			parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
	}
}

func nimBlockInitializer(tokens []Token) bool {
	if len(tokens) < 4 || tokens[len(tokens)-1].Text != ":" {
		return false
	}
	for index := 0; index+1 < len(tokens); index++ {
		if tokens[index].Text == "=" && strings.EqualFold(tokens[index+1].Text, "block") {
			return true
		}
	}
	return false
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
	return analyzeALSource(ctx, document, options)
}

func analyzeALVariant(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
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
		if kind, ok := objects[lower]; ok && !members {
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
	nameIndex := alObjectNameToken(p.tokens, keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	name := alTokenValue(p.tokens[nameIndex])
	if name == "" {
		return keyword + 1
	}
	nameRange := OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset}
	if p.tokens[nameIndex].Kind == TokenString && len(p.tokens[nameIndex].Text) >= 2 {
		nameRange.Start++
		nameRange.End--
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
		Kind: kind, NativeKind: native, Name: name, Parent: parent,
		Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[close].EndOffset},
		NameRange:   nameRange,
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
	if next < end && p.tokens[next].Text == ";" {
		after := nextStructuralToken(p.tokens, next+1, end)
		if after < end && p.tokens[after].Nesting == p.tokens[keyword].Nesting &&
			(strings.EqualFold(p.tokens[after].Text, "var") || strings.EqualFold(p.tokens[after].Text, "begin")) {
			next++
		}
	}
	kind := SymbolKindFunction
	if members {
		kind = SymbolKindMethod
	}
	signatureEnd := declarationEnd
	var body *OffsetRange
	if begin, close, ok := p.procedureBody(next, end, p.tokens[keyword].Nesting); ok {
		endToken := close
		if close+1 < end && p.tokens[close+1].Text == ";" {
			endToken++
		}
		declarationEnd = p.tokens[endToken].EndOffset
		value := OffsetRange{Start: p.tokens[begin].StartOffset, End: p.tokens[endToken].EndOffset}
		body = &value
		next = endToken + 1
	}
	p.add(SymbolSpec{
		Kind: kind, NativeKind: native, Name: p.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: p.tokens[nameIndex].StartOffset, End: p.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: p.tokens[start].StartOffset, End: signatureEnd},
		Body:        body, Evidence: SymbolEvidenceStructural,
	})
	return next
}

// AL procedures may have local variable declarations between their header
// and begin/end block. A case statement has its own end; repeat/until does not.
// Stop at another member to avoid borrowing an interface method's successor.
func (p *alParser) procedureBody(start, end, nesting int) (int, int, bool) {
	vars := false
	for i := start; i < end; i++ {
		tok := p.tokens[i]
		if tok.Nesting != nesting {
			continue
		}
		switch strings.ToLower(tok.Text) {
		case "var":
			vars = true
		case "procedure", "trigger":
			return 0, 0, false
		case ";":
			if !vars {
				return 0, 0, false
			}
		case "begin":
			depth := 0
			for j := i; j < end; j++ {
				inner := p.tokens[j]
				if inner.Nesting != nesting {
					continue
				}
				switch strings.ToLower(inner.Text) {
				case "begin", "case":
					depth++
				case "end":
					depth--
					if depth == 0 {
						return i, j, true
					}
				}
			}
			p.builder.MarkIncomplete()
			return 0, 0, false
		}
	}
	return 0, 0, false
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
	nameIndex := alFieldNameToken(p.tokens, paren, close)
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

func alFieldNameToken(tokens []Token, paren, close int) int {
	if paren < 0 || close <= paren || close > len(tokens) {
		return -1
	}
	depth := tokens[paren].Nesting
	firstSemicolon := -1
	secondSemicolon := -1
	for index := paren + 1; index < close; index++ {
		if tokens[index].Text != ";" || tokens[index].Nesting != depth {
			continue
		}
		if firstSemicolon < 0 {
			firstSemicolon = index
			continue
		}
		secondSemicolon = index
		break
	}
	if firstSemicolon < 0 {
		return -1
	}
	first := alFieldSlotToken(tokens, paren+1, firstSemicolon)
	if first < 0 {
		return -1
	}
	if tokens[first].Kind == TokenNumber {
		if secondSemicolon <= firstSemicolon {
			return -1
		}
		return alNameToken(tokens, firstSemicolon+1, secondSemicolon)
	}
	return alNameToken(tokens, paren+1, firstSemicolon)
}

func alFieldSlotToken(tokens []Token, start, end int) int {
	for index := start; index < end; index++ {
		switch tokens[index].Kind {
		case TokenNumber, TokenIdentifier, TokenString:
			return index
		}
	}
	return -1
}

func alNameToken(tokens []Token, start, end int) int {
	for index := start; index < end; index++ {
		if tokens[index].Kind == TokenIdentifier || tokens[index].Kind == TokenString {
			return index
		}
	}
	return -1
}

func alObjectNameToken(tokens []Token, start, end int) int {
	for index := start; index < end; index++ {
		if tokens[index].Text == "{" || tokens[index].Text == ";" || strings.EqualFold(tokens[index].Text, "extends") {
			return -1
		}
		if tokens[index].Kind == TokenIdentifier || tokens[index].Kind == TokenString {
			return index
		}
	}
	return -1
}

func alTokenValue(token Token) string {
	if token.Kind == TokenString {
		return quotedStringValue(token.Text)
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
