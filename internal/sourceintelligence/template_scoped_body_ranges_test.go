package sourceintelligence

import (
	"context"
	"testing"
)

func TestTemplateMatchedBlockAndMacroExposeCompleteSourceScopes(t *testing.T) {
	src := `{% block toolbar %}
{% macro render_badge(value) %}
<span>{{ value }}</span>
{% endmacro %}
{{ render_badge('active') }}
{% endblock %}
{% block unrelated %}ok{% endblock %}
`
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{"twig", TwigAnalyzer{}},
		{"jinja", JinjaAnalyzer{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := sourceDocumentForScanner(src)
			result, err := tc.analyzer.Analyze(context.Background(), document, testAnalyzeOptions(true, 50))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("valid template incomplete: %+v", result.Analysis)
			}
			syms := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, expect := range []struct {
				name       string
				start, end int
			}{
				{"toolbar", 1, 6},
				{"toolbar.render_badge", 2, 4},
				{"unrelated", 7, 7},
			} {
				sym, exists := syms[expect.name]
				if !exists || sym.NameRange.Start.Line != expect.start ||
					sym.DeclarationRange.End.Line != expect.end ||
					sym.BodyRange == nil {
					t.Errorf("%s: expected complete paired scope %d-%d; exists=%v sym=%+v", expect.name, expect.start, expect.end, exists, sym)
				}
			}
		})
	}
}

func TestTemplateFirstRetainedScopeKeepsBodyUnderSymbolLimit(t *testing.T) {
	src := "{% block outer %}\n{% block inner %}\nvalue\n{% endblock %}\n{% endblock %}\n"
	for _, tc := range []SourceAnalyzer{TwigAnalyzer{}, JinjaAnalyzer{}} {
		result, err := tc.Analyze(context.Background(), sourceDocumentForScanner(src), testAnalyzeOptions(true, 1))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.Truncated || len(result.Analysis.Symbols) != 1 {
			t.Fatalf("%s: expected one retained symbol and explicit truncation: %+v", tc.Language(), result.Analysis)
		}
		sym := result.Analysis.Symbols[0]
		if sym.Name != "outer" || sym.BodyRange == nil || sym.DeclarationRange.End.Line != 5 {
			t.Fatalf("%s: first retained outer block lost its complete body: %+v", tc.Language(), sym)
		}
	}
}

func TestTemplateMismatchedNamedBlockRemainsIncomplete(t *testing.T) {
	src := `{% block toolbar %}first{% endblock incorrect %}`
	for _, tc := range []SourceAnalyzer{TwigAnalyzer{}, JinjaAnalyzer{}} {
		result, err := tc.Analyze(context.Background(), sourceDocumentForScanner(src), testAnalyzeOptions(true, 20))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.CoverageComplete {
			t.Fatalf("%s: mismatched named block erroneously complete", tc.Language())
		}
		for _, sym := range result.Analysis.Symbols {
			if sym.BodyRange != nil || sym.DeclarationRange.End.Line != 1 {
				t.Errorf("%s: borrowed invalid block closer: %+v", tc.Language(), sym)
			}
		}
	}
}
