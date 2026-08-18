package sourceintelligence

import (
	"context"
	"sort"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

type ObjectiveCAnalyzer struct{}
type ObjectiveCPPAnalyzer struct{}

func (ObjectiveCAnalyzer) ID() AnalyzerID     { return AnalyzerObjectiveC }
func (ObjectiveCAnalyzer) Language() string   { return "objective-c" }
func (ObjectiveCPPAnalyzer) ID() AnalyzerID   { return AnalyzerObjectiveCPP }
func (ObjectiveCPPAnalyzer) Language() string { return "objective-cpp" }

func (ObjectiveCAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeObjectiveCFamily(ctx, document, options, "objective-c", AnalyzerObjectiveC, false)
}
func (ObjectiveCPPAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeObjectiveCFamily(ctx, document, options, "objective-cpp", AnalyzerObjectiveCPP, true)
}

func analyzeObjectiveCFamily(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID, includeCPP bool) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_objective_c_source", document.Path, err)
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: language, Analyzer: string(analyzer), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, document, ObjectiveCScannerProfile(language), ScannerLimits{MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range scan.Diagnostics {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: language + "-" + diagnostic.Code, Message: diagnostic.Message, Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	parser := &objectiveCParser{ctx: ctx, document: document, builder: builder, language: language, analyzer: analyzer, types: make(map[string]SymbolParent)}
	parser.collectDirectives(scan.Tokens)
	parser.parse(BuildLogicalLines(scan.Tokens, LogicalLineProfile{SkipDirectives: true}))
	base := AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}
	if !includeCPP {
		return base, nil
	}

	masked := maskObjectiveCBlocksForCPP(document.Text)
	clone := *document
	clone.Text = masked
	clone.lineStarts = buildLineStarts(masked)
	cpp, err := (CPPAnalyzer{}).Analyze(ctx, &clone, options)
	if err != nil {
		return AnalyzerResult{}, err
	}
	reprojected, err := reprojectAnalyzerSymbols(ctx, document, cpp, options, language, analyzer, "", 0, nil)
	if err != nil {
		return AnalyzerResult{}, err
	}
	mergeAnalysisSymbols(&base.Analysis, reprojected)
	sort.Slice(base.Analysis.Symbols, func(i, j int) bool {
		left := base.Analysis.Symbols[i]
		right := base.Analysis.Symbols[j]
		if left.declarationOffsets.Start != right.declarationOffsets.Start {
			return left.declarationOffsets.Start < right.declarationOffsets.Start
		}
		if left.declarationOffsets.End != right.declarationOffsets.End {
			return left.declarationOffsets.End < right.declarationOffsets.End
		}
		return left.ID < right.ID
	})
	base.Dependencies = appendUniqueDependencies(base.Dependencies, cpp.Dependencies)
	base.Relations = append(base.Relations, cpp.Relations...)
	return base, nil
}

type objectiveCParser struct {
	ctx          context.Context
	document     *SourceDocument
	builder      *SymbolBuilder
	language     string
	analyzer     AnalyzerID
	types        map[string]SymbolParent
	current      *SymbolParent
	owner        string
	dependencies []StructuralDependency
	relations    []StructuralRelation
	stopped      bool
}

func (p *objectiveCParser) collectDirectives(tokens []Token) {
	for _, token := range tokens {
		if token.Kind != TokenDirective {
			continue
		}
		trimmed := strings.TrimSpace(token.Text)
		lower := strings.ToLower(trimmed)
		if !strings.HasPrefix(lower, "#import") && !strings.HasPrefix(lower, "#include") {
			continue
		}
		keyword := "#import"
		kind := StructuralDependencyImport
		if strings.HasPrefix(lower, "#include") {
			keyword = "#include"
			kind = StructuralDependencyInclude
		}
		value := phase7QuotedOrAngleValue(strings.TrimSpace(trimmed[len(keyword):]))
		if value == "" {
			continue
		}
		rangeValue, err := p.document.RangeFromUTF8Offsets(token.StartOffset, token.EndOffset)
		if err == nil {
			p.dependencies = appendUniqueDependencies(p.dependencies, []StructuralDependency{{Kind: kind, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural}})
		}
	}
}

func (p *objectiveCParser) parse(lines []LogicalLine) {
	for _, line := range lines {
		if p.stopped || p.ctx.Err() != nil || len(line.Tokens) == 0 {
			continue
		}
		at, keyword := objectiveCDirective(line.Tokens)
		if at >= 0 {
			switch keyword {
			case "interface", "protocol":
				p.openType(line, at, keyword)
			case "implementation":
				p.openImplementation(line, at)
			case "property":
				p.property(line, at)
			case "end":
				p.current = nil
				p.owner = ""
			}
			continue
		}
		if p.current != nil && (line.Tokens[0].Text == "-" || line.Tokens[0].Text == "+") {
			p.method(line)
		}
	}
}

func objectiveCDirective(tokens []Token) (int, string) {
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].Text != "@" {
			continue
		}
		keyword := strings.ToLower(tokens[i+1].Text)
		switch keyword {
		case "interface", "implementation", "protocol", "property", "end":
			return i, keyword
		}
	}
	return -1, ""
}

func (p *objectiveCParser) openType(line LogicalLine, at int, native string) {
	nameIndex := nextIdentifierToken(line.Tokens, at+2, len(line.Tokens))
	if nameIndex < 0 {
		p.builder.MarkIncomplete()
		return
	}
	kind := SymbolKindClass
	if native == "protocol" {
		kind = SymbolKindInterface
	}
	name := line.Tokens[nameIndex].Text
	symbol, ok := p.add(SymbolSpec{Kind: kind, NativeKind: "@" + native, Name: name, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
	if !ok {
		return
	}
	parent := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
	p.types[name] = parent
	p.current = &parent
	p.owner = name
	p.collectTypeRelations(line.Tokens, nameIndex+1, symbol.QualifiedName)
}

func (p *objectiveCParser) openImplementation(line LogicalLine, at int) {
	nameIndex := nextIdentifierToken(line.Tokens, at+2, len(line.Tokens))
	if nameIndex < 0 {
		p.builder.MarkIncomplete()
		return
	}
	name := line.Tokens[nameIndex].Text
	_, _ = p.add(SymbolSpec{Kind: SymbolKindImplementation, NativeKind: "@implementation", Name: name, QualifiedName: name + "@implementation", Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: line.Tokens[nameIndex].StartOffset, End: line.Tokens[nameIndex].EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural, Disambiguator: "implementation"})
	if existing, ok := p.types[name]; ok {
		value := existing
		p.current = &value
	} else {
		p.current = &SymbolParent{QualifiedName: name}
	}
	p.owner = name
}

func (p *objectiveCParser) collectTypeRelations(tokens []Token, start int, source string) {
	for i := start; i < len(tokens); i++ {
		if tokens[i].Text == ":" {
			base := nextIdentifierToken(tokens, i+1, len(tokens))
			if base >= 0 {
				p.addRelation("inherits", source, tokens[base].Text, tokens[base])
				i = base
			}
			continue
		}
		if tokens[i].Text == "<" {
			for j := i + 1; j < len(tokens) && tokens[j].Text != ">"; j++ {
				if tokens[j].Kind == TokenIdentifier {
					p.addRelation("implements", source, tokens[j].Text, tokens[j])
				}
			}
			return
		}
	}
}

func (p *objectiveCParser) property(line LogicalLine, at int) {
	if p.current == nil {
		return
	}
	name := previousIdentifierToken(line.Tokens, len(line.Tokens)-1, at+2)
	if name < 0 {
		return
	}
	token := line.Tokens[name]
	p.add(SymbolSpec{Kind: SymbolKindProperty, NativeKind: "@property", Name: token.Text, Parent: p.current, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: token.StartOffset, End: token.EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
}

func (p *objectiveCParser) method(line LogicalLine) {
	if p.current == nil || len(line.Tokens) < 3 {
		return
	}
	returnOpen := -1
	for i := 1; i < len(line.Tokens); i++ {
		if line.Tokens[i].Text == "(" {
			returnOpen = i
			break
		}
	}
	if returnOpen < 0 {
		return
	}
	returnClose := -1
	depth := 0
	for i := returnOpen; i < len(line.Tokens); i++ {
		switch line.Tokens[i].Text {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				returnClose = i
				i = len(line.Tokens)
			}
		}
	}
	if returnClose < 0 {
		p.builder.MarkIncomplete()
		return
	}
	first := nextIdentifierToken(line.Tokens, returnClose+1, len(line.Tokens))
	if first < 0 {
		return
	}
	selector := line.Tokens[first].Text
	colonCount := 0
	for i := first + 1; i < len(line.Tokens); i++ {
		if line.Tokens[i].Text == ":" {
			if colonCount == 0 {
				selector += ":"
			} else {
				piece := previousIdentifierToken(line.Tokens, i-1, first+1)
				if piece >= 0 {
					selector += line.Tokens[piece].Text + ":"
				}
			}
			colonCount++
		}
	}
	kind := SymbolKindMethod
	lower := strings.ToLower(selector)
	if strings.HasPrefix(lower, "init") {
		kind = SymbolKindConstructor
	}
	native := "instance-method"
	if line.Tokens[0].Text == "+" {
		native = "class-method"
	}
	nameToken := line.Tokens[first]
	p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: selector, Parent: p.current, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: nameToken.StartOffset, End: nameToken.EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural, Disambiguator: selector})
}

func (p *objectiveCParser) addRelation(kind, source, target string, token Token) {
	rangeValue, err := p.document.RangeFromUTF8Offsets(token.StartOffset, token.EndOffset)
	if err == nil {
		p.relations = append(p.relations, StructuralRelation{Kind: kind, Source: source, Target: target, Range: rangeValue, Evidence: SymbolEvidenceStructural})
	}
}
func (p *objectiveCParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
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

func maskObjectiveCBlocksForCPP(text string) string {
	masked := []byte(text)
	inside := false
	for lineStart := 0; lineStart < len(text); {
		lineEnd := strings.IndexAny(text[lineStart:], "\r\n")
		if lineEnd < 0 {
			lineEnd = len(text)
		} else {
			lineEnd += lineStart
		}
		trimmed := strings.TrimSpace(text[lineStart:lineEnd])
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "@interface") || strings.HasPrefix(lower, "@implementation") || strings.HasPrefix(lower, "@protocol") {
			inside = true
		}
		if inside {
			for i := lineStart; i < lineEnd; i++ {
				if masked[i] != '\r' && masked[i] != '\n' {
					masked[i] = ' '
				}
			}
		}
		if inside && strings.HasPrefix(lower, "@end") {
			inside = false
		}
		if lineEnd >= len(text) {
			break
		}
		if text[lineEnd] == '\r' && lineEnd+1 < len(text) && text[lineEnd+1] == '\n' {
			lineStart = lineEnd + 2
		} else {
			lineStart = lineEnd + 1
		}
	}
	return string(masked)
}
