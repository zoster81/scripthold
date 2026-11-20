package sourceintelligence

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/operation"
)

type FSharpAnalyzer struct{}

func (FSharpAnalyzer) ID() AnalyzerID   { return AnalyzerFSharp }
func (FSharpAnalyzer) Language() string { return "fsharp" }

func FSharpScannerProfile() ScannerProfile {
	return ScannerProfile{Name: "fsharp", Keywords: []string{"abstract", "and", "class", "end", "inherit", "interface", "let", "member", "module", "namespace", "open", "override", "static", "type", "val", "with"}, LineComments: []string{"//"}, BlockComments: []BlockCommentRule{{Start: "(*", End: "*)", Nestable: true}}, Strings: []StringRule{{Prefixes: []string{""}, Delimiter: "\"\"\"", Multiline: true}, {Prefixes: []string{"@", ""}, Delimiter: "\"", BackslashEscapes: true, DoubledDelimiterEscape: true}}, Directives: true, Indentation: true, AllowNonStackDedent: true, IndentationNeutralDirectives: true}
}

func maskFSharpMultiplySymbolicKeyword(text string) string {
	masked := []byte(text)
	for at := 0; at+2 < len(text); {
		relative := strings.Index(text[at:], "(*)")
		if relative < 0 {
			break
		}
		start := at + relative
		masked[start+1] = ' '
		at = start + 3
	}
	return string(masked)
}

func maskFSharpCharacterLiterals(text string) string {
	masked := []byte(text)
	for at := 0; at < len(text); {
		if text[at] == '\'' {
			if end, ok := fsharpCharacterLiteralEnd(text, at); ok {
				maskRangePreservingLines(masked, at, end)
				at = end
				continue
			}
		}
		_, size := utf8.DecodeRuneInString(text[at:])
		at += max(size, 1)
	}
	return string(masked)
}

func fsharpCharacterLiteralEnd(text string, start int) (int, bool) {
	at := start + 1
	if at >= len(text) || text[at] == '\r' || text[at] == '\n' {
		return 0, false
	}
	if text[at] != '\\' {
		_, size := utf8.DecodeRuneInString(text[at:])
		at += max(size, 1)
		if at < len(text) && text[at] == '\'' {
			return at + 1, true
		}
		return 0, false
	}
	at++
	if at >= len(text) || text[at] == '\r' || text[at] == '\n' {
		return 0, false
	}
	switch text[at] {
	case 'u':
		at++
		if !fsharpConsumeHex(text, &at, 4, 4) {
			return 0, false
		}
	case 'U':
		at++
		if !fsharpConsumeHex(text, &at, 8, 8) {
			return 0, false
		}
	case 'x':
		at++
		if !fsharpConsumeHex(text, &at, 1, 4) {
			return 0, false
		}
	default:
		_, size := utf8.DecodeRuneInString(text[at:])
		at += max(size, 1)
	}
	if at < len(text) && text[at] == '\'' {
		return at + 1, true
	}
	return 0, false
}

func fsharpConsumeHex(text string, at *int, minDigits, maxDigits int) bool {
	start := *at
	for *at < len(text) && *at-start < maxDigits && fsharpHexByte(text[*at]) {
		*at = *at + 1
	}
	return *at-start >= minDigits
}

func fsharpHexByte(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func (FSharpAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	state, err := newStructuralAnalyzerState(ctx, document, options, "fsharp", AnalyzerFSharp)
	if err != nil {
		return AnalyzerResult{}, err
	}
	masked := maskFSharpMultiplySymbolicKeyword(document.Text)
	masked = maskFSharpCharacterLiterals(masked)
	scan, err := state.scan(options, FSharpScannerProfile(), masked)
	if err != nil {
		return AnalyzerResult{}, err
	}
	lines := BuildLogicalLines(scan.Tokens, LogicalLineProfile{TrackIndentation: true})
	p := &fsharpParser{ctx: state.ctx, document: document, builder: state.builder}
	p.parse(lines)
	if err := state.ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_fsharp_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: state.builder.Result(), Dependencies: p.dependencies, Relations: p.relations}, nil
}

type fsharpModuleScope struct {
	parent   *SymbolParent
	indent   int
	indented bool
}

type fsharpParser struct {
	ctx           context.Context
	document      *SourceDocument
	builder       *SymbolBuilder
	namespace     *SymbolParent
	moduleScopes  []fsharpModuleScope
	bindingIndent int
	inBinding     bool
	currentType   *SymbolParent
	typeParents   map[string]SymbolParent
	typeIndent    int
	dependencies  []StructuralDependency
	relations     []StructuralRelation
	stopped       bool
}

func (p *fsharpParser) parse(lines []LogicalLine) {
	for _, line := range lines {
		if p.stopped || p.ctx.Err() != nil || len(line.Tokens) == 0 {
			continue
		}
		t := line.Tokens
		first := strings.ToLower(t[0].Text)
		for len(p.moduleScopes) > 0 {
			scope := p.moduleScopes[len(p.moduleScopes)-1]
			if !scope.indented || line.Indent > scope.indent {
				break
			}
			p.namespace = scope.parent
			p.moduleScopes = p.moduleScopes[:len(p.moduleScopes)-1]
		}
		if p.inBinding && line.Indent <= p.bindingIndent {
			p.inBinding = false
		}
		if p.currentType != nil && line.Indent <= p.typeIndent && (first == "type" || first == "let" || first == "module" || first == "namespace") {
			p.currentType = nil
		}
		switch first {
		case "namespace", "module":
			p.inBinding = false
			p.addContainer(line, first)
		case "open":
			p.addDependency(line)
		case "type":
			p.inBinding = false
			p.addType(line)
		case "abstract":
			if p.currentType != nil {
				p.addMember(line, "abstract-member")
			}
		case "member", "override":
			if p.currentType != nil {
				p.addMember(line, first)
			}
		case "static":
			if p.currentType != nil && len(t) > 1 && strings.EqualFold(t[1].Text, "member") {
				p.addMember(line, "member")
			}
		case "inherit", "interface":
			if p.currentType != nil {
				p.addRelation(line, first)
			}
		case "let":
			if p.currentType == nil && !p.inBinding {
				p.addLet(line)
				p.bindingIndent = line.Indent
				p.inBinding = true
			}
		}
	}
}
func (p *fsharpParser) addContainer(line LogicalLine, kind string) {
	tokens := line.Tokens
	idx := nextIdentifierToken(tokens, 1, len(tokens))
	for idx >= 0 && (strings.EqualFold(tokens[idx].Text, "internal") ||
		strings.EqualFold(tokens[idx].Text, "private") ||
		strings.EqualFold(tokens[idx].Text, "public") ||
		strings.EqualFold(tokens[idx].Text, "rec")) {
		idx = nextIdentifierToken(tokens, idx+1, len(tokens))
	}
	if idx < 0 {
		return
	}
	end := idx
	for end+2 < len(tokens) && tokens[end+1].Text == "." && tokens[end+2].Kind == TokenIdentifier {
		end += 2
	}
	name := tokenRangeText(tokens, idx, end+1)
	sk := SymbolKindNamespace
	if kind == "module" {
		sk = SymbolKindModule
	}
	var parent *SymbolParent
	if kind == "module" {
		parent = p.namespace
	}
	s, ok := p.add(SymbolSpec{Kind: sk, NativeKind: kind, Name: name, Parent: parent, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: tokens[idx].StartOffset, End: tokens[end].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
	if !ok {
		return
	}
	v := SymbolParent{ID: s.ID, QualifiedName: s.QualifiedName}
	if kind == "namespace" {
		p.moduleScopes = nil
		p.namespace = &v
		return
	}
	// A module abbreviation (module Alias = Other.Module) declares an alias,
	// but does not make subsequent declarations children of that alias.
	alias := end+2 < len(tokens) && tokens[end+1].Text == "="
	if alias {
		for _, token := range tokens[end+2:] {
			if token.Kind != TokenIdentifier && token.Text != "." {
				alias = false
				break
			}
		}
	}
	if alias {
		return
	}
	p.moduleScopes = append(p.moduleScopes, fsharpModuleScope{
		parent: p.namespace, indent: line.Indent,
		indented: tokens[len(tokens)-1].Text == "=",
	})
	p.namespace = &v
}
func (p *fsharpParser) addDependency(line LogicalLine) {
	if len(line.Tokens) < 2 {
		return
	}
	start := nextIdentifierToken(line.Tokens, 1, len(line.Tokens))
	if start < 0 {
		return
	}
	value := tokenRangeText(line.Tokens, start, len(line.Tokens))
	r, err := p.document.RangeFromUTF8Offsets(line.Tokens[start].StartOffset, line.Tokens[len(line.Tokens)-1].EndOffset)
	if err == nil {
		p.dependencies = append(p.dependencies, StructuralDependency{Kind: StructuralDependencyImport, Value: value, Range: r, Evidence: SymbolEvidenceStructural})
	}
}
func (p *fsharpParser) addType(line LogicalLine) {
	idx := nextIdentifierToken(line.Tokens, 1, len(line.Tokens))
	for idx >= 0 && (strings.EqualFold(line.Tokens[idx].Text, "internal") ||
		strings.EqualFold(line.Tokens[idx].Text, "private") ||
		strings.EqualFold(line.Tokens[idx].Text, "public") ||
		strings.EqualFold(line.Tokens[idx].Text, "rec")) {
		idx = nextIdentifierToken(line.Tokens, idx+1, len(line.Tokens))
	}
	if idx < 0 {
		return
	}
	parent := p.namespace
	name := line.Tokens[idx].Text
	qualifiedName := name
	if parent != nil {
		qualifiedName = parent.QualifiedName + "." + name
	}
	for i := idx + 1; i < len(line.Tokens); i++ {
		if line.Tokens[i].Text == "=" {
			break
		}
		if strings.EqualFold(line.Tokens[i].Text, "with") {
			if existing, ok := p.typeParents[qualifiedName]; ok {
				p.currentType = &existing
			} else {
				p.currentType = nil
				p.builder.MarkIncomplete()
			}
			p.typeIndent = line.Indent
			return
		}
	}
	s, ok := p.add(SymbolSpec{Kind: SymbolKindType, NativeKind: "type", Name: line.Tokens[idx].Text, Parent: parent, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[idx].StartOffset, End: line.Tokens[idx].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
	if ok {
		v := SymbolParent{ID: s.ID, QualifiedName: s.QualifiedName}
		if p.typeParents == nil {
			p.typeParents = make(map[string]SymbolParent)
		}
		p.typeParents[v.QualifiedName] = v
		p.currentType = &v
		p.typeIndent = line.Indent
	}
}
func (p *fsharpParser) addMember(line LogicalLine, native string) {
	memberAt := -1
	for i, t := range line.Tokens {
		if strings.EqualFold(t.Text, "member") || (native == "override" && i == 0 && strings.EqualFold(t.Text, "override")) {
			memberAt = i
			break
		}
	}
	if memberAt < 0 {
		return
	}
	// F# permits backtick-delimited symbolic member names (for example,
	// static member ``***``). These are not identifier tokens in the
	// generic scanner, so use the exact bounded source span after "member".
	start := line.Tokens[memberAt].EndOffset
	if memberAt+1 < len(line.Tokens) && strings.EqualFold(line.Tokens[memberAt+1].Text, "inline") {
		start = line.Tokens[memberAt+1].EndOffset
	}
	if start >= 0 && start <= line.EndOffset && line.EndOffset <= len(p.document.Text) {
		at := start
		for at < line.EndOffset && (p.document.Text[at] == ' ' || p.document.Text[at] == '\t') {
			at++
		}
		if at+2 <= line.EndOffset && p.document.Text[at:at+2] == "``" {
			nameStart := at + 2
			if closeAt := strings.Index(p.document.Text[nameStart:line.EndOffset], "``"); closeAt > 0 {
				nameEnd := nameStart + closeAt
				p.add(SymbolSpec{Kind: SymbolKindMethod, NativeKind: native,
					Name: p.document.Text[nameStart:nameEnd], Parent: p.currentType,
					Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset},
					NameRange:   OffsetRange{Start: nameStart, End: nameEnd},
					Signature:   &OffsetRange{Start: line.StartOffset, End: line.EndOffset},
					Evidence:    SymbolEvidenceStructural})
				return
			}
		}
	}
	name := -1
	for i := memberAt + 1; i < len(line.Tokens); i++ {
		if name < 0 && (strings.EqualFold(line.Tokens[i].Text, "inline") ||
			strings.EqualFold(line.Tokens[i].Text, "private") ||
			strings.EqualFold(line.Tokens[i].Text, "internal") ||
			strings.EqualFold(line.Tokens[i].Text, "public")) {
			continue
		}
		if line.Tokens[i].Kind == TokenIdentifier {
			if i+2 < len(line.Tokens) && line.Tokens[i+1].Text == "." && line.Tokens[i+2].Kind == TokenIdentifier {
				name = i + 2
			} else if name < 0 {
				name = i
			}
		}
		if line.Tokens[i].Text == ":" || line.Tokens[i].Text == "(" || line.Tokens[i].Text == "=" {
			break
		}
	}
	if name < 0 {
		return
	}
	p.add(SymbolSpec{Kind: SymbolKindMethod, NativeKind: native, Name: line.Tokens[name].Text, Parent: p.currentType, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[name].StartOffset, End: line.Tokens[name].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
}
func (p *fsharpParser) addRelation(line LogicalLine, kind string) {
	if p.currentType == nil || len(line.Tokens) < 2 {
		return
	}
	target := tokenRangeText(line.Tokens, 1, len(line.Tokens))
	if target == "" {
		return
	}
	r, err := p.document.RangeFromUTF8Offsets(line.Tokens[1].StartOffset, line.Tokens[len(line.Tokens)-1].EndOffset)
	if err == nil {
		p.relations = append(p.relations, StructuralRelation{Kind: kind, Source: p.currentType.QualifiedName, Target: target, Range: r, Evidence: SymbolEvidenceStructural})
	}
}
func (p *fsharpParser) addLet(line LogicalLine) {
	tokens := line.Tokens
	bindingAt := 1
	for bindingAt < len(tokens) {
		modifier := strings.ToLower(tokens[bindingAt].Text)
		if modifier != "inline" && modifier != "rec" && modifier != "mutable" &&
			modifier != "private" && modifier != "internal" && modifier != "public" {
			break
		}
		bindingAt++
	}
	// Scanner-owned source tokens distinguish a directly authored symbolic
	// function name from parameter identifiers; do not search through strings
	// or comments and accidentally promote their contents to declarations.
	if bindingAt+2 < len(tokens) &&
		tokens[bindingAt].Text == "(" && tokens[bindingAt+1].Text == "?" &&
		tokens[bindingAt+2].Text == ")" &&
		tokens[bindingAt].EndOffset == tokens[bindingAt+1].StartOffset &&
		tokens[bindingAt+1].EndOffset == tokens[bindingAt+2].StartOffset {
		name := tokens[bindingAt+1]
		p.add(SymbolSpec{Kind: SymbolKindFunction, NativeKind: "let", Name: name.Text, Parent: p.namespace,
			Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset},
			NameRange:   OffsetRange{Start: name.StartOffset, End: name.EndOffset},
			Signature:   &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
		return
	}
	idx := nextIdentifierToken(tokens, bindingAt, len(tokens))
	if idx < 0 {
		return
	}
	kind := SymbolKindFunction
	if idx+1 < len(tokens) && (tokens[idx+1].Text == ":" || tokens[idx+1].Text == "=") {
		kind = SymbolKindVariable
	}
	p.add(SymbolSpec{Kind: kind, NativeKind: "let", Name: tokens[idx].Text, Parent: p.namespace,
		Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset},
		NameRange:   OffsetRange{Start: tokens[idx].StartOffset, End: tokens[idx].EndOffset},
		Signature:   &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
}
func (p *fsharpParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
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

type CPPCLIAnalyzer struct{}

func (CPPCLIAnalyzer) ID() AnalyzerID   { return AnalyzerCPPCLI }
func (CPPCLIAnalyzer) Language() string { return "cpp-cli" }
func (CPPCLIAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	masked := maskCPPCLIModifiers(document.Text)
	masked, err := maskCPPCLIBlockPropertyDelimiters(ctx, document, masked, options.MaxNesting)
	if err != nil {
		return AnalyzerResult{}, err
	}
	clone := *document
	clone.Text = masked
	clone.lineStarts = buildLineStarts(masked)
	source, err := (CPPAnalyzer{}).Analyze(ctx, &clone, options)
	if err != nil {
		return AnalyzerResult{}, err
	}
	classifyCPPCLISimpleProperties(document.Text, source.Analysis.Symbols)
	classifyCPPCLIBlockProperties(document.Text, source.Analysis.Symbols)
	analysis, err := reprojectAnalyzerSymbols(ctx, document, source, options, "cpp-cli", AnalyzerCPPCLI, "", 0, nil)
	if err != nil {
		return AnalyzerResult{}, err
	}
	return AnalyzerResult{Analysis: analysis, Dependencies: source.Dependencies, Relations: source.Relations}, nil
}

// The shared C++ parser recognizes a simple managed property as a field.
// Reclassify only an authored property-prefixed field; compound properties
// and their accessor ownership remain outside this narrow adapter change.
func classifyCPPCLISimpleProperties(text string, symbols []NormalizedSymbol) {
	for index := range symbols {
		symbol := &symbols[index]
		if symbol.Kind != SymbolKindField {
			continue
		}
		declaration, _, _, _ := symbol.SourceOffsets()
		if declaration.Start < 0 || declaration.End > len(text) || declaration.Start >= declaration.End {
			continue
		}
		spelling := strings.TrimLeft(text[declaration.Start:declaration.End], " \t\r\n")
		if len(spelling) <= len("property") || !strings.HasPrefix(spelling, "property") ||
			(spelling[len("property")] != ' ' && spelling[len("property")] != '\t') ||
			!strings.HasSuffix(strings.TrimSpace(spelling), ";") {
			continue
		}
		symbol.Kind = SymbolKindProperty
		symbol.NativeKind = "simple-property"
	}
}

// The shared parser sees paired block properties as lexical struct surrogates.
// Restore authored property semantics without changing their source ranges.
func classifyCPPCLIBlockProperties(text string, symbols []NormalizedSymbol) {
	for index := range symbols {
		symbol := &symbols[index]
		if symbol.Kind != SymbolKindStruct {
			continue
		}
		declaration, _, _, _ := symbol.SourceOffsets()
		if declaration.Start < 0 || declaration.End > len(text) || declaration.Start >= declaration.End {
			continue
		}
		spelling := strings.TrimLeft(text[declaration.Start:declaration.End], " \t\r\n")
		if !strings.HasPrefix(spelling, "property ") && !strings.HasPrefix(spelling, "property\t") {
			continue
		}
		symbol.Kind = SymbolKindProperty
		symbol.NativeKind = "block-property"
	}
}

var (
	cppCLIAssemblyAttribute     = regexp.MustCompile(`(?mi)^[ \t]*\[assembly[ \t]*:[^\r\n]*\]`)
	cppCLIStandaloneAttribute   = regexp.MustCompile(`(?m)^[ \t]*\[[A-Za-z_][A-Za-z0-9_:]*(?:\([^\]\r\n]*\))?\][ \t]*\r?$`)
	cppCLITrailingClassModifier = regexp.MustCompile(`(?i)\bclass[ \t]+[A-Za-z_][A-Za-z0-9_]*((?:[ \t]+(?:sealed|abstract))+)\b`)
	cppCLIInternalAccessLabel   = regexp.MustCompile(`(?m)^[ \t]*internal[ \t]*:`)
)

func maskCPPCLIModifiers(text string) string {
	result := []byte(text)
	words := []string{"ref", "value", "interface", "sealed", "abstract"}
	for _, word := range words {
		search := 0
		for search < len(text) {
			relative := strings.Index(text[search:], word)
			if relative < 0 {
				break
			}
			start := search + relative
			end := start + len(word)
			before := start == 0 || !isASCIIIdentifierByte(text[start-1])
			after := end == len(text) || !isASCIIIdentifierByte(text[end])
			if before && after {
				tail := strings.TrimLeft(text[end:], " \t")
				if strings.HasPrefix(tail, "class") || strings.HasPrefix(tail, "struct") {
					for i := start; i < end; i++ {
						if result[i] != '\r' && result[i] != '\n' {
							result[i] = ' '
						}
					}
				}
			}
			search = end
		}
	}
	// C++/CLI metadata is not a C++ declaration. A standalone managed
	// attribute must not conceal the following function or expose its body.
	for _, match := range cppCLIAssemblyAttribute.FindAllStringIndex(text, -1) {
		maskByteRangePreservingLines(result, match[0], match[1])
	}
	for _, match := range cppCLIStandaloneAttribute.FindAllStringIndex(text, -1) {
		maskByteRangePreservingLines(result, match[0], match[1])
	}
	// One or more trailing managed type modifiers (e.g. abstract sealed)
	// follow the name. Mask the entire suffix; otherwise the C++ parser
	// treats the remaining modifier as a variable and loses the class scope.
	for _, match := range cppCLITrailingClassModifier.FindAllStringSubmatchIndex(text, -1) {
		maskByteRangePreservingLines(result, match[2], match[3])
	}
	// The managed-only internal: access label is not a C++ declarator.
	// Preserve its coordinates without leaking it into the following property.
	for _, match := range cppCLIInternalAccessLabel.FindAllStringIndex(text, -1) {
		maskByteRangePreservingLines(result, match[0], match[1])
	}
	return string(result)
}

// Rewrite paired managed property headers into same-offset struct surrogates.
// The shared C++ parser then retains get/set ownership and body ranges, while
// the bounded scanner/pairing ignores braces in strings and comments.
func maskCPPCLIBlockPropertyDelimiters(ctx context.Context, document *SourceDocument, text string, maxNesting int) (string, error) {
	if !strings.Contains(text, "property") {
		return text, nil
	}
	probe := *document
	probe.Text = text
	probe.lineStarts = buildLineStarts(text)
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, &probe, cFamilyCPPScannerProfile, ScannerLimits{
		MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting,
	})
	if err != nil {
		return "", err
	}
	if !scan.Complete {
		return text, nil
	}
	pairs := PairDelimiterTokens(scan.Tokens, nil)
	result := []byte(text)
	for index, token := range scan.Tokens {
		if token.Text != "property" {
			continue
		}
		lineIndex := sort.SearchInts(probe.lineStarts, token.StartOffset+1) - 1
		if lineIndex < 0 {
			continue
		}
		lineStart := probe.lineStarts[lineIndex]
		if token.StartOffset-lineStart > 512 || strings.TrimSpace(text[lineStart:token.StartOffset]) != "" {
			continue // Only a directly authored property header, not an expression.
		}
		depth := token.Nesting
		for next := index + 1; next < len(scan.Tokens) && next-index <= 32; next++ {
			candidate := scan.Tokens[next]
			if candidate.StartOffset-token.StartOffset > 512 || candidate.Nesting < depth ||
				candidate.Kind == TokenEOF || candidate.Kind == TokenDirective {
				break
			}
			if candidate.Text == ";" || candidate.Text == "}" || candidate.Text == "=" || candidate.Text == "(" {
				break
			}
			if candidate.Text != "{" {
				continue
			}
			if candidate.Nesting == depth+1 {
				if close, ok := pairs[next]; ok && close > next {
					nameIndex := -1
					for at := index + 1; at < next; at++ {
						if scan.Tokens[at].Kind == TokenIdentifier && scan.Tokens[at].Nesting == depth {
							nameIndex = at
						}
					}
					validName := nameIndex > index+1 &&
						scan.Tokens[nameIndex].StartOffset-token.StartOffset >= len("struct ") &&
						previousStructuralToken(scan.Tokens, next-1, index+1) == nameIndex
					beforeName := previousStructuralToken(scan.Tokens, nameIndex-1, index+1)
					if validName && beforeName >= index+1 &&
						(scan.Tokens[beforeName].Text == ":" || scan.Tokens[beforeName].Text == ".") {
						validName = false
					}
					if validName {
						maskRangePreservingLines(result, token.StartOffset, scan.Tokens[nameIndex].StartOffset)
						copy(result[token.StartOffset:], "struct")
					} else {
						// Preserve the previous conservative mask for ambiguous or
						// malformed property headers; do not fabricate an owner.
						maskRangePreservingLines(result, token.StartOffset, candidate.EndOffset)
						maskRangePreservingLines(result, scan.Tokens[close].StartOffset, scan.Tokens[close].EndOffset)
					}
				}
			}
			break
		}
	}
	return string(result), nil
}

func isASCIIIdentifierByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}
