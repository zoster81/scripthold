package sourceintelligence

import (
	"context"
	"testing"
)

func TestOCamlParametricTypesUseDeclaredTypeName(t *testing.T) {
	source := "type 'a t = None | Some of 'a\n" +
		"type ('a, 'b) result = Ok of 'a | Error of 'b\n" +
		"type plain = Plain\n" +
		"let after value = value\n"
	result, err := (OCamlAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid type declarations marked incomplete: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"t", "result", "plain"} {
		sym, ok := byName[name]
		if !ok || sym.Kind != SymbolKindType {
			t.Errorf("real OCaml type %q omitted or misclassified: %+v", name, sym)
		}
	}
	for _, wrong := range []string{"a", "b"} {
		if _, found := byName[wrong]; found {
			t.Errorf("type parameter %q fabricated as type declaration", wrong)
		}
	}
	if _, ok := byName["after"]; !ok {
		t.Error("ordinary function following parametric types omitted")
	}
}
