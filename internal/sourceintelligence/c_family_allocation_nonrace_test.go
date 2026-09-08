//go:build !race

package sourceintelligence

import (
	"context"
	"testing"
)

func TestCAnalyzerCommonPathAllocationBounded(t *testing.T) {
	document := sourceDocumentForScanner("#include <stddef.h>\nstruct Box { int value; };\nint run(int value) { return value; }\n")
	document.Path = "c-allocation.c"
	options := AnalyzeOptions{IncludeSignatures: true, MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256}}

	var observed int
	allocations := testing.AllocsPerRun(20, func() {
		result, err := (CAnalyzer{}).Analyze(context.Background(), document, options)
		if err != nil {
			panic(err)
		}
		observed += len(result.Analysis.Symbols)
	})
	if observed == 0 {
		t.Fatal("C allocation guard produced no symbols")
	}
	if allocations > 80 {
		t.Fatalf("C common-path allocations = %.0f, want <= 80", allocations)
	}
}

func TestCPPAnalyzerCommonPathAllocationBounded(t *testing.T) {
	document := sourceDocumentForScanner("#include <vector>\nnamespace demo { class Box { public: int run(int value) { return value; } }; }\n")
	document.Path = "cpp-allocation.cpp"
	options := AnalyzeOptions{IncludeSignatures: true, MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256}}

	var observed int
	allocations := testing.AllocsPerRun(20, func() {
		result, err := (CPPAnalyzer{}).Analyze(context.Background(), document, options)
		if err != nil {
			panic(err)
		}
		observed += len(result.Analysis.Symbols)
	})
	if observed == 0 {
		t.Fatal("C++ allocation guard produced no symbols")
	}
	if allocations > 100 {
		t.Fatalf("C++ common-path allocations = %.0f, want <= 100", allocations)
	}
}
