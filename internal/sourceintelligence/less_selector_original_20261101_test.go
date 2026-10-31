package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestLessOriginalSelectorSyntaxAndCommentBoundaries(t *testing.T) {
	cases := []struct {
		name, source, want string
		line               int
		forbidden          []string
	}{
		{name: "less line comments", source: "// Button description\n// more text\n.Button { color: red; }\n", want: ".Button", line: 3, forbidden: []string{"// Button description\n// more text\n.Button"}},
		{name: "less interpolation", source: "@prefix: 'ant';\n.@{prefix}-alert { color: red; }\n", want: ".@{prefix}-alert", line: 2, forbidden: []string{".@", "-alert"}},
		{name: "less mixin semicolon arguments", source: ".button-size(@padding; @font-size;) { padding: @padding; font-size: @font-size; }\n", want: ".button-size(@padding; @font-size;)", line: 1, forbidden: []string{")"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, eol := range []string{"\n", "\r\n"} {
				result, err := (LessAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(strings.ReplaceAll(tc.source, "\n", eol)), testAnalyzeOptions(true, 256))
				if err != nil {
					t.Fatal(err)
				}
				if !result.Analysis.CoverageComplete {
					t.Fatalf("partial: %+v", result.Analysis.Diagnostics)
				}
				found := symbolsByQualifiedName(result.Analysis.Symbols)
				want, ok := found[tc.want]
				if !ok || want.Kind != SymbolKindSelector || want.NameRange.Start.Line != tc.line {
					t.Errorf("selector %q on line %d not found: %+v; symbols=%v", tc.want, tc.line, want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
				for _, name := range tc.forbidden {
					if _, ok := found[name]; ok {
						t.Errorf("spurious selector %q", name)
					}
				}
			}
		})
	}
}
