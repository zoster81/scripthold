package sourceintelligence

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

type VueAnalyzer struct{}
type SvelteAnalyzer struct{}
type AstroAnalyzer struct{}
type PHPHTMLAnalyzer struct{}
type JSPAnalyzer struct{}
type JinjaAnalyzer struct{}
type TwigAnalyzer struct{}
type BladeAnalyzer struct{}
type EJSAnalyzer struct{}

func (VueAnalyzer) ID() AnalyzerID       { return AnalyzerVue }
func (VueAnalyzer) Language() string     { return "vue" }
func (SvelteAnalyzer) ID() AnalyzerID    { return AnalyzerSvelte }
func (SvelteAnalyzer) Language() string  { return "svelte" }
func (AstroAnalyzer) ID() AnalyzerID     { return AnalyzerAstro }
func (AstroAnalyzer) Language() string   { return "astro" }
func (PHPHTMLAnalyzer) ID() AnalyzerID   { return AnalyzerPHPHTML }
func (PHPHTMLAnalyzer) Language() string { return "php-html" }
func (JSPAnalyzer) ID() AnalyzerID       { return AnalyzerJSP }
func (JSPAnalyzer) Language() string     { return "jsp" }
func (JinjaAnalyzer) ID() AnalyzerID     { return AnalyzerJinja }
func (JinjaAnalyzer) Language() string   { return "jinja" }
func (TwigAnalyzer) ID() AnalyzerID      { return AnalyzerTwig }
func (TwigAnalyzer) Language() string    { return "twig" }
func (BladeAnalyzer) ID() AnalyzerID     { return AnalyzerBlade }
func (BladeAnalyzer) Language() string   { return "blade" }
func (EJSAnalyzer) ID() AnalyzerID       { return AnalyzerEJS }
func (EJSAnalyzer) Language() string     { return "ejs" }

type phase11EmbeddedRegion struct {
	kind     string
	language string
	full     OffsetRange
	content  OffsetRange
}

var (
	phase11ScriptTag           = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script\s*>`)
	phase11StyleTag            = regexp.MustCompile(`(?is)<style\b([^>]*)>(.*?)</style\s*>`)
	phase11ScriptOpen          = regexp.MustCompile(`(?is)<script\b`)
	phase11StyleOpen           = regexp.MustCompile(`(?is)<style\b`)
	phase11LangAttr            = regexp.MustCompile(`(?i)\blang\s*=\s*["']([^"']+)["']`)
	phase11PHPBlock            = regexp.MustCompile(`(?is)<\?(?:php|=)?(.*?)\?>`)
	phase11PHPOpen             = regexp.MustCompile(`(?is)<\?(?:php\b|=)`)
	phase11JSPBlock            = regexp.MustCompile(`(?is)<%[!@=]?(.*?)%>`)
	phase11EJSBlock            = regexp.MustCompile(`(?is)<%[-_=#]?(.*?)[-_]?%>`)
	phase11PercentOpen         = regexp.MustCompile(`(?is)<%`)
	phase11BladePHP            = regexp.MustCompile(`(?is)@php\b(.*?)@endphp\b`)
	phase11BladePHPOpen        = regexp.MustCompile(`(?is)@php\b`)
	phase11BladeSection        = regexp.MustCompile(`(?i)@section\s*\(\s*["']([^"']+)["']\s*\)`)
	phase11BladeDependency     = regexp.MustCompile(`(?i)@(extends|include)\s*\(\s*["']([^"']+)["']\s*\)`)
	phase11TemplateDecl        = regexp.MustCompile(`(?i)\{%[-+]?\s*(block|macro)\s+([A-Za-z_][A-Za-z0-9_-]*)`)
	phase11TemplateDependency  = regexp.MustCompile(`(?i)\{%[-+]?\s*(extends|include|import|from)\s+["']([^"']+)["']`)
	phase11JSPIncludeDirective = regexp.MustCompile(`(?is)<%@\s*include\b[^%>]*\bfile\s*=\s*["']([^"']+)["'][^%>]*%>`)
	phase11SrcAttr             = regexp.MustCompile(`(?i)\bsrc\s*=\s*["']([^"']+)["']`)
)

func (VueAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePhase11ScriptStyleHost(ctx, document, options, "vue", AnalyzerVue, false)
}

func (SvelteAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePhase11ScriptStyleHost(ctx, document, options, "svelte", AnalyzerSvelte, false)
}

func (AstroAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePhase11ScriptStyleHost(ctx, document, options, "astro", AnalyzerAstro, true)
}

func (PHPHTMLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	probe := phase11MaskHostComments(document.Text)
	regions := phase11RegexRegions(probe, phase11PHPBlock, "php", "php", 2, 3)
	complete := phase11OpeningsCovered(probe, regions, phase11PHPOpen)
	result, err := analyzePhase11DelimitedHost(ctx, document, options, "php-html", AnalyzerPHPHTML, regions)
	if err != nil {
		return AnalyzerResult{}, err
	}
	if !complete {
		phase11MarkPartial(&result.Analysis, options.Limits, false, "php-html-unterminated-region", "PHP region is not terminated")
	}
	return result, nil
}

func (JSPAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	probe := phase11MaskHostComments(document.Text)
	regions := phase11RegexRegions(probe, phase11JSPBlock, "jsp-java", "java", 2, 3)
	complete := phase11OpeningsCovered(probe, regions, phase11PercentOpen)
	result, err := analyzePhase11DelimitedHost(ctx, document, options, "jsp", AnalyzerJSP, regions)
	if err != nil {
		return AnalyzerResult{}, err
	}
	phase11AppendDependencies(&result, phase11JSPDependencies(document, probe), options.Limits)
	if !complete {
		phase11MarkPartial(&result.Analysis, options.Limits, false, "jsp-unterminated-region", "JSP region is not terminated")
	}
	return result, nil
}

func (EJSAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	probe := phase11MaskHostComments(document.Text)
	regions := phase11RegexRegions(probe, phase11EJSBlock, "ejs-js", "javascript", 2, 3)
	complete := phase11OpeningsCovered(probe, regions, phase11PercentOpen)
	result, err := analyzePhase11DelimitedHost(ctx, document, options, "ejs", AnalyzerEJS, regions)
	if err != nil {
		return AnalyzerResult{}, err
	}
	retainedRegions, _ := phase11CapRegions(regions, options.Limits.MaxSymbols)
	dependencies, err := phase11EJSDependencies(ctx, document, retainedRegions, options.MaxNesting)
	if err != nil {
		return AnalyzerResult{}, err
	}
	phase11AppendDependencies(&result, dependencies, options.Limits)
	if !complete {
		phase11MarkPartial(&result.Analysis, options.Limits, false, "ejs-unterminated-region", "EJS region is not terminated")
	}
	return result, nil
}

func (BladeAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	probe := phase10MaskDelimitedRegions(document.Text, [][2]string{{"<!--", "-->"}, {"{{--", "--}}"}})
	regions := phase11RegexRegions(probe, phase11BladePHP, "blade-php", "php", 2, 3)
	complete := phase11OpeningsCovered(probe, regions, phase11BladePHPOpen)
	result, err := analyzePhase11DelimitedHost(ctx, document, options, "blade", AnalyzerBlade, regions)
	if err != nil {
		return AnalyzerResult{}, err
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: "blade", Analyzer: string(AnalyzerBlade), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	for _, match := range phase11BladeSection.FindAllStringSubmatchIndex(probe, -1) {
		name := document.Text[match[2]:match[3]]
		_, _ = builder.Add(SymbolSpec{Kind: SymbolKindSection, NativeKind: "section", Name: name, Declaration: OffsetRange{Start: match[0], End: match[1]}, NameRange: OffsetRange{Start: match[2], End: match[3]}, Evidence: SymbolEvidenceStructural})
	}
	result.Analysis = phase11MergeAnalysis(result.Analysis, builder.Result(), options.Limits)
	phase11AppendDependencies(&result, phase11BladeDependencies(document, probe), options.Limits)
	if !complete {
		phase11MarkPartial(&result.Analysis, options.Limits, false, "blade-unterminated-region", "Blade @php region is not terminated")
	}
	return result, nil
}

func (JinjaAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePhase11TemplateHost(ctx, document, options, "jinja", AnalyzerJinja)
}

func (TwigAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePhase11TemplateHost(ctx, document, options, "twig", AnalyzerTwig)
}

func analyzePhase11ScriptStyleHost(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID, astro bool) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	regions := make([]phase11EmbeddedRegion, 0, 8)
	probe := phase10MaskDelimitedRegions(document.Text, [][2]string{{"<!--", "-->"}})
	if astro {
		if frontmatter, ok := phase11AstroFrontmatter(document.Text); ok {
			regions = append(regions, phase11EmbeddedRegion{kind: "frontmatter", language: "typescript", full: frontmatter.Full, content: frontmatter.Content})
			var err error
			probe, err = phase11MaskRanges(probe, []OffsetRange{frontmatter.Full})
			if err != nil {
				return AnalyzerResult{}, err
			}
		}
	}
	regions = append(regions, phase11TagRegions(probe, phase11ScriptTag, "script", phase11ScriptLanguage)...)
	regions = append(regions, phase11TagRegions(probe, phase11StyleTag, "style", phase11StyleLanguage)...)
	regions = phase11OrderedNonOverlappingRegions(regions)
	complete := phase11OpeningsCovered(probe, regions, phase11ScriptOpen) && phase11OpeningsCovered(probe, regions, phase11StyleOpen)
	result, err := analyzePhase11DelimitedHost(ctx, document, options, language, analyzer, regions)
	if err != nil {
		return AnalyzerResult{}, err
	}
	retainedRegions, _ := phase11CapRegions(regions, options.Limits.MaxSymbols)
	phase11AppendDependencies(&result, phase11ScriptSourceDependencies(document, retainedRegions), options.Limits)
	if !complete {
		phase11MarkPartial(&result.Analysis, options.Limits, false, language+"-unterminated-region", "script/style region is not terminated")
	}
	return result, nil
}

func analyzePhase11DelimitedHost(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID, regions []phase11EmbeddedRegion) (AnalyzerResult, error) {
	allRegions := regions
	regions, regionsTruncated := phase11CapRegions(regions, options.Limits.MaxSymbols)
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_composite_source", document.Path, err)
	}
	maskedHost, err := phase11MaskRanges(document.Text, phase11FullRanges(allRegions))
	if err != nil {
		return AnalyzerResult{}, err
	}
	result := AnalyzerResult{Analysis: AnalysisResult{CoverageComplete: true}}
	hostAnalysis, err := phase11AnalyzeMasked(ctx, document, maskedHost, options, language, analyzer, "host-html", "html")
	if err != nil {
		return AnalyzerResult{}, err
	}
	result.Analysis = phase11MergeAnalysis(result.Analysis, hostAnalysis.Analysis, options.Limits)
	phase11AppendDependencies(&result, hostAnalysis.Dependencies, options.Limits)
	phase11AppendRelations(&result, hostAnalysis.Relations, options.Limits)

	for index, region := range regions {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_composite_source", document.Path, err)
		}
		regionID := fmt.Sprintf("%s-%d", region.kind, index+1)
		rangeValue, rangeErr := document.RangeFromUTF8Offsets(region.full.Start, region.full.End)
		if rangeErr != nil {
			return AnalyzerResult{}, rangeErr
		}
		supported := phase11LanguageSupported(region.language)
		result.Regions = append(result.Regions, SourceRegion{ID: regionID, Kind: region.kind, Language: region.language, Range: rangeValue, Evidence: SymbolEvidenceStructural, Supported: supported})
		if !supported {
			result.Analysis.CoverageComplete = false
			continue
		}
		masked, maskErr := MaskOutsideRanges(document.Text, []OffsetRange{region.content})
		if maskErr != nil {
			return AnalyzerResult{}, maskErr
		}
		embedded, analyzeErr := phase11AnalyzeMasked(ctx, document, masked, options, language, analyzer, regionID, region.language)
		if analyzeErr != nil {
			return AnalyzerResult{}, analyzeErr
		}
		result.Analysis = phase11MergeAnalysis(result.Analysis, embedded.Analysis, options.Limits)
		phase11AppendDependencies(&result, embedded.Dependencies, options.Limits)
		phase11AppendRelations(&result, embedded.Relations, options.Limits)
	}
	if regionsTruncated {
		phase11MarkPartial(&result.Analysis, options.Limits, true, language+"-region-limit", "composite region retention limit reached")
	}
	return result, nil
}

func analyzePhase11TemplateHost(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	masked, ranges, complete := phase11MaskTemplateSyntax(document.Text)
	declarationProbe := phase10MaskDelimitedRegions(document.Text, [][2]string{{"<!--", "-->"}, {"{#", "#}"}, {"{{", "}}"}})
	result, err := analyzePhase11DelimitedHost(ctx, document, options, language, analyzer, nil)
	if err != nil {
		return AnalyzerResult{}, err
	}
	host, err := phase11AnalyzeMasked(ctx, document, masked, options, language, analyzer, "host-html", "html")
	if err != nil {
		return AnalyzerResult{}, err
	}
	result.Analysis = phase11MergeAnalysis(AnalysisResult{CoverageComplete: true}, host.Analysis, options.Limits)
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: language, Analyzer: string(analyzer), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	for _, match := range phase11TemplateDecl.FindAllStringSubmatchIndex(declarationProbe, -1) {
		kindWord := strings.ToLower(document.Text[match[2]:match[3]])
		name := document.Text[match[4]:match[5]]
		kind := SymbolKindSection
		if kindWord == "macro" {
			kind = SymbolKindFunction
		}
		_, _ = builder.Add(SymbolSpec{Kind: kind, NativeKind: kindWord, Name: name, Declaration: OffsetRange{Start: match[0], End: match[1]}, NameRange: OffsetRange{Start: match[4], End: match[5]}, Evidence: SymbolEvidenceStructural})
	}
	result.Analysis = phase11MergeAnalysis(result.Analysis, builder.Result(), options.Limits)
	phase11AppendDependencies(&result, phase11TemplateDependencies(document, declarationProbe), options.Limits)
	retainedRanges, rangesTruncated := phase11CapOffsetRanges(ranges, options.Limits.MaxSymbols)
	for index, value := range retainedRanges {
		rangeValue, rangeErr := document.RangeFromUTF8Offsets(value.Start, value.End)
		if rangeErr == nil {
			result.Regions = append(result.Regions, SourceRegion{ID: fmt.Sprintf("template-%d", index+1), Kind: "template", Language: language, Range: rangeValue, Evidence: SymbolEvidenceStructural, Supported: true})
		}
	}
	if rangesTruncated {
		phase11MarkPartial(&result.Analysis, options.Limits, true, language+"-region-limit", "template region retention limit reached")
	}
	if !complete {
		phase11MarkPartial(&result.Analysis, options.Limits, false, language+"-unterminated-region", "template region is not terminated")
	}
	return result, nil
}

func phase11AnalyzeMasked(ctx context.Context, host *SourceDocument, masked string, options AnalyzeOptions, _ string, analyzer AnalyzerID, regionID, embeddedLanguage string) (AnalyzerResult, error) {
	registry, err := NewLanguageRegistry(defaultLanguageDescriptors())
	if err != nil {
		return AnalyzerResult{}, err
	}
	descriptor, ok := registry.Resolve(embeddedLanguage)
	if !ok {
		return AnalyzerResult{}, operation.New(operation.KindUnsupported, "embedded language is not registered")
	}
	delegate, ok := AnalyzerFor(descriptor)
	if !ok {
		return AnalyzerResult{}, operation.New(operation.KindUnsupported, "embedded language analyzer is not available")
	}
	clone := *host
	clone.Text = masked
	clone.lineStarts = buildLineStarts(masked)
	source, err := delegate.Analyze(ctx, &clone, options)
	if err != nil {
		return AnalyzerResult{}, err
	}
	analysis, err := reprojectAnalyzerSymbols(ctx, host, source, options, embeddedLanguage, analyzer, regionID, 0, nil)
	if err != nil {
		return AnalyzerResult{}, err
	}
	return AnalyzerResult{Analysis: analysis, Dependencies: source.Dependencies, Relations: source.Relations}, nil
}

func phase11MergeAnalysis(dst, src AnalysisResult, limits SymbolBuilderLimits) AnalysisResult {
	if !src.CoverageComplete {
		dst.CoverageComplete = false
	}
	if src.Truncated {
		dst.Truncated = true
		dst.CoverageComplete = false
	}
	if src.DiagnosticsTruncated {
		dst.DiagnosticsTruncated = true
		dst.CoverageComplete = false
	}
	seen := make(map[string]struct{}, len(dst.Symbols))
	for _, symbol := range dst.Symbols {
		seen[symbol.ID] = struct{}{}
	}
	for _, symbol := range src.Symbols {
		if _, exists := seen[symbol.ID]; exists {
			continue
		}
		if len(dst.Symbols) >= limits.MaxSymbols {
			dst.Truncated = true
			dst.CoverageComplete = false
			break
		}
		seen[symbol.ID] = struct{}{}
		dst.Symbols = append(dst.Symbols, symbol)
	}
	for _, diagnostic := range src.Diagnostics {
		if len(dst.Diagnostics) >= limits.MaxDiagnostics {
			dst.DiagnosticsTruncated = true
			dst.CoverageComplete = false
			break
		}
		dst.Diagnostics = append(dst.Diagnostics, diagnostic)
	}
	return dst
}

func phase11LanguageSupported(language string) bool {
	registry, err := NewLanguageRegistry(defaultLanguageDescriptors())
	if err != nil {
		return false
	}
	descriptor, ok := registry.Resolve(language)
	if !ok {
		return false
	}
	_, ok = AnalyzerFor(descriptor)
	return ok
}

func phase11TagRegions(text string, pattern *regexp.Regexp, kind string, language func(string) string) []phase11EmbeddedRegion {
	matches := pattern.FindAllStringSubmatchIndex(text, -1)
	regions := make([]phase11EmbeddedRegion, 0, len(matches))
	for _, match := range matches {
		attrs := ""
		if match[2] >= 0 {
			attrs = text[match[2]:match[3]]
		}
		regions = append(regions, phase11EmbeddedRegion{kind: kind, language: language(attrs), full: OffsetRange{Start: match[0], End: match[1]}, content: OffsetRange{Start: match[4], End: match[5]}})
	}
	return regions
}

func phase11RegexRegions(text string, pattern *regexp.Regexp, kind, language string, contentStartIndex, contentEndIndex int) []phase11EmbeddedRegion {
	matches := pattern.FindAllStringSubmatchIndex(text, -1)
	regions := make([]phase11EmbeddedRegion, 0, len(matches))
	for _, match := range matches {
		if contentStartIndex >= len(match) || contentEndIndex >= len(match) || match[contentStartIndex] < 0 || match[contentEndIndex] < 0 {
			continue
		}
		regions = append(regions, phase11EmbeddedRegion{kind: kind, language: language, full: OffsetRange{Start: match[0], End: match[1]}, content: OffsetRange{Start: match[contentStartIndex], End: match[contentEndIndex]}})
	}
	return regions
}

func phase11ScriptLanguage(attrs string) string {
	if match := phase11LangAttr.FindStringSubmatch(attrs); len(match) > 1 {
		switch strings.ToLower(strings.TrimSpace(match[1])) {
		case "ts", "typescript":
			return "typescript"
		case "js", "javascript", "module":
			return "javascript"
		default:
			return strings.ToLower(strings.TrimSpace(match[1]))
		}
	}
	return "javascript"
}

func phase11StyleLanguage(attrs string) string {
	if match := phase11LangAttr.FindStringSubmatch(attrs); len(match) > 1 {
		switch strings.ToLower(strings.TrimSpace(match[1])) {
		case "css":
			return "css"
		case "scss":
			return "scss"
		case "sass":
			return "sass"
		case "less":
			return "less"
		default:
			return strings.ToLower(strings.TrimSpace(match[1]))
		}
	}
	return "css"
}

func phase11AstroFrontmatter(text string) (CompositeSegment, bool) {
	if !strings.HasPrefix(text, "---") {
		return CompositeSegment{}, false
	}
	lineEnd := strings.IndexByte(text, '\n')
	if lineEnd < 0 {
		return CompositeSegment{}, false
	}
	contentStart := lineEnd + 1
	closeRel := strings.Index(text[contentStart:], "\n---")
	if closeRel < 0 {
		return CompositeSegment{}, false
	}
	closeStart := contentStart + closeRel + 1
	closeEnd := closeStart + 3
	if closeEnd < len(text) && text[closeEnd] == '\r' {
		closeEnd++
	}
	if closeEnd < len(text) && text[closeEnd] == '\n' {
		closeEnd++
	}
	return CompositeSegment{Kind: "frontmatter", Language: "typescript", Full: OffsetRange{Start: 0, End: closeEnd}, Content: OffsetRange{Start: contentStart, End: closeStart}}, true
}

func phase11FullRanges(regions []phase11EmbeddedRegion) []OffsetRange {
	ranges := make([]OffsetRange, 0, len(regions))
	for _, region := range regions {
		ranges = append(ranges, region.full)
	}
	return ranges
}

func phase11MaskRanges(text string, ranges []OffsetRange) (string, error) {
	masked := []byte(text)
	for _, value := range ranges {
		if value.Start < 0 || value.End < value.Start || value.End > len(text) || !utf8Boundary(text, value.Start) || !utf8Boundary(text, value.End) {
			return "", operation.New(operation.KindInvalidInput, "composite mask range is invalid")
		}
		phase10MaskRange(masked, value.Start, value.End)
	}
	return string(masked), nil
}

func phase11MaskTemplateSyntax(text string) (string, []OffsetRange, bool) {
	probe := phase11MaskHostComments(text)
	masked := []byte(probe)
	ranges := make([]OffsetRange, 0, 16)
	complete := true
	for position := 0; position < len(probe); {
		open, delimiters := phase11NextTemplateDelimiter(probe, position)
		if open < 0 {
			break
		}
		close := strings.Index(probe[open+len(delimiters[0]):], delimiters[1])
		if close < 0 {
			complete = false
			value := OffsetRange{Start: open, End: len(probe)}
			ranges = append(ranges, value)
			phase10MaskRange(masked, value.Start, value.End)
			break
		}
		close += open + len(delimiters[0]) + len(delimiters[1])
		value := OffsetRange{Start: open, End: close}
		ranges = append(ranges, value)
		phase10MaskRange(masked, value.Start, value.End)
		position = close
	}
	return string(masked), ranges, complete
}

func phase11NextTemplateDelimiter(text string, position int) (int, [2]string) {
	best := -1
	var selected [2]string
	for _, delimiters := range [][2]string{{"{#", "#}"}, {"{{", "}}"}, {"{%", "%}"}} {
		relative := strings.Index(text[position:], delimiters[0])
		if relative < 0 {
			continue
		}
		absolute := position + relative
		if best < 0 || absolute < best {
			best = absolute
			selected = delimiters
		}
	}
	return best, selected
}

func phase11MaskHostComments(text string) string {
	return phase10MaskDelimitedRegions(text, [][2]string{{"<!--", "-->"}})
}

func phase11AnalyzeClientWebRegions(ctx context.Context, document *SourceDocument, options AnalyzeOptions, hostLanguage string, analyzer AnalyzerID, excluded []OffsetRange) (AnalyzerResult, error) {
	probe := phase10MaskDelimitedRegions(document.Text, [][2]string{{"<!--", "-->"}, {"@*", "*@"}})
	var err error
	if len(excluded) > 0 {
		probe, err = phase11MaskRanges(probe, excluded)
		if err != nil {
			return AnalyzerResult{}, err
		}
	}
	regions := append(
		phase11TagRegions(probe, phase11ScriptTag, "script", phase11ScriptLanguage),
		phase11TagRegions(probe, phase11StyleTag, "style", phase11StyleLanguage)...,
	)
	regions = phase11OrderedNonOverlappingRegions(regions)
	allRegions := regions
	complete := phase11OpeningsCovered(probe, allRegions, phase11ScriptOpen) && phase11OpeningsCovered(probe, allRegions, phase11StyleOpen)
	regions, regionsTruncated := phase11CapRegions(regions, options.Limits.MaxSymbols)

	hostMasks := append([]OffsetRange(nil), excluded...)
	hostMasks = append(hostMasks, phase11FullRanges(allRegions)...)
	hostMasked, err := phase11MaskRanges(document.Text, hostMasks)
	if err != nil {
		return AnalyzerResult{}, err
	}
	result, err := phase11AnalyzeMasked(ctx, document, hostMasked, options, hostLanguage, analyzer, "client-host", "html")
	if err != nil {
		return AnalyzerResult{}, err
	}
	remaining := max(0, options.Limits.MaxSymbols-len(result.Analysis.Symbols))
	for index, region := range regions {
		regionID := fmt.Sprintf("client-%s-%d", region.kind, index+1)
		supported := phase11LanguageSupported(region.language)
		public, rangeErr := sourceRegionForOffsets(document, regionID, region.kind, region.language, region.full.Start, region.full.End, supported)
		if rangeErr != nil {
			return AnalyzerResult{}, rangeErr
		}
		result.Regions = append(result.Regions, public)
		if !supported {
			result.Analysis.CoverageComplete = false
			continue
		}
		regionOptions := options
		if remaining > 0 {
			regionOptions.Limits.MaxSymbols = remaining
		}
		masked, maskErr := MaskOutsideRanges(document.Text, []OffsetRange{region.content})
		if maskErr != nil {
			return AnalyzerResult{}, maskErr
		}
		embedded, analyzeErr := phase11AnalyzeMasked(ctx, document, masked, regionOptions, hostLanguage, analyzer, regionID, region.language)
		if analyzeErr != nil {
			return AnalyzerResult{}, analyzeErr
		}
		result.Analysis = phase11MergeAnalysis(result.Analysis, embedded.Analysis, options.Limits)
		phase11AppendDependencies(&result, embedded.Dependencies, options.Limits)
		phase11AppendRelations(&result, embedded.Relations, options.Limits)
		remaining = max(0, options.Limits.MaxSymbols-len(result.Analysis.Symbols))
	}
	phase11AppendDependencies(&result, phase11ScriptSourceDependencies(document, regions), options.Limits)
	if regionsTruncated {
		phase11MarkPartial(&result.Analysis, options.Limits, true, hostLanguage+"-region-limit", "client region retention limit reached")
	}
	if !complete {
		phase11MarkPartial(&result.Analysis, options.Limits, false, hostLanguage+"-unterminated-client-region", "client script/style region is not terminated")
	}
	return result, nil
}

func phase11ScriptSourceDependencies(document *SourceDocument, regions []phase11EmbeddedRegion) []StructuralDependency {
	var dependencies []StructuralDependency
	for _, region := range regions {
		if region.kind != "script" || region.content.Start <= region.full.Start || region.content.Start > len(document.Text) {
			continue
		}
		opening := document.Text[region.full.Start:region.content.Start]
		match := phase11SrcAttr.FindStringSubmatchIndex(opening)
		if len(match) < 4 || match[2] < 0 {
			continue
		}
		start := region.full.Start + match[2]
		end := region.full.Start + match[3]
		phase11AddDependency(document, &dependencies, StructuralDependencyInclude, document.Text[start:end], start, end)
	}
	return dependencies
}

func phase11JSPDependencies(document *SourceDocument, probe string) []StructuralDependency {
	var dependencies []StructuralDependency
	for _, match := range phase11JSPIncludeDirective.FindAllStringSubmatchIndex(probe, -1) {
		if len(match) < 4 || match[2] < 0 {
			continue
		}
		phase11AddDependency(document, &dependencies, StructuralDependencyInclude, document.Text[match[2]:match[3]], match[2], match[3])
	}
	return dependencies
}

func phase11TemplateDependencies(document *SourceDocument, probe string) []StructuralDependency {
	var dependencies []StructuralDependency
	for _, match := range phase11TemplateDependency.FindAllStringSubmatchIndex(probe, -1) {
		if len(match) < 6 || match[4] < 0 {
			continue
		}
		kind := StructuralDependencyInclude
		directive := strings.ToLower(document.Text[match[2]:match[3]])
		if directive == "import" || directive == "from" {
			kind = StructuralDependencyImport
		}
		phase11AddDependency(document, &dependencies, kind, document.Text[match[4]:match[5]], match[4], match[5])
	}
	return dependencies
}

func phase11BladeDependencies(document *SourceDocument, probe string) []StructuralDependency {
	var dependencies []StructuralDependency
	for _, match := range phase11BladeDependency.FindAllStringSubmatchIndex(probe, -1) {
		if len(match) < 6 || match[4] < 0 {
			continue
		}
		phase11AddDependency(document, &dependencies, StructuralDependencyInclude, document.Text[match[4]:match[5]], match[4], match[5])
	}
	return dependencies
}

func phase11EJSDependencies(ctx context.Context, document *SourceDocument, regions []phase11EmbeddedRegion, maxNesting int) ([]StructuralDependency, error) {
	var dependencies []StructuralDependency
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	for _, region := range regions {
		if region.kind != "ejs-js" || region.content.End <= region.content.Start {
			continue
		}
		text := document.Text[region.content.Start:region.content.End]
		sub := &SourceDocument{Path: document.Path + "#ejs", Text: text, Encoding: "utf-8", lineStarts: buildLineStarts(text)}
		scan, err := ScanSource(ctx, sub, JavaScriptScannerProfile(), ScannerLimits{MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
		if err != nil {
			return nil, err
		}
		for index := 0; index < len(scan.Tokens); index++ {
			if scan.Tokens[index].Text != "include" {
				continue
			}
			open := phase11NextCodeToken(scan.Tokens, index+1)
			valueIndex := phase11NextCodeToken(scan.Tokens, open+1)
			close := phase11NextCodeToken(scan.Tokens, valueIndex+1)
			if open < 0 || valueIndex < 0 || close < 0 || scan.Tokens[open].Text != "(" || scan.Tokens[valueIndex].Kind != TokenString || scan.Tokens[close].Text != ")" {
				continue
			}
			value := ecmaStringLiteralValue(scan.Tokens[valueIndex].Text)
			if value == "" {
				continue
			}
			start := region.content.Start + scan.Tokens[valueIndex].StartOffset
			end := region.content.Start + scan.Tokens[valueIndex].EndOffset
			phase11AddDependency(document, &dependencies, StructuralDependencyInclude, value, start, end)
		}
	}
	return dependencies, nil
}

func phase11NextCodeToken(tokens []Token, start int) int {
	if start < 0 {
		return -1
	}
	for index := start; index < len(tokens); index++ {
		if tokens[index].Kind != TokenNewline && tokens[index].Kind != TokenDirective && tokens[index].Kind != TokenEOF {
			return index
		}
	}
	return -1
}

func phase11AddDependency(document *SourceDocument, dependencies *[]StructuralDependency, kind StructuralDependencyKind, value string, start, end int) {
	value = strings.TrimSpace(value)
	if value == "" || start < 0 || end <= start || end > len(document.Text) {
		return
	}
	rangeValue, err := document.RangeFromUTF8Offsets(start, end)
	if err != nil {
		return
	}
	*dependencies = appendUniqueDependencies(*dependencies, []StructuralDependency{{Kind: kind, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural}})
}

func phase11CapRegions(regions []phase11EmbeddedRegion, limit int) ([]phase11EmbeddedRegion, bool) {
	limit = max(1, limit)
	if len(regions) <= limit {
		return regions, false
	}
	return regions[:limit], true
}

func phase11CapOffsetRanges(ranges []OffsetRange, limit int) ([]OffsetRange, bool) {
	limit = max(1, limit)
	if len(ranges) <= limit {
		return ranges, false
	}
	return ranges[:limit], true
}

func phase11OpeningsCovered(text string, regions []phase11EmbeddedRegion, opening *regexp.Regexp) bool {
	locations := opening.FindAllStringIndex(text, -1)
	regionIndex := 0
	for _, location := range locations {
		start := location[0]
		for regionIndex < len(regions) && regions[regionIndex].full.End <= start {
			regionIndex++
		}
		if regionIndex >= len(regions) || start < regions[regionIndex].full.Start || start >= regions[regionIndex].full.End {
			return false
		}
	}
	return true
}

func phase11MarkPartial(result *AnalysisResult, limits SymbolBuilderLimits, truncated bool, code, message string) {
	result.CoverageComplete = false
	if truncated {
		result.Truncated = true
	}
	limit := max(1, limits.MaxDiagnostics)
	if len(result.Diagnostics) < limit {
		result.Diagnostics = append(result.Diagnostics, AnalysisDiagnostic{Code: code, Message: message, Severity: DiagnosticWarning})
	} else {
		result.DiagnosticsTruncated = true
	}
}

func phase11AppendDependencies(result *AnalyzerResult, extra []StructuralDependency, limits SymbolBuilderLimits) {
	merged := appendUniqueDependencies(result.Dependencies, extra)
	limit := max(1, limits.MaxSymbols)
	if len(merged) > limit {
		merged = merged[:limit]
		phase11MarkPartial(&result.Analysis, limits, true, "dependency-limit", "dependency retention limit reached")
	}
	result.Dependencies = merged
}

func phase11AppendRegions(result *AnalyzerResult, extra []SourceRegion, limits SymbolBuilderLimits) {
	limit := max(1, limits.MaxSymbols)
	remaining := limit - len(result.Regions)
	if remaining <= 0 {
		if len(extra) > 0 {
			phase11MarkPartial(&result.Analysis, limits, true, "region-limit", "composite region retention limit reached")
		}
		return
	}
	if len(extra) > remaining {
		result.Regions = append(result.Regions, extra[:remaining]...)
		phase11MarkPartial(&result.Analysis, limits, true, "region-limit", "composite region retention limit reached")
		return
	}
	result.Regions = append(result.Regions, extra...)
}

func phase11AppendRelations(result *AnalyzerResult, extra []StructuralRelation, limits SymbolBuilderLimits) {
	limit := max(1, limits.MaxSymbols)
	remaining := limit - len(result.Relations)
	if remaining <= 0 {
		if len(extra) > 0 {
			phase11MarkPartial(&result.Analysis, limits, true, "relation-limit", "relation retention limit reached")
		}
		return
	}
	if len(extra) > remaining {
		result.Relations = append(result.Relations, extra[:remaining]...)
		phase11MarkPartial(&result.Analysis, limits, true, "relation-limit", "relation retention limit reached")
		return
	}
	result.Relations = append(result.Relations, extra...)
}

func phase11OrderedNonOverlappingRegions(regions []phase11EmbeddedRegion) []phase11EmbeddedRegion {
	sort.SliceStable(regions, func(i, j int) bool {
		if regions[i].full.Start != regions[j].full.Start {
			return regions[i].full.Start < regions[j].full.Start
		}
		return regions[i].full.End > regions[j].full.End
	})
	result := make([]phase11EmbeddedRegion, 0, len(regions))
	end := -1
	for _, region := range regions {
		if region.full.Start < end {
			continue
		}
		result = append(result, region)
		end = region.full.End
	}
	return result
}
