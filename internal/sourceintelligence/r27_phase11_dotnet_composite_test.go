package sourceintelligence

import (
	"context"
	"testing"
)

func TestR27Phase11DotNetCompositesDelegateServerClientAndStyleRegions(t *testing.T) {
	tests := []struct {
		language string
		analyzer SourceAnalyzer
		text     string
	}{
		{
			language: "aspnet-webforms", analyzer: ASPNetWebFormsAnalyzer{},
			text: `<%@ Page Language="C#" %>
<main id="hero"></main>
<script runat="server">public void ServerRun() {}</script>
<script>function ClientRun() {}</script>
<style>.card { display: block; }</style>`,
		},
		{
			language: "razor", analyzer: RazorAnalyzer{},
			text: `@functions { public void ServerRun() {} }
<main id="hero"></main>
<script>function ClientRun() {}</script>
<style>.card { display: block; }</style>`,
		},
		{
			language: "blazor", analyzer: BlazorAnalyzer{},
			text: `@code { public void ServerRun() {} }
<main id="hero"></main>
<script>function ClientRun() {}</script>
<style>.card { display: block; }</style>`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), phase9TestDocument("fixture", tc.text), phase3AnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, want := range map[string]struct {
				kind     SymbolKind
				language string
			}{
				"ServerRun": {kind: SymbolKindMethod, language: "csharp"},
				"ClientRun": {kind: SymbolKindFunction, language: "javascript"},
				".card":     {kind: SymbolKindSelector, language: "css"},
				"hero":      {kind: SymbolKindEntity, language: "html"},
			} {
				symbol, ok := byName[name]
				if !ok || symbol.Kind != want.kind {
					t.Fatalf("%s %s = %+v exists=%v; symbols=%v", tc.language, name, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
				if symbol.Language != want.language || symbol.Analyzer != string(tc.analyzer.ID()) || symbol.RegionID == "" {
					t.Fatalf("%s %s provider/region identity = %+v, want language=%s", tc.language, name, symbol, want.language)
				}
			}
			clientKinds := map[string]bool{"script": false, "style": false}
			for _, region := range result.Regions {
				if _, ok := clientKinds[region.Kind]; ok && region.Supported {
					clientKinds[region.Kind] = true
				}
			}
			for kind, found := range clientKinds {
				if !found {
					t.Fatalf("%s missing client %s region: %+v", tc.language, kind, result.Regions)
				}
			}
		})
	}
}
