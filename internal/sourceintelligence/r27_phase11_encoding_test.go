package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestR27Phase11ConformanceAcrossEncodingsAndDeterminism(t *testing.T) {
	tests := []struct {
		name, extension, encoding, text string
		bom                             bool
		analyzer                        SourceAnalyzer
		want                            []string
	}{
		{name: "vue-windows1252-crlf", extension: ".vue", encoding: "windows-1252", analyzer: VueAnalyzer{}, text: "<!-- café -->\r\n<main id=\"hero\">Résumé</main>\r\n<script lang=\"ts\">function load() {}</script>\r\n<style>.card { display: block; }</style>\r\n", want: []string{"hero", "load", ".card"}},
		{name: "svelte-utf16le", extension: ".svelte", encoding: "utf-16le", bom: true, analyzer: SvelteAnalyzer{}, text: "<main id=\"hero\">Résumé</main>\n<script>function load() {}</script>\n<style>.card { display: block; }</style>\n", want: []string{"hero", "load", ".card"}},
		{name: "astro-utf16be", extension: ".astro", encoding: "utf-16be", bom: true, analyzer: AstroAnalyzer{}, text: "---\n// résumé\nfunction load() {}\n---\n<main id=\"hero\">Café</main>\n<style>.card { display: block; }</style>\n", want: []string{"load", "hero", ".card"}},
		{name: "php-html-utf32le", extension: ".php", encoding: "utf-32le", bom: true, analyzer: PHPHTMLAnalyzer{}, text: "<main id=\"hero\">Résumé</main>\n<?php function load() {} ?>\n", want: []string{"hero", "load"}},
		{name: "jsp-windows1252-crlf", extension: ".jsp", encoding: "windows-1252", analyzer: JSPAnalyzer{}, text: "<main id=\"hero\">Café</main>\r\n<%! class Helper { void load() {} } %>\r\n", want: []string{"hero", "Helper"}},
		{name: "jinja-utf16le", extension: ".jinja", encoding: "utf-16le", bom: true, analyzer: JinjaAnalyzer{}, text: "<main id=\"hero\">Résumé</main>\n{% macro render() %}café{% endmacro %}\n", want: []string{"hero", "render"}},
		{name: "twig-utf16be", extension: ".twig", encoding: "utf-16be", bom: true, analyzer: TwigAnalyzer{}, text: "<main id=\"hero\">Résumé</main>\n{% block content %}café{% endblock %}\n", want: []string{"hero", "content"}},
		{name: "blade-utf32le", extension: ".blade.php", encoding: "utf-32le", bom: true, analyzer: BladeAnalyzer{}, text: "<main id=\"hero\">Résumé</main>\n@section('content')\n@php function load() {} @endphp\n", want: []string{"hero", "content", "load"}},
		{name: "ejs-windows1252-crlf", extension: ".ejs", encoding: "windows-1252", analyzer: EJSAnalyzer{}, text: "<main id=\"hero\">Café</main>\r\n<% function load() {} %>\r\n", want: []string{"hero", "load"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture"+tc.extension)
			if err := os.WriteFile(path, encodeSourceFixture(t, tc.encoding, tc.text, tc.bom), 0o600); err != nil {
				t.Fatal(err)
			}
			document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
				RequestedEncoding: tc.encoding, MaxFileBytes: 4 * 1024 * 1024, MaxDecodedCharacters: 1_000_000,
			})
			if err != nil {
				t.Fatal(err)
			}
			first, err := tc.analyzer.Analyze(context.Background(), document, phase3AnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			second, err := tc.analyzer.Analyze(context.Background(), document, phase3AnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("%s output is nondeterministic", tc.analyzer.Language())
			}
			if !first.Analysis.CoverageComplete || first.Analysis.Truncated {
				t.Fatalf("%s conformance partial: %+v", tc.analyzer.Language(), first.Analysis)
			}
			names := sortedSymbolQualifiedNames(first.Analysis.Symbols)
			for _, want := range tc.want {
				if !containsSortedString(names, want) {
					t.Fatalf("%s missing %s; symbols=%v", tc.analyzer.Language(), want, names)
				}
			}
			for _, symbol := range first.Analysis.Symbols {
				if symbol.NameRange.Start.Line < 1 || symbol.NameRange.Start.Column < 1 || symbol.NameRange.End.Line < symbol.NameRange.Start.Line {
					t.Fatalf("%s invalid host range: %+v", tc.analyzer.Language(), symbol)
				}
			}
		})
	}
}
