package sourceintelligence

import (
	"reflect"
	"testing"
)

func TestMergeFortranConditionalVariantsPreservesDedupSemantics(t *testing.T) {
	first := NormalizedSymbol{
		ID: "id-first", Kind: SymbolKindFunction, NativeKind: "first", Name: "shared", QualifiedName: "demo::shared",
		declarationOffsets: OffsetRange{Start: 10, End: 14},
	}
	differentKind := NormalizedSymbol{
		ID: "id-type", Kind: SymbolKindType, NativeKind: "type", Name: "shared", QualifiedName: "demo::shared",
		declarationOffsets: OffsetRange{Start: 20, End: 24},
	}
	emptyFirst := NormalizedSymbol{
		ID: "id-empty-first", Kind: SymbolKindFunction, NativeKind: "empty-first", Name: "empty-first",
		declarationOffsets: OffsetRange{Start: 30, End: 34},
	}
	duplicateLogical := NormalizedSymbol{
		ID: "id-second", Kind: SymbolKindFunction, NativeKind: "second", Name: "shared-second", QualifiedName: "demo::shared",
		declarationOffsets: OffsetRange{Start: 40, End: 44},
	}
	duplicateID := NormalizedSymbol{
		ID: differentKind.ID, Kind: SymbolKindClass, NativeKind: "duplicate-id", Name: "other", QualifiedName: "demo::other",
		declarationOffsets: OffsetRange{Start: 50, End: 54},
	}
	emptySecond := NormalizedSymbol{
		ID: "id-empty-second", Kind: SymbolKindFunction, NativeKind: "empty-second", Name: "empty-second",
		declarationOffsets: OffsetRange{Start: 60, End: 64},
	}
	variants := []AnalyzerResult{
		{Analysis: AnalysisResult{Symbols: []NormalizedSymbol{first, differentKind, emptyFirst}, CoverageComplete: true}},
		{Analysis: AnalysisResult{Symbols: []NormalizedSymbol{duplicateLogical, duplicateID, emptySecond}, CoverageComplete: true}},
	}
	options := AnalyzeOptions{Limits: SymbolBuilderLimits{MaxSymbols: 64, MaxDiagnostics: 16}}

	got := mergeFortranConditionalVariants(options, variants).Analysis.Symbols
	want := []NormalizedSymbol{first, differentKind, emptyFirst, emptySecond}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merged symbols = %+v, want %+v", got, want)
	}
}

func TestMergeFortranConditionalVariantsPreservesEmbeddedNULCollision(t *testing.T) {
	first := NormalizedSymbol{
		ID: "id-first", Kind: SymbolKind("function\x00qualified"), QualifiedName: "target",
		declarationOffsets: OffsetRange{Start: 10, End: 14},
	}
	colliding := NormalizedSymbol{
		ID: "id-second", Kind: SymbolKindFunction, QualifiedName: "qualified\x00target",
		declarationOffsets: OffsetRange{Start: 20, End: 24},
	}
	variants := []AnalyzerResult{
		{Analysis: AnalysisResult{Symbols: []NormalizedSymbol{first}, CoverageComplete: true}},
		{Analysis: AnalysisResult{Symbols: []NormalizedSymbol{colliding}, CoverageComplete: true}},
	}
	options := AnalyzeOptions{Limits: SymbolBuilderLimits{MaxSymbols: 16, MaxDiagnostics: 8}}

	got := mergeFortranConditionalVariants(options, variants).Analysis.Symbols
	if len(got) != 1 || !reflect.DeepEqual(got[0], first) {
		t.Fatalf("embedded-NUL compatibility = %+v, want first symbol only", got)
	}
}

func TestMergeFortranConditionalVariantsAllocationBudget(t *testing.T) {
	const variantCount = 4
	const symbolsPerVariant = 512
	variants := make([]AnalyzerResult, variantCount)
	for variantIndex := 0; variantIndex < variantCount; variantIndex++ {
		symbols := make([]NormalizedSymbol, symbolsPerVariant)
		for symbolIndex := 0; symbolIndex < symbolsPerVariant; symbolIndex++ {
			value := dependencyMergeTestValue(symbolIndex)
			symbols[symbolIndex] = NormalizedSymbol{
				ID: string(rune('a'+variantIndex)) + value, Kind: SymbolKindFunction, QualifiedName: "demo::" + value,
				declarationOffsets: OffsetRange{Start: symbolIndex * 8, End: symbolIndex*8 + 4},
			}
		}
		variants[variantIndex] = AnalyzerResult{Analysis: AnalysisResult{Symbols: symbols, CoverageComplete: true}}
	}
	options := AnalyzeOptions{Limits: SymbolBuilderLimits{MaxSymbols: 2048, MaxDiagnostics: 16}}

	allocations := testing.AllocsPerRun(20, func() {
		merged := mergeFortranConditionalVariants(options, variants)
		if len(merged.Analysis.Symbols) != symbolsPerVariant {
			panic("unexpected merged symbol count")
		}
	})
	if allocations > 128 {
		t.Fatalf("mergeFortranConditionalVariants allocations = %.0f, want <= 128", allocations)
	}
}
