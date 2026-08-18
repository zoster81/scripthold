package sourceintelligence

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestCancellationAcrossNewProviders(t *testing.T) {
	for _, analyzer := range []SourceAnalyzer{
		VueAnalyzer{}, SvelteAnalyzer{}, AstroAnalyzer{}, PHPHTMLAnalyzer{}, JSPAnalyzer{},
		JinjaAnalyzer{}, TwigAnalyzer{}, BladeAnalyzer{}, EJSAnalyzer{},
	} {
		t.Run(analyzer.Language(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err := analyzer.Analyze(ctx, scientificLegacyFunctionalTestDocument("fixture", `<main id="hero"></main>`), testAnalyzeOptions(true, 32))
			if operation.KindOf(err) != operation.KindCancelled {
				t.Fatalf("cancel error=%v kind=%v", err, operation.KindOf(err))
			}
		})
	}
}

func TestCompositeRegionOutputIsBounded(t *testing.T) {
	const declarations = 1200
	var vue strings.Builder
	var jinja strings.Builder
	for index := 0; index < declarations; index++ {
		fmt.Fprintf(&vue, "<script>function f%d() {}</script>\n", index)
		fmt.Fprintf(&jinja, "{%% macro f%d() %%}{%% endmacro %%}\n", index)
	}
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
		text     string
	}{
		{"vue", VueAnalyzer{}, vue.String()},
		{"jinja", JinjaAnalyzer{}, jinja.String()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := testAnalyzeOptions(true, 128)
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture", tc.text), options)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) > 128 || len(result.Regions) > 128 {
				t.Fatalf("unbounded result: symbols=%d regions=%d", len(result.Analysis.Symbols), len(result.Regions))
			}
			if !result.Analysis.Truncated || result.Analysis.CoverageComplete {
				t.Fatalf("bounded result did not report truncation: %+v", result.Analysis)
			}
		})
	}
}

func TestDotNetCompositeAggregateRegionBudgetAndMalformedClient(t *testing.T) {
	var mixed strings.Builder
	mixed.WriteString(`<%@ Page Language="C#" %>\n`)
	for index := 0; index < 90; index++ {
		fmt.Fprintf(&mixed, `<script runat="server">public void S%d() {}</script>\n`, index)
		fmt.Fprintf(&mixed, `<script>function C%d() {}</script>\n`, index)
	}
	options := testAnalyzeOptions(true, 128)
	result, err := (ASPNetWebFormsAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.aspx", mixed.String()), options)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Regions) > 128 || !result.Analysis.Truncated || result.Analysis.CoverageComplete {
		t.Fatalf(".NET composite aggregate region budget not enforced: regions=%d analysis=%+v", len(result.Regions), result.Analysis)
	}

	malformed, err := (RazorAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.cshtml", `@functions { public void ServerRun() {} }<script>function broken() {}`), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if malformed.Analysis.CoverageComplete || len(malformed.Analysis.Diagnostics) == 0 {
		t.Fatalf("unterminated Razor client region reported complete: %+v", malformed.Analysis)
	}
}

func TestUnterminatedCompositeRegionsLowerCoverage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
		text     string
	}{
		{"vue-script", VueAnalyzer{}, `<script>function load() {}`},
		{"php", PHPHTMLAnalyzer{}, `<?php function load() {}`},
		{"jinja", JinjaAnalyzer{}, `{% macro render() %`},
		{"blade", BladeAnalyzer{}, `@php function load() {}`},
		{"ejs", EJSAnalyzer{}, `<% function load() {}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture", tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
				t.Fatalf("unterminated region reported complete: %+v", result.Analysis)
			}
		})
	}
}
