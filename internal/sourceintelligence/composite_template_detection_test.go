package sourceintelligence

import (
	"context"
	"testing"
)

func TestCompositeTemplateAliasesResolveCanonically(t *testing.T) {
	registry, err := NewLanguageRegistry(defaultLanguageDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	if descriptor, ok := registry.Resolve("jinja2"); !ok || descriptor.ID != "jinja" {
		t.Fatalf("jinja2 alias = %+v ok=%v, want jinja", descriptor, ok)
	}
}

func TestDedicatedSuffixDetection(t *testing.T) {
	registry, err := NewLanguageRegistry(defaultLanguageDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path      string
		want      string
		wantState DetectionState
	}{
		{path: "Widget.vue", want: "vue", wantState: DetectionProbable},
		{path: "Widget.svelte", want: "svelte", wantState: DetectionProbable},
		{path: "Widget.astro", want: "astro", wantState: DetectionProbable},
		{path: "Widget.jsp", want: "jsp", wantState: DetectionProbable},
		{path: "Widget.jinja", want: "jinja", wantState: DetectionProbable},
		{path: "Widget.j2", want: "jinja", wantState: DetectionProbable},
		{path: "Widget.twig", want: "twig", wantState: DetectionProbable},
		{path: "Widget.ejs", want: "ejs", wantState: DetectionProbable},
		{path: "Widget.blade.php", want: "blade", wantState: DetectionExact},
	} {
		t.Run(tc.path, func(t *testing.T) {
			result, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: tc.path})
			if err != nil {
				t.Fatal(err)
			}
			if result.State != tc.wantState || result.Language != tc.want {
				t.Fatalf("detection = %+v, want %s %s", result, tc.wantState, tc.want)
			}
		})
	}
}

func TestPHPHTMLDetectionRequiresRealHostMarkup(t *testing.T) {
	registry, err := NewLanguageRegistry(defaultLanguageDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		path string
		text string
		want string
	}{
		{
			name: "pure php stays php",
			path: "index.php",
			text: "<?php function load(): int { return 1; } ?>\n",
			want: "php",
		},
		{
			name: "mixed php file becomes composite",
			path: "index.php",
			text: "<!doctype html><main id=\"hero\"></main><?php function load(): int { return 1; } ?>\n",
			want: "php-html",
		},
		{
			name: "mixed html file becomes composite",
			path: "index.html",
			text: "<main id=\"hero\"></main><?= $title ?>\n",
			want: "php-html",
		},
		{
			name: "markup inside php string does not create composite",
			path: "index.php",
			text: "<?php $markup = \"<main id='fake'></main>\"; function load() {} ?>\n",
			want: "php",
		},
		{
			name: "php-like text inside html comment stays html",
			path: "index.html",
			text: "<!-- <?php function fake() {} ?> --><main id=\"hero\"></main>\n",
			want: "html",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: tc.path, Text: tc.text})
			if err != nil {
				t.Fatal(err)
			}
			if result.State != DetectionProbable || result.Language != tc.want {
				t.Fatalf("detection = %+v, want probable %s", result, tc.want)
			}
		})
	}
}
