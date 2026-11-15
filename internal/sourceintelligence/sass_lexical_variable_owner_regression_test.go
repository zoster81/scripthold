package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Variable assignments remain navigable with the nearest recognized lexical
// owner rather than being misreported as unrelated file-level Sass variables.
func TestSassVariableAssignmentsRetainLexicalOwner(t *testing.T) {
	const source = "$browser-context: 16\n" +
		"@function rem($pixels, $context: $browser-context)\n" +
		"  @if unitless($pixels)\n" +
		"    $pixels: $pixels * 1px\n" +
		"  @if unitless($context)\n" +
		"    $context: $context * 1px\n" +
		"  @return $pixels / $context * 1rem\n" +
		"=round-corners($num: 4)\n" +
		"  $radius: $num\n" +
		"  @if $radius > 0\n" +
		"    $adjusted: $radius * 1px\n" +
		".card\n" +
		"  $spacing: 4px\n" +
		"  @media screen\n" +
		"    $mobile: 2px\n" +
		"  content: '$fake: 5px'\n" +
		"$outside: 10px\n" +
		".other\n" +
		"  $spacing: 8px\n" +
		"// $not-a-variable: 12px\n"

	expected := map[int]struct {
		name, parent string
	}{
		1:  {"browser-context", ""},
		4:  {"pixels", "rem"},
		6:  {"context", "rem"},
		9:  {"radius", "round-corners"},
		11: {"adjusted", "round-corners"},
		13: {"spacing", ".card"},
		15: {"mobile", ".card"},
		17: {"outside", ""},
		19: {"spacing", ".other"},
	}
	for _, variant := range []struct{ name, newline string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(variant.name, func(t *testing.T) {
			result, err := (SassAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(strings.ReplaceAll(source, "\n", variant.newline)),
				testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("bounded supported Sass declarations should remain covered: %+v", result.Analysis)
			}
			owners := map[string]NormalizedSymbol{}
			for _, symbol := range result.Analysis.Symbols {
				owners[symbol.ID] = symbol
			}
			seen := map[int]bool{}
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Kind != SymbolKindVariable {
					continue
				}
				line := symbol.DeclarationRange.Start.Line
				want, ok := expected[line]
				if !ok {
					t.Errorf("unexpected variable on line %d: %+v", line, symbol)
					continue
				}
				seen[line] = true
				if symbol.Name != want.name {
					t.Errorf("variable at line %d name=%q want=%q", line, symbol.Name, want.name)
				}
				owner := ""
				if symbol.ParentID != "" {
					parent, exists := owners[symbol.ParentID]
					if !exists {
						t.Errorf("variable %s has unknown parent %s", symbol.Name, symbol.ParentID)
					} else {
						owner = parent.Name
					}
				}
				if owner != want.parent || (want.parent == "" && symbol.ParentID != "") {
					t.Errorf("variable %s at line %d parent=%q want=%q", symbol.Name, line, owner, want.parent)
				}
			}
			for line := range expected {
				if !seen[line] {
					t.Errorf("expected variable at line %d absent", line)
				}
			}
		})
	}
}
