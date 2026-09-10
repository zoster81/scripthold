//go:build !race

package sourceintelligence

import (
	"context"
	"testing"
)

func TestRAnalyzerCommonPathAllocationBounded(t *testing.T) {
	document := sourceDocumentForScanner("library(stats)\nrun <- function(x) { x }\n")
	document.Path = "r-allocation.R"
	options := AnalyzeOptions{IncludeSignatures: true, MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256}}

	var observed int
	allocations := testing.AllocsPerRun(20, func() {
		result, err := (RAnalyzer{}).Analyze(context.Background(), document, options)
		if err != nil {
			panic(err)
		}
		observed += len(result.Analysis.Symbols)
	})
	if observed == 0 {
		t.Fatal("R allocation guard produced no symbols")
	}
	if allocations > 27 {
		t.Fatalf("R common-path allocations = %.0f, want <= 27", allocations)
	}
}
