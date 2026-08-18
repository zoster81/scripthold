package sourceintelligence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

func analyzeCSharpMemberRegion(ctx context.Context, host *SourceDocument, options AnalyzeOptions, start, end int, language string, analyzer AnalyzerID, regionID string) (AnalysisResult, error) {
	if start < 0 || end < start || end > len(host.Text) {
		return AnalysisResult{}, operation.New(operation.KindInvalidInput, "invalid C# member region")
	}
	text := host.Text[start:end]
	sub := &SourceDocument{Path: host.Path + "#" + regionID, Text: text, Encoding: "utf-8", lineStarts: buildLineStarts(text)}
	builder := NewSymbolBuilder(sub, SymbolBuilderOptions{Context: ctx, Language: "csharp", Analyzer: string(AnalyzerCSharp), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalysisResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, sub, CSharpScannerProfile(), ScannerLimits{MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalysisResult{}, err
	}
	for _, d := range scan.Diagnostics {
		v := OffsetRange{Start: d.StartOffset, End: d.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: "csharp-" + d.Code, Message: d.Message, Severity: DiagnosticWarning, Range: &v, AffectsCoverage: true})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	parser := &csharpParser{ctx: ctx, document: sub, tokens: scan.Tokens, builder: builder, pairs: buildTokenPairs(scan.Tokens)}
	parser.parseScope(0, len(scan.Tokens), nil, true, "")
	source := AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies}
	return reprojectAnalyzerSymbols(ctx, host, source, options, language, analyzer, regionID, start, nil)
}

func analyzeVBMemberRegion(ctx context.Context, host *SourceDocument, options AnalyzeOptions, start, end int, language string, analyzer AnalyzerID, regionID string) (AnalysisResult, error) {
	text := host.Text[start:end]
	sub := &SourceDocument{Path: host.Path + "#" + regionID, Text: text, Encoding: "utf-8", lineStarts: buildLineStarts(text)}
	source, err := (VBNetAnalyzer{}).Analyze(ctx, sub, options)
	if err != nil {
		return AnalysisResult{}, err
	}
	return reprojectAnalyzerSymbols(ctx, host, source, options, language, analyzer, regionID, start, nil)
}

func compositeRegionID(path, kind, language string, start, end int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d", path, kind, language, start, end)))
	return hex.EncodeToString(sum[:])
}
func sourceRegionForOffsets(document *SourceDocument, id, kind, language string, start, end int, supported bool) (SourceRegion, error) {
	r, err := document.RangeFromUTF8Offsets(start, end)
	if err != nil {
		return SourceRegion{}, err
	}
	return SourceRegion{ID: id, Kind: kind, Language: language, Range: r, Evidence: SymbolEvidenceStructural, Supported: supported}, nil
}

// ASPNetWebFormsAnalyzer treats markup as host text and only delegates declared
// runat=server code regions whose page/script language is structurally known.
type ASPNetWebFormsAnalyzer struct{}

func (ASPNetWebFormsAnalyzer) ID() AnalyzerID   { return AnalyzerASPNetWebForms }
func (ASPNetWebFormsAnalyzer) Language() string { return "aspnet-webforms" }
func (ASPNetWebFormsAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: "aspnet-webforms", Analyzer: string(AnalyzerASPNetWebForms), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	pageLanguage := aspNetPageLanguage(document.Text)
	segments, incomplete := segmentClassicASP(document.Path, document.Text, pageLanguage)
	if incomplete {
		builder.MarkIncomplete()
	}
	regions := make([]SourceRegion, 0, len(segments))
	excludedClientRanges := make([]OffsetRange, 0, len(segments))
	remaining := options.Limits.MaxSymbols
	for _, segment := range segments {
		language := normalizeDotNetScriptLanguage(segment.language)
		supported := segment.kind != "server-script" || language == "csharp" || language == "vbnet"
		regionID := compositeRegionID(document.Path, segment.kind, language, segment.start, segment.end)
		region, err := sourceRegionForOffsets(document, regionID, segment.kind, language, segment.start, segment.end, supported)
		if err != nil {
			return AnalyzerResult{}, err
		}
		regions = append(regions, region)
		if segment.kind != "host" {
			excludedClientRanges = append(excludedClientRanges, OffsetRange{Start: segment.start, End: segment.end})
		}
		if segment.kind != "server-script" {
			continue
		}
		if !supported {
			builder.MarkIncomplete()
			continue
		}
		regionOptions := options
		if remaining > 0 {
			regionOptions.Limits.MaxSymbols = remaining
		}
		var analysis AnalysisResult
		if language == "csharp" {
			analysis, err = analyzeCSharpMemberRegion(ctx, document, regionOptions, segment.codeStart, segment.codeEnd, "csharp", AnalyzerASPNetWebForms, regionID)
		} else {
			analysis, err = analyzeVBMemberRegion(ctx, document, regionOptions, segment.codeStart, segment.codeEnd, "vbnet", AnalyzerASPNetWebForms, regionID)
		}
		if err != nil {
			return AnalyzerResult{}, err
		}
		mergeAnalysisSymbols(&builder.result, analysis)
		remaining -= len(analysis.Symbols)
	}
	result := AnalyzerResult{Analysis: builder.Result(), Dependencies: aspNetDirectiveDependencies(document), Regions: regions}
	clientOptions := options
	clientOptions.Limits.MaxSymbols = max(1, remaining)
	client, err := phase11AnalyzeClientWebRegions(ctx, document, clientOptions, "aspnet-webforms", AnalyzerASPNetWebForms, excludedClientRanges)
	if err != nil {
		return AnalyzerResult{}, err
	}
	result.Analysis = phase11MergeAnalysis(result.Analysis, client.Analysis, options.Limits)
	phase11AppendDependencies(&result, client.Dependencies, options.Limits)
	phase11AppendRelations(&result, client.Relations, options.Limits)
	serverRegions := result.Regions
	result.Regions = nil
	phase11AppendRegions(&result, serverRegions, options.Limits)
	phase11AppendRegions(&result, client.Regions, options.Limits)
	return result, nil
}
func aspNetPageLanguage(text string) string {
	lower := strings.ToLower(text)
	search := 0
	for search < len(text) {
		r := strings.Index(lower[search:], "<%@")
		if r < 0 {
			break
		}
		start := search + r
		er := strings.Index(lower[start+3:], "%>")
		if er < 0 {
			break
		}
		end := start + 3 + er
		body := text[start+3 : end]
		if strings.Contains(strings.ToLower(body), "page") || strings.Contains(strings.ToLower(body), "control") || strings.Contains(strings.ToLower(body), "master") {
			if value := htmlLikeAttribute(body, "language"); value != "" {
				return normalizeDotNetScriptLanguage(value)
			}
		}
		search = end + 2
	}
	return "csharp"
}
func normalizeDotNetScriptLanguage(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "c#", "cs", "csharp":
		return "csharp"
	case "vb", "visualbasic", "vb.net", "vbnet":
		return "vbnet"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

var aspNetCodeFileRE = regexp.MustCompile(`(?is)<%@\s*(?:Page|Control|Master)\b[^%>]*\b(?:CodeFile|CodeBehind)\s*=\s*["']([^"']+)["']`)

func aspNetDirectiveDependencies(document *SourceDocument) []StructuralDependency {
	matches := aspNetCodeFileRE.FindAllStringSubmatchIndex(document.Text, -1)
	result := make([]StructuralDependency, 0, len(matches))
	for _, m := range matches {
		if len(m) < 4 || m[2] < 0 {
			continue
		}
		r, err := document.RangeFromUTF8Offsets(m[2], m[3])
		if err == nil {
			result = append(result, StructuralDependency{Kind: StructuralDependencyInclude, Value: document.Text[m[2]:m[3]], Range: r, Evidence: SymbolEvidenceStructural})
		}
	}
	return result
}

// RazorAnalyzer and BlazorAnalyzer preserve host offsets while analyzing only
// balanced @functions/@code member regions.
type RazorAnalyzer struct{}
type BlazorAnalyzer struct{}

func (RazorAnalyzer) ID() AnalyzerID    { return AnalyzerRazor }
func (RazorAnalyzer) Language() string  { return "razor" }
func (BlazorAnalyzer) ID() AnalyzerID   { return AnalyzerBlazor }
func (BlazorAnalyzer) Language() string { return "blazor" }
func (RazorAnalyzer) Analyze(ctx context.Context, d *SourceDocument, o AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeRazorFamily(ctx, d, o, "razor", AnalyzerRazor, []string{"functions", "code"})
}
func (BlazorAnalyzer) Analyze(ctx context.Context, d *SourceDocument, o AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeRazorFamily(ctx, d, o, "blazor", AnalyzerBlazor, []string{"code", "functions"})
}

type embeddedCodeRange struct {
	full, content OffsetRange
	kind          string
}

func analyzeRazorFamily(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID, directives []string) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: language, Analyzer: string(analyzer), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	ranges, complete, err := findRazorCodeRanges(ctx, document, directives)
	if err != nil {
		return AnalyzerResult{}, err
	}
	if !complete {
		builder.MarkIncomplete()
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: language + "-unterminated-region", Message: "embedded Razor/Blazor code or comment region is not terminated", Severity: DiagnosticWarning, AffectsCoverage: true})
	}
	regions := make([]SourceRegion, 0, len(ranges)*2+1)
	cursor := 0
	remaining := options.Limits.MaxSymbols
	for _, item := range ranges {
		if item.full.Start > cursor {
			r, err := sourceRegionForOffsets(document, compositeRegionID(document.Path, "host", "html", cursor, item.full.Start), "host", "html", cursor, item.full.Start, true)
			if err != nil {
				return AnalyzerResult{}, err
			}
			regions = append(regions, r)
		}
		regionID := compositeRegionID(document.Path, item.kind, "csharp", item.full.Start, item.full.End)
		r, err := sourceRegionForOffsets(document, regionID, item.kind, "csharp", item.full.Start, item.full.End, true)
		if err != nil {
			return AnalyzerResult{}, err
		}
		regions = append(regions, r)
		regionOptions := options
		if remaining > 0 {
			regionOptions.Limits.MaxSymbols = remaining
		}
		analysis, err := analyzeCSharpMemberRegion(ctx, document, regionOptions, item.content.Start, item.content.End, "csharp", analyzer, regionID)
		if err != nil {
			return AnalyzerResult{}, err
		}
		mergeAnalysisSymbols(&builder.result, analysis)
		remaining -= len(analysis.Symbols)
		cursor = item.full.End
	}
	if cursor < len(document.Text) {
		r, err := sourceRegionForOffsets(document, compositeRegionID(document.Path, "host", "html", cursor, len(document.Text)), "host", "html", cursor, len(document.Text), true)
		if err != nil {
			return AnalyzerResult{}, err
		}
		regions = append(regions, r)
	}
	result := AnalyzerResult{Analysis: builder.Result(), Dependencies: razorUsingDependencies(document), Regions: regions}
	excludedClientRanges := make([]OffsetRange, 0, len(ranges))
	for _, item := range ranges {
		excludedClientRanges = append(excludedClientRanges, item.full)
	}
	clientOptions := options
	clientOptions.Limits.MaxSymbols = max(1, remaining)
	client, err := phase11AnalyzeClientWebRegions(ctx, document, clientOptions, language, analyzer, excludedClientRanges)
	if err != nil {
		return AnalyzerResult{}, err
	}
	result.Analysis = phase11MergeAnalysis(result.Analysis, client.Analysis, options.Limits)
	phase11AppendDependencies(&result, client.Dependencies, options.Limits)
	phase11AppendRelations(&result, client.Relations, options.Limits)
	serverRegions := result.Regions
	result.Regions = nil
	phase11AppendRegions(&result, serverRegions, options.Limits)
	phase11AppendRegions(&result, client.Regions, options.Limits)
	return result, nil
}
func findRazorCodeRanges(ctx context.Context, document *SourceDocument, directives []string) ([]embeddedCodeRange, bool, error) {
	masked, complete := maskSimpleDelimited(document.Text, "@*", "*@")
	var result []embeddedCodeRange
	search := 0
	for search < len(masked) {
		if err := ctx.Err(); err != nil {
			return nil, false, operation.Wrap(operation.KindCancelled, "find_razor_code_ranges", document.Path, err)
		}
		best := -1
		kind := ""
		for _, directive := range directives {
			needle := "@" + directive
			relative := strings.Index(strings.ToLower(masked[search:]), strings.ToLower(needle))
			if relative >= 0 && (best < 0 || search+relative < best) {
				best = search + relative
				kind = directive
			}
		}
		if best < 0 {
			break
		}
		brace := best + len(kind) + 1
		for brace < len(masked) && (masked[brace] == ' ' || masked[brace] == '\t' || masked[brace] == '\r' || masked[brace] == '\n') {
			brace++
		}
		if brace >= len(masked) || masked[brace] != '{' {
			search = best + 1
			continue
		}
		sub := &SourceDocument{Path: document.Path + "#razor", Text: masked[brace:], Encoding: "utf-8", lineStarts: buildLineStarts(masked[brace:])}
		scan, err := ScanSource(ctx, sub, CSharpScannerProfile(), ScannerLimits{MaxTokens: scannerTokenBudget(sub.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: 2048})
		if err != nil {
			return nil, false, err
		}
		pairs := PairDelimiterTokens(scan.Tokens, nil)
		openToken := -1
		for i, t := range scan.Tokens {
			if t.Text == "{" {
				openToken = i
				break
			}
		}
		if openToken < 0 || pairs[openToken] <= openToken {
			complete = false
			break
		}
		closeToken := pairs[openToken]
		close := brace + scan.Tokens[closeToken].StartOffset
		end := brace + scan.Tokens[closeToken].EndOffset
		result = append(result, embeddedCodeRange{full: OffsetRange{Start: best, End: end}, content: OffsetRange{Start: brace + 1, End: close}, kind: "server-code"})
		search = end
	}
	sort.Slice(result, func(i, j int) bool { return result[i].full.Start < result[j].full.Start })
	return result, complete, nil
}
func maskSimpleDelimited(text, open, close string) (string, bool) {
	bytes := []byte(text)
	complete := true
	for search := 0; search < len(text); {
		r := strings.Index(text[search:], open)
		if r < 0 {
			break
		}
		start := search + r
		er := strings.Index(text[start+len(open):], close)
		end := len(text)
		if er < 0 {
			complete = false
		} else {
			end = start + len(open) + er + len(close)
		}
		for i := start; i < end; i++ {
			if bytes[i] != '\r' && bytes[i] != '\n' {
				bytes[i] = ' '
			}
		}
		if er < 0 {
			break
		}
		search = end
	}
	return string(bytes), complete
}

var razorUsingRE = regexp.MustCompile(`(?im)^\s*@using\s+([A-Za-z_][A-Za-z0-9_.]*)`)

func razorUsingDependencies(document *SourceDocument) []StructuralDependency {
	matches := razorUsingRE.FindAllStringSubmatchIndex(document.Text, -1)
	result := make([]StructuralDependency, 0, len(matches))
	for _, m := range matches {
		r, err := document.RangeFromUTF8Offsets(m[2], m[3])
		if err == nil {
			result = append(result, StructuralDependency{Kind: StructuralDependencyImport, Value: document.Text[m[2]:m[3]], Range: r, Evidence: SymbolEvidenceStructural})
		}
	}
	return result
}

// XAMLAnalyzer recognizes code-behind identities and named elements while
// leaving binding/runtime resolution to later project-semantic phases.
type XAMLAnalyzer struct{}

func (XAMLAnalyzer) ID() AnalyzerID   { return AnalyzerXAML }
func (XAMLAnalyzer) Language() string { return "xaml" }

var xamlClassRE = regexp.MustCompile(`(?is)\bx:Class\s*=\s*(?:"([^"'\s=<>]+)"|'([^"'\s=<>]+)')`)
var xamlNameRE = regexp.MustCompile(`(?is)\bx:Name\s*=\s*(?:"([^"'\s=<>]+)"|'([^"'\s=<>]+)')`)
var xamlXMLNSRE = regexp.MustCompile(`(?is)\bxmlns(?::[A-Za-z_][A-Za-z0-9_.-]*)?\s*=\s*(?:"([^"'\s<>]+)"|'([^"'\s<>]+)')`)

func xamlCapturedValueRange(match []int) (int, int, bool) {
	if len(match) >= 4 && match[2] >= 0 && match[3] >= match[2] {
		return match[2], match[3], true
	}
	if len(match) >= 6 && match[4] >= 0 && match[5] >= match[4] {
		return match[4], match[5], true
	}
	return 0, 0, false
}

func (XAMLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: "xaml", Analyzer: string(AnalyzerXAML), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	masked, complete := maskSimpleDelimited(document.Text, "<!--", "-->")
	if !complete {
		builder.MarkIncomplete()
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: "xaml-unterminated-comment", Message: "XAML comment is not terminated", Severity: DiagnosticWarning, AffectsCoverage: true})
	}
	classMatches := xamlClassRE.FindAllStringSubmatchIndex(masked, -1)
	var parent *SymbolParent
	if len(classMatches) > 0 {
		m := classMatches[0]
		nameStart, nameEnd, ok := xamlCapturedValueRange(m)
		if ok {
			s, err := builder.Add(SymbolSpec{Kind: SymbolKindClass, NativeKind: "x:Class", Name: document.Text[nameStart:nameEnd], QualifiedName: document.Text[nameStart:nameEnd], Declaration: OffsetRange{Start: m[0], End: m[1]}, NameRange: OffsetRange{Start: nameStart, End: nameEnd}, Evidence: SymbolEvidenceStructural})
			if operation.KindOf(err) == operation.KindLimit {
				builder.MarkTruncated()
			} else if err == nil {
				v := SymbolParent{ID: s.ID, QualifiedName: s.QualifiedName}
				parent = &v
			} else {
				builder.MarkIncomplete()
			}
		}
	}
	if parent != nil {
		for _, m := range xamlNameRE.FindAllStringSubmatchIndex(masked, -1) {
			nameStart, nameEnd, ok := xamlCapturedValueRange(m)
			if !ok {
				continue
			}
			_, err := builder.Add(SymbolSpec{Kind: SymbolKindField, NativeKind: "x:Name", Name: document.Text[nameStart:nameEnd], Parent: parent, Declaration: OffsetRange{Start: m[0], End: m[1]}, NameRange: OffsetRange{Start: nameStart, End: nameEnd}, Evidence: SymbolEvidenceStructural})
			if operation.KindOf(err) == operation.KindLimit {
				builder.MarkTruncated()
				break
			}
			if err != nil {
				builder.MarkIncomplete()
			}
		}
	}
	deps := make([]StructuralDependency, 0)
	for _, m := range xamlXMLNSRE.FindAllStringSubmatchIndex(masked, -1) {
		start, end, ok := xamlCapturedValueRange(m)
		if !ok {
			continue
		}
		r, err := document.RangeFromUTF8Offsets(start, end)
		if err == nil {
			deps = append(deps, StructuralDependency{Kind: StructuralDependencyImport, Value: document.Text[start:end], Range: r, Evidence: SymbolEvidenceStructural})
		}
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: deps}, nil
}
