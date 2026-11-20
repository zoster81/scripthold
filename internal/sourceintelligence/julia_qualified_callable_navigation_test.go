package sourceintelligence

import (
	"context"
	"testing"
)

func TestJuliaModuleQualifiedFunctionsAreRealDeclarations(t *testing.T) {
	source := "struct Fiberator\n n::Int\nend\n" +
		"function Base.iterate(fib::Fiberator, state=nothing)\n return nothing\nend\n" +
		"Base.length(fib::Fiberator) = fib.n\n" +
		"Base.eltype(::Type{Fiberator}) = Int\n" +
		"function union!(x, y)\n return x\nend\n"
	result, err := (JuliaAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid Julia source incompletely analyzed: %+v", result.Analysis)
	}
	names := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"Fiberator", "Base.iterate", "Base.length", "Base.eltype", "union!"} {
		if _, ok := names[name]; !ok {
			t.Errorf("real Julia callable %s not found: %+v", name, result.Analysis.Symbols)
		}
	}
	if _, ok := names["Base"]; ok {
		t.Fatal("Julia namespace Base fabricated as function declaration")
	}
}
