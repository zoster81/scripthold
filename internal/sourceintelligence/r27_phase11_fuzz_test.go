package sourceintelligence

import (
	"context"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func FuzzR27Phase11AnalyzersNoPanic(f *testing.F) {
	seeds := []struct {
		text     string
		selector uint8
	}{
		{`<main id="hero"></main><script lang="ts">function load() {}</script><style>.card {}</style>`, 0},
		{`<main id="hero"></main><script>function load() {}</script>`, 1},
		{"---\nfunction load() {}\n---\n<main id=\"hero\"></main>\n", 2},
		{`<main id="hero"></main><?php function load() {} ?>`, 3},
		{`<main id="hero"></main><%! class Helper {} %>`, 4},
		{`<main id="hero"></main>{% macro render() %}{% endmacro %}`, 5},
		{`<main id="hero"></main>{% block content %}{% endblock %}`, 6},
		{`<main id="hero"></main>@section('content') @php function load() {} @endphp`, 7},
		{`<main id="hero"></main><% function load() {} %>`, 8},
	}
	for _, seed := range seeds {
		f.Add(seed.text, seed.selector)
	}
	f.Fuzz(func(t *testing.T, text string, selector uint8) {
		analyzers := []SourceAnalyzer{
			VueAnalyzer{}, SvelteAnalyzer{}, AstroAnalyzer{}, PHPHTMLAnalyzer{}, JSPAnalyzer{},
			JinjaAnalyzer{}, TwigAnalyzer{}, BladeAnalyzer{}, EJSAnalyzer{},
		}
		analyzer := analyzers[int(selector)%len(analyzers)]
		result, err := analyzer.Analyze(context.Background(), phase9TestDocument("fuzz.fixture", text), phase3AnalyzeOptions(false, 128))
		if err != nil {
			if kind := operation.KindOf(err); kind != operation.KindInvalidInput && kind != operation.KindLimit && kind != operation.KindUnsupported {
				t.Fatalf("unexpected %s fuzz error: %v kind=%v", analyzer.Language(), err, kind)
			}
			return
		}
		if len(result.Analysis.Symbols) > 128 || len(result.Regions) > 128 || len(result.Dependencies) > 128 || len(result.Relations) > 128 {
			t.Fatalf("%s fuzz result exceeded bound: symbols=%d regions=%d deps=%d relations=%d", analyzer.Language(), len(result.Analysis.Symbols), len(result.Regions), len(result.Dependencies), len(result.Relations))
		}
		for _, symbol := range result.Analysis.Symbols {
			if symbol.Name == "" || symbol.QualifiedName == "" || symbol.DeclarationRange.Start.Line <= 0 || symbol.DeclarationRange.Start.Column <= 0 || symbol.DeclarationRange.End.Line <= 0 || symbol.DeclarationRange.End.Column <= 0 || symbol.NameRange.Start.Line <= 0 || symbol.NameRange.Start.Column <= 0 {
				t.Fatalf("%s fuzz emitted invalid symbol: %+v", analyzer.Language(), symbol)
			}
		}
		for _, region := range result.Regions {
			if region.ID == "" || region.Kind == "" || region.Range.Start.Line <= 0 || region.Range.Start.Column <= 0 || region.Range.End.Line <= 0 || region.Range.End.Column <= 0 {
				t.Fatalf("%s fuzz emitted invalid region: %+v", analyzer.Language(), region)
			}
		}
	})
}
