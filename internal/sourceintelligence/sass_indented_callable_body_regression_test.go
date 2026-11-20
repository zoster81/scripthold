package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Indented Sass function and mixin declarations own their authored bodies,
// without absorbing neighboring declarations or treating +mixin calls as definitions.
func TestSassIndentedCallableDeclarationIncludesBody(t *testing.T) {
	const source = "=round-corners($num: 4)\n" +
		"  @extend .round-corners-4\n" +
		"  +border-radius($num)\n" +
		"=round-top-corners($num: 4)\n" +
		"  +round-corners($num)\n" +
		"@function rem($pixels)\n" +
		"  @if unitless($pixels)\n" +
		"    $pixels: $pixels * 1px\n" +
		"  @return $pixels / 16px * 1rem\n" +
		".card\n" +
		"  +round-top-corners(4px)\n" +
		"// =fake($v)\n" +
		"+round-corners(2px)\n"

	for _, variant := range []struct{ name, newline string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(variant.name, func(t *testing.T) {
			result, err := (SassAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(strings.ReplaceAll(source, "\n", variant.newline)),
				testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("valid Sass definitions must stay covered: %+v", result.Analysis)
			}
			want := map[string]struct {
				start, end int
				header     string
			}{
				"round-corners":     {1, 3, "=round-corners($num: 4)"},
				"round-top-corners": {4, 5, "=round-top-corners($num: 4)"},
				"rem":               {6, 9, "@function rem($pixels)"},
			}
			seen := make(map[string]bool)
			for _, symbol := range result.Analysis.Symbols {
				expected, ok := want[symbol.Name]
				if !ok {
					if symbol.Kind == SymbolKindFunction {
						t.Errorf("unexpected Sass function/mixin definition: %+v", symbol)
					}
					continue
				}
				seen[symbol.Name] = true
				if symbol.Kind != SymbolKindFunction ||
					symbol.DeclarationRange.Start.Line != expected.start ||
					symbol.DeclarationRange.End.Line != expected.end {
					t.Errorf("wrong Sass function/mixin declaration boundaries for %q: %+v", symbol.Name, symbol)
				}
				if symbol.Signature != expected.header {
					t.Errorf("outline must retain only the original header for %q: %q", symbol.Name, symbol.Signature)
				}
			}
			for name := range want {
				if !seen[name] {
					t.Errorf("Sass function/mixin %q absent: %+v", name, result.Analysis.Symbols)
				}
			}
		})
	}
}
