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
			want: map[string]SymbolKind{"hero": SymbolKindEntity, "content": SymbolKindSection, "render": SymbolKindFunction},
		},
		{
			language: "twig", analyzer: TwigAnalyzer{},
			text: `<main id="hero"></main>{% block content %}{% macro render(value) %}{{ value }}{% endmacro %}{% endblock %}`,
			want: map[string]SymbolKind{"hero": SymbolKindEntity, "content": SymbolKindSection, "render": SymbolKindFunction},
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
