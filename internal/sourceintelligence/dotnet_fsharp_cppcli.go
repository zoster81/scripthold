package sourceintelligence

import (
	"context"
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
				phase8MaskRange(masked, at, end)
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
	state, err := newPhase8State(ctx, document, options, "fsharp", AnalyzerFSharp)
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

type fsharpParser struct {
	ctx          context.Context
	document     *SourceDocument
	builder      *SymbolBuilder
	namespace    *SymbolParent
	currentType  *SymbolParent
	typeIndent   int
	dependencies []StructuralDependency
	relations    []StructuralRelation
	stopped      bool
}

func (p *fsharpParser) parse(lines []LogicalLine) {
	for _, line := range lines {
		if p.stopped || p.ctx.Err() != nil || len(line.Tokens) == 0 {
			continue
		}
		t := line.Tokens
		first := strings.ToLower(t[0].Text)
		if p.currentType != nil && line.Indent <= p.typeIndent && (first == "type" || first == "let" || first == "module" || first == "namespace") {
			p.currentType = nil
		}
		switch first {
		case "namespace", "module":
			p.addContainer(line, first)
		case "open":
			p.addDependency(line)
		case "type":
			p.addType(line)
		case "abstract":
			if p.currentType != nil {
				p.addMember(line, "abstract-member")
			}
		case "member", "override":
			if p.currentType != nil {
				p.addMember(line, first)
			}
		case "inherit", "interface":
			if p.currentType != nil {
				p.addRelation(line, first)
			}
		case "let":
			if p.currentType == nil {
				p.addLet(line)
			}
		}
	}
}
func (p *fsharpParser) addContainer(line LogicalLine, kind string) {
	idx := nextIdentifierToken(line.Tokens, 1, len(line.Tokens))
	if idx < 0 {
		return
	}
	sk := SymbolKindNamespace
	if kind == "module" {
		sk = SymbolKindModule
	}
	s, ok := p.add(SymbolSpec{Kind: sk, NativeKind: kind, Name: line.Tokens[idx].Text, Parent: nil, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[idx].StartOffset, End: line.Tokens[idx].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
	if ok {
		v := SymbolParent{ID: s.ID, QualifiedName: s.QualifiedName}
		p.namespace = &v
	}
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
	if idx < 0 {
		return
	}
	parent := p.namespace
	s, ok := p.add(SymbolSpec{Kind: SymbolKindType, NativeKind: "type", Name: line.Tokens[idx].Text, Parent: parent, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[idx].StartOffset, End: line.Tokens[idx].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
	if ok {
		v := SymbolParent{ID: s.ID, QualifiedName: s.QualifiedName}
		p.currentType = &v
		p.typeIndent = line.Indent
	}
}
func (p *fsharpParser) addMember(line LogicalLine, native string) {
	memberAt := -1
	for i, t := range line.Tokens {
		if strings.EqualFold(t.Text, "member") {
			memberAt = i
			break
		}
	}
	if memberAt < 0 {
		return
	}
	name := -1
	for i := memberAt + 1; i < len(line.Tokens); i++ {
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
	idx := nextIdentifierToken(line.Tokens, 1, len(line.Tokens))
	if idx < 0 {
		return
	}
	p.add(SymbolSpec{Kind: SymbolKindFunction, NativeKind: "let", Name: line.Tokens[idx].Text, Parent: p.namespace, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[idx].StartOffset, End: line.Tokens[idx].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
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
	clone := *document
	clone.Text = masked
	clone.lineStarts = buildLineStarts(masked)
	source, err := (CPPAnalyzer{}).Analyze(ctx, &clone, options)
	if err != nil {
		return AnalyzerResult{}, err
	}
	analysis, err := reprojectAnalyzerSymbols(ctx, document, source, options, "cpp-cli", AnalyzerCPPCLI, "", 0, nil)
	if err != nil {
		return AnalyzerResult{}, err
	}
	return AnalyzerResult{Analysis: analysis, Dependencies: source.Dependencies, Relations: source.Relations}, nil
}
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
	return string(result)
}
func isASCIIIdentifierByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}
