//go:build !race

package sourceintelligence

import (
	"context"
	"testing"
)

func TestPythonAnalyzerAvoidsParentSnapshotCopies(t *testing.T) {
	document := sourceDocumentForScanner(generatedPythonSource(400))
	document.Path = "python-parent-allocation.py"
	options := AnalyzeOptions{
		MaxNesting: 256,
		Limits: SymbolBuilderLimits{
			MaxSymbols:        10_000,
			MaxSignatureBytes: 8192,
			MaxDiagnostics:    256,
		},
	}

	var observed int
	allocations := testing.AllocsPerRun(5, func() {
		result, err := (PythonAnalyzer{}).Analyze(context.Background(), document, options)
		if err != nil {
			panic(err)
		}
		observed += len(result.Analysis.Symbols)
	})
	if observed == 0 {
		t.Fatal("allocation guard produced no Python symbols")
	}
	if allocations > 4300 {
		t.Fatalf("Python parent-path allocations = %.0f, want <= 4300", allocations)
	}
}
