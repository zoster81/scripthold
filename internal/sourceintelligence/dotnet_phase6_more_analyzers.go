package sourceintelligence

import (
	"context"
	"regexp"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// JScriptNetAnalyzer handles the legacy CLR JScript declaration surface without
// executing imports, packages, CLR binding, or dynamic JavaScript behavior.
type JScriptNetAnalyzer struct{}

func (JScriptNetAnalyzer) ID() AnalyzerID   { return AnalyzerJScriptNet }
func (JScriptNetAnalyzer) Language() string { return "jscript-net" }

func (JScriptNetAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_jscript_net_source", document.Path, err)
	}
	masked := maskJScriptNetModifiers(document.Text)
	scanDoc := *document
	scanDoc.Text = masked
	scanDoc.lineStarts = buildLineStarts(masked)
	scan, err := ScanSource(ctx, &scanDoc, JavaScriptScannerProfile(), ScannerLimits{MaxTokens: scannerTokenBudget(masked), MaxTokenBytes: 1024 * 1024, MaxNesting: phase6MaxNesting(options.MaxNesting)})
	if err != nil {
		return AnalyzerResult{}, err
	}
	pairs := PairDelimiterTokens(scan.Tokens, nil)
	packageKeyword := -1
	packageName := -1
	open := -1
	close := -1
	for i := 0; i < len(scan.Tokens); i++ {
		if strings.EqualFold(scan.Tokens[i].Text, "package") {
			packageKeyword = i
			packageName = nextIdentifierToken(scan.Tokens, i+1, len(scan.Tokens))
			if packageName >= 0 {
				for j := packageName + 1; j < len(scan.Tokens); j++ {
					if scan.Tokens[j].Text == "{" {
						open = j
						close = pairs[j]
						break
					}
					if scan.Tokens[j].Kind == TokenNewline {
						break
					}
				}
			}
			break
		}
	}
	if packageKeyword < 0 || packageName < 0 || open < 0 || close <= open {
		return analyzeJScriptNetBody(ctx, document, options, masked, 0, len(masked), nil)
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: "jscript-net", Analyzer: string(AnalyzerJScriptNet), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	pkg, err := builder.Add(SymbolSpec{Kind: SymbolKindPackage, NativeKind: "package", Name: scan.Tokens[packageName].Text, Declaration: OffsetRange{Start: scan.Tokens[packageKeyword].StartOffset, End: scan.Tokens[open].StartOffset}, NameRange: OffsetRange{Start: scan.Tokens[packageName].StartOffset, End: scan.Tokens[packageName].EndOffset}, Signature: &OffsetRange{Start: scan.Tokens[packageKeyword].StartOffset, End: scan.Tokens[open].StartOffset}, Evidence: SymbolEvidenceStructural})
	if err != nil {
		return AnalyzerResult{}, err
	}
	parent := &SymbolParent{ID: pkg.ID, QualifiedName: pkg.QualifiedName}
	remaining := options
	if remaining.Limits.MaxSymbols > 0 {
		remaining.Limits.MaxSymbols--
	}
	body, err := analyzeJScriptNetBody(ctx, document, remaining, masked, scan.Tokens[open].EndOffset, scan.Tokens[close].StartOffset, parent)
	if err != nil {
		return AnalyzerResult{}, err
	}
	base := builder.Result()
	mergeAnalysisSymbols(&base, body.Analysis)
	return AnalyzerResult{Analysis: base, Dependencies: body.Dependencies, Relations: body.Relations}, nil
}

func analyzeJScriptNetBody(ctx context.Context, host *SourceDocument, options AnalyzeOptions, masked string, start, end int, parent *SymbolParent) (AnalyzerResult, error) {
	if start < 0 || end < start || end > len(masked) {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "invalid JScript.NET package range")
	}
	text := masked[start:end]
	sub := &SourceDocument{Path: host.Path + "#jscript-net", Text: text, Encoding: "utf-8", lineStarts: buildLineStarts(text)}
	source, err := (JavaScriptAnalyzer{}).Analyze(ctx, sub, options)
	if err != nil {
		return AnalyzerResult{}, err
	}
	analysis, err := reprojectAnalyzerSymbols(ctx, host, source, options, "jscript-net", AnalyzerJScriptNet, "", start, parent)
	if err != nil {
		return AnalyzerResult{}, err
	}
	dependencies := jscriptNetDependencies(host, start, end)
	relations := jscriptNetRelations(host, start, end, parent)
	return AnalyzerResult{Analysis: analysis, Dependencies: dependencies, Relations: relations}, nil
}

func maskJScriptNetModifiers(text string) string {
	result := []byte(text)
	for _, word := range []string{"public", "private", "protected", "internal", "final", "abstract"} {
		for search := 0; search < len(text); {
			relative := strings.Index(text[search:], word)
			if relative < 0 {
				break
			}
			start := search + relative
			end := start + len(word)
			if (start == 0 || !isASCIIIdentifierByte(text[start-1])) && (end == len(text) || !isASCIIIdentifierByte(text[end])) {
				for i := start; i < end; i++ {
					if result[i] != '\r' && result[i] != '\n' {
						result[i] = ' '
					}
				}
			}
			search = end
		}
	}
	return string(result)
}

var jscriptNetImportRE = regexp.MustCompile(`(?im)^[ \t]*import[ \t]+([A-Za-z_][A-Za-z0-9_.]*)[ \t]*;`)
var jscriptNetInheritanceRE = regexp.MustCompile(`(?im)\bclass[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]+(?:extends|implements)[ \t]+([A-Za-z_][A-Za-z0-9_.]*)`)

func jscriptNetDependencies(document *SourceDocument, start, end int) []StructuralDependency {
	if document == nil || start < 0 || end < start || end > len(document.Text) {
		return nil
	}
	text := document.Text[start:end]
	matches := jscriptNetImportRE.FindAllStringSubmatchIndex(text, -1)
	result := make([]StructuralDependency, 0, len(matches))
	for _, match := range matches {
		if len(match) < 4 || match[2] < 0 {
			continue
		}
		valueStart := start + match[2]
		valueEnd := start + match[3]
		rangeValue, err := document.RangeFromUTF8Offsets(valueStart, valueEnd)
		if err == nil {
			result = append(result, StructuralDependency{Kind: StructuralDependencyImport, Value: document.Text[valueStart:valueEnd], Range: rangeValue, Evidence: SymbolEvidenceStructural})
		}
	}
	return result
}

func jscriptNetRelations(document *SourceDocument, start, end int, parent *SymbolParent) []StructuralRelation {
	if document == nil || start < 0 || end < start || end > len(document.Text) {
		return nil
	}
	text := document.Text[start:end]
	matches := jscriptNetInheritanceRE.FindAllStringSubmatchIndex(text, -1)
	result := make([]StructuralRelation, 0, len(matches))
	for _, match := range matches {
		if len(match) < 6 || match[2] < 0 || match[4] < 0 {
			continue
		}
		className := text[match[2]:match[3]]
		target := text[match[4]:match[5]]
		source := className
		if parent != nil && parent.QualifiedName != "" {
			source = parent.QualifiedName + "." + className
		}
		kind := "extends"
		clause := strings.ToLower(text[match[0]:match[1]])
		if strings.Contains(clause, "implements") {
			kind = "implements"
		}
		targetStart := start + match[4]
		targetEnd := start + match[5]
		rangeValue, err := document.RangeFromUTF8Offsets(targetStart, targetEnd)
		if err == nil {
			result = append(result, StructuralRelation{Kind: kind, Source: source, Target: target, Range: rangeValue, Evidence: SymbolEvidenceStructural})
		}
	}
	return result
}

// CILAnalyzer recognizes IL assembly/module/type/member declarations without
// interpreting metadata tokens or executing IL.
type CILAnalyzer struct{}

func (CILAnalyzer) ID() AnalyzerID   { return AnalyzerCIL }
func (CILAnalyzer) Language() string { return "cil" }
func CILScannerProfile() ScannerProfile {
	return ScannerProfile{Name: "cil", Identifier: IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraStart: ".", ExtraContinue: ".-_/`"}, LineComments: []string{"//"}, BlockComments: []BlockCommentRule{{Start: "/*", End: "*/"}}, Strings: []StringRule{{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true}}}
}
func (CILAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: "cil", Analyzer: string(AnalyzerCIL), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, document, CILScannerProfile(), ScannerLimits{MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, d := range scan.Diagnostics {
		v := OffsetRange{Start: d.StartOffset, End: d.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: "cil-" + d.Code, Message: d.Message, Severity: DiagnosticWarning, Range: &v, AffectsCoverage: true})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	p := &cilParser{ctx: ctx, document: document, builder: builder}
	p.parse(BuildLogicalLines(scan.Tokens, LogicalLineProfile{}))
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: p.dependencies, Relations: p.relations}, nil
}

type cilParser struct {
	ctx          context.Context
	document     *SourceDocument
	builder      *SymbolBuilder
	module       *SymbolParent
	class        *SymbolParent
	dependencies []StructuralDependency
	relations    []StructuralRelation
	stopped      bool
}

func (p *cilParser) parse(lines []LogicalLine) {
	for _, line := range lines {
		if p.stopped || p.ctx.Err() != nil || len(line.Tokens) == 0 {
			continue
		}
		first := strings.ToLower(line.Tokens[0].Text)
		if first == "}" {
			p.class = nil
			continue
		}
		switch first {
		case ".assembly":
			p.assembly(line)
		case ".module":
			if p.module == nil {
				p.moduleDecl(line)
			}
		case ".class":
			p.classDecl(line)
		case ".field":
			if p.class != nil {
				p.member(line, SymbolKindField, "field")
			}
		case ".method":
			if p.class != nil {
				p.member(line, SymbolKindMethod, "method")
			}
		}
	}
}
func (p *cilParser) assembly(line LogicalLine) {
	if len(line.Tokens) < 2 {
		return
	}
	extern := strings.EqualFold(line.Tokens[1].Text, "extern")
	idx := 1
	if extern {
		idx++
	}
	if idx >= len(line.Tokens) {
		return
	}
	tok := line.Tokens[idx]
	if extern {
		r, err := p.document.RangeFromUTF8Offsets(tok.StartOffset, tok.EndOffset)
		if err == nil {
			p.dependencies = append(p.dependencies, StructuralDependency{Kind: StructuralDependencyImport, Value: tok.Text, Range: r, Evidence: SymbolEvidenceStructural})
		}
		return
	}
	s, ok := p.add(SymbolSpec{Kind: SymbolKindModule, NativeKind: "assembly", Name: tok.Text, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Evidence: SymbolEvidenceStructural})
	if ok {
		v := SymbolParent{ID: s.ID, QualifiedName: s.QualifiedName}
		p.module = &v
	}
}
func (p *cilParser) moduleDecl(line LogicalLine) {
	idx := nextIdentifierToken(line.Tokens, 1, len(line.Tokens))
	if idx < 0 {
		return
	}
	p.assembly(LogicalLine{Tokens: []Token{line.Tokens[0], line.Tokens[idx]}, StartOffset: line.StartOffset, EndOffset: line.EndOffset})
}
func (p *cilParser) classDecl(line LogicalLine) {
	end := len(line.Tokens)
	marker := end
	for i, t := range line.Tokens {
		if strings.EqualFold(t.Text, "extends") || t.Text == "{" {
			marker = i
			break
		}
	}
	idx := previousIdentifierToken(line.Tokens, marker-1, 1)
	if idx < 0 {
		return
	}
	tok := line.Tokens[idx]
	s, ok := p.add(SymbolSpec{Kind: SymbolKindClass, NativeKind: "class", Name: tok.Text, Parent: p.module, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
	if ok {
		v := SymbolParent{ID: s.ID, QualifiedName: s.QualifiedName}
		p.class = &v
	}
	for i, t := range line.Tokens {
		if strings.EqualFold(t.Text, "extends") && i+1 < len(line.Tokens) {
			target := tokenRangeText(line.Tokens, i+1, len(line.Tokens))
			target = strings.TrimSuffix(target, "{")
			if target != "" && p.class != nil {
				r, err := p.document.RangeFromUTF8Offsets(line.Tokens[i+1].StartOffset, line.Tokens[len(line.Tokens)-1].EndOffset)
				if err == nil {
					p.relations = append(p.relations, StructuralRelation{Kind: "extends", Source: p.class.QualifiedName, Target: target, Range: r, Evidence: SymbolEvidenceStructural})
				}
			}
			break
		}
	}
}
func (p *cilParser) member(line LogicalLine, kind SymbolKind, native string) {
	name := -1
	if kind == SymbolKindMethod {
		for i, t := range line.Tokens {
			if t.Text == "(" {
				name = previousIdentifierToken(line.Tokens, i-1, 1)
				break
			}
		}
	} else {
		name = previousIdentifierToken(line.Tokens, len(line.Tokens)-1, 1)
		if name >= 0 && line.Tokens[name].Text == "}" {
			name = previousIdentifierToken(line.Tokens, name-1, 1)
		}
	}
	if name < 0 {
		return
	}
	tok := line.Tokens[name]
	p.add(SymbolSpec{Kind: kind, NativeKind: native, Name: tok.Text, Parent: p.class, Declaration: OffsetRange{Start: line.StartOffset, End: line.EndOffset}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: line.StartOffset, End: line.EndOffset}, Evidence: SymbolEvidenceStructural})
}
func (p *cilParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
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

// PowerShellAnalyzer recognizes declarations without executing profiles,
// modules, classes, dynamic commands, or interpolation.
type PowerShellAnalyzer struct{}

func (PowerShellAnalyzer) ID() AnalyzerID   { return AnalyzerPowerShell }
func (PowerShellAnalyzer) Language() string { return "powershell" }
func PowerShellScannerProfile() ScannerProfile {
	return ScannerProfile{Name: "powershell", Keywords: []string{"class", "enum", "filter", "function", "module", "using"}, Identifier: IdentifierPolicy{UnicodeLetters: true, UnicodeDigits: true, UnicodeMarks: true, Underscore: true, ExtraStart: "$@", ExtraContinue: "$@-?"}, LineComments: []string{"#"}, BlockComments: []BlockCommentRule{{Start: "<#", End: "#>"}}, Strings: []StringRule{{Prefixes: []string{""}, Delimiter: "\"", BackslashEscapes: true}, {Prefixes: []string{""}, Delimiter: "'", DoubledDelimiterEscape: true}}}
}
func (PowerShellAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	masked, diags, err := maskPowerShellHereStrings(ctx, document.Text)
	if err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_powershell_source", document.Path, err)
	}
	clone := *document
	clone.Text = masked
	clone.lineStarts = buildLineStarts(masked)
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: "powershell", Analyzer: string(AnalyzerPowerShell), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, &clone, PowerShellScannerProfile(), ScannerLimits{MaxTokens: scannerTokenBudget(masked), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, d := range append(diags, scan.Diagnostics...) {
		v := OffsetRange{Start: d.StartOffset, End: d.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: "powershell-" + d.Code, Message: d.Message, Severity: DiagnosticWarning, Range: &v, AffectsCoverage: true})
	}
	if !scan.Complete || len(diags) > 0 {
		builder.MarkIncomplete()
	}
	p := &powerShellParser{ctx: ctx, document: document, tokens: scan.Tokens, pairs: PairDelimiterTokens(scan.Tokens, nil), builder: builder}
	p.parseScope(0, len(scan.Tokens), nil, false)
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: p.dependencies, Relations: p.relations}, nil
}

type powerShellParser struct {
	ctx          context.Context
	document     *SourceDocument
	tokens       []Token
	pairs        map[int]int
	builder      *SymbolBuilder
	dependencies []StructuralDependency
	relations    []StructuralRelation
	stopped      bool
}

func (p *powerShellParser) parseScope(start, end int, parent *SymbolParent, members bool) {
	for i := start; i < end && !p.stopped; {
		if p.ctx.Err() != nil {
			return
		}
		i = nextStructuralToken(p.tokens, i, end)
		if i >= end || p.tokens[i].Kind == TokenEOF {
			return
		}
		if !members && strings.EqualFold(p.tokens[i].Text, "using") {
			i = p.parseUsing(i, end)
			continue
		}
		if strings.EqualFold(p.tokens[i].Text, "class") {
			i = p.parseClass(i, end, parent)
			continue
		}
		if !members && (strings.EqualFold(p.tokens[i].Text, "function") || strings.EqualFold(p.tokens[i].Text, "filter")) {
			i = p.parseFunction(i, end, parent)
			continue
		}
		if members {
			if next, ok := p.parseMember(i, end, parent); ok {
				i = next
				continue
			}
		}
		i = p.skip(i, end)
	}
}
func (p *powerShellParser) parseUsing(start, end int) int {
	lineEnd := p.lineEnd(start, end)
	if start+1 < lineEnd && strings.EqualFold(p.tokens[start+1].Text, "module") {
		value := tokenRangeText(p.tokens, start+2, lineEnd)
		if value != "" {
			r, err := p.document.RangeFromUTF8Offsets(p.tokens[start+2].StartOffset, p.tokens[lineEnd-1].EndOffset)
			if err == nil {
				p.dependencies = append(p.dependencies, StructuralDependency{Kind: StructuralDependencyImport, Value: value, Range: r, Evidence: SymbolEvidenceStructural})
			}
		}
	}
	return min(lineEnd+1, end)
}
func (p *powerShellParser) parseClass(start, end int, parent *SymbolParent) int {
	name := nextIdentifierToken(p.tokens, start+1, end)
	if name < 0 {
		return start + 1
	}
	open := -1
	for i := name + 1; i < end; i++ {
		if p.tokens[i].Text == "{" {
			open = i
			break
		}
		if p.tokens[i].Kind == TokenNewline {
			return i + 1
		}
	}
	if open < 0 {
		return start + 1
	}
	close := p.pairs[open]
	if close <= open {
		p.builder.MarkIncomplete()
		return end
	}
	tok := p.tokens[name]
	s, ok := p.add(SymbolSpec{Kind: SymbolKindClass, NativeKind: "class", Name: tok.Text, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[close].EndOffset}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[open].StartOffset}, Body: &OffsetRange{Start: p.tokens[open].StartOffset, End: p.tokens[close].EndOffset}, Evidence: SymbolEvidenceStructural})
	if ok {
		child := &SymbolParent{ID: s.ID, QualifiedName: s.QualifiedName}
		colon := -1
		for i := name + 1; i < open; i++ {
			if p.tokens[i].Text == ":" {
				colon = i
				break
			}
		}
		if colon >= 0 && colon+1 < open {
			target := tokenRangeText(p.tokens, colon+1, open)
			if target != "" {
				r, err := p.document.RangeFromUTF8Offsets(p.tokens[colon+1].StartOffset, p.tokens[open-1].EndOffset)
				if err == nil {
					p.relations = append(p.relations, StructuralRelation{Kind: "inherits", Source: s.QualifiedName, Target: target, Range: r, Evidence: SymbolEvidenceStructural})
				}
			}
		}
		p.parseScope(open+1, close, child, true)
	}
	return close + 1
}
func (p *powerShellParser) parseFunction(start, end int, parent *SymbolParent) int {
	name := nextIdentifierToken(p.tokens, start+1, end)
	if name < 0 {
		return start + 1
	}
	open := -1
	for i := name + 1; i < end; i++ {
		if p.tokens[i].Text == "{" {
			open = i
			break
		}
		if p.tokens[i].Kind == TokenNewline {
			break
		}
	}
	last := p.tokens[name].EndOffset
	next := p.lineEnd(start, end)
	var body *OffsetRange
	if open >= 0 {
		close := p.pairs[open]
		if close > open {
			last = p.tokens[close].EndOffset
			v := OffsetRange{Start: p.tokens[open].StartOffset, End: p.tokens[close].EndOffset}
			body = &v
			next = close + 1
		}
	}
	tok := p.tokens[name]
	p.add(SymbolSpec{Kind: SymbolKindFunction, NativeKind: strings.ToLower(p.tokens[start].Text), Name: tok.Text, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: last}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: p.tokens[start].StartOffset, End: choosePowerShellSignatureEnd(p.tokens, open, name)}, Body: body, Evidence: SymbolEvidenceStructural})
	return next
}
func choosePowerShellSignatureEnd(tokens []Token, open, name int) int {
	if open >= 0 {
		return tokens[open].StartOffset
	}
	return tokens[name].EndOffset
}
func (p *powerShellParser) parseMember(start, end int, parent *SymbolParent) (int, bool) {
	lineEnd := p.lineEnd(start, end)
	paren := -1
	for i := start; i < lineEnd; i++ {
		if p.tokens[i].Text == "(" {
			paren = i
			break
		}
	}
	if paren >= 0 {
		name := previousIdentifierToken(p.tokens, paren-1, start)
		if name >= 0 {
			tok := p.tokens[name]
			open := -1
			for i := paren + 1; i < end; i++ {
				if p.tokens[i].Text == "{" {
					open = i
					break
				}
				if p.tokens[i].Kind == TokenNewline {
					break
				}
			}
			next := lineEnd
			last := p.tokens[lineEnd-1].EndOffset
			var body *OffsetRange
			if open >= 0 {
				close := p.pairs[open]
				if close > open {
					last = p.tokens[close].EndOffset
					v := OffsetRange{Start: p.tokens[open].StartOffset, End: p.tokens[close].EndOffset}
					body = &v
					next = close + 1
				}
			}
			p.add(SymbolSpec{Kind: SymbolKindMethod, NativeKind: "method", Name: tok.Text, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: last}, NameRange: OffsetRange{Start: tok.StartOffset, End: tok.EndOffset}, Signature: &OffsetRange{Start: p.tokens[start].StartOffset, End: choosePowerShellSignatureEnd(p.tokens, open, name)}, Body: body, Evidence: SymbolEvidenceStructural})
			return next, true
		}
	}
	for i := start; i < lineEnd; i++ {
		if p.tokens[i].Kind == TokenIdentifier && strings.HasPrefix(p.tokens[i].Text, "$") {
			tok := p.tokens[i]
			name := strings.TrimPrefix(tok.Text, "$")
			p.add(SymbolSpec{Kind: SymbolKindField, NativeKind: "field", Name: name, Parent: parent, Declaration: OffsetRange{Start: p.tokens[start].StartOffset, End: p.tokens[lineEnd-1].EndOffset}, NameRange: OffsetRange{Start: tok.StartOffset + 1, End: tok.EndOffset}, Evidence: SymbolEvidenceStructural})
			return lineEnd, true
		}
	}
	return start + 1, false
}
func (p *powerShellParser) lineEnd(start, end int) int {
	for i := start; i < end; i++ {
		if p.tokens[i].Kind == TokenNewline || p.tokens[i].Kind == TokenEOF {
			return i
		}
	}
	return end
}
func (p *powerShellParser) skip(start, end int) int {
	line := p.lineEnd(start, end)
	return min(line+1, end)
}
func (p *powerShellParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
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

func maskPowerShellHereStrings(ctx context.Context, text string) (string, []ScannerDiagnostic, error) {
	result := []byte(text)
	changed := false
	var diagnostics []ScannerDiagnostic
	for at := 0; at < len(text); {
		if at&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return "", nil, err
			}
		}
		lineStart := at
		lineEnd := strings.IndexAny(text[lineStart:], "\r\n")
		if lineEnd < 0 {
			lineEnd = len(text)
		} else {
			lineEnd += lineStart
		}
		trim := strings.TrimSpace(text[lineStart:lineEnd])
		if trim != "@\"" && trim != "@'" {
			at = nextPhysicalLine(text, lineEnd)
			continue
		}
		quote := trim[1]
		open := lineStart
		cursor := nextPhysicalLine(text, lineEnd)
		end := -1
		for cursor <= len(text) {
			le := strings.IndexAny(text[cursor:], "\r\n")
			if le < 0 {
				le = len(text)
			} else {
				le += cursor
			}
			candidate := strings.TrimSpace(text[cursor:le])
			if (quote == '"' && candidate == "\"@") || (quote == '\'' && candidate == "'@") {
				end = le
				break
			}
			if le >= len(text) {
				break
			}
			cursor = nextPhysicalLine(text, le)
		}
		if end < 0 {
			diagnostics = append(diagnostics, ScannerDiagnostic{Code: "unterminated-here-string", Message: "PowerShell here-string is not terminated", StartOffset: open, EndOffset: len(text)})
			end = len(text)
		}
		for i := open; i < end; i++ {
			if result[i] != '\r' && result[i] != '\n' {
				result[i] = ' '
			}
		}
		changed = true
		at = end
		if at < len(text) {
			at = nextPhysicalLine(text, at)
		}
	}
	if !changed {
		return text, diagnostics, nil
	}
	return string(result), diagnostics, nil
}
func nextPhysicalLine(text string, end int) int {
	if end >= len(text) {
		return len(text)
	}
	if text[end] == '\r' && end+1 < len(text) && text[end+1] == '\n' {
		return end + 2
	}
	return end + 1
}

func phase6MaxNesting(value int) int {
	if value > 0 {
		return value
	}
	return 2048
}
