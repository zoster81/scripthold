package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestR27Phase11SourceSymbolsRoutesCompositeTemplateProviders(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	files := map[string]string{
		"Widget.vue":     `<main id="VueHost"></main><script lang="ts">function VueBox() {}</script><style>.vue-card { display: block; }</style>`,
		"Widget.svelte":  `<main id="SvelteHost"></main><script>function SvelteBox() {}</script>`,
		"Widget.astro":   "---\nfunction AstroBox() {}\n---\n<main id=\"AstroHost\"></main>\n",
		"mixed.php":      `<main id="PHPHost"></main><?php function PHPBox() {} ?>`,
		"page.jsp":       `<main id="JSPHost"></main><%! class JSPBox {} %>`,
		"view.jinja":     `<main id="JinjaHost"></main>{% macro JinjaBox() %}{% endmacro %}`,
		"view.twig":      `<main id="TwigHost"></main>{% block TwigBox %}{% endblock %}`,
		"view.blade.php": `<main id="BladeHost"></main>@section('BladeBox')`,
		"view.ejs":       `<main id="EJSHost"></main><% function EJSBox() {} %>`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	h := NewHandler([]string{filepath.Dir(root)})
	toolErr, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{root}, Encoding: "utf-8", IncludeSignatures: true, MaxSymbols: 512,
	})
	if err != nil || toolErr != nil {
		t.Fatalf("Phase 11 outline err=%v toolErr=%+v", err, toolErr)
	}
	if result.FilesConsidered != len(files) || result.FilesParsed != len(files) || result.FilesSkipped != 0 || !result.CoverageComplete {
		t.Fatalf("Phase 11 outline summary = %+v", result)
	}

	wantLanguages := map[string]bool{
		"vue": false, "svelte": false, "astro": false, "php-html": false, "jsp": false,
		"jinja": false, "twig": false, "blade": false, "ejs": false,
	}
	for _, file := range result.Files {
		if file.ErrorCode != "" || file.Detection.Language == "" {
			t.Fatalf("Phase 11 file routing = %+v", file)
		}
		if _, expected := wantLanguages[file.Detection.Language]; expected {
			wantLanguages[file.Detection.Language] = true
		}
	}
	for language, found := range wantLanguages {
		if !found {
			t.Fatalf("missing auto-routed Phase 11 language %s: %+v", language, result.Files)
		}
	}

	for _, name := range []string{
		"VueHost", "VueBox", ".vue-card", "SvelteHost", "SvelteBox", "AstroBox", "AstroHost",
		"PHPHost", "PHPBox", "JSPHost", "JSPBox", "JinjaHost", "JinjaBox", "TwigHost", "TwigBox",
		"BladeHost", "BladeBox", "EJSHost", "EJSBox",
	} {
		found := false
		for _, symbol := range result.Symbols {
			if symbol.QualifiedName == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing public Phase 11 symbol %s; symbols=%+v", name, result.Symbols)
		}
	}

	for name, language := range map[string]string{
		"VueBox": "typescript", "PHPBox": "php", "JSPBox": "java", "JinjaBox": "jinja", "EJSBox": "javascript",
	} {
		toolErr, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{root}, Query: name, Match: "exact", Encoding: "utf-8", MaxSymbols: 16,
		})
		if err != nil || toolErr != nil {
			t.Fatalf("Phase 11 find %s err=%v toolErr=%+v", name, err, toolErr)
		}
		if len(found.Symbols) != 1 || found.Symbols[0].Name != name || found.Symbols[0].Language != language {
			t.Fatalf("Phase 11 find %s = %+v, want one %s symbol", name, found.Symbols, language)
		}
	}
}
