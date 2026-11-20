package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestECMAScriptFinalObjectBindingBeforeClosingNewline(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
		source   string
		expected map[string]int
		partial  bool
	}{
		{"javascript-lf", JavaScriptAnalyzer{}, "const { first,\n last\n} = read();\n", map[string]int{"first": 1, "last": 2}, false},
		{"javascript-crlf", JavaScriptAnalyzer{}, strings.ReplaceAll("const { first,\n last\n} = read();\n", "\n", "\r\n"), map[string]int{"first": 1, "last": 2}, false},
		{"javascript-bom", JavaScriptAnalyzer{}, "\uFEFFconst { first,\n last\n} = read();\n", map[string]int{"first": 1, "last": 2}, false},
		{"typescript-typed", TypeScriptAnalyzer{}, "let { first,\n last\n}: Params = $props();\n", map[string]int{"first": 1, "last": 2}, false},
		{"javascript-alias-and-default", JavaScriptAnalyzer{}, "const { head: first,\n last = false\n} = read();\n", map[string]int{"first": 1, "last": 2}, false},
		{"javascript-trailing-comma", JavaScriptAnalyzer{}, "const { first,\n last,\n} = read();\n", map[string]int{"first": 1, "last": 2}, false},
		{"typescript-nested-partial", TypeScriptAnalyzer{}, "const { nested: { inner }, safe\n} = read();\n", map[string]int{"safe": 1}, true},
		{"svelte-typescript", SvelteAnalyzer{}, "<script lang=\"ts\">\nlet { first,\n last\n}: Params = $props();\n</script>\n", map[string]int{"first": 2, "last": 3}, false},
		{"vue-typescript", VueAnalyzer{}, "<script lang=\"ts\">\nlet { first,\n last\n}: Params = read();\n</script>\n", map[string]int{"first": 2, "last": 3}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The decoding boundary removes a UTF-8 BOM before analysis.
			doc := sourceDocumentForScanner(strings.TrimPrefix(tc.source, "\uFEFF"))
			result, err := tc.analyzer.Analyze(context.Background(), doc, testAnalyzeOptions(true, 40))
			if err != nil {
				t.Fatal(err)
			}
			symbols := result.Analysis.Symbols
			if len(symbols) != len(tc.expected) {
				t.Errorf("number of authored bindings=%d, actual=%v", len(tc.expected), sortedSymbolQualifiedNames(symbols))
			}
			for name, line := range tc.expected {
				found := 0
				for _, symbol := range symbols {
					if symbol.Name == name && symbol.NameRange.Start.Line == line {
						found++
					}
				}
				if found != 1 {
					t.Errorf("authored %s at %d has %d symbols; actual=%v", name, line, found,
						sortedSymbolQualifiedNames(symbols))
				}
			}
			if result.Analysis.Truncated || result.Analysis.CoverageComplete == tc.partial {
				t.Errorf("coverage misreported for supported/unsupported pattern: %+v", result.Analysis)
			}
		})
	}
}
