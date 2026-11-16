package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
			result, err := tc.analyzer.Analyze(context.Background(), testSourceDocument("fixture", tc.text), testAnalyzeOptions(true, 128))
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
	result, err := (VueAnalyzer{}).Analyze(context.Background(), testSourceDocument("fixture.vue", text), testAnalyzeOptions(true, 128))
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
	result, err := (VueAnalyzer{}).Analyze(context.Background(), testSourceDocument("fixture.vue", text), testAnalyzeOptions(true, 128))
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

// Vue's generic script attribute accepts TypeScript constraints with a
// quoted '>' (as in Record<string, number>). The original Vue playground
// VersionSelect source provides the maintenance-function oracle when present.
func TestVueQuotedGreaterThanInGenericScriptTag(t *testing.T) {
	check := func(t *testing.T, text string, want map[string]int) {
		t.Helper()
		result, err := (VueAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("VersionSelect.vue", text), testAnalyzeOptions(true, 128))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
			t.Fatalf("supported Vue generic script unexpectedly partial: %+v", result.Analysis)
		}
		scriptSupported := false
		for _, region := range result.Regions {
			if region.Kind == "script" && region.Supported {
				scriptSupported = true
			}
		}
		if !scriptSupported {
			t.Fatalf("Vue script content boundaries misdetected: %+v", result.Regions)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		for name, line := range want {
			symbol, ok := byName[name]
			if !ok || symbol.NameRange.Start.Line != line {
				t.Errorf("Vue source declaration %s:%d not found: %+v, all=%v",
					name, line, symbol, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
	}
	for _, quote := range []string{"\"", "'"} {
		t.Run(map[string]string{"\"": "double", "'": "single"}[quote], func(t *testing.T) {
			start := "<script setup lang=" + quote + "ts" + quote + " generic=" + quote +
				"T extends Record<string, number>" + quote + ">"
			fixture := start + "\nasync function fetchVersions(): Promise<string[]> { return [] }\n" +
				"function setVersion(v: string) { return v }\n</script>\n"
			for _, ending := range []string{"\n", "\r\n"} {
				t.Run(map[string]string{"\n": "LF", "\r\n": "CRLF"}[ending], func(t *testing.T) {
					check(t, strings.ReplaceAll(fixture, "\n", ending), map[string]int{
						"fetchVersions": 2, "setVersion": 3,
					})
				})
			}
		})
	}
	root := os.Getenv("SCRIPTHOLD_REAL_VUE93_CORE_ROOT")
	if root != "" {
		t.Run("source-derived-original-Vue-playground", func(t *testing.T) {
			path := filepath.Join(root, "VersionSelect.vue")
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() > 1<<20 {
				t.Fatalf("original Vue source is not a bounded regular file: %v %v", stat, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			const oldHeader = "<script setup lang=\"ts\">"
			const newHeader = "<script setup lang=\"ts\" generic=\"T extends Record<string, number>\">"
			if strings.Count(string(data), oldHeader) != 1 ||
				!strings.Contains(string(data), "async function fetchVersions(): Promise<string[]>") ||
				!strings.Contains(string(data), "function setVersion(v: string)") {
				t.Fatal("original Vue source oracle changed")
			}
			source := strings.Replace(string(data), oldHeader, newHeader, 1)
			check(t, source, map[string]int{"fetchVersions": 21, "setVersion": 55, "copyVersion": 60})
		})
	}
}

// Quoted HTML attribute values may include '>'. Astro script attributes must
// not leak the remainder of an opening tag into JavaScript/TypeScript analysis.
// The optional Starlight case changes only one script tag in original source.
func TestAstroQuotedGreaterThanInScriptAttribute(t *testing.T) {
	check := func(t *testing.T, source string, want map[string]int) AnalyzerResult {
		t.Helper()
		result, err := (AstroAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("ThemeSelect.astro", source), testAnalyzeOptions(true, 128))
		if err != nil {
			t.Fatal(err)
		}
		symbols := symbolsByQualifiedName(result.Analysis.Symbols)
		for name, line := range want {
			symbol, ok := symbols[name]
			if !ok || symbol.NameRange.Start.Line != line {
				t.Errorf("Astro script declaration %s:%d missing: %+v all=%v",
					name, line, symbol, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
		return result
	}
	for _, quote := range []string{"\"", "'"} {
		t.Run(map[string]string{"\"": "double", "'": "single"}[quote], func(t *testing.T) {
			fixture := "<script data-note=" + quote + "source > fallback" + quote + ">\n" +
				"function storeTheme() { return 'dark' }\n" +
				"function onThemeChange() { return storeTheme() }\n" +
				"</script>\n"
			for _, eol := range []string{"\n", "\r\n"} {
				t.Run(map[string]string{"\n": "LF", "\r\n": "CRLF"}[eol], func(t *testing.T) {
					result := check(t, strings.ReplaceAll(fixture, "\n", eol),
						map[string]int{"storeTheme": 2, "onThemeChange": 3})
					if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
						t.Fatalf("valid quoted Astro script attribute marked partial: %+v",
							result.Analysis.Diagnostics)
					}
					if len(result.Regions) != 1 || result.Regions[0].Kind != "script" ||
						!result.Regions[0].Supported {
						t.Fatalf("Astro script bounds incorrect: %+v", result.Regions)
					}
				})
			}
		})
	}
	if root := os.Getenv("SCRIPTHOLD_REAL_ASTRO95_STARLIGHT_ROOT"); root != "" {
		t.Run("source-derived-original-Starlight", func(t *testing.T) {
			path := filepath.Join(root, "ThemeSelect.astro")
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original Starlight source unavailable: %v %v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(raw)
			if strings.Count(source, "<script>") != 1 ||
				!strings.Contains(source, "function storeTheme(theme: Theme): void {") ||
				!strings.Contains(source, "function onThemeChange(theme: Theme): void {") {
				t.Fatal("original Starlight theme source oracle changed")
			}
			want := map[string]int{"storeTheme": 39, "onThemeChange": 50}
			baseline := check(t, source, want)
			derived := strings.Replace(source, "<script>",
				"<script data-note=\"source > fallback\">", 1)
			result := check(t, derived, want)
			if baseline.Analysis.CoverageComplete && !result.Analysis.CoverageComplete {
				t.Fatalf("valid quote in Astro script tag reduced coverage: %+v",
					result.Analysis.Diagnostics)
			}
			for _, diagnostic := range result.Analysis.Diagnostics {
				if strings.Contains(diagnostic.Code, "unterminated-string") {
					t.Fatalf("Astro header leaked into TypeScript as a string: %+v", diagnostic)
				}
			}
		})
	}
}
