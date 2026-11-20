package sourceintelligence

import (
	"context"
	"testing"
)

// Ada permits quoted operator symbols as subprogram names. Their parameter
// identifiers are not callable declarations.
func TestAdaOperatorFunctionDoesNotFabricateParameterAsCallable(t *testing.T) {
	source := "package body Scanner is\n" +
		"  function \"*\" (Left, Right : in Integer) return Integer is\n" +
		"  begin\n" +
		"    return Left * Right;\n" +
		"  end \"*\";\n" +
		"  function Named (Value : Integer) return Integer;\n" +
		"end Scanner;\n"
	result, err := (AdaAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	got := symbolsByQualifiedName(result.Analysis.Symbols)
	if sym, ok := got["Scanner.*"]; !ok || sym.NameRange.Start.Line != 2 || sym.ID == "" {
		t.Errorf("authored Ada multiplication operator not navigable: %+v", sym)
	}
	if sym, ok := got["Scanner.Named"]; !ok || sym.NameRange.Start.Line != 6 {
		t.Errorf("ordinary Ada function regressed: %+v", sym)
	}
	for _, name := range []string{"Scanner.Left", "Scanner.Right", "Scanner.Value"} {
		if sym, ok := got[name]; ok {
			t.Errorf("fabricated Ada function from a parameter %q: %+v", name, sym)
		}
	}
}

func TestAdaOperatorNamesRejectArbitraryStringsAndProcedureOperators(t *testing.T) {
	source := "package Operations is\n" +
		"  function \"unsupported\" (Left : Integer) return Integer;\n" +
		"  procedure \"*\" (Wrong : Integer);\n" +
		"  function \"+\" (X, Y : Integer) return Integer;\n" +
		"  function Normal (Value : Integer) return Integer;\n" +
		"end Operations;\n"
	result, err := (AdaAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 50))
	if err != nil {
		t.Fatal(err)
	}
	got := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, wanted := range []string{"Operations.+", "Operations.Normal"} {
		if sym, ok := got[wanted]; !ok || sym.ID == "" {
			t.Errorf("valid Ada callable missing %s: %+v", wanted, sym)
		}
	}
	for _, unwanted := range []string{
		"Operations.unsupported", "Operations.Left", "Operations.*",
		"Operations.Wrong", "Operations.X", "Operations.Value",
	} {
		if sym, ok := got[unwanted]; ok {
			t.Errorf("invalid Ada operator name or parameter fabricated %s: %+v", unwanted, sym)
		}
	}
}
