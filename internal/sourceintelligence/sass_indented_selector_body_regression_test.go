package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Indented Sass selectors own the direct and nested source needed to maintain
// their layout; siblings and unrelated top-level blocks must remain separate.
func TestSassIndentedSelectorDeclarationIncludesBody(t *testing.T) {
	const source = ".hbox\n" +
		"  +display-box\n" +
		"  +box-orient\n" +
		"  & > *\n" +
		"    +box-flex\n" +
		"  // a nested source comment\n" +
		"\n" +
		".vbox\n" +
		"  +box-orient(vertical)\n" +
		"\n" +
		"#app\n" +
		"  color: $text\n" +
		"  @media (max-width: $break-small)\n" +
		"    width: 250px\n" +
		"  box-shadow: 0px 10px $shadow\n" +
		".secondary\n" +
		"  color: black\n"
	for _, variant := range []struct{ name, newline string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(variant.name, func(t *testing.T) {
			result, err := (SassAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(strings.ReplaceAll(source, "\n", variant.newline)),
				testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("valid Sass must remain covered: %+v", result.Analysis)
			}
			want := map[string]struct{ start, end int }{
				".hbox":      {1, 5},
				"& > *":      {4, 5},
				".vbox":      {8, 9},
				"#app":       {11, 15},
				".secondary": {16, 17},
			}
			seen := make(map[string]bool)
			for _, symbol := range result.Analysis.Symbols {
				expected, ok := want[symbol.Name]
				if !ok {
					continue
				}
				seen[symbol.Name] = true
				if symbol.Kind != SymbolKindSelector ||
					symbol.DeclarationRange.Start.Line != expected.start ||
					symbol.DeclarationRange.End.Line != expected.end {
					t.Errorf("Sass selector %q must own its exact indented block: %+v", symbol.Name, symbol)
				}
				if strings.Contains(symbol.Signature, "\n") {
					t.Errorf("outline signature for %q must stay header-only: %q", symbol.Name, symbol.Signature)
				}
			}
			for name := range want {
				if !seen[name] {
					t.Errorf("expected selector %q absent: %+v", name, result.Analysis.Symbols)
				}
			}
		})
	}
}

func TestSassSelectorBodyStopsBeforePeerAtSameIndentWithTabs(t *testing.T) {
	const source = ".outer\n" +
		"\tcolor: red\n" +
		"\t.nested\n" +
		"\t\tcolor: blue\n" +
		"\t.other\n" +
		"\t\tcolor: green\n" +
		".next\n" +
		"\tcolor: black\n"
	for _, newline := range []string{"\n", "\r\n"} {
		result, err := (SassAnalyzer{}).Analyze(context.Background(),
			sourceDocumentForScanner(strings.ReplaceAll(source, "\n", newline)),
			testAnalyzeOptions(true, 128))
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]int{".outer": 6, ".nested": 4, ".other": 6, ".next": 8}
		for _, symbol := range result.Analysis.Symbols {
			if end, ok := want[symbol.Name]; ok && symbol.DeclarationRange.End.Line != end {
				t.Errorf("tab-indented peer should not leak into %s: %+v", symbol.Name, symbol)
			}
		}
	}
}
