package sourceintelligence

import (
	"fmt"
	"testing"
)

func TestMergeCompositeAnalysisInitialSymbolsPreserveFirstOccurrenceAndBounds(t *testing.T) {
	limits := SymbolBuilderLimits{MaxSymbols: 2, MaxDiagnostics: 4}
	dst := AnalysisResult{
		CoverageComplete: true,
		Diagnostics:      []AnalysisDiagnostic{{Code: "dst-diagnostic", Message: "dst"}},
	}
	src := AnalysisResult{
		CoverageComplete: true,
		Symbols: []NormalizedSymbol{
			compositeMergeTestSymbol("b", "first-b", 20, 25),
			compositeMergeTestSymbol("a", "first-a", 10, 15),
			compositeMergeTestSymbol("b", "duplicate-b", 5, 8),
			compositeMergeTestSymbol("c", "first-c", 30, 35),
		},
		Diagnostics: []AnalysisDiagnostic{{Code: "src-diagnostic", Message: "src"}},
	}

	got := mergeCompositeAnalysis(dst, src, limits)

	if len(got.Symbols) != 2 {
		t.Fatalf("merged symbol count = %d, want 2", len(got.Symbols))
	}
	if got.Symbols[0].ID != "a" || got.Symbols[0].Name != "first-a" {
		t.Fatalf("first merged symbol = %#v, want first occurrence of a", got.Symbols[0])
	}
	if got.Symbols[1].ID != "b" || got.Symbols[1].Name != "first-b" {
		t.Fatalf("second merged symbol = %#v, want first occurrence of b", got.Symbols[1])
	}
	if !got.Truncated {
		t.Fatal("merged result is not truncated after a third unique symbol exceeds MaxSymbols")
	}
	if got.CoverageComplete {
		t.Fatal("merged result reports complete coverage after symbol truncation")
	}
	if len(got.Diagnostics) != 2 || got.Diagnostics[0].Code != "dst-diagnostic" || got.Diagnostics[1].Code != "src-diagnostic" {
		t.Fatalf("merged diagnostics = %#v, want destination then source diagnostics", got.Diagnostics)
	}
}

func TestMergeCompositeAnalysisInitialSymbolsAtExactLimitRemainComplete(t *testing.T) {
	src := AnalysisResult{
		CoverageComplete: true,
		Symbols: []NormalizedSymbol{
			compositeMergeTestSymbol("b", "first-b", 20, 25),
			compositeMergeTestSymbol("a", "first-a", 10, 15),
			compositeMergeTestSymbol("b", "duplicate-b", 5, 8),
		},
	}

	got := mergeCompositeAnalysis(
		AnalysisResult{CoverageComplete: true},
		src,
		SymbolBuilderLimits{MaxSymbols: 2, MaxDiagnostics: 4},
	)

	if got.Truncated || !got.CoverageComplete {
		t.Fatalf("exact-limit merge flags = truncated:%v complete:%v, want false/true", got.Truncated, got.CoverageComplete)
	}
	if len(got.Symbols) != 2 || got.Symbols[0].ID != "a" || got.Symbols[1].ID != "b" || got.Symbols[1].Name != "first-b" {
		t.Fatalf("exact-limit merged symbols = %#v, want sorted first occurrences a,b", got.Symbols)
	}
}

func TestMergeCompositeAnalysisPreservesNonNilEmptyDestination(t *testing.T) {
	dstSymbols := make([]NormalizedSymbol, 0, 8)
	got := mergeCompositeAnalysis(
		AnalysisResult{CoverageComplete: true, Symbols: dstSymbols},
		AnalysisResult{
			CoverageComplete: true,
			Symbols:          []NormalizedSymbol{compositeMergeTestSymbol("a", "a", 0, 1)},
		},
		SymbolBuilderLimits{MaxSymbols: 0, MaxDiagnostics: 4},
	)

	if got.Symbols == nil {
		t.Fatal("non-nil empty destination symbols became nil")
	}
	if cap(got.Symbols) != cap(dstSymbols) {
		t.Fatalf("non-nil empty destination capacity = %d, want %d", cap(got.Symbols), cap(dstSymbols))
	}
	if !got.Truncated || got.CoverageComplete {
		t.Fatalf("zero-limit merge flags = truncated:%v complete:%v, want true/false", got.Truncated, got.CoverageComplete)
	}
}

func TestMergeCompositeAnalysisInitialSymbolAllocationBudget(t *testing.T) {
	const symbolCount = 512
	symbols := make([]NormalizedSymbol, symbolCount)
	for index := range symbols {
		symbols[index] = compositeMergeTestSymbol(
			fmt.Sprintf("symbol-%04d", index),
			fmt.Sprintf("symbol_%04d", index),
			index*4,
			index*4+3,
		)
	}
	limits := SymbolBuilderLimits{MaxSymbols: symbolCount, MaxDiagnostics: 4}

	var observed int
	allocs := testing.AllocsPerRun(20, func() {
		merged := mergeCompositeAnalysis(
			AnalysisResult{CoverageComplete: true},
			AnalysisResult{CoverageComplete: true, Symbols: symbols},
			limits,
		)
		observed += len(merged.Symbols)
	})
	if observed == 0 {
		t.Fatal("allocation run did not retain symbols")
	}
	if allocs > 22 {
		t.Fatalf("initial composite merge allocations = %.0f, want <= 22", allocs)
	}
}

func compositeMergeTestSymbol(id, name string, start, end int) NormalizedSymbol {
	return NormalizedSymbol{
		ID:                 id,
		Kind:               SymbolKindFunction,
		Name:               name,
		QualifiedName:      "test." + name,
		declarationOffsets: OffsetRange{Start: start, End: end},
		nameOffsets:        OffsetRange{Start: start, End: end},
	}
}
