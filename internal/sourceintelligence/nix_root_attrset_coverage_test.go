package sourceintelligence

import (
	"context"
	"testing"
)

func TestNixTopLevelAttributeSetReportsIncompleteCoverage(t *testing.T) {
	source := "# A flake-like root document\n{\n  description = \"demo\";\n  outputs = self: { package = \"example\"; };\n}\n"
	result, err := (NixAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("unmodeled root attribute set incorrectly declared complete: %+v", result.Analysis)
	}
	found := false
	for _, diagnostic := range result.Analysis.Diagnostics {
		if diagnostic.Code == "nix-unmodeled-root-structure" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing root Nix structure limitation: %+v", result.Analysis.Diagnostics)
	}
	if len(result.Analysis.Symbols) != 0 {
		t.Fatalf("unparsed root attributes invented declarations: %+v", result.Analysis.Symbols)
	}
}

func TestNixPlainLetExpressionRemainsComplete(t *testing.T) {
	source := "let\n  answer = 42;\nin answer\n"
	result, err := (NixAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 16))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated ||
		len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("simple indexed let expression misclassified: %+v", result.Analysis)
	}
	names := symbolsByQualifiedName(result.Analysis.Symbols)
	if _, ok := names["answer"]; !ok || len(names) != 1 {
		t.Fatalf("plain Nix binding unexpectedly absent: %+v", result.Analysis.Symbols)
	}
}
