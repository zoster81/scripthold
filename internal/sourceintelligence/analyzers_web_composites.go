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

type compositeEmbeddedRegion struct {
	kind     string
	language string
	full     OffsetRange
	content  OffsetRange
}

type templateScope struct {
	kind        string
	name        string
	parent      SymbolParent
	declaration OffsetRange
}

type bladeDeclaration struct {
	kind        SymbolKind
	nativeKind  string
	name        string
	declaration OffsetRange
	nameRange   OffsetRange
	volt        bool
}

type bladeVoltComponent struct {
	symbol      NormalizedSymbol
	declaration OffsetRange
}

type bladeAnonymousClass struct {
	classIndex  int
	openIndex   int
	closeIndex  int
	regionIndex int
}

var (
	scriptTagPattern              = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script\s*>`)
	styleTagPattern               = regexp.MustCompile(`(?is)<style\b([^>]*)>(.*?)</style\s*>`)
	scriptOpenPattern             = regexp.MustCompile(`(?is)<script\b`)
	styleOpenPattern              = regexp.MustCompile(`(?is)<style\b`)
	languageAttributePattern      = regexp.MustCompile(`(?i)\blang\s*=\s*["']([^"']+)["']`)
	typeAttributePattern          = regexp.MustCompile(`(?i)\btype\s*=\s*["']([^"']+)["']`)
	phpBlockPattern               = regexp.MustCompile(`(?is)<\?(?:php|=)?(.*?)\?>`)
	phpOpenPattern                = regexp.MustCompile(`(?is)<\?(?:php\b|=)`)
	jspBlockPattern               = regexp.MustCompile(`(?is)<%[!@=]?(.*?)%>`)
	ejsBlockPattern               = regexp.MustCompile(`(?is)<%[-_=#]?(.*?)[-_]?%>`)
	percentOpenPattern            = regexp.MustCompile(`(?is)<%`)
	bladeSectionPattern           = regexp.MustCompile(`(?i)@section\s*\(\s*["']([^"']+)["']\s*\)`)
	bladeVoltPattern              = regexp.MustCompile(`(?i)@volt\s*\(\s*["']([^"']+)["']\s*\)`)
	bladeDependencyPattern        = regexp.MustCompile(`(?i)@(extends|include)\s*\(\s*["']([^"']+)["']\s*\)`)
	templateScopeDirectivePattern = regexp.MustCompile(`(?is)^\{%[-+]?\s*(block|macro|endblock|endmacro)\b(?:\s+([A-Za-z_][A-Za-z0-9_-]*))?`)
	templateDependencyPattern     = regexp.MustCompile(`(?i)\{%[-+]?\s*(extends|include|import|from)\s+["']([^"']+)["']`)
	jspIncludeDirectivePattern    = regexp.MustCompile(`(?is)<%@\s*include\b[^%>]*\bfile\s*=\s*["']([^"']+)["'][^%>]*%>`)
	sourceAttributePattern        = regexp.MustCompile(`(?i)\bsrc\s*=\s*["']([^"']+)["']`)
)

func (VueAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeScriptStyleHost(ctx, document, options, "vue", AnalyzerVue, false)
}

func (SvelteAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeScriptStyleHost(ctx, document, options, "svelte", AnalyzerSvelte, false)
}

func (AstroAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeScriptStyleHost(ctx, document, options, "astro", AnalyzerAstro, true)
}

func (PHPHTMLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	probe := maskHostComments(document.Text)
	regions, complete, err := phpRegions(ctx, probe)
	if err != nil {
		return AnalyzerResult{}, err
	}
	result, err := analyzePHPHTMLHost(ctx, document, options, regions)
	if err != nil {
		return AnalyzerResult{}, err
	}
	if !complete {
		markCompositePartial(&result.Analysis, options.Limits, false, "php-html-unterminated-region", "PHP region is not terminated")
	}
	return result, nil
}

func phpRegions(ctx context.Context, probe string) ([]compositeEmbeddedRegion, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	regions := make([]compositeEmbeddedRegion, 0, 8)
	for search := 0; search < len(probe); {
		if err := ctx.Err(); err != nil {
			return nil, false, operation.Wrap(operation.KindCancelled, "segment_php_html_source", "", err)
		}
		opening := phpOpenPattern.FindStringIndex(probe[search:])
		if opening == nil {
			break
		}
		fullStart := search + opening[0]
		contentStart := search + opening[1]
		closeStart, found, err := phpClosingTag(ctx, probe, contentStart)
		if err != nil {
			return nil, false, err
		}
		fullEnd := len(probe)
		contentEnd := len(probe)
		if found {
			fullEnd = closeStart + len("?>")
			contentEnd = closeStart
		}
		regions = append(regions, compositeEmbeddedRegion{
			kind:     "php",
			language: "php",
			full:     OffsetRange{Start: fullStart, End: fullEnd},
			content:  OffsetRange{Start: contentStart, End: contentEnd},
		})
		if !found {
			break
		}
		search = fullEnd
	}
	return regions, true, nil
}

func phpClosingTag(ctx context.Context, text string, start int) (int, bool, error) {
	nextContextCheck := start
	for at := start; at < len(text); {
		if err := checkPHPContext(ctx, at, &nextContextCheck); err != nil {
			return 0, false, err
		}
		if strings.HasPrefix(text[at:], "?>") {
			return at, true, nil
		}
		if strings.HasPrefix(text[at:], "/*") {
			at += 2
			closed := false
			for at < len(text) {
				if err := checkPHPContext(ctx, at, &nextContextCheck); err != nil {
					return 0, false, err
				}
				if strings.HasPrefix(text[at:], "*/") {
					at += 2
					closed = true
					break
				}
				at++
			}
			if !closed {
				return len(text), false, nil
			}
			continue
		}
		if strings.HasPrefix(text[at:], "//") || text[at] == '#' && !strings.HasPrefix(text[at:], "#[") {
			for at < len(text) && text[at] != '\r' && text[at] != '\n' {
				if err := checkPHPContext(ctx, at, &nextContextCheck); err != nil {
					return 0, false, err
				}
				if strings.HasPrefix(text[at:], "?>") {
					return at, true, nil
				}
				at++
			}
			continue
		}
		if text[at] == '\'' || text[at] == '"' || text[at] == '`' {
			quote := text[at]
			at++
			for at < len(text) {
				if err := checkPHPContext(ctx, at, &nextContextCheck); err != nil {
					return 0, false, err
				}
				if text[at] == '\\' {
					at += min(2, len(text)-at)
					continue
				}
				if text[at] == quote {
					at++
					break
				}
				at++
			}
			continue
		}
		if strings.HasPrefix(text[at:], "<<<") {
			end, recognized, err := phpHeredocEnd(ctx, text, at)
			if err != nil {
				return 0, false, err
			}
			if recognized {
				at = end
				continue
			}
		}
		at++
	}
	return len(text), false, nil
}

func checkPHPContext(ctx context.Context, at int, next *int) error {
	if at < *next {
		return nil
	}
	*next = at + 4096
	if err := ctx.Err(); err != nil {
		return operation.Wrap(operation.KindCancelled, "segment_php_html_source", "", err)
	}
	return nil
}

func phpHeredocEnd(ctx context.Context, text string, open int) (int, bool, error) {
	cursor := open + len("<<<")
	for cursor < len(text) && (text[cursor] == ' ' || text[cursor] == '\t') {
		cursor++
	}
	quote := byte(0)
	if cursor < len(text) && (text[cursor] == '\'' || text[cursor] == '"') {
		quote = text[cursor]
		cursor++
	}
	nameStart := cursor
	for cursor < len(text) && (text[cursor] == '_' || text[cursor] >= 'A' && text[cursor] <= 'Z' || text[cursor] >= 'a' && text[cursor] <= 'z' || cursor > nameStart && text[cursor] >= '0' && text[cursor] <= '9') {
		cursor++
	}
	if cursor == nameStart {
		return open, false, nil
	}
	name := text[nameStart:cursor]
	if quote != 0 {
		if cursor >= len(text) || text[cursor] != quote {
			return open, false, nil
		}
		cursor++
	}
	for cursor < len(text) && text[cursor] != '\r' && text[cursor] != '\n' {
		if text[cursor] != ' ' && text[cursor] != '\t' {
			return open, false, nil
		}
		cursor++
	}
	if cursor >= len(text) {
		return len(text), true, nil
	}
	if text[cursor] == '\r' && cursor+1 < len(text) && text[cursor+1] == '\n' {
		cursor += 2
	} else {
		cursor++
	}
	nextContextCheck := cursor
	for lineStart := cursor; lineStart <= len(text); {
		if err := checkPHPContext(ctx, lineStart, &nextContextCheck); err != nil {
			return 0, false, err
		}
		lineEnd := lineStart
		for lineEnd < len(text) && text[lineEnd] != '\r' && text[lineEnd] != '\n' {
			if err := checkPHPContext(ctx, lineEnd, &nextContextCheck); err != nil {
				return 0, false, err
			}
			lineEnd++
		}
		markerEnd, found, valid := phpHeredocClosingMarker(text, lineStart, lineEnd, name)
		if found && valid {
			return markerEnd, true, nil
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
	return len(text), true, nil
}

func analyzePHPHTMLHost(ctx context.Context, document *SourceDocument, options AnalyzeOptions, regions []compositeEmbeddedRegion) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_php_html_source", document.Path, err)
	}
	allRegions := regions
	retainedRegions, regionsTruncated := capEmbeddedRegions(regions, options.Limits.MaxSymbols)
	maskedHost, err := maskCompositeRanges(document.Text, embeddedRegionFullRanges(allRegions))
	if err != nil {
		return AnalyzerResult{}, err
	}
	result := AnalyzerResult{Analysis: AnalysisResult{CoverageComplete: true}}
	host, err := analyzeMaskedEmbeddedRegion(ctx, document, maskedHost, options, "php-html", AnalyzerPHPHTML, "host-html", "html")
	if err != nil {
		return AnalyzerResult{}, err
	}
	mergeCompositeAnalyzerResult(&result, host, options.Limits)

	phpRanges := make([]OffsetRange, 0, len(retainedRegions))
	for index, region := range retainedRegions {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_php_html_source", document.Path, err)
		}
		public, rangeErr := sourceRegionForOffsets(document, fmt.Sprintf("%s-%d", region.kind, index+1), region.kind, region.language, region.full.Start, region.full.End, true)
		if rangeErr != nil {
			return AnalyzerResult{}, rangeErr
		}
		result.Regions = append(result.Regions, public)
		phpRanges = append(phpRanges, region.content)
	}
	if len(phpRanges) > 0 {
		projection, projectionErr := MaskOutsideRanges(document.Text, phpRanges)
		if projectionErr != nil {
			return AnalyzerResult{}, projectionErr
		}
		embedded, analyzeErr := analyzeMaskedEmbeddedRegion(ctx, document, projection, options, "php-html", AnalyzerPHPHTML, "", "php")
		if analyzeErr != nil {
			return AnalyzerResult{}, analyzeErr
		}
		assignEmbeddedRegionIDs(embedded.Analysis.Symbols, retainedRegions)
		mergeCompositeAnalyzerResult(&result, embedded, options.Limits)
	}
	if regionsTruncated {
		markCompositePartial(&result.Analysis, options.Limits, true, "php-html-region-limit", "PHP region retention limit reached")
	}
	return result, nil
}

func (JSPAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	probe := maskHostComments(document.Text)
	regions := regexEmbeddedRegions(probe, jspBlockPattern, "jsp-java", "java")
	complete := embeddedOpeningsCovered(probe, regions, percentOpenPattern)
	result, err := analyzeJSPHost(ctx, document, options, regions)
	if err != nil {
		return AnalyzerResult{}, err
	}
	appendCompositeDependencies(&result, jspDependencies(document, probe), options.Limits)
	if !complete {
		markCompositePartial(&result.Analysis, options.Limits, false, "jsp-unterminated-region", "JSP region is not terminated")
	}
	return result, nil
}

func analyzeJSPHost(ctx context.Context, document *SourceDocument, options AnalyzeOptions, regions []compositeEmbeddedRegion) (AnalyzerResult, error) {
	allRegions := regions
	regions, regionsTruncated := capEmbeddedRegions(regions, options.Limits.MaxSymbols)
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_jsp_source", document.Path, err)
	}

	maskedHost, err := maskCompositeRanges(document.Text, embeddedRegionFullRanges(allRegions))
	if err != nil {
		return AnalyzerResult{}, err
	}
	result := AnalyzerResult{Analysis: AnalysisResult{CoverageComplete: true}}
	host, err := analyzeMaskedEmbeddedRegion(ctx, document, maskedHost, options, "jsp", AnalyzerJSP, "host-html", "html")
	if err != nil {
		return AnalyzerResult{}, err
	}
	mergeCompositeAnalyzerResult(&result, host, options.Limits)

	javaRanges := make([]OffsetRange, 0, len(regions))
	for index, region := range regions {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_jsp_source", document.Path, err)
		}
		regionID := fmt.Sprintf("%s-%d", region.kind, index+1)
		rangeValue, rangeErr := document.RangeFromUTF8Offsets(region.full.Start, region.full.End)
		if rangeErr != nil {
			return AnalyzerResult{}, rangeErr
		}
		result.Regions = append(result.Regions, SourceRegion{ID: regionID, Kind: region.kind, Language: region.language, Range: rangeValue, Evidence: SymbolEvidenceStructural, Supported: true})
		if jspJavaRegion(document.Text, region) {
			javaRanges = append(javaRanges, region.content)
		}
	}

	if len(javaRanges) > 0 {
		maskedJava, maskErr := MaskOutsideRanges(document.Text, javaRanges)
		if maskErr != nil {
			return AnalyzerResult{}, maskErr
		}
		embedded, analyzeErr := analyzeMaskedEmbeddedRegion(ctx, document, maskedJava, options, "jsp", AnalyzerJSP, "", "java")
		if analyzeErr != nil {
			return AnalyzerResult{}, analyzeErr
		}
		assignEmbeddedRegionIDs(embedded.Analysis.Symbols, regions)
		mergeCompositeAnalyzerResult(&result, embedded, options.Limits)
	}
	if regionsTruncated {
		markCompositePartial(&result.Analysis, options.Limits, true, "jsp-region-limit", "composite region retention limit reached")
	}
	return result, nil
}

func jspJavaRegion(text string, region compositeEmbeddedRegion) bool {
	marker := region.full.Start + len("<%")
	if marker >= len(text) || marker >= region.full.End {
		return false
	}
	return text[marker] != '@' && !strings.HasPrefix(text[marker:], "--")
}

func assignEmbeddedRegionIDs(symbols []NormalizedSymbol, regions []compositeEmbeddedRegion) {
	for index := range symbols {
		offset := symbols[index].declarationOffsets.Start
		for regionIndex, region := range regions {
			if offset < region.content.Start || offset >= region.content.End {
				continue
			}
			symbols[index].RegionID = fmt.Sprintf("%s-%d", region.kind, regionIndex+1)
			break
		}
	}
}

func (EJSAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	probe := maskHostComments(document.Text)
	regions := regexEmbeddedRegions(probe, ejsBlockPattern, "ejs-js", "javascript")
	complete := embeddedOpeningsCovered(probe, regions, percentOpenPattern)
	result, err := analyzeEJSHost(ctx, document, options, regions)
	if err != nil {
		return AnalyzerResult{}, err
	}
	retainedRegions, _ := capEmbeddedRegions(regions, options.Limits.MaxSymbols)
	dependencies, err := ejsDependencies(ctx, document, retainedRegions, options.MaxNesting)
	if err != nil {
		return AnalyzerResult{}, err
	}
	appendCompositeDependencies(&result, dependencies, options.Limits)
	if !complete {
		markCompositePartial(&result.Analysis, options.Limits, false, "ejs-unterminated-region", "EJS region is not terminated")
	}
	return result, nil
}

func analyzeEJSHost(ctx context.Context, document *SourceDocument, options AnalyzeOptions, regions []compositeEmbeddedRegion) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	allRegions := regions
	retainedRegions, regionsTruncated := capEmbeddedRegions(regions, options.Limits.MaxSymbols)
	maskedHost, err := maskCompositeRanges(document.Text, embeddedRegionFullRanges(allRegions))
	if err != nil {
		return AnalyzerResult{}, err
	}
	result := AnalyzerResult{Analysis: AnalysisResult{CoverageComplete: true}}
	host, err := analyzeMaskedEmbeddedRegion(ctx, document, maskedHost, options, "ejs", AnalyzerEJS, "host-html", "html")
	if err != nil {
		return AnalyzerResult{}, err
	}
	mergeCompositeAnalyzerResult(&result, host, options.Limits)

	for index, region := range retainedRegions {
		public, rangeErr := sourceRegionForOffsets(document, fmt.Sprintf("ejs-js-%d", index+1), region.kind, region.language, region.full.Start, region.full.End, true)
		if rangeErr != nil {
			return AnalyzerResult{}, rangeErr
		}
		result.Regions = append(result.Regions, public)
	}

	executable := make([]OffsetRange, 0, len(allRegions))
	for _, region := range allRegions {
		if ejsRegionExecutable(document.Text, region) {
			executable = append(executable, region.content)
		}
	}
	if len(executable) > 0 {
		projection, projectionErr := MaskOutsideRanges(document.Text, executable)
		if projectionErr != nil {
			return AnalyzerResult{}, projectionErr
		}
		embedded, analyzeErr := analyzeMaskedEmbeddedRegion(ctx, document, projection, options, "ejs", AnalyzerEJS, "ejs-scriptlets", "javascript")
		if analyzeErr != nil {
			return AnalyzerResult{}, analyzeErr
		}
		mergeCompositeAnalyzerResult(&result, embedded, options.Limits)
	}
	if regionsTruncated {
		markCompositePartial(&result.Analysis, options.Limits, true, "ejs-region-limit", "EJS region retention limit reached")
	}
	return result, nil
}

func ejsRegionExecutable(text string, region compositeEmbeddedRegion) bool {
	if region.full.Start < 0 || region.full.Start >= len(text) || region.content.Start < region.full.Start || region.content.Start > len(text) {
		return false
	}
	tail := text[region.full.Start:]
	return !strings.HasPrefix(tail, "<%#") && !strings.HasPrefix(tail, "<%%")
}

func (BladeAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	probe := maskDelimitedSourceRegions(document.Text, [][2]string{{"<!--", "-->"}, {"{{--", "--}}"}})
	regions, complete := bladePHPRegions(probe)
	hostProbe, err := maskCompositeRanges(probe, embeddedRegionFullRanges(regions))
	if err != nil {
		return AnalyzerResult{}, err
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: "blade", Analyzer: string(AnalyzerBlade), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	voltComponents, err := addBladeDeclarations(builder, document, hostProbe)
	if err != nil {
		return AnalyzerResult{}, err
	}
	result, err := analyzeBladeHost(ctx, document, options, regions)
	if err != nil {
		return AnalyzerResult{}, err
	}
	result.Analysis = mergeCompositeAnalysis(result.Analysis, builder.Result(), options.Limits)
	remaining := max(0, options.Limits.MaxSymbols-len(result.Analysis.Symbols))
	if len(voltComponents) > 0 {
		voltRegions, _ := capEmbeddedRegions(regions, options.Limits.MaxSymbols)
		voltOptions := options
		voltOptions.Limits.MaxSymbols = max(1, remaining)
		volt, voltErr := analyzeBladeVoltAnonymousClasses(ctx, document, voltOptions, voltRegions, voltComponents)
		if voltErr != nil {
			return AnalyzerResult{}, voltErr
		}
		mergeCompositeAnalyzerResult(&result, volt, options.Limits)
	}
	appendCompositeDependencies(&result, bladeDependencies(document, hostProbe), options.Limits)
	if !complete {
		markCompositePartial(&result.Analysis, options.Limits, false, "blade-unterminated-region", "Blade PHP region is not terminated")
	}
	return result, nil
}

func analyzeBladeHost(ctx context.Context, document *SourceDocument, options AnalyzeOptions, regions []compositeEmbeddedRegion) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	allRegions := regions
	retainedRegions, regionsTruncated := capEmbeddedRegions(regions, options.Limits.MaxSymbols)
	maskedHost, err := maskCompositeRanges(document.Text, embeddedRegionFullRanges(allRegions))
	if err != nil {
		return AnalyzerResult{}, err
	}
	result := AnalyzerResult{Analysis: AnalysisResult{CoverageComplete: true}}
	host, err := analyzeMaskedEmbeddedRegion(ctx, document, maskedHost, options, "blade", AnalyzerBlade, "host-html", "html")
	if err != nil {
		return AnalyzerResult{}, err
	}
	mergeCompositeAnalyzerResult(&result, host, options.Limits)

	phpRanges := make([]OffsetRange, 0, len(retainedRegions))
	for index, region := range retainedRegions {
		public, rangeErr := sourceRegionForOffsets(document, fmt.Sprintf("%s-%d", region.kind, index+1), region.kind, region.language, region.full.Start, region.full.End, true)
		if rangeErr != nil {
			return AnalyzerResult{}, rangeErr
		}
		result.Regions = append(result.Regions, public)
		phpRanges = append(phpRanges, region.content)
	}
	if len(phpRanges) > 0 {
		projection, projectionErr := MaskOutsideRanges(document.Text, phpRanges)
		if projectionErr != nil {
			return AnalyzerResult{}, projectionErr
		}
		embedded, analyzeErr := analyzeMaskedEmbeddedRegion(ctx, document, projection, options, "blade", AnalyzerBlade, "blade-php-blocks", "php")
		if analyzeErr != nil {
			return AnalyzerResult{}, analyzeErr
		}
		assignEmbeddedRegionIDs(embedded.Analysis.Symbols, retainedRegions)
		mergeCompositeAnalyzerResult(&result, embedded, options.Limits)
	}
	if regionsTruncated {
		markCompositePartial(&result.Analysis, options.Limits, true, "blade-region-limit", "Blade PHP region retention limit reached")
	}
	return result, nil
}

func (JinjaAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeTemplateHost(ctx, document, options, "jinja", AnalyzerJinja)
}

func (TwigAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeTemplateHost(ctx, document, options, "twig", AnalyzerTwig)
}

func analyzeScriptStyleHost(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID, astro bool) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	regions := make([]compositeEmbeddedRegion, 0, 8)
	probe := maskDelimitedSourceRegions(document.Text, [][2]string{{"<!--", "-->"}})
	if astro {
		if frontmatter, ok := astroFrontmatter(document.Text); ok {
			regions = append(regions, compositeEmbeddedRegion{kind: "frontmatter", language: "typescript", full: frontmatter.Full, content: frontmatter.Content})
			var err error
			probe, err = maskCompositeRanges(probe, []OffsetRange{frontmatter.Full})
			if err != nil {
				return AnalyzerResult{}, err
			}
		}
	}
	regions = append(regions, tagEmbeddedRegions(probe, scriptTagPattern, "script", scriptLanguage)...)
	regions = append(regions, tagEmbeddedRegions(probe, styleTagPattern, "style", styleLanguage)...)
	regions = orderedNonOverlappingEmbeddedRegions(regions)
	complete := embeddedOpeningsCovered(probe, regions, scriptOpenPattern) && embeddedOpeningsCovered(probe, regions, styleOpenPattern)
	result, err := analyzeDelimitedCompositeHost(ctx, document, options, language, analyzer, regions)
	if err != nil {
		return AnalyzerResult{}, err
	}
	retainedRegions, _ := capEmbeddedRegions(regions, options.Limits.MaxSymbols)
	appendCompositeDependencies(&result, scriptSourceDependencies(document, retainedRegions), options.Limits)
	if !complete {
		markCompositePartial(&result.Analysis, options.Limits, false, language+"-unterminated-region", "script/style region is not terminated")
	}
	return result, nil
}

func analyzeDelimitedCompositeHost(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID, regions []compositeEmbeddedRegion) (AnalyzerResult, error) {
	allRegions := regions
	regions, regionsTruncated := capEmbeddedRegions(regions, options.Limits.MaxSymbols)
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_composite_source", document.Path, err)
	}
	maskedHost, err := maskCompositeRanges(document.Text, embeddedRegionFullRanges(allRegions))
	if err != nil {
		return AnalyzerResult{}, err
	}
	result := AnalyzerResult{Analysis: AnalysisResult{CoverageComplete: true}}
	hostAnalysis, err := analyzeMaskedEmbeddedRegion(ctx, document, maskedHost, options, language, analyzer, "host-html", "html")
	if err != nil {
		return AnalyzerResult{}, err
	}
	mergeCompositeAnalyzerResult(&result, hostAnalysis, options.Limits)

	for index, region := range regions {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_composite_source", document.Path, err)
		}
		regionID := fmt.Sprintf("%s-%d", region.kind, index+1)
		rangeValue, rangeErr := document.RangeFromUTF8Offsets(region.full.Start, region.full.End)
		if rangeErr != nil {
			return AnalyzerResult{}, rangeErr
		}
		supported := embeddedLanguageSupported(region.language)
		result.Regions = append(result.Regions, SourceRegion{ID: regionID, Kind: region.kind, Language: region.language, Range: rangeValue, Evidence: SymbolEvidenceStructural, Supported: supported})
		if !supported {
			result.Analysis.CoverageComplete = false
			continue
		}
		masked, maskErr := MaskOutsideRanges(document.Text, []OffsetRange{region.content})
		if maskErr != nil {
			return AnalyzerResult{}, maskErr
		}
		embedded, analyzeErr := analyzeMaskedEmbeddedRegion(ctx, document, masked, options, language, analyzer, regionID, region.language)
		if analyzeErr != nil {
			return AnalyzerResult{}, analyzeErr
		}
		mergeCompositeAnalyzerResult(&result, embedded, options.Limits)
	}
	if regionsTruncated {
		markCompositePartial(&result.Analysis, options.Limits, true, language+"-region-limit", "composite region retention limit reached")
	}
	return result, nil
}

func analyzeTemplateHost(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	masked, ranges, complete := maskTemplateSyntax(document.Text)
	declarationProbe := maskDelimitedSourceRegions(document.Text, [][2]string{{"<!--", "-->"}, {"{#", "#}"}, {"{{", "}}"}})
	result, err := analyzeMaskedEmbeddedRegion(ctx, document, masked, options, language, analyzer, "host-html", "html")
	if err != nil {
		return AnalyzerResult{}, err
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{Context: ctx, Language: language, Analyzer: string(analyzer), IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	if err := addTemplateDeclarations(builder, document, ranges, language, options.MaxNesting); err != nil {
		return AnalyzerResult{}, err
	}
	result.Analysis = mergeCompositeAnalysis(result.Analysis, builder.Result(), options.Limits)
	appendCompositeDependencies(&result, templateDependencies(document, declarationProbe), options.Limits)
	retainedRanges, rangesTruncated := capCompositeOffsetRanges(ranges, options.Limits.MaxSymbols)
	for index, value := range retainedRanges {
		rangeValue, rangeErr := document.RangeFromUTF8Offsets(value.Start, value.End)
		if rangeErr == nil {
			result.Regions = append(result.Regions, SourceRegion{ID: fmt.Sprintf("template-%d", index+1), Kind: "template", Language: language, Range: rangeValue, Evidence: SymbolEvidenceStructural, Supported: true})
		}
	}
	if rangesTruncated {
		markCompositePartial(&result.Analysis, options.Limits, true, language+"-region-limit", "template region retention limit reached")
	}
	if !complete {
		markCompositePartial(&result.Analysis, options.Limits, false, language+"-unterminated-region", "template region is not terminated")
	}
	return result, nil
}

func addTemplateDeclarations(builder *SymbolBuilder, document *SourceDocument, ranges []OffsetRange, language string, maxNesting int) error {
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scopes := make([]templateScope, 0, min(8, maxNesting))
	for _, value := range ranges {
		if value.Start < 0 || value.End <= value.Start || value.End > len(document.Text) {
			continue
		}
		statement := document.Text[value.Start:value.End]
		if !strings.HasPrefix(statement, "{%") {
			continue
		}
		match := templateScopeDirectivePattern.FindStringSubmatchIndex(statement)
		if len(match) < 4 || match[2] < 0 {
			continue
		}
		directive := strings.ToLower(statement[match[2]:match[3]])
		switch directive {
		case "block", "macro":
			if len(match) < 6 || match[4] < 0 {
				continue
			}
			if len(scopes) >= maxNesting {
				rangeValue := value
				_ = builder.AddDiagnostic(DiagnosticSpec{Code: language + "-template-scope-limit", Message: "template declaration nesting exceeds the configured limit", Severity: DiagnosticWarning, Range: &rangeValue, AffectsCoverage: true})
				return nil
			}
			nameStart := value.Start + match[4]
			nameEnd := value.Start + match[5]
			name := document.Text[nameStart:nameEnd]
			kind := SymbolKindSection
			if directive == "macro" {
				kind = SymbolKindFunction
			}
			var parent *SymbolParent
			if len(scopes) > 0 {
				value := scopes[len(scopes)-1].parent
				parent = &value
			}
			declaration := OffsetRange{Start: value.Start, End: nameEnd}
			symbol, err := builder.Add(SymbolSpec{Kind: kind, NativeKind: directive, Name: name, Parent: parent, Declaration: declaration, NameRange: OffsetRange{Start: nameStart, End: nameEnd}, Evidence: SymbolEvidenceStructural})
			if err != nil {
				if operation.KindOf(err) == operation.KindLimit {
					return nil
				}
				return err
			}
			scopes = append(scopes, templateScope{kind: directive, name: name, parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}, declaration: declaration})
		case "endblock", "endmacro":
			expected := strings.TrimPrefix(directive, "end")
			if len(scopes) == 0 {
				rangeValue := value
				_ = builder.AddDiagnostic(DiagnosticSpec{Code: language + "-unmatched-declaration-scope", Message: "template declaration closer has no matching opener", Severity: DiagnosticWarning, Range: &rangeValue, AffectsCoverage: true})
				continue
			}
			top := scopes[len(scopes)-1]
			if top.kind != expected {
				rangeValue := value
				_ = builder.AddDiagnostic(DiagnosticSpec{Code: language + "-mismatched-declaration-scope", Message: "template declaration closer does not match the current declaration scope", Severity: DiagnosticWarning, Range: &rangeValue, AffectsCoverage: true})
				continue
			}
			if expected == "block" && len(match) >= 6 && match[4] >= 0 {
				closingName := statement[match[4]:match[5]]
				if closingName != top.name {
					rangeValue := value
					_ = builder.AddDiagnostic(DiagnosticSpec{Code: language + "-mismatched-declaration-name", Message: "named template block closer does not match the current block", Severity: DiagnosticWarning, Range: &rangeValue, AffectsCoverage: true})
					continue
				}
			}
			scopes = scopes[:len(scopes)-1]
		}
	}
	if len(scopes) > 0 {
		rangeValue := scopes[len(scopes)-1].declaration
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: language + "-unterminated-declaration-scope", Message: "template declaration scope is not terminated", Severity: DiagnosticWarning, Range: &rangeValue, AffectsCoverage: true})
	}
	return nil
}

func analyzeMaskedEmbeddedRegion(ctx context.Context, host *SourceDocument, masked string, options AnalyzeOptions, _ string, analyzer AnalyzerID, regionID, embeddedLanguage string) (AnalyzerResult, error) {
	registry, err := DefaultLanguageRegistry()
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

func mergeInitialCompositeSymbols(dst *AnalysisResult, symbols []NormalizedSymbol, maxSymbols int) {
	seen := make(map[string]bool)
	retained := 0
	for _, symbol := range symbols {
		if _, exists := seen[symbol.ID]; exists {
			continue
		}
		if retained >= maxSymbols {
			dst.Truncated = true
			dst.CoverageComplete = false
			break
		}
		seen[symbol.ID] = false
		retained++
	}
	if retained == 0 {
		return
	}
	dst.Symbols = make([]NormalizedSymbol, 0, retained)
	for _, symbol := range symbols {
		emitted, exists := seen[symbol.ID]
		if !exists || emitted {
			continue
		}
		dst.Symbols = append(dst.Symbols, symbol)
		seen[symbol.ID] = true
	}
}

func mergeCompositeAnalysis(dst, src AnalysisResult, limits SymbolBuilderLimits) AnalysisResult {
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
	if dst.Symbols == nil {
		mergeInitialCompositeSymbols(&dst, src.Symbols, limits.MaxSymbols)
	} else {
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
	}
	sort.SliceStable(dst.Symbols, func(i, j int) bool {
		left := dst.Symbols[i]
		right := dst.Symbols[j]
		if left.declarationOffsets.Start != right.declarationOffsets.Start {
			return left.declarationOffsets.Start < right.declarationOffsets.Start
		}
		if left.declarationOffsets.End != right.declarationOffsets.End {
			return left.declarationOffsets.End < right.declarationOffsets.End
		}
		return left.ID < right.ID
	})
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

func mergeCompositeAnalyzerResult(dst *AnalyzerResult, src AnalyzerResult, limits SymbolBuilderLimits) {
	dst.Analysis = mergeCompositeAnalysis(dst.Analysis, src.Analysis, limits)
	appendCompositeDependencies(dst, src.Dependencies, limits)
	appendCompositeRelations(dst, src.Relations, limits)
}

func embeddedLanguageSupported(language string) bool {
	registry, err := DefaultLanguageRegistry()
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

func tagEmbeddedRegions(text string, pattern *regexp.Regexp, kind string, language func(string) string) []compositeEmbeddedRegion {
	regions := make([]compositeEmbeddedRegion, 0, 8)
	for search := 0; search < len(text); {
		match := pattern.FindStringSubmatchIndex(text[search:])
		if match == nil {
			break
		}
		start := search + match[0]
		end := search + match[1]
		contentStart := search + match[4]
		if selfClosingTagOpening(text[start:contentStart]) {
			search = contentStart
			continue
		}
		attrs := ""
		if match[2] >= 0 {
			attrs = text[search+match[2] : search+match[3]]
		}
		regions = append(regions, compositeEmbeddedRegion{kind: kind, language: language(attrs), full: OffsetRange{Start: start, End: end}, content: OffsetRange{Start: contentStart, End: search + match[5]}})
		search = end
	}
	return regions
}

func selfClosingTagOpening(opening string) bool {
	close := strings.LastIndexByte(opening, '>')
	return close >= 0 && strings.HasSuffix(strings.TrimSpace(opening[:close]), "/")
}

func addBladeDeclarations(builder *SymbolBuilder, document *SourceDocument, probe string) ([]bladeVoltComponent, error) {
	declarations := make([]bladeDeclaration, 0, 8)
	for _, match := range bladeSectionPattern.FindAllStringSubmatchIndex(probe, -1) {
		declarations = append(declarations, bladeDeclaration{
			kind: SymbolKindSection, nativeKind: "section", name: document.Text[match[2]:match[3]],
			declaration: OffsetRange{Start: match[0], End: match[1]}, nameRange: OffsetRange{Start: match[2], End: match[3]},
		})
	}
	for _, match := range bladeVoltPattern.FindAllStringSubmatchIndex(probe, -1) {
		if match[0] > 0 && probe[match[0]-1] == '@' {
			continue
		}
		declarations = append(declarations, bladeDeclaration{
			kind: SymbolKindEntity, nativeKind: "volt-component", name: document.Text[match[2]:match[3]], volt: true,
			declaration: OffsetRange{Start: match[0], End: match[1]}, nameRange: OffsetRange{Start: match[2], End: match[3]},
		})
	}
	sort.SliceStable(declarations, func(i, j int) bool { return declarations[i].declaration.Start < declarations[j].declaration.Start })
	components := make([]bladeVoltComponent, 0, len(declarations))
	for _, declaration := range declarations {
		symbol, err := builder.Add(SymbolSpec{
			Kind: declaration.kind, NativeKind: declaration.nativeKind, Name: declaration.name,
			Declaration: declaration.declaration, NameRange: declaration.nameRange, Evidence: SymbolEvidenceStructural,
		})
		if operation.KindOf(err) == operation.KindLimit {
			break
		}
		if err != nil {
			return nil, err
		}
		if declaration.volt {
			components = append(components, bladeVoltComponent{symbol: symbol, declaration: declaration.declaration})
		}
	}
	return components, nil
}

func analyzeBladeVoltAnonymousClasses(ctx context.Context, document *SourceDocument, options AnalyzeOptions, regions []compositeEmbeddedRegion, components []bladeVoltComponent) (AnalyzerResult, error) {
	result := AnalyzerResult{Analysis: AnalysisResult{CoverageComplete: true}}
	retainedRegions := regions
	if len(retainedRegions) == 0 || len(components) == 0 {
		return result, nil
	}
	projection, err := MaskOutsideRanges(document.Text, func() []OffsetRange {
		ranges := make([]OffsetRange, 0, len(retainedRegions))
		for _, region := range retainedRegions {
			ranges = append(ranges, region.content)
		}
		return ranges
	}())
	if err != nil {
		return AnalyzerResult{}, err
	}
	maskedProjection, _, err := maskPHPHeredocs(ctx, projection)
	if err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_blade_volt_source", document.Path, err)
	}
	clone := *document
	clone.Text = maskedProjection
	clone.lineStarts = buildLineStarts(maskedProjection)
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, &clone, PHPScannerProfile(), ScannerLimits{MaxTokens: scannerTokenBudget(maskedProjection), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalyzerResult{}, err
	}
	pairs := PairDelimiterTokens(scan.Tokens, nil)
	classes := bladeAnonymousClasses(scan.Tokens, pairs, retainedRegions)
	componentIndex := 0
	remaining := max(1, options.Limits.MaxSymbols)
	for _, class := range classes {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_blade_volt_source", document.Path, err)
		}
		classEnd := scan.Tokens[class.closeIndex].EndOffset
		for componentIndex < len(components) && components[componentIndex].declaration.Start < classEnd {
			componentIndex++
		}
		if componentIndex >= len(components) {
			break
		}
		associationEnd := len(document.Text)
		if class.regionIndex+1 < len(regions) {
			associationEnd = regions[class.regionIndex+1].full.Start
		}
		component := components[componentIndex]
		if component.declaration.Start >= associationEnd || !bladeAnonymousClassExtendsComponent(scan.Tokens, class.classIndex, class.openIndex) {
			continue
		}
		if remaining <= 0 {
			markCompositePartial(&result.Analysis, options.Limits, true, "blade-volt-symbol-limit", "Blade Volt member retention limit reached")
			break
		}
		regionID := fmt.Sprintf("%s-%d", retainedRegions[class.regionIndex].kind, class.regionIndex+1)
		classOptions := options
		classOptions.Limits.MaxSymbols = remaining
		memberBuilder := NewSymbolBuilder(document, SymbolBuilderOptions{
			Context: ctx, Language: "php", Analyzer: string(AnalyzerBlade), RegionID: regionID,
			IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: classOptions.Limits,
		})
		if err := memberBuilder.checkReady(); err != nil {
			return AnalyzerResult{}, err
		}
		parser := &phpParser{ctx: ctx, document: document, tokens: scan.Tokens, pairs: pairs, builder: memberBuilder}
		parent := &SymbolParent{ID: component.symbol.ID, QualifiedName: component.symbol.QualifiedName}
		parser.collectTypeRelations(component.symbol.QualifiedName, class.classIndex+1, class.openIndex, scan.Tokens[class.classIndex].Nesting, "class")
		parser.parseScope(class.openIndex+1, class.closeIndex, parent, true, bladeVoltOwner(component.symbol.Name))
		mergeCompositeAnalyzerResult(&result, AnalyzerResult{Analysis: memberBuilder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, options.Limits)
		remaining = max(0, options.Limits.MaxSymbols-len(result.Analysis.Symbols))
		componentIndex++
	}
	return result, nil
}

func bladeAnonymousClasses(tokens []Token, pairs map[int]int, regions []compositeEmbeddedRegion) []bladeAnonymousClass {
	classes := make([]bladeAnonymousClass, 0, 4)
	for index := 0; index < len(tokens); index++ {
		if tokens[index].Nesting != 0 || !strings.EqualFold(tokens[index].Text, "new") {
			continue
		}
		classIndex := -1
		for cursor := index + 1; cursor < len(tokens); cursor++ {
			if tokens[cursor].Kind == TokenEOF || tokens[cursor].Nesting < tokens[index].Nesting {
				break
			}
			if tokens[cursor].Nesting == tokens[index].Nesting && tokens[cursor].Text == ";" {
				break
			}
			if tokens[cursor].Nesting == tokens[index].Nesting && strings.EqualFold(tokens[cursor].Text, "class") {
				classIndex = cursor
				break
			}
		}
		if classIndex < 0 {
			continue
		}
		openIndex := -1
		for cursor := classIndex + 1; cursor < len(tokens); cursor++ {
			if tokens[cursor].Text == "{" && tokens[cursor].Nesting == tokens[classIndex].Nesting+1 {
				openIndex = cursor
				break
			}
			if tokens[cursor].Kind == TokenEOF || tokens[cursor].Text == ";" && tokens[cursor].Nesting == tokens[classIndex].Nesting {
				break
			}
		}
		if openIndex < 0 {
			continue
		}
		closeIndex := pairs[openIndex]
		if closeIndex <= openIndex || closeIndex >= len(tokens) {
			continue
		}
		regionIndex := -1
		for candidate, region := range regions {
			if tokens[index].StartOffset >= region.content.Start && tokens[index].StartOffset < region.content.End {
				regionIndex = candidate
				break
			}
		}
		if regionIndex < 0 {
			continue
		}
		classes = append(classes, bladeAnonymousClass{classIndex: classIndex, openIndex: openIndex, closeIndex: closeIndex, regionIndex: regionIndex})
		index = closeIndex
	}
	return classes
}

func bladeAnonymousClassExtendsComponent(tokens []Token, classIndex, openIndex int) bool {
	base := tokens[classIndex].Nesting
	for index := classIndex + 1; index < openIndex; index++ {
		if tokens[index].Nesting != base || !strings.EqualFold(tokens[index].Text, "extends") {
			continue
		}
		start := nextCompositeCodeToken(tokens, index+1)
		if start < 0 || start >= openIndex {
			return false
		}
		end := openIndex
		for cursor := start + 1; cursor < openIndex; cursor++ {
			if tokens[cursor].Nesting == base && strings.EqualFold(tokens[cursor].Text, "implements") {
				end = cursor
				break
			}
		}
		target := strings.TrimSpace(tokenRangeText(tokens, start, end))
		target = strings.TrimPrefix(target, "\\")
		parts := strings.Split(target, "\\")
		return len(parts) > 0 && strings.EqualFold(parts[len(parts)-1], "Component")
	}
	return false
}

func bladeVoltOwner(name string) string {
	if index := strings.LastIndexByte(name, '.'); index >= 0 && index+1 < len(name) {
		return name[index+1:]
	}
	return name
}

func bladePHPRegions(text string) ([]compositeEmbeddedRegion, bool) {
	const closeDirective = "@endphp"
	lower := asciiLowerPreservingBytes(text)
	regions := make([]compositeEmbeddedRegion, 0)
	complete := true
	for search := 0; search < len(lower); {
		directiveStart, directiveOpenEnd := nextBladePHPDirective(lower, search)
		rawStart := -1
		if match := phpOpenPattern.FindStringIndex(lower[search:]); match != nil {
			rawStart = search + match[0]
		}
		if directiveStart < 0 && rawStart < 0 {
			break
		}
		if rawStart >= 0 && (directiveStart < 0 || rawStart < directiveStart) {
			match := phpBlockPattern.FindStringSubmatchIndex(text[rawStart:])
			if len(match) < 4 || match[0] != 0 || match[2] < 0 || match[3] < 0 {
				complete = false
				break
			}
			fullEnd := rawStart + match[1]
			regions = append(regions, compositeEmbeddedRegion{
				kind: "blade-raw-php", language: "php",
				full:    OffsetRange{Start: rawStart, End: fullEnd},
				content: OffsetRange{Start: rawStart + match[2], End: rawStart + match[3]},
			})
			search = fullEnd
			continue
		}

		closeStart, closeEnd, found := bladeEndPHP(lower, directiveOpenEnd, closeDirective)
		if !found {
			complete = false
			break
		}
		regions = append(regions, compositeEmbeddedRegion{
			kind: "blade-php", language: "php",
			full:    OffsetRange{Start: directiveStart, End: closeEnd},
			content: OffsetRange{Start: directiveOpenEnd, End: closeStart},
		})
		search = closeEnd
	}
	return regions, complete
}

func nextBladePHPDirective(text string, search int) (int, int) {
	const openDirective = "@php"
	for search < len(text) {
		relative := strings.Index(text[search:], openDirective)
		if relative < 0 {
			return -1, -1
		}
		start := search + relative
		openEnd := start + len(openDirective)
		if start > 0 && text[start-1] == '@' {
			search = openEnd
			continue
		}
		if openEnd < len(text) && isASCIIIdentifierByte(text[openEnd]) {
			search = openEnd
			continue
		}
		cursor := openEnd
		for cursor < len(text) && (text[cursor] == ' ' || text[cursor] == '\t') {
			cursor++
		}
		if cursor < len(text) && text[cursor] == '(' {
			search = cursor + 1
			continue
		}
		return start, openEnd
	}
	return -1, -1
}

func bladeEndPHP(text string, search int, directive string) (int, int, bool) {
	for search < len(text) {
		relative := strings.Index(text[search:], directive)
		if relative < 0 {
			return 0, 0, false
		}
		start := search + relative
		end := start + len(directive)
		if end == len(text) || !isASCIIIdentifierByte(text[end]) {
			return start, end, true
		}
		search = end
	}
	return 0, 0, false
}

func regexEmbeddedRegions(text string, pattern *regexp.Regexp, kind, language string) []compositeEmbeddedRegion {
	matches := pattern.FindAllStringSubmatchIndex(text, -1)
	regions := make([]compositeEmbeddedRegion, 0, len(matches))
	for _, match := range matches {
		if len(match) <= 3 || match[2] < 0 || match[3] < 0 {
			continue
		}
		regions = append(regions, compositeEmbeddedRegion{kind: kind, language: language, full: OffsetRange{Start: match[0], End: match[1]}, content: OffsetRange{Start: match[2], End: match[3]}})
	}
	return regions
}

func scriptLanguage(attrs string) string {
	if match := languageAttributePattern.FindStringSubmatch(attrs); len(match) > 1 {
		switch strings.ToLower(strings.TrimSpace(match[1])) {
		case "ts", "typescript":
			return "typescript"
		case "js", "javascript", "module":
			return "javascript"
		default:
			return strings.ToLower(strings.TrimSpace(match[1]))
		}
	}
	if match := typeAttributePattern.FindStringSubmatch(attrs); len(match) > 1 {
		switch strings.ToLower(strings.TrimSpace(match[1])) {
		case "text/html":
			return "html"
		case "application/json", "application/ld+json", "importmap", "speculationrules":
			return "json"
		case "module", "text/javascript", "application/javascript":
			return "javascript"
		}
	}
	return "javascript"
}

func styleLanguage(attrs string) string {
	if match := languageAttributePattern.FindStringSubmatch(attrs); len(match) > 1 {
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

func astroFrontmatter(text string) (CompositeSegment, bool) {
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

func embeddedRegionFullRanges(regions []compositeEmbeddedRegion) []OffsetRange {
	ranges := make([]OffsetRange, 0, len(regions))
	for _, region := range regions {
		ranges = append(ranges, region.full)
	}
	return ranges
}

func maskCompositeRanges(text string, ranges []OffsetRange) (string, error) {
	masked := []byte(text)
	for _, value := range ranges {
		if value.Start < 0 || value.End < value.Start || value.End > len(text) || !utf8Boundary(text, value.Start) || !utf8Boundary(text, value.End) {
			return "", operation.New(operation.KindInvalidInput, "composite mask range is invalid")
		}
		maskByteRangePreservingLines(masked, value.Start, value.End)
	}
	return string(masked), nil
}

func maskTemplateSyntax(text string) (string, []OffsetRange, bool) {
	probe := maskHostComments(text)
	masked := []byte(probe)
	ranges := make([]OffsetRange, 0, 16)
	complete := true
	for position := 0; position < len(probe); {
		open, delimiters := nextTemplateDelimiter(probe, position)
		if open < 0 {
			break
		}
		close := strings.Index(probe[open+len(delimiters[0]):], delimiters[1])
		if close < 0 {
			complete = false
			value := OffsetRange{Start: open, End: len(probe)}
			ranges = append(ranges, value)
			maskByteRangePreservingLines(masked, value.Start, value.End)
			break
		}
		close += open + len(delimiters[0]) + len(delimiters[1])
		value := OffsetRange{Start: open, End: close}
		ranges = append(ranges, value)
		maskByteRangePreservingLines(masked, value.Start, value.End)
		position = close
	}
	return string(masked), ranges, complete
}

func nextTemplateDelimiter(text string, position int) (int, [2]string) {
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

func maskHostComments(text string) string {
	return maskDelimitedSourceRegions(text, [][2]string{{"<!--", "-->"}})
}

func analyzeClientWebRegions(ctx context.Context, document *SourceDocument, options AnalyzeOptions, hostLanguage string, analyzer AnalyzerID, excluded []OffsetRange) (AnalyzerResult, error) {
	probe := maskDelimitedSourceRegions(document.Text, [][2]string{{"<!--", "-->"}, {"@*", "*@"}})
	var err error
	if len(excluded) > 0 {
		probe, err = maskCompositeRanges(probe, excluded)
		if err != nil {
			return AnalyzerResult{}, err
		}
	}
	regions := append(
		tagEmbeddedRegions(probe, scriptTagPattern, "script", scriptLanguage),
		tagEmbeddedRegions(probe, styleTagPattern, "style", styleLanguage)...,
	)
	regions = orderedNonOverlappingEmbeddedRegions(regions)
	allRegions := regions
	complete := embeddedOpeningsCovered(probe, allRegions, scriptOpenPattern) && embeddedOpeningsCovered(probe, allRegions, styleOpenPattern)
	regions, regionsTruncated := capEmbeddedRegions(regions, options.Limits.MaxSymbols)

	hostMasks := append([]OffsetRange(nil), excluded...)
	hostMasks = append(hostMasks, embeddedRegionFullRanges(allRegions)...)
	hostMasked, err := maskCompositeRanges(document.Text, hostMasks)
	if err != nil {
		return AnalyzerResult{}, err
	}
	result, err := analyzeMaskedEmbeddedRegion(ctx, document, hostMasked, options, hostLanguage, analyzer, "client-host", "html")
	if err != nil {
		return AnalyzerResult{}, err
	}
	remaining := max(0, options.Limits.MaxSymbols-len(result.Analysis.Symbols))
	for index, region := range regions {
		regionID := fmt.Sprintf("client-%s-%d", region.kind, index+1)
		supported := embeddedLanguageSupported(region.language)
		public, rangeErr := sourceRegionForOffsets(document, regionID, region.kind, region.language, region.full.Start, region.full.End, supported)
		if rangeErr != nil {
			return AnalyzerResult{}, rangeErr
		}
		result.Regions = append(result.Regions, public)
		if !supported {
			result.Analysis.CoverageComplete = false
			continue
		}
		if razorGeneratedDataRegion(document.Text, hostLanguage, region) {
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
		embedded, analyzeErr := analyzeMaskedEmbeddedRegion(ctx, document, masked, regionOptions, hostLanguage, analyzer, regionID, region.language)
		if analyzeErr != nil {
			return AnalyzerResult{}, analyzeErr
		}
		mergeCompositeAnalyzerResult(&result, embedded, options.Limits)
		remaining = max(0, options.Limits.MaxSymbols-len(result.Analysis.Symbols))
	}
	appendCompositeDependencies(&result, scriptSourceDependencies(document, regions), options.Limits)
	if regionsTruncated {
		markCompositePartial(&result.Analysis, options.Limits, true, hostLanguage+"-region-limit", "client region retention limit reached")
	}
	if !complete {
		markCompositePartial(&result.Analysis, options.Limits, false, hostLanguage+"-unterminated-client-region", "client script/style region is not terminated")
	}
	return result, nil
}

func razorGeneratedDataRegion(text, hostLanguage string, region compositeEmbeddedRegion) bool {
	if region.kind != "script" || region.language != "json" || hostLanguage != "razor" && hostLanguage != "blazor" ||
		region.content.Start < 0 || region.content.End < region.content.Start || region.content.End > len(text) {
		return false
	}
	content := strings.TrimSpace(text[region.content.Start:region.content.End])
	return strings.HasPrefix(content, "@") && !strings.HasPrefix(content, "@@")
}

func scriptSourceDependencies(document *SourceDocument, regions []compositeEmbeddedRegion) []StructuralDependency {
	var dependencies []StructuralDependency
	for _, region := range regions {
		if region.kind != "script" || region.content.Start <= region.full.Start || region.content.Start > len(document.Text) {
			continue
		}
		opening := document.Text[region.full.Start:region.content.Start]
		match := sourceAttributePattern.FindStringSubmatchIndex(opening)
		if len(match) < 4 || match[2] < 0 {
			continue
		}
		start := region.full.Start + match[2]
		end := region.full.Start + match[3]
		addCompositeDependency(document, &dependencies, StructuralDependencyInclude, document.Text[start:end], start, end)
	}
	return dependencies
}

func jspDependencies(document *SourceDocument, probe string) []StructuralDependency {
	var dependencies []StructuralDependency
	for _, match := range jspIncludeDirectivePattern.FindAllStringSubmatchIndex(probe, -1) {
		if len(match) < 4 || match[2] < 0 {
			continue
		}
		addCompositeDependency(document, &dependencies, StructuralDependencyInclude, document.Text[match[2]:match[3]], match[2], match[3])
	}
	return dependencies
}

func templateDependencies(document *SourceDocument, probe string) []StructuralDependency {
	var dependencies []StructuralDependency
	for _, match := range templateDependencyPattern.FindAllStringSubmatchIndex(probe, -1) {
		if len(match) < 6 || match[4] < 0 {
			continue
		}
		kind := StructuralDependencyInclude
		directive := strings.ToLower(document.Text[match[2]:match[3]])
		if directive == "import" || directive == "from" {
			kind = StructuralDependencyImport
		}
		addCompositeDependency(document, &dependencies, kind, document.Text[match[4]:match[5]], match[4], match[5])
	}
	return dependencies
}

func bladeDependencies(document *SourceDocument, probe string) []StructuralDependency {
	var dependencies []StructuralDependency
	for _, match := range bladeDependencyPattern.FindAllStringSubmatchIndex(probe, -1) {
		if len(match) < 6 || match[4] < 0 {
			continue
		}
		addCompositeDependency(document, &dependencies, StructuralDependencyInclude, document.Text[match[4]:match[5]], match[4], match[5])
	}
	return dependencies
}

func ejsDependencies(ctx context.Context, document *SourceDocument, regions []compositeEmbeddedRegion, maxNesting int) ([]StructuralDependency, error) {
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
			open := nextCompositeCodeToken(scan.Tokens, index+1)
			valueIndex := nextCompositeCodeToken(scan.Tokens, open+1)
			close := nextCompositeCodeToken(scan.Tokens, valueIndex+1)
			if open < 0 || valueIndex < 0 || close < 0 || scan.Tokens[open].Text != "(" || scan.Tokens[valueIndex].Kind != TokenString || scan.Tokens[close].Text != ")" {
				continue
			}
			value := ecmaStringLiteralValue(scan.Tokens[valueIndex].Text)
			if value == "" {
				continue
			}
			start := region.content.Start + scan.Tokens[valueIndex].StartOffset
			end := region.content.Start + scan.Tokens[valueIndex].EndOffset
			addCompositeDependency(document, &dependencies, StructuralDependencyInclude, value, start, end)
		}
	}
	return dependencies, nil
}

func nextCompositeCodeToken(tokens []Token, start int) int {
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

func addCompositeDependency(document *SourceDocument, dependencies *[]StructuralDependency, kind StructuralDependencyKind, value string, start, end int) {
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

func capEmbeddedRegions(regions []compositeEmbeddedRegion, limit int) ([]compositeEmbeddedRegion, bool) {
	limit = max(1, limit)
	if len(regions) <= limit {
		return regions, false
	}
	return regions[:limit], true
}

func capCompositeOffsetRanges(ranges []OffsetRange, limit int) ([]OffsetRange, bool) {
	limit = max(1, limit)
	if len(ranges) <= limit {
		return ranges, false
	}
	return ranges[:limit], true
}

func embeddedOpeningsCovered(text string, regions []compositeEmbeddedRegion, opening *regexp.Regexp) bool {
	locations := opening.FindAllStringIndex(text, -1)
	regionIndex := 0
	for _, location := range locations {
		start := location[0]
		if closeRelative := strings.IndexByte(text[start:], '>'); closeRelative >= 0 && selfClosingTagOpening(text[start:start+closeRelative+1]) {
			continue
		}
		for regionIndex < len(regions) && regions[regionIndex].full.End <= start {
			regionIndex++
		}
		if regionIndex >= len(regions) || start < regions[regionIndex].full.Start || start >= regions[regionIndex].full.End {
			return false
		}
	}
	return true
}

func markCompositePartial(result *AnalysisResult, limits SymbolBuilderLimits, truncated bool, code, message string) {
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

func appendCompositeDependencies(result *AnalyzerResult, extra []StructuralDependency, limits SymbolBuilderLimits) {
	merged := appendUniqueDependencies(result.Dependencies, extra)
	limit := max(1, limits.MaxSymbols)
	if len(merged) > limit {
		merged = merged[:limit]
		markCompositePartial(&result.Analysis, limits, true, "dependency-limit", "dependency retention limit reached")
	}
	result.Dependencies = merged
}

func appendCompositeRegions(result *AnalyzerResult, extra []SourceRegion, limits SymbolBuilderLimits) {
	limit := max(1, limits.MaxSymbols)
	remaining := limit - len(result.Regions)
	if remaining <= 0 {
		if len(extra) > 0 {
			markCompositePartial(&result.Analysis, limits, true, "region-limit", "composite region retention limit reached")
		}
		return
	}
	if len(extra) > remaining {
		result.Regions = append(result.Regions, extra[:remaining]...)
		markCompositePartial(&result.Analysis, limits, true, "region-limit", "composite region retention limit reached")
		return
	}
	result.Regions = append(result.Regions, extra...)
}

func appendCompositeRelations(result *AnalyzerResult, extra []StructuralRelation, limits SymbolBuilderLimits) {
	limit := max(1, limits.MaxSymbols)
	remaining := limit - len(result.Relations)
	if remaining <= 0 {
		if len(extra) > 0 {
			markCompositePartial(&result.Analysis, limits, true, "relation-limit", "relation retention limit reached")
		}
		return
	}
	if len(extra) > remaining {
		result.Relations = append(result.Relations, extra[:remaining]...)
		markCompositePartial(&result.Analysis, limits, true, "relation-limit", "relation retention limit reached")
		return
	}
	result.Relations = append(result.Relations, extra...)
}

func orderedNonOverlappingEmbeddedRegions(regions []compositeEmbeddedRegion) []compositeEmbeddedRegion {
	sort.SliceStable(regions, func(i, j int) bool {
		if regions[i].full.Start != regions[j].full.Start {
			return regions[i].full.Start < regions[j].full.Start
		}
		return regions[i].full.End > regions[j].full.End
	})
	result := make([]compositeEmbeddedRegion, 0, len(regions))
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
