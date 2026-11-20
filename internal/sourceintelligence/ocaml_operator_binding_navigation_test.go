package sourceintelligence

import (
	"context"
	"testing"
)

func TestOCamlOperatorBindingNamesAreNotParameterNames(t *testing.T) {
	source := `module Syntax = struct
  let ( let* ) = bind
  let ( and* ) = product
  let ( let+ ) o f = map f o
  let ( and+ ) = product
end
let next value = value
`
	result, err := (OCamlAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("balanced OCaml module incomplete: %+v", result.Analysis)
	}
	got := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"Syntax.let*", "Syntax.and*", "Syntax.let+", "Syntax.and+", "next"} {
		if _, exists := got[name]; !exists {
			t.Errorf("expected actual OCaml binding %q: symbols=%+v", name, result.Analysis.Symbols)
		}
	}
	for _, fake := range []string{"Syntax.o", "Syntax.f", "Syntax.bind", "Syntax.product"} {
		if _, exists := got[fake]; exists {
			t.Errorf("invented parameter or referenced value as OCaml binding %q", fake)
		}
	}
}
