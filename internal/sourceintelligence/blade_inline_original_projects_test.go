package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestBladeQualifiedOriginalInlineSections(t *testing.T) {
	cases := []struct {
		name, env string
		want      map[string]int
	}{
		{"TrianglePOS", "SCRIPTHOLD_REAL_BLADE_TRIANGLE_CREATE", map[string]int{"title": 3, "breadcrumb": 5, "content": 13}},
		{"LaravelFramework", "SCRIPTHOLD_REAL_BLADE_FRAMEWORK_401", map[string]int{"title": 3, "code": 4, "message": 5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("original qualified Blade source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatal("qualified Blade file must be regular and at most 1 MiB")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			doc := sourceDocumentForScanner(string(data))
			doc.Path = path
			result, err := (BladeAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("qualified Blade source incomplete: %+v", result.Analysis)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, line := range tc.want {
				got, ok := symbols[name]
				if !ok || got.Kind != SymbolKindSection || got.NameRange.Start.Line != line {
					t.Errorf("section %s found=%v got=%+v want line=%d", name, ok, got, line)
				}
			}
		})
	}
}

func TestBladeInlineSectionsWithNestedExpressionsAndLineEndings(t *testing.T) {
	const source = "@section('title', __('Unauthorized'))\n@section('code', '401')\n@section('message', sprintf('%s', $value))\n@section('body')\n@@section('fake', 1)\n{{-- @section('commented', 1) --}}\n"
	for _, tc := range []struct{ name, text string }{{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (BladeAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("incomplete Blade sections: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, line := range map[string]int{"title": 1, "code": 2, "message": 3, "body": 4} {
				sym, ok := byName[name]
				if !ok || sym.Kind != SymbolKindSection || sym.NameRange.Start.Line != line {
					t.Errorf("%s missing or incorrect: present=%v symbol=%+v", name, ok, sym)
				}
			}
			for _, name := range []string{"fake", "commented"} {
				if sym, ok := byName[name]; ok {
					t.Errorf("literal/comment created phantom section %s: %+v", name, sym)
				}
			}
		})
	}
}
