package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestStylesheetSelectorOffsetsAfterEarlierDeclarations(t *testing.T) {
	for _, tc := range []struct {
		language   string
		analyzer   AnalyzerID
		scss, less bool
	}{
		{language: "css", analyzer: AnalyzerCSS},
		{language: "scss", analyzer: AnalyzerSCSS, scss: true},
		{language: "less", analyzer: AnalyzerLess, less: true},
	} {
		t.Run(tc.language, func(t *testing.T) {
			for _, eol := range []string{"\n", "\r\n"} {
				source := strings.ReplaceAll("a { x: y; }\n.some-long-selector { display: block; }\n", "\n", eol)
				result, err := analyzeCSSDocument(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 256), tc.language, tc.analyzer, tc.scss, tc.less)
				if err != nil {
					t.Fatal(err)
				}
				found := symbolsByQualifiedName(result.Analysis.Symbols)
				got, ok := found[".some-long-selector"]
				if !ok || got.Kind != SymbolKindSelector || got.NameRange.Start.Line != 2 {
					t.Errorf("second selector should be on line 2, got %+v; all=%v", got, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
		})
	}
}

func TestLessUnclosedInterpolationHasBoundedMasking(t *testing.T) {
	source := strings.Repeat("@{", 8192)
	got := maskLessSelectorPunctuation(source)
	if got != source {
		t.Fatal("unclosed interpolation must not alter source positions")
	}
}

func TestLessQualifiedOriginalSelectorFixtures(t *testing.T) {
	cases := []struct {
		env, name string
		line      int
	}{
		{"SCRIPTHOLD_REAL_LESS_ALERT_20261101", ".@{prefix}-alert", 3},
		{"SCRIPTHOLD_REAL_LESS_MIXIN_20261101", ".button-size(@padding; @font-size;)", 4},
		{"SCRIPTHOLD_REAL_LESS_BUTTON_20261101", ".Button", 11},
	}
	for _, tc := range cases {
		t.Run(tc.env, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("qualified real corpus fixture not configured")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (LessAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(string(data)), testAnalyzeOptions(true, 2048))
			if err != nil {
				t.Fatal(err)
			}
			found := symbolsByQualifiedName(result.Analysis.Symbols)
			got, ok := found[tc.name]
			if !ok || got.Kind != SymbolKindSelector || got.NameRange.Start.Line != tc.line {
				t.Errorf("qualified original selector %q at line %d: got %+v; all=%v", tc.name, tc.line, got, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}
