package sourceintelligence

import (
	"context"
	"testing"
)

func TestOpaqueHostAndTemplateRegionsDoNotLeakSymbols(t *testing.T) {
	tests := []struct {
		name      string
		analyzer  SourceAnalyzer
		text      string
		want      []string
		forbidden []string
	}{
		{
			name: "vue-html-comment", analyzer: VueAnalyzer{},
			text: `<!-- <script>function fake() {}</script> -->
<script>function load() {}</script>`,
			want: []string{"load"}, forbidden: []string{"fake"},
		},
		{
			name: "astro-frontmatter-string", analyzer: AstroAnalyzer{},
			text: `---
const sample = "<script>function fake() {}</script>";
function load() {}
---
<main id="hero"></main>`,
			want: []string{"load", "hero"}, forbidden: []string{"fake"},
		},
		{
			name: "svelte-style-looking-script-string", analyzer: SvelteAnalyzer{},
			text: `<script>const sample = "<style>.fake { color: red; }</style>"; function load() {}</script>
<style>.real { color: blue; }</style>`,
			want: []string{"load", ".real"}, forbidden: []string{".fake"},
		},
		{
			name: "php-html-comment", analyzer: PHPHTMLAnalyzer{},
			text: `<!-- <?php function fake() {} ?> --><main id="hero"></main><?php function load() {} ?>`,
			want: []string{"hero", "load"}, forbidden: []string{"fake"},
		},
		{
			name: "jsp-html-comment", analyzer: JSPAnalyzer{},
			text: `<!-- <%! class Fake {} %> --><main id="hero"></main><%! class Helper {} %>`,
			want: []string{"hero", "Helper"}, forbidden: []string{"Fake"},
		},
		{
			name: "jinja-template-comment", analyzer: JinjaAnalyzer{},
			text: `{# {% macro fake() %}{% endmacro %} #}<main id="hero"></main>{% macro render() %}{% endmacro %}`,
			want: []string{"hero", "render"}, forbidden: []string{"fake"},
		},
		{
			name: "twig-template-comment", analyzer: TwigAnalyzer{},
			text: `{# {% block fake %}{% endblock %} #}<main id="hero"></main>{% block content %}{% endblock %}`,
			want: []string{"hero", "content"}, forbidden: []string{"fake"},
		},
		{
			name: "blade-comment", analyzer: BladeAnalyzer{},
			text: `{{-- @section('fake') @php function fakeFn() {} @endphp --}}<main id="hero"></main>@section('content')`,
			want: []string{"hero", "content"}, forbidden: []string{"fake", "fakeFn"},
		},
		{
			name: "ejs-html-comment", analyzer: EJSAnalyzer{},
			text: `<!-- <% function fake() {} %> --><main id="hero"></main><% function load() {} %>`,
			want: []string{"hero", "load"}, forbidden: []string{"fake"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture", tc.text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, name := range tc.want {
				if _, ok := byName[name]; !ok {
					t.Fatalf("missing %s; symbols=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			for _, name := range tc.forbidden {
				if _, ok := byName[name]; ok {
					t.Fatalf("opaque symbol %s leaked; symbols=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
		})
	}
}

func TestScriptStyleRegionsRemainInSourceOrder(t *testing.T) {
	text := `<style>.card { display: block; }</style>
<main id="hero"></main>
<script>function load() {}</script>`
	result, err := (VueAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.vue", text), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Regions) != 2 {
		t.Fatalf("regions=%+v, want 2", result.Regions)
	}
	if result.Regions[0].Kind != "style" || result.Regions[1].Kind != "script" {
		t.Fatalf("regions not in source order: %+v", result.Regions)
	}
}

func TestUnsupportedEmbeddedLanguageIsExplicitPartialRegion(t *testing.T) {
	text := `<main id="hero"></main><script lang="coffee">class Fake {}</script>`
	result, err := (VueAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.vue", text), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete {
		t.Fatalf("unsupported embedded language reported complete: %+v", result)
	}
	if len(result.Regions) != 1 || result.Regions[0].Language != "coffee" || result.Regions[0].Supported {
		t.Fatalf("unsupported region not explicit: %+v", result.Regions)
	}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Name == "Fake" {
			t.Fatalf("unsupported region leaked symbol: %+v", symbol)
		}
	}
}
