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

// Only bindings of the outer let are safe top-level edit targets. Nested
// attrsets and function-local let expressions are not the same declarations.
func TestNixOuterLetDoesNotInventNestedBindings(t *testing.T) {
	const source = `let
  outer = {
    name = "inside";
    nested = {
      option = true;
    };
  };
  builder = value:
    let
      local = 1;
    in local;
  after = 42;
in
{
  result = outer;
}
`
	for _, tc := range []struct{ name, text string }{
		{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (NixAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 40))
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]int{"outer": 2, "builder": 8, "after": 12}
			for _, symbol := range result.Analysis.Symbols {
				line, ok := want[symbol.Name]
				if !ok {
					t.Errorf("nested Nix binding misreported at top level: %+v", symbol)
				} else if symbol.NameRange.Start.Line != line {
					t.Errorf("top-level Nix binding misplaced: %+v", symbol)
				} else {
					delete(want, symbol.Name)
				}
			}
			if len(want) != 0 {
				t.Errorf("real top-level let bindings missing: %+v", want)
			}
			if result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Errorf("Nix result attrset must still report partial coverage: %+v", result.Analysis)
			}
		})
	}
}

// Nix indentation is only formatting: an inner attribute can be written
// at the same column as the surrounding let bindings.
func TestNixNestedAttributeAtSameIndentIsNotTopLevel(t *testing.T) {
	const source = "let\n" +
		"  container = {\n" +
		"  decoy = true;\n" +
		"  };\n" +
		"  selected = 42;\n" +
		"in selected\n"
	for _, tc := range []struct{ name, text string }{
		{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (NixAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 20))
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			for _, sym := range result.Analysis.Symbols {
				if sym.Name == "decoy" {
					t.Fatalf("nested Nix key falsely classified by indentation: %+v", sym)
				}
				found[sym.Name] = true
			}
			if !found["container"] || !found["selected"] {
				t.Fatalf("top-level Nix bindings missing: %+v", result.Analysis.Symbols)
			}
		})
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
