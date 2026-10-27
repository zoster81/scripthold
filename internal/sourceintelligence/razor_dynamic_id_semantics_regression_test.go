package sourceintelligence

import (
	"context"
	"os"
	"testing"
)

func TestRazorComputedIDsAreNotStaticEntities(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
		ext      string
	}{
		{"razor", RazorAnalyzer{}, ".cshtml"},
		{"blazor", BlazorAnalyzer{}, ".razor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := `<main id="fixed"></main>
<aside id="help@example.com"></aside>
<div id="@Id"></div>
<button id="section-@Id"></button>
<Widget Id="@(Model.Selected)" />
@code { private void OnClick() {} }
`
			res, err := tc.analyzer.Analyze(context.Background(), testSourceDocument("sample"+tc.ext, text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			foundFixed, foundLiteralEmail := false, false
			for _, sym := range res.Analysis.Symbols {
				if sym.Kind == SymbolKindEntity && sym.Name == "fixed" {
					foundFixed = true
				}
				if sym.Kind == SymbolKindEntity && sym.Name == "help@example.com" {
					foundLiteralEmail = true
				}
				if sym.Kind == SymbolKindEntity && (sym.Name == "@Id" || sym.Name == "section-@Id" || sym.Name == "@(Model.Selected)") {
					t.Errorf("computed Razor markup id falsely declared as static entity: %+v", sym)
				}
			}
			if !foundFixed || !foundLiteralEmail {
				t.Errorf("static ids in %s lost: fixed=%v email=%v", tc.name, foundFixed, foundLiteralEmail)
			}
		})
	}
}

func TestHTMLLiteralAtIDRemainsAnEntity(t *testing.T) {
	text := `<main id="@Id"></main>`
	res, err := (HTMLAnalyzer{}).Analyze(context.Background(), testSourceDocument("plain.html", text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if sym, ok := symbolsByQualifiedName(res.Analysis.Symbols)["@Id"]; !ok || sym.Kind != SymbolKindEntity {
		t.Fatalf("plain HTML id must remain literal: %+v present=%v", sym, ok)
	}
}

func TestBlazorRealComputedIDOracles(t *testing.T) {
	for _, env := range []string{"SCRIPTHOLD_BLAZOR_ANTDESIGN_MARKER", "SCRIPTHOLD_BLAZOR_ANTDESIGN_BUTTON"} {
		t.Run(env, func(t *testing.T) {
			path := os.Getenv(env)
			if path == "" {
				t.Skip("set real source corpus path")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			res, err := (BlazorAnalyzer{}).Analyze(context.Background(), testSourceDocument(path, string(data)), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			for _, sym := range res.Analysis.Symbols {
				if sym.Kind == SymbolKindEntity && sym.Name == "@Id" {
					t.Errorf("computed component Id recorded as literal declaration: %+v", sym)
				}
			}
			if !res.Analysis.CoverageComplete {
				t.Errorf("original file rejected as incomplete: %+v", res.Analysis)
			}
		})
	}
}

// TestRazorRealInlineComputedIDOracle exercises an independent source file;
// its path is injected by the operator and never stored in repository fixtures.
func TestRazorRealInlineComputedIDOracle(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_RAZOR_NOPCOMMERCE_CONFIGURE")
	if path == "" {
		t.Skip("set the independent Razor corpus fixture path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (RazorAnalyzer{}).Analyze(context.Background(), testSourceDocument(path, string(data)), testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	got := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{
		"savecustomerrolesrequirement@(Model.RequirementId)",
		"pnl-save-requirement-result@(Model.RequirementId)",
	} {
		if symbol, exists := got[name]; exists {
			t.Errorf("runtime Razor ID emitted as a static declaration: %+v", symbol)
		}
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Errorf("previously supported Razor file became incomplete: %+v", result.Analysis)
	}
}
