package sourceintelligence

import (
	"context"

	"github.com/zoster81/scripthold/internal/operation"
)

func reprojectAnalyzerSymbols(ctx context.Context, host *SourceDocument, source AnalyzerResult, options AnalyzeOptions, language string, analyzer AnalyzerID, regionID string, delta int, rootParent *SymbolParent) (AnalysisResult, error) {
	builder := NewSymbolBuilder(host, SymbolBuilderOptions{Context: ctx, Language: language, Analyzer: string(analyzer), RegionID: regionID, IncludeSignatures: options.IncludeSignatures, MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits})
	if err := builder.checkReady(); err != nil {
		return AnalysisResult{}, err
	}
	parents := make(map[string]SymbolParent, len(source.Analysis.Symbols))
	for _, current := range source.Analysis.Symbols {
		declaration, nameRange, signature, body := current.SourceOffsets()
		declaration = shiftOffsetRange(declaration, delta)
		nameRange = shiftOffsetRange(nameRange, delta)
		if signature != nil {
			value := shiftOffsetRange(*signature, delta)
			signature = &value
		}
		if body != nil {
			value := shiftOffsetRange(*body, delta)
			body = &value
		}
		parent := rootParent
		if current.ParentID != "" {
			if mapped, ok := parents[current.ParentID]; ok {
				value := mapped
				parent = &value
			}
		}
		target, err := builder.Add(SymbolSpec{Kind: current.Kind, NativeKind: current.NativeKind, Name: current.Name, Parent: parent, RegionID: regionID, Declaration: declaration, NameRange: nameRange, Signature: signature, Body: body, Visibility: current.Visibility, Modifiers: current.Modifiers, Evidence: current.Evidence, Disambiguator: current.ID})
		if operation.KindOf(err) == operation.KindLimit {
			builder.MarkTruncated()
			break
		}
		if err != nil {
			builder.MarkIncomplete()
			continue
		}
		parents[current.ID] = SymbolParent{ID: target.ID, QualifiedName: target.QualifiedName}
	}
	if !source.Analysis.CoverageComplete {
		builder.MarkIncomplete()
	}
	if source.Analysis.Truncated {
		builder.MarkTruncated()
	}
	result := builder.Result()
	if len(source.Analysis.Diagnostics) > 0 {
		origin, err := host.PositionAtUTF8Offset(delta)
		if err != nil {
			return AnalysisResult{}, err
		}
		limit := max(1, options.Limits.MaxDiagnostics)
		for _, diagnostic := range source.Analysis.Diagnostics {
			if len(result.Diagnostics) >= limit {
				result.DiagnosticsTruncated = true
				result.CoverageComplete = false
				break
			}
			if diagnostic.Range != nil {
				value := reprojectDiagnosticRange(*diagnostic.Range, origin)
				diagnostic.Range = &value
			}
			result.Diagnostics = append(result.Diagnostics, diagnostic)
		}
	}
	if source.Analysis.DiagnosticsTruncated {
		result.DiagnosticsTruncated = true
		result.CoverageComplete = false
	}
	return result, nil
}

func reprojectDiagnosticRange(value Range, origin Position) Range {
	return Range{
		Start: reprojectDiagnosticPosition(value.Start, origin),
		End:   reprojectDiagnosticPosition(value.End, origin),
	}
}

func reprojectDiagnosticPosition(value Position, origin Position) Position {
	position := Position{Line: value.Line + origin.Line - 1, Column: value.Column}
	if value.Line == 1 {
		position.Column += origin.Column - 1
	}
	return position
}

func mergeAnalysisSymbols(base *AnalysisResult, extra AnalysisResult) {
	base.Symbols = append(base.Symbols, extra.Symbols...)
	base.Diagnostics = append(base.Diagnostics, extra.Diagnostics...)
	if !extra.CoverageComplete {
		base.CoverageComplete = false
	}
	if extra.Truncated {
		base.Truncated = true
		base.CoverageComplete = false
	}
	if extra.DiagnosticsTruncated {
		base.DiagnosticsTruncated = true
	}
}
