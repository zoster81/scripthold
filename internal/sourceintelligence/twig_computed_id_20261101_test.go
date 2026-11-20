package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestTemplateComputedHTMLIDsAreNotStaticEntities(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{"twig", TwigAnalyzer{}},
		{"jinja", JinjaAnalyzer{}},
	} {
		for _, eol := range []struct {
			name  string
			value string
		}{
			{"lf", "\n"},
			{"crlf", "\r\n"},
		} {
			t.Run(tc.name+"-"+eol.name, func(t *testing.T) {
				source := strings.Join([]string{
					`<main id="fixed"></main>`,
					`<a id="help@example.com"></a>`,
					`<div id="{{ _id }}-heading-{{ loop.index }}"></div>`,
					`<aside id='prefix-{{ item.id }}'></aside>`,
					`<button class="{{ dynamic }}" id="after"></button>`,
					`{% block content %}<p id="inside"></p>{% endblock %}`,
				}, eol.value)
				res, err := tc.analyzer.Analyze(context.Background(), testSourceDocument("fixture.twig", source), testAnalyzeOptions(true, 128))
				if err != nil {
					t.Fatal(err)
				}
				byName := symbolsByQualifiedName(res.Analysis.Symbols)
				for _, name := range []string{"fixed", "help@example.com", "after", "inside", "content"} {
					if _, ok := byName[name]; !ok {
						t.Errorf("missing literal id or template declaration %q; symbols=%v", name, sortedSymbolQualifiedNames(res.Analysis.Symbols))
					}
				}
				for _, name := range []string{"-heading-", "prefix-"} {
					if sym, ok := byName[name]; ok {
						t.Errorf("dynamic HTML id was reported as static entity: %+v", sym)
					}
				}
				if !res.Analysis.CoverageComplete || res.Analysis.Truncated {
					t.Errorf("valid template became partial: %+v", res.Analysis)
				}
			})
		}
	}
}

func TestTwigOriginalComputedIDIsNotStatic(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_TWIG_COMPUTED_ID_ORIGINAL")
	if path == "" {
		t.Skip("optional operator-provided original Twig fixture")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := (TwigAnalyzer{}).Analyze(context.Background(), testSourceDocument(path, string(data)), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	names := symbolsByQualifiedName(res.Analysis.Symbols)
	if _, exists := names["accordion"]; !exists {
		t.Errorf("original Twig macro missing: %v", sortedSymbolQualifiedNames(res.Analysis.Symbols))
	}
	if symbol, exists := names["-heading-"]; exists {
		t.Errorf("original dynamic Twig id falsely indexed as static: %+v", symbol)
	}
}
