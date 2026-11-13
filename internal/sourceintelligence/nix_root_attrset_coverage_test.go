package sourceintelligence

import (
	"context"
	"strings"
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

func TestNixIndentedStringsCannotInventLetBindings(t *testing.T) {
	source := strings.Join([]string{
		"let",
		"  explanation = ''",
		"    phantom = 1;",
		"    ''${interpolation} phantom2 = 2;",
		"    ''' phantom3 = 3;",
		"    in result",
		"  '';",
		"  # '' a commented delimiter is not an opening quote",
		"  quoted = \"double '' quoted\";",
		"  answer = 42;",
		"in answer",
		"",
	}, "\n")
	for _, tc := range []struct{ name, source string }{
		{"lf", source},
		{"crlf", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			masked := maskNixIndentedStrings(tc.source)
			if len(masked) != len(tc.source) ||
				strings.Count(masked, "\n") != strings.Count(tc.source, "\n") ||
				strings.Contains(masked, "phantom") ||
				!strings.Contains(masked, "answer = 42;") {
				t.Fatalf("Nix quoted text masking changed offsets or escaped real code: %q", masked)
			}
			result, err := (NixAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 32))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("simple let expression incorrectly partial after string masking: %+v", result.Analysis)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, name := range []string{"explanation", "quoted", "answer"} {
				if _, ok := got[name]; !ok {
					t.Errorf("real Nix let binding %s omitted: %+v", name, got)
				}
			}
			for _, fake := range []string{"phantom", "phantom2", "phantom3", "result"} {
				if sym, ok := got[fake]; ok {
					t.Errorf("Nix string falsely indexed %s: %+v", fake, sym)
				}
			}
		})
	}
}
