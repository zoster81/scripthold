package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// FlowAnalyzer reuses the proven typed-ECMAScript structural parser only after
// normalizing Flow-only syntax with byte-length-preserving masking. It does not
// claim TypeScript namespace/module semantics or Flow type checking.
type FlowAnalyzer struct{}

func (FlowAnalyzer) ID() AnalyzerID   { return AnalyzerFlow }
func (FlowAnalyzer) Language() string { return "flow" }

func (FlowAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_flow_source", document.Path, err)
	}
	masked := maskFlowOnlyKeywords(document.Text)
	clone := *document
	clone.Text = masked
	clone.lineStarts = buildLineStarts(masked)
	source, err := (TypeScriptAnalyzer{}).Analyze(ctx, &clone, options)
	if err != nil {
		return AnalyzerResult{}, err
	}
	filtered := source
	filtered.Analysis.Symbols = filtered.Analysis.Symbols[:0]
	for _, symbol := range source.Analysis.Symbols {
		if symbol.NativeKind == "namespace" || symbol.NativeKind == "module" {
			continue
		}
		filtered.Analysis.Symbols = append(filtered.Analysis.Symbols, symbol)
	}
	analysis, err := reprojectAnalyzerSymbols(ctx, document, filtered, options, "flow", AnalyzerFlow, "", 0, nil)
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range source.Analysis.Diagnostics {
		if strings.HasPrefix(diagnostic.Code, "typescript-") {
			diagnostic.Code = "flow-" + strings.TrimPrefix(diagnostic.Code, "typescript-")
		}
		if options.Limits.MaxDiagnostics > 0 && len(analysis.Diagnostics) >= options.Limits.MaxDiagnostics {
			analysis.DiagnosticsTruncated = true
			analysis.CoverageComplete = false
			break
		}
		analysis.Diagnostics = append(analysis.Diagnostics, diagnostic)
	}
	if source.Analysis.DiagnosticsTruncated {
		analysis.DiagnosticsTruncated = true
		analysis.CoverageComplete = false
	}
	for index := range analysis.Symbols {
		symbol := &analysis.Symbols[index]
		if symbol.Kind != SymbolKindType && symbol.Kind != SymbolKindAlias {
			continue
		}
		declaration, _, _, _ := symbol.SourceOffsets()
		if declaration.Start < 0 || declaration.End > len(document.Text) || declaration.End <= declaration.Start {
			continue
		}
		if strings.Contains(document.Text[declaration.Start:declaration.End], "opaque type") {
			symbol.NativeKind = "opaque-type"
		}
	}
	return AnalyzerResult{Analysis: analysis, Dependencies: source.Dependencies, Relations: source.Relations}, nil
}

func maskFlowOnlyKeywords(text string) string {
	bytes := []byte(text)
	for index := 0; index+len("opaque") <= len(bytes); index++ {
		if string(bytes[index:index+len("opaque")]) != "opaque" {
			continue
		}
		if index > 0 && isFlowIdentifierByte(bytes[index-1]) {
			continue
		}
		end := index + len("opaque")
		if end < len(bytes) && isFlowIdentifierByte(bytes[end]) {
			continue
		}
		next := end
		for next < len(bytes) && isFlowWhitespace(bytes[next]) {
			next++
		}
		if next+len("type") > len(bytes) || string(bytes[next:next+len("type")]) != "type" {
			continue
		}
		typeEnd := next + len("type")
		if typeEnd < len(bytes) && isFlowIdentifierByte(bytes[typeEnd]) {
			continue
		}
		for cursor := index; cursor < end; cursor++ {
			bytes[cursor] = ' '
		}
		index = end - 1
	}
	return string(bytes)
}

func isFlowIdentifierByte(value byte) bool {
	return value == '_' || value == '$' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func isFlowWhitespace(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}
