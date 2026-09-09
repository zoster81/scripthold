//go:build !race

package sourceintelligence

import (
	"context"
	"testing"
)

func TestBashAnalyzerCommonPathAllocationBounded(t *testing.T) {
	document := sourceDocumentForScanner("#!/usr/bin/env bash\nsource ./lib.sh\nrun() { echo \"$1\"; }\n")
	document.Path = "bash-allocation.sh"
	options := AnalyzeOptions{IncludeSignatures: true, MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256}}

	var observed int
	allocations := testing.AllocsPerRun(20, func() {
		result, err := (BashAnalyzer{}).Analyze(context.Background(), document, options)
		if err != nil {
			panic(err)
		}
		observed += len(result.Analysis.Symbols)
	})
	if observed == 0 {
		t.Fatal("Bash allocation guard produced no symbols")
	}
	if allocations > 38 {
		t.Fatalf("Bash common-path allocations = %.0f, want <= 38", allocations)
	}
}
