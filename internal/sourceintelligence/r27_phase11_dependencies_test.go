package sourceintelligence

import (
	"context"
	"testing"
)

func TestR27Phase11StaticHostDependencies(t *testing.T) {
	tests := []struct {
		name     string
		analyzer SourceAnalyzer
		text     string
		want     map[string]StructuralDependencyKind
	}{
		{
			name: "vue-script-src-and-import", analyzer: VueAnalyzer{},
			text: `<script src="./legacy.js"></script><script>import { load } from "./load.js";</script>`,
			want: map[string]StructuralDependencyKind{"./legacy.js": StructuralDependencyInclude, "./load.js": StructuralDependencyImport},
		},
		{
			name: "svelte-script-src", analyzer: SvelteAnalyzer{},
			text: `<script src="./client.js"></script>`,
			want: map[string]StructuralDependencyKind{"./client.js": StructuralDependencyInclude},
		},
		{
			name: "astro-script-src", analyzer: AstroAnalyzer{},
			text: `<script src="./client.js"></script>`,
			want: map[string]StructuralDependencyKind{"./client.js": StructuralDependencyInclude},
		},
		{
			name: "php-literal-include", analyzer: PHPHTMLAnalyzer{},
			text: `<main id="hero"></main><?php include "partials/header.php"; ?>`,
			want: map[string]StructuralDependencyKind{"partials/header.php": StructuralDependencyInclude},
		},
		{
			name: "jsp-directive-include", analyzer: JSPAnalyzer{},
			text: `<%@ include file="header.jsp" %><main id="hero"></main>`,
			want: map[string]StructuralDependencyKind{"header.jsp": StructuralDependencyInclude},
		},
		{
			name: "jinja-extends-include", analyzer: JinjaAnalyzer{},
			text: `{% extends "base.html" %}{% include "header.html" %}`,
			want: map[string]StructuralDependencyKind{"base.html": StructuralDependencyInclude, "header.html": StructuralDependencyInclude},
		},
		{
			name: "twig-extends-include", analyzer: TwigAnalyzer{},
			text: `{% extends "base.twig" %}{% include "header.twig" %}`,
			want: map[string]StructuralDependencyKind{"base.twig": StructuralDependencyInclude, "header.twig": StructuralDependencyInclude},
		},
		{
			name: "blade-extends-include", analyzer: BladeAnalyzer{},
			text: `@extends('layouts.app') @include('partials.header')`,
			want: map[string]StructuralDependencyKind{"layouts.app": StructuralDependencyInclude, "partials.header": StructuralDependencyInclude},
		},
		{
			name: "ejs-static-include", analyzer: EJSAnalyzer{},
			text: `<%- include("header.ejs") %>`,
			want: map[string]StructuralDependencyKind{"header.ejs": StructuralDependencyInclude},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), phase9TestDocument("fixture", tc.text), phase3AnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			got := make(map[string]StructuralDependencyKind)
			for _, dependency := range result.Dependencies {
				got[dependency.Value] = dependency.Kind
			}
			for value, kind := range tc.want {
				if got[value] != kind {
					t.Fatalf("dependency %q = %q, want %q; all=%+v", value, got[value], kind, result.Dependencies)
				}
			}
		})
	}
}

func TestR27Phase11DynamicTemplateTargetsAreNotPromotedToDependencies(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
		text     string
	}{
		{"jinja", JinjaAnalyzer{}, `{% include target %}`},
		{"twig", TwigAnalyzer{}, `{% include target %}`},
		{"blade", BladeAnalyzer{}, `@include($target)`},
		{"ejs", EJSAnalyzer{}, `<%- include(target) %>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), phase9TestDocument("fixture", tc.text), phase3AnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Dependencies) != 0 {
				t.Fatalf("dynamic dependency overclaimed: %+v", result.Dependencies)
			}
		})
	}
}
