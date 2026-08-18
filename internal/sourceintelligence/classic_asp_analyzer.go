package sourceintelligence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// ClassicASPAnalyzer segments the host document and delegates only supported
// embedded Basic-family regions. Unsupported languages remain explicit regions.
type ClassicASPAnalyzer struct{}

func (ClassicASPAnalyzer) ID() AnalyzerID   { return AnalyzerClassicASP }
func (ClassicASPAnalyzer) Language() string { return "classic-asp" }

type aspSegment struct {
	kind      string
	language  string
	start     int
	end       int
	codeStart int
	codeEnd   int
	supported bool
	regionID  string
}

func (ClassicASPAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_classic_asp_source", document.Path, err)
	}
	hostBuilder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: "classic-asp", Analyzer: string(AnalyzerClassicASP), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := hostBuilder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	language := classicASPPageLanguage(document.Text)
	segments, incomplete := segmentClassicASP(document.Path, document.Text, language)
	if incomplete {
		hostBuilder.MarkIncomplete()
		_ = hostBuilder.AddDiagnostic(DiagnosticSpec{Code: "asp-unterminated-region", Message: "Classic ASP region is not terminated", Severity: DiagnosticWarning, AffectsCoverage: true})
	}
	regions := make([]SourceRegion, 0, len(segments))
	for _, segment := range segments {
		rangeValue, err := document.RangeFromUTF8Offsets(segment.start, segment.end)
		if err != nil {
			return AnalyzerResult{}, err
		}
		regions = append(regions, SourceRegion{ID: segment.regionID, Kind: segment.kind, Language: segment.language, Range: rangeValue, Evidence: SymbolEvidenceStructural, Supported: segment.supported})
	}

	dependencies := classicASPIncludes(document)
	var symbols []NormalizedSymbol
	remaining := options.Limits.MaxSymbols
	for _, segment := range segments {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_classic_asp_source", document.Path, err)
		}
		if segment.kind != "server-script" {
			continue
		}
		if !segment.supported {
			hostBuilder.MarkIncomplete()
			rangeValue := OffsetRange{Start: segment.start, End: segment.end}
			_ = hostBuilder.AddDiagnostic(DiagnosticSpec{Code: "asp-unsupported-language", Message: "embedded server language " + segment.language + " is not supported", Severity: DiagnosticWarning, Range: &rangeValue, AffectsCoverage: true})
			continue
		}
		if remaining <= 0 {
			hostBuilder.MarkTruncated()
			break
		}
		regionOptions := options
		regionOptions.Limits.MaxSymbols = remaining
		regionSymbols, complete, truncated, err := analyzeClassicASPRegion(ctx, document, segment, regionOptions)
		if err != nil {
			return AnalyzerResult{}, err
		}
		symbols = append(symbols, regionSymbols...)
		remaining -= len(regionSymbols)
		if !complete {
			hostBuilder.MarkIncomplete()
			rangeValue := OffsetRange{Start: segment.start, End: segment.end}
			_ = hostBuilder.AddDiagnostic(DiagnosticSpec{Code: "asp-embedded-partial", Message: "embedded server-script analysis is partial", Severity: DiagnosticWarning, Range: &rangeValue, AffectsCoverage: true})
		}
		if truncated {
			hostBuilder.MarkTruncated()
		}
	}

	hostResult := hostBuilder.Result()
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].declarationOffsets.Start != symbols[j].declarationOffsets.Start {
			return symbols[i].declarationOffsets.Start < symbols[j].declarationOffsets.Start
		}
		return symbols[i].ID < symbols[j].ID
	})
	hostResult.Symbols = symbols
	return AnalyzerResult{Analysis: hostResult, Dependencies: dependencies, Regions: regions}, nil
}

func analyzeClassicASPRegion(ctx context.Context, host *SourceDocument, segment aspSegment, options AnalyzeOptions) ([]NormalizedSymbol, bool, bool, error) {
	text := host.Text[segment.codeStart:segment.codeEnd]
	subdocument := &SourceDocument{Path: host.Path + "#" + segment.regionID, Text: text, Encoding: "utf-8", lineStarts: buildLineStarts(text)}
	language := normalizeASPLanguage(segment.language)
	var analyzer SourceAnalyzer
	switch language {
	case "vbscript":
		analyzer = VBScriptAnalyzer{}
	case "jscript":
		analyzer = JavaScriptAnalyzer{}
	default:
		return nil, false, false, operation.New(operation.KindUnsupported, "unsupported Classic ASP embedded language")
	}
	subresult, err := analyzer.Analyze(ctx, subdocument, options)
	if err != nil {
		return nil, false, false, err
	}
	builder := NewSymbolBuilder(host, SymbolBuilderOptions{
		Context: ctx, Language: language, Analyzer: string(AnalyzerClassicASP), RegionID: segment.regionID,
		IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return nil, false, false, err
	}
	parents := make(map[string]SymbolParent)
	for _, source := range subresult.Analysis.Symbols {
		declaration, signature, body := source.sourceOffsets()
		declaration = shiftOffsetRange(declaration, segment.codeStart)
		if signature != nil {
			shifted := shiftOffsetRange(*signature, segment.codeStart)
			signature = &shifted
		}
		if body != nil {
			shifted := shiftOffsetRange(*body, segment.codeStart)
			body = &shifted
		}
		nameRange := shiftOffsetRange(source.nameOffsets, segment.codeStart)
		var parent *SymbolParent
		if source.ParentID != "" {
			if mapped, ok := parents[source.ParentID]; ok {
				copy := mapped
				parent = &copy
			}
		}
		if parent == nil && source.ParentQualifiedName != "" {
			parent = &SymbolParent{QualifiedName: source.ParentQualifiedName}
		}
		kind := source.Kind
		if language == "vbscript" && parent == nil && kind == SymbolKindFunction {
			kind = SymbolKindMethod
		}
		target, addErr := builder.Add(SymbolSpec{
			Kind: kind, NativeKind: source.NativeKind, Name: source.Name, QualifiedName: source.QualifiedName, Parent: parent,
			RegionID: segment.regionID, Declaration: declaration, NameRange: nameRange, Signature: signature, Body: body,
			Visibility: source.Visibility, Modifiers: source.Modifiers, Evidence: SymbolEvidenceStructural, Disambiguator: source.ID,
		})
		if operation.KindOf(addErr) == operation.KindLimit {
			builder.MarkTruncated()
			break
		}
		if addErr != nil {
			builder.MarkIncomplete()
			continue
		}
		parents[source.ID] = SymbolParent{ID: target.ID, QualifiedName: target.QualifiedName}
	}
	result := builder.Result()
	return result.Symbols, subresult.Analysis.CoverageComplete && result.CoverageComplete, subresult.Analysis.Truncated || result.Truncated, nil
}
func shiftOffsetRange(value OffsetRange, delta int) OffsetRange {
	return OffsetRange{Start: value.Start + delta, End: value.End + delta}
}

func segmentClassicASP(path, text, pageLanguage string) ([]aspSegment, bool) {
	lower := asciiLowerPreservingBytes(text)
	var segments []aspSegment
	position := 0
	incomplete := false
	add := func(kind, language string, start, end, codeStart, codeEnd int, supported bool) {
		if end <= start {
			return
		}
		segments = append(segments, aspSegment{kind: kind, language: language, start: start, end: end, codeStart: codeStart, codeEnd: codeEnd, supported: supported, regionID: classicASPRegionID(path, kind, language, start, end)})
	}
	for position < len(text) {
		aspIndex := strings.Index(lower[position:], "<%")
		if aspIndex >= 0 {
			aspIndex += position
		}
		scriptIndex := nextServerScriptTag(lower, position)
		next := minPositive(aspIndex, scriptIndex)
		if next < 0 {
			add("host", "html", position, len(text), position, len(text), true)
			break
		}
		if next > position {
			add("host", "html", position, next, position, next, true)
		}
		if next == scriptIndex {
			openEndRelative := strings.Index(lower[scriptIndex:], ">")
			if openEndRelative < 0 {
				add("host", "html", scriptIndex, len(text), scriptIndex, len(text), true)
				incomplete = true
				break
			}
			openEnd := scriptIndex + openEndRelative + 1
			closeRelative := strings.Index(lower[openEnd:], "</script>")
			if closeRelative < 0 {
				language := classicASPScriptTagLanguage(text[scriptIndex:openEnd], pageLanguage)
				add("server-script", language, scriptIndex, len(text), openEnd, len(text), classicASPSupportedLanguage(language))
				incomplete = true
				break
			}
			closeStart := openEnd + closeRelative
			end := closeStart + len("</script>")
			language := classicASPScriptTagLanguage(text[scriptIndex:openEnd], pageLanguage)
			add("server-script", language, scriptIndex, end, openEnd, closeStart, classicASPSupportedLanguage(language))
			position = end
			continue
		}

		closeRelative := strings.Index(lower[aspIndex+2:], "%>")
		if closeRelative < 0 {
			kind, codeStart := classicASPBlockKind(lower, aspIndex, len(text))
			language := pageLanguage
			add(kind, language, aspIndex, len(text), codeStart, len(text), kind != "server-script" || classicASPSupportedLanguage(language))
			incomplete = true
			break
		}
		closeStart := aspIndex + 2 + closeRelative
		end := closeStart + 2
		kind, codeStart := classicASPBlockKind(lower, aspIndex, closeStart)
		language := pageLanguage
		supported := true
		if kind == "server-script" {
			supported = classicASPSupportedLanguage(language)
		}
		add(kind, language, aspIndex, end, codeStart, closeStart, supported)
		position = end
	}
	return segments, incomplete
}

func classicASPBlockKind(lower string, start, end int) (string, int) {
	if start+3 <= end {
		switch lower[start+2] {
		case '@':
			return "directive", start + 3
		case '=':
			return "expression", start + 3
		}
	}
	return "server-script", start + 2
}

func nextServerScriptTag(lower string, start int) int {
	search := start
	for search < len(lower) {
		relative := strings.Index(lower[search:], "<script")
		if relative < 0 {
			return -1
		}
		index := search + relative
		endRelative := strings.Index(lower[index:], ">")
		if endRelative < 0 {
			return index
		}
		opening := lower[index : index+endRelative+1]
		if strings.Contains(opening, "runat") && strings.Contains(opening, "server") {
			return index
		}
		search = index + endRelative + 1
	}
	return -1
}

func classicASPPageLanguage(text string) string {
	lower := asciiLowerPreservingBytes(text)
	search := 0
	for {
		startRelative := strings.Index(lower[search:], "<%@")
		if startRelative < 0 {
			return "vbscript"
		}
		start := search + startRelative
		endRelative := strings.Index(lower[start+3:], "%>")
		if endRelative < 0 {
			return "vbscript"
		}
		end := start + 3 + endRelative
		if value := htmlLikeAttribute(text[start+3:end], "language"); value != "" {
			return normalizeASPLanguage(value)
		}
		search = end + 2
	}
}

func classicASPScriptTagLanguage(openingTag, fallback string) string {
	if value := htmlLikeAttribute(openingTag, "language"); value != "" {
		return normalizeASPLanguage(value)
	}
	return fallback
}

func normalizeASPLanguage(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "vbscript", "vb", "visualbasic":
		return "vbscript"
	case "jscript", "javascript", "js":
		return "jscript"
	default:
		return value
	}
}

func classicASPSupportedLanguage(language string) bool {
	normalized := normalizeASPLanguage(language)
	return normalized == "vbscript" || normalized == "jscript"
}

func htmlLikeAttribute(text, name string) string {
	lower := asciiLowerPreservingBytes(text)
	name = strings.ToLower(name)
	search := 0
	for search < len(lower) {
		indexRelative := strings.Index(lower[search:], name)
		if indexRelative < 0 {
			return ""
		}
		index := search + indexRelative
		beforeOK := index == 0 || !isDirectiveWordByte(lower[index-1])
		after := index + len(name)
		afterOK := after == len(lower) || !isDirectiveWordByte(lower[after])
		if beforeOK && afterOK {
			for after < len(text) && (text[after] == ' ' || text[after] == '\t' || text[after] == '\r' || text[after] == '\n') {
				after++
			}
			if after < len(text) && text[after] == '=' {
				after++
				for after < len(text) && (text[after] == ' ' || text[after] == '\t' || text[after] == '\r' || text[after] == '\n') {
					after++
				}
				if after < len(text) && (text[after] == '\'' || text[after] == '"') {
					quote := text[after]
					valueStart := after + 1
					if valueEndRelative := strings.IndexByte(text[valueStart:], quote); valueEndRelative >= 0 {
						return text[valueStart : valueStart+valueEndRelative]
					}
				}
			}
		}
		search = index + len(name)
	}
	return ""
}

func classicASPIncludes(document *SourceDocument) []StructuralDependency {
	lower := asciiLowerPreservingBytes(document.Text)
	var result []StructuralDependency
	search := 0
	for search < len(lower) {
		relative := strings.Index(lower[search:], "<!--#include")
		if relative < 0 {
			break
		}
		start := search + relative
		endRelative := strings.Index(lower[start:], "-->")
		if endRelative < 0 {
			break
		}
		end := start + endRelative + 3
		body := document.Text[start:end]
		value := htmlLikeAttribute(body, "file")
		if value == "" {
			value = htmlLikeAttribute(body, "virtual")
		}
		if value != "" {
			rangeValue, err := document.RangeFromUTF8Offsets(start, end)
			if err == nil {
				result = append(result, StructuralDependency{Kind: StructuralDependencyInclude, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural})
			}
		}
		search = end
	}
	return result
}

func classicASPRegionID(path, kind, language string, start, end int) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d", path, kind, language, start, end)))
	return hex.EncodeToString(hash[:])
}

func minPositive(values ...int) int {
	best := -1
	for _, value := range values {
		if value >= 0 && (best < 0 || value < best) {
			best = value
		}
	}
	return best
}
