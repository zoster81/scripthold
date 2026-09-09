//go:build !race

package sourceintelligence

import (
	"context"
	"testing"
)

func TestMATLABAnalyzerCommonPathAllocationBounded(t *testing.T) {
	document := sourceDocumentForScanner("function y = run(x)\n  y = x + 1;\nend\n")
	document.Path = "matlab-allocation.m"
	options := AnalyzeOptions{IncludeSignatures: true, MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256}}

	var observed int
	allocations := testing.AllocsPerRun(20, func() {
		result, err := (MATLABAnalyzer{}).Analyze(context.Background(), document, options)
		if err != nil {
			panic(err)
		}
		observed += len(result.Analysis.Symbols)
	})
	if observed == 0 {
		t.Fatal("MATLAB allocation guard produced no symbols")
	}
	if allocations > 28 {
		t.Fatalf("MATLAB common-path allocations = %.0f, want <= 28", allocations)
	}
}
