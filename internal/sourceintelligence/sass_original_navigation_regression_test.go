package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Indented Sass has bare nested selectors and declarations beginning with '='.
// Mixin calls ('+') and CSS properties are not definitions.
func TestSassIndentedOriginalNavigation(t *testing.T) {
	const fixture = "body\n  padding: 50px\nheader\n  img\n    max-width: 88%\n=round-corners($num: 4)\n  @extend .corner\n=reset\n  display: none\n.card\n  +round-corners(5)\n  &:hover\n    color: red\n"
	for _, tc := range []struct {
		name, eol string
	}{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			source := strings.ReplaceAll(fixture, "\n", tc.eol)
			doc := sourceDocumentForScanner(source)
			doc.Path = "original.sass"
			result, err := (SassAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 80))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("unexpected coverage: %+v", result.Analysis)
			}
			want := map[string]struct {
				kind   SymbolKind
				native string
			}{
				"body":          {SymbolKindSelector, "selector"},
				"header":        {SymbolKindSelector, "selector"},
				"img":           {SymbolKindSelector, "selector"},
				"round-corners": {SymbolKindFunction, "mixin"},
				"reset":         {SymbolKindFunction, "mixin"},
				".card":         {SymbolKindSelector, "selector"},
				"&:hover":       {SymbolKindSelector, "selector"},
			}
			for _, symbol := range result.Analysis.Symbols {
				expected, exists := want[symbol.Name]
				if !exists {
					t.Errorf("false structural declaration %q (%s)", symbol.Name, symbol.NativeKind)
					continue
				}
				if symbol.Kind != expected.kind || symbol.NativeKind != expected.native {
					t.Errorf("%s kind/native=%s/%s, want %s/%s", symbol.Name, symbol.Kind, symbol.NativeKind, expected.kind, expected.native)
				}
				delete(want, symbol.Name)
			}
			if len(want) > 0 {
				t.Errorf("missing indented Sass declarations: %v", want)
			}
		})
	}
}

func TestSassMalformedMixinRemainsIncomplete(t *testing.T) {
	for _, text := range []string{
		"=bad($v))\n  color: red\n",
		"=bad($v\n  color: red\n",
		"=bad($v) trailing\n  color: red\n",
	} {
		doc := sourceDocumentForScanner(text)
		doc.Path = "malformed.sass"
		result, err := (SassAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 30))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
			t.Errorf("malformed mixin must not claim complete analysis: %+v", result.Analysis)
		}
		for _, symbol := range result.Analysis.Symbols {
			if symbol.Name == "bad" {
				t.Errorf("malformed mixin incorrectly indexed: %+v", symbol)
			}
		}
	}
}
