package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestProviderIdentityAndHostCoordinatePreservation(t *testing.T) {
	tests := []struct {
		language string
		analyzer SourceAnalyzer
		text     string
		want     map[string]SymbolKind
		embedded []string
	}{
		{
			language: "vue", analyzer: VueAnalyzer{},
			text: `<template><main id="hero"></main></template>
<script lang="ts">export function load(): number { return 1 }</script>
<style>.card { display: block; }</style>`,
			want:     map[string]SymbolKind{"hero": SymbolKindEntity, "load": SymbolKindFunction, ".card": SymbolKindSelector},
			embedded: []string{"load", ".card"},
		},
		{
			language: "svelte", analyzer: SvelteAnalyzer{},
			text: `<script lang="ts">export function load(): number { return 1 }</script>
<div id="hero"></div>
<style>.card { display: block; }</style>`,
			want:     map[string]SymbolKind{"hero": SymbolKindEntity, "load": SymbolKindFunction, ".card": SymbolKindSelector},
			embedded: []string{"load", ".card"},
		},
		{
			language: "astro", analyzer: AstroAnalyzer{},
			text: `---
function load() { return 1 }
---
<main id="hero"></main>
<style>.card { display: block; }</style>`,
			want:     map[string]SymbolKind{"hero": SymbolKindEntity, "load": SymbolKindFunction, ".card": SymbolKindSelector},
			embedded: []string{"load", ".card"},
		},
		{
			language: "php-html", analyzer: PHPHTMLAnalyzer{},
			text:     `<main id="hero"></main><?php function load(): int { return 1; } ?>`,
			want:     map[string]SymbolKind{"hero": SymbolKindEntity, "load": SymbolKindFunction},
			embedded: []string{"load"},
		},
		{
			language: "jsp", analyzer: JSPAnalyzer{},
			text:     `<main id="hero"></main><%! class Helper { void run() {} } %>`,
			want:     map[string]SymbolKind{"hero": SymbolKindEntity, "Helper": SymbolKindClass, "Helper.run": SymbolKindMethod},
			embedded: []string{"Helper", "Helper.run"},
		},
		{
			language: "jinja", analyzer: JinjaAnalyzer{},
			text: `<main id="hero"></main>{% block content %}{% macro render(value) %}{{ value }}{% endmacro %}{% endblock %}`,
			want: map[string]SymbolKind{"hero": SymbolKindEntity, "content": SymbolKindSection, "content.render": SymbolKindFunction},
		},
		{
			language: "twig", analyzer: TwigAnalyzer{},
			text: `<main id="hero"></main>{% block content %}{% macro render(value) %}{{ value }}{% endmacro %}{% endblock %}`,
			want: map[string]SymbolKind{"hero": SymbolKindEntity, "content": SymbolKindSection, "content.render": SymbolKindFunction},
		},
		{
			language: "blade", analyzer: BladeAnalyzer{},
			text:     `<main id="hero"></main>@section('content')@php function load(): int { return 1; } @endphp@endsection`,
			want:     map[string]SymbolKind{"hero": SymbolKindEntity, "content": SymbolKindSection, "load": SymbolKindFunction},
			embedded: []string{"load"},
		},
		{
			language: "ejs", analyzer: EJSAnalyzer{},
			text:     `<main id="hero"></main><% function load() { return 1 } %>`,
			want:     map[string]SymbolKind{"hero": SymbolKindEntity, "load": SymbolKindFunction},
			embedded: []string{"load"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			document := scientificLegacyFunctionalTestDocument("fixture", tc.text)
			result, err := tc.analyzer.Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("%s analysis partial: %+v", tc.language, result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for qualified, kind := range tc.want {
				symbol, ok := byName[qualified]
				if !ok || symbol.Kind != kind || symbol.Analyzer != string(tc.analyzer.ID()) {
					t.Fatalf("%s %s = %+v exists=%v; symbols=%v", tc.language, qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
				if wantLanguage := expectedCompositeSymbolLanguage(tc.language, qualified); symbol.Language != wantLanguage {
					t.Fatalf("%s %s language=%q want %q", tc.language, qualified, symbol.Language, wantLanguage)
				}
				offset := strings.Index(document.Text, symbol.Name)
				if offset < 0 {
					t.Fatalf("%s %s name %q missing from host", tc.language, qualified, symbol.Name)
				}
				expectedRange, err := document.RangeFromUTF8Offsets(offset, offset+len(symbol.Name))
				if err != nil {
					t.Fatal(err)
				}
				if symbol.NameRange != expectedRange {
					t.Fatalf("%s %s host range %v want %v", tc.language, qualified, symbol.NameRange, expectedRange)
				}
			}
			for _, qualified := range tc.embedded {
				if symbol := byName[qualified]; symbol.RegionID == "" {
					t.Fatalf("%s embedded symbol %s has no RegionID: %+v", tc.language, qualified, symbol)
				}
			}
		})
	}
}

func TestPHPHTMLCodeOnlyFileMayEndInsidePHPRegion(t *testing.T) {
	text := `<?php
class Demo {
    public function run(): void {}
}`
	result, err := (PHPHTMLAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.php", text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("valid code-only PHP/HTML file reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, qualified := range []string{"Demo", "Demo.run"} {
		symbol, ok := byName[qualified]
		if !ok || symbol.RegionID == "" {
			t.Fatalf("PHP/HTML EOF region symbol %q = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestPHPHTMLControlFlowMaySpanEmbeddedRegions(t *testing.T) {
	text := `<?php if ($show) { ?>
<main id="hero"></main>
<?php } ?>
<?php function after(): void {} ?>`
	result, err := (PHPHTMLAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.php", text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("PHP control flow spanning embedded regions reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	if _, ok := byName["hero"]; !ok {
		t.Fatalf("host HTML symbol missing across PHP regions: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if symbol, ok := byName["after"]; !ok || symbol.RegionID == "" {
		t.Fatalf("PHP declaration after cross-region control flow = %+v exists=%v; symbols=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestPHPHTMLClosingTagDetectionRespectsPHPLexicalContext(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "single quoted regex",
			text: `<?php $pattern = '/^(?>a|b)$/'; function afterRegex(): void {} ?>`,
			want: "afterRegex",
		},
		{
			name: "double quoted XML",
			text: `<?php $xml = "<?xml version=\"1.0\"?>"; function afterXML(): void {} ?>`,
			want: "afterXML",
		},
		{
			name: "block comment",
			text: `<?php /* fake ?> */ function afterComment(): void {} ?>`,
			want: "afterComment",
		},
		{
			name: "nowdoc body",
			text: `<?php $value = <<<'HTML'
?>
HTML;
function afterNowdoc(): void {}
?>`,
			want: "afterNowdoc",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (PHPHTMLAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.php", tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
				t.Fatalf("PHP lexical closing-tag context reported partial: %+v", result.Analysis)
			}
			if symbol, ok := symbolsByQualifiedName(result.Analysis.Symbols)[tc.want]; !ok || symbol.RegionID == "" {
				t.Fatalf("PHP declaration %q = %+v exists=%v; symbols=%v", tc.want, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}

	for _, tc := range []struct {
		name string
		text string
	}{
		{name: "slash comment closes region", text: "<?php // closes ?>\n<main id=\"hero\"></main>"},
		{name: "hash comment closes region", text: "<?php # closes ?>\n<main id=\"hero\"></main>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (PHPHTMLAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.php", tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
				t.Fatalf("PHP line-comment closing tag reported partial: %+v", result.Analysis)
			}
			if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)["hero"]; !ok {
				t.Fatalf("host HTML after PHP line-comment closing tag missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}

func TestJinjaTwigDeclarationHierarchyAndMalformedScopes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{name: "jinja", analyzer: JinjaAnalyzer{}},
		{name: "twig", analyzer: TwigAnalyzer{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := `{% block page %}
  {% block body %}<main id="hero"></main>{% endblock %}
  {% macro render(value) %}{{ value }}{% endmacro %}
{% endblock %}`
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture", text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("valid %s nested declarations reported partial: %+v", tc.name, result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			page, ok := byName["page"]
			if !ok || page.Kind != SymbolKindSection {
				t.Fatalf("%s page block = %+v exists=%v; symbols=%v", tc.name, page, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			for qualified, kind := range map[string]SymbolKind{"page.body": SymbolKindSection, "page.render": SymbolKindFunction} {
				child, exists := byName[qualified]
				if !exists || child.Kind != kind || child.ParentID != page.ID || child.ParentQualifiedName != "page" {
					t.Fatalf("%s child %s = %+v exists=%v; page=%+v symbols=%v", tc.name, qualified, child, exists, page, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}

			malformed := `{% block outer %}{% macro render() %}{% endblock %}{% endmacro %}`
			bad, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture", malformed), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if bad.Analysis.CoverageComplete || len(bad.Analysis.Diagnostics) == 0 {
				t.Fatalf("mismatched %s declaration scopes were hidden: %+v", tc.name, bad.Analysis)
			}
		})
	}
}

func TestJSPControlFlowMaySpanScriptletRegions(t *testing.T) {
	text := `<%@ page import="java.util.List" %>
<% if (request != null) { %>
<main id="hero"></main>
<%= request.getMethod() %>
<% } %>
<%! class Helper { void run() {} } %>`
	result, err := (JSPAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.jsp", text), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("valid JSP spanning scriptlet regions reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		"hero":       SymbolKindEntity,
		"Helper":     SymbolKindClass,
		"Helper.run": SymbolKindMethod,
	} {
		symbol, ok := byName[qualified]
		if !ok || symbol.Kind != kind {
			t.Fatalf("JSP %s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	if byName["Helper"].RegionID == "" || byName["Helper.run"].RegionID == "" {
		t.Fatalf("JSP declaration symbols lost embedded RegionID: %+v", result.Analysis.Symbols)
	}
}

func TestBladePHPControlFlowMaySpanDirectiveRegions(t *testing.T) {
	text := `<main id="hero">
@php if ($ready) { @endphp
<span>ready</span>
@php } else { @endphp
<span>not ready</span>
@php } @endphp
</main>
@php class Helper { public function run() {} } @endphp`
	result, err := (BladeAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.blade.php", text), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("valid Blade PHP spanning directive regions reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		"hero":       SymbolKindEntity,
		"Helper":     SymbolKindClass,
		"Helper.run": SymbolKindMethod,
	} {
		symbol, ok := byName[qualified]
		if !ok || symbol.Kind != kind {
			t.Fatalf("Blade %s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	if byName["Helper"].RegionID == "" || byName["Helper.run"].RegionID == "" {
		t.Fatalf("Blade PHP declaration symbols lost embedded RegionID: %+v", result.Analysis.Symbols)
	}
}

func TestBladeRawPHPRegionPreservesHierarchy(t *testing.T) {
	text := `<main id="hero"></main>
<?php class RawHelper { public function run() {} } ?>`
	result, err := (BladeAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.blade.php", text), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("valid Blade raw PHP region reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	hero, ok := byName["hero"]
	if !ok || hero.Kind != SymbolKindEntity {
		t.Fatalf("Blade host hero = %+v exists=%v; symbols=%v", hero, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	parent, ok := byName["RawHelper"]
	if !ok || parent.Kind != SymbolKindClass || parent.RegionID == "" {
		t.Fatalf("Blade raw PHP class = %+v exists=%v; symbols=%v", parent, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	child, ok := byName["RawHelper.run"]
	if !ok || child.Kind != SymbolKindMethod || child.ParentID != parent.ID || child.ParentQualifiedName != parent.QualifiedName || child.RegionID == "" {
		t.Fatalf("Blade raw PHP method = %+v exists=%v; parent=%+v symbols=%v", child, ok, parent, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestBladeEmbeddedPHPDelimitersRemainOpaque(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
	}{
		{
			name: "directive-contains-raw-marker",
			text: `@php $marker = "<?php class Fake { public function nope() {} } ?>"; class Real { public function run() {} } @endphp`,
		},
		{
			name: "raw-contains-directive-marker",
			text: `<?php $marker = '@php class Fake { public function nope() {} } @endphp'; class Real { public function run() {} } ?>`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (BladeAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.blade.php", tc.text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
				t.Fatalf("valid Blade embedded PHP reported partial: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			parent, ok := byName["Real"]
			if !ok || parent.Kind != SymbolKindClass {
				t.Fatalf("Blade Real = %+v exists=%v; symbols=%v", parent, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			child, ok := byName["Real.run"]
			if !ok || child.Kind != SymbolKindMethod || child.ParentID != parent.ID {
				t.Fatalf("Blade Real.run = %+v exists=%v; parent=%+v symbols=%v", child, ok, parent, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			for _, forbidden := range []string{"Fake", "Fake.nope", "nope"} {
				if _, exists := byName[forbidden]; exists {
					t.Fatalf("Blade nested delimiter marker leaked %s: %v", forbidden, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
		})
	}
}

func TestBladeVoltAnonymousClassUsesDirectiveIdentityForHierarchy(t *testing.T) {
	text := `<?php
use Livewire\Volt\Component;
new class extends Component {
    use HasConfigs;
    public $email = '';
    public function authenticate(): void {}
};
?>
@volt('auth.login')
<form wire:submit="authenticate"></form>
@endvolt`
	result, err := (BladeAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("login.blade.php", text), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("valid Blade Volt component reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	parent, ok := byName["auth.login"]
	if !ok || parent.Kind != SymbolKindEntity || parent.NativeKind != "volt-component" || parent.Language != "blade" {
		t.Fatalf("Blade Volt component = %+v exists=%v; symbols=%v", parent, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	for qualified, kind := range map[string]SymbolKind{
		"auth.login.$email":       SymbolKindProperty,
		"auth.login.authenticate": SymbolKindMethod,
	} {
		child, exists := byName[qualified]
		if !exists || child.Kind != kind || child.ParentID != parent.ID || child.ParentQualifiedName != parent.QualifiedName || child.Language != "php" || child.RegionID == "" {
			t.Fatalf("Blade Volt child %s = %+v exists=%v; parent=%+v symbols=%v", qualified, child, exists, parent, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	if !hasStructuralRelation(result.Relations, "extends", "auth.login", "Component") || !hasStructuralRelation(result.Relations, "uses-trait", "auth.login", "HasConfigs") {
		t.Fatalf("Blade Volt structural relations = %+v", result.Relations)
	}
	for _, forbidden := range []string{"extends", "extends.authenticate"} {
		if _, exists := byName[forbidden]; exists {
			t.Fatalf("Blade Volt anonymous class invented %q: %v", forbidden, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestBladeVoltAnonymousClassAssociationIsBounded(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		parent     string
		wantParent bool
	}{
		{
			name: "escaped-directive",
			text: `<?php new class extends Component { public function run() {} }; ?>
@@volt('fake')`,
			parent: "fake",
		},
		{
			name:   "directive-marker-inside-php",
			text:   `<?php $marker = "@volt('fake')"; new class extends Component { public function run() {} }; ?>`,
			parent: "fake",
		},
		{
			name: "malformed-directive",
			text: `<?php new class extends Component { public function run() {} }; ?>
@volt('fake'`,
			parent: "fake",
		},
		{
			name: "non-component-anonymous-class",
			text: `<?php new class extends Other { public function run() {} }; ?>
@volt('other')`,
			parent:     "other",
			wantParent: true,
		},
		{
			name: "later-php-region-breaks-association",
			text: `<?php new class extends Component { public function run() {} }; ?>
<?php $value = 1; ?>
@volt('late')`,
			parent:     "late",
			wantParent: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (BladeAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.blade.php", tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
				t.Fatalf("Blade Volt association case reported partial: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			_, parentExists := byName[tc.parent]
			if parentExists != tc.wantParent {
				t.Fatalf("Blade Volt parent %q exists=%v want=%v; symbols=%v", tc.parent, parentExists, tc.wantParent, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			if _, exists := byName[tc.parent+".run"]; exists {
				t.Fatalf("Blade Volt association leaked child %q: %v", tc.parent+".run", sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}

func TestBladeVoltAnonymousClassHonorsSymbolLimit(t *testing.T) {
	text := `<?php new class extends Component { public function run() {} }; ?>
@volt('limited')`
	result, err := (BladeAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.blade.php", text), testAnalyzeOptions(true, 1))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.Truncated || result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 1 {
		t.Fatalf("Blade Volt exact symbol limit did not lower coverage: %+v", result.Analysis)
	}
	if symbol := result.Analysis.Symbols[0]; symbol.QualifiedName != "limited" || symbol.Kind != SymbolKindEntity {
		t.Fatalf("Blade Volt retained symbol = %+v", symbol)
	}
}

func TestBladeVoltAnonymousClassFindsLaterRegionUnderTightSymbolBudget(t *testing.T) {
	text := `<?php $noop = 1; ?>
<?php new class extends Component { public function run() {} }; ?>
@volt('late')`
	result, err := (BladeAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.blade.php", text), testAnalyzeOptions(true, 2))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("Blade Volt later-region hierarchy reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	parent, ok := byName["late"]
	if !ok || parent.Kind != SymbolKindEntity {
		t.Fatalf("Blade Volt later-region parent = %+v exists=%v; symbols=%v", parent, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	child, ok := byName["late.run"]
	if !ok || child.Kind != SymbolKindMethod || child.ParentID != parent.ID || child.ParentQualifiedName != parent.QualifiedName {
		t.Fatalf("Blade Volt later-region child = %+v exists=%v; parent=%+v symbols=%v", child, ok, parent, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestJSPJavaClassLiteralInExecutionScriptletIsNotADeclaration(t *testing.T) {
	text := `<%! class Environment { String value; String getValue() { return value; } } %>
<%
Environment env = new Environment();
String json = gson.toJson(env, Environment.class);
response.setContentType("application/json");
%>`
	result, err := (JSPAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.jsp", text), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("valid JSP class literal reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		"Environment":          SymbolKindClass,
		"Environment.value":    SymbolKindField,
		"Environment.getValue": SymbolKindMethod,
	} {
		symbol, ok := byName[qualified]
		if !ok || symbol.Kind != kind {
			t.Fatalf("JSP %s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func expectedCompositeSymbolLanguage(provider, qualified string) string {
	switch provider {
	case "vue", "svelte", "astro":
		switch qualified {
		case "load":
			return "typescript"
		case ".card":
			return "css"
		default:
			return "html"
		}
	case "php-html":
		if qualified == "load" {
			return "php"
		}
		return "html"
	case "jsp":
		if qualified == "hero" {
			return "html"
		}
		return "java"
	case "jinja", "twig":
		if qualified == "hero" {
			return "html"
		}
		return provider
	case "blade":
		switch qualified {
		case "hero":
			return "html"
		case "load":
			return "php"
		default:
			return "blade"
		}
	case "ejs":
		if qualified == "hero" {
			return "html"
		}
		return "javascript"
	default:
		return provider
	}
}

func TestExistingDotNetCompositesKeepHostCoordinates(t *testing.T) {
	tests := []struct {
		language string
		analyzer SourceAnalyzer
		text     string
		want     string
	}{
		{"aspnet-webforms", ASPNetWebFormsAnalyzer{}, `<script runat="server" language="C#">public void Run() {}</script>`, "Run"},
		{"razor", RazorAnalyzer{}, `@functions { public void Run() {} }`, "Run"},
		{"blazor", BlazorAnalyzer{}, `@code { public void Run() {} }`, "Run"},
	}
	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			document := scientificLegacyFunctionalTestDocument("fixture", tc.text)
			result, err := tc.analyzer.Analyze(context.Background(), document, testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Name != tc.want {
					continue
				}
				found = true
				if symbol.RegionID == "" {
					t.Fatalf("%s embedded symbol has no RegionID: %+v", tc.language, symbol)
				}
				offset := strings.Index(document.Text, tc.want)
				if offset < 0 {
					t.Fatalf("%s missing host text %q", tc.language, tc.want)
				}
				expectedRange, err := document.RangeFromUTF8Offsets(offset, offset+len(tc.want))
				if err != nil {
					t.Fatal(err)
				}
				if symbol.NameRange != expectedRange {
					t.Fatalf("%s host range %v want %v", tc.language, symbol.NameRange, expectedRange)
				}
			}
			if !found {
				t.Fatalf("%s missing %s; symbols=%v", tc.language, tc.want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}
