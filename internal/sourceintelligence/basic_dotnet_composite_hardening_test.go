package sourceintelligence

import (
	"context"
	"testing"
)

func TestJScriptNetEvidenceUsesHostCoordinates(t *testing.T) {
	text := `package Demo {
import System;
public class Service extends BaseService {
    public function Run(value : int) : int { return value; }
}
}
`
	document := sourceDocumentForScanner(text)
	result, err := (JScriptNetAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dependencies) != 1 || result.Dependencies[0].Value != "System" {
		t.Fatalf("JScript.NET dependencies=%+v", result.Dependencies)
	}
	dependencyRange := result.Dependencies[0].Range
	if dependencyRange.Start.Line != 2 || dependencyRange.Start.Column <= 1 || dependencyRange.End.Line != 2 {
		t.Fatalf("JScript.NET dependency range=%+v, want host line 2", dependencyRange)
	}
	if !hasStructuralRelation(result.Relations, "extends", "Demo.Service", "BaseService") {
		t.Fatalf("JScript.NET relations=%+v", result.Relations)
	}
	for _, relation := range result.Relations {
		if relation.Kind == "extends" && relation.Source == "Demo.Service" && relation.Target == "BaseService" {
			if relation.Range.Start.Line != 3 || relation.Range.Start.Column <= 1 || relation.Range.End.Line != 3 {
				t.Fatalf("JScript.NET relation range=%+v, want host line 3", relation.Range)
			}
			return
		}
	}
	t.Fatal("JScript.NET extends relation missing after structural predicate")
}

func TestASPNetPageLanguagePreservesOffsetsAcrossUnicodeCaseFolding(t *testing.T) {
	if got := aspNetPageLanguage("<%@ȺȺȺ Page Language=\"VB\" %>"); got != "vbnet" {
		t.Fatalf("ASP.NET page language = %q, want vbnet", got)
	}
}

func TestRazorDirectiveSearchPreservesOffsetsAcrossUnicodeCaseFolding(t *testing.T) {
	text := "ȺȺȺ@CoDe { void Run() {} }"
	document := sourceDocumentForScanner(text)
	ranges, complete, err := findRazorCodeRanges(context.Background(), document, []string{"code"})
	if err != nil {
		t.Fatal(err)
	}
	if !complete || len(ranges) != 1 || ranges[0].full.Start != len("ȺȺȺ") {
		t.Fatalf("Razor ranges complete=%v ranges=%+v", complete, ranges)
	}
}

func TestRazorClientScriptTypeSelectsDataLanguage(t *testing.T) {
	text := `<script type="text/html" id="product-template">
<div id="product-card">{{ Model.Name }}</div>
</script>
<script type="application/json" data-vm>{"components":[{"name":"product-card"}]}</script>`
	result, err := (RazorAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Razor data-script regions reported partial: %+v", result.Analysis)
	}
	var scriptLanguages []string
	for _, region := range result.Regions {
		if region.Kind == "script" {
			scriptLanguages = append(scriptLanguages, region.Language)
		}
	}
	if len(scriptLanguages) != 2 || scriptLanguages[0] != "html" || scriptLanguages[1] != "json" {
		t.Fatalf("Razor script languages=%v, want [html json]", scriptLanguages)
	}
}

func TestRazorGeneratedJSONDataScriptRemainsOpaque(t *testing.T) {
	text := `<script type="application/json" data-vm="confirmDelete">@Json.Serialize(new {
    route = Url.Action("Delete", "Account"),
    field = "addressId"
})</script>`
	result, err := (RazorAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("Razor-generated JSON data block reported partial: %+v", result.Analysis)
	}
	scriptRegions := 0
	for _, region := range result.Regions {
		if region.Kind != "script" {
			continue
		}
		scriptRegions++
		if region.Language != "json" || !region.Supported {
			t.Fatalf("Razor-generated JSON region=%+v", region)
		}
	}
	if scriptRegions != 1 {
		t.Fatalf("Razor-generated JSON script regions=%d; all=%+v", scriptRegions, result.Regions)
	}
}

func TestXAMLRequiresBalancedAttributeQuotes(t *testing.T) {
	text := `<Window x:Class="Demo.Bad' xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"><Grid x:Name='Root" /></Window>`
	result, err := (XAMLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Analysis.Symbols) != 0 {
		t.Fatalf("mismatched XAML quotes leaked symbols: %+v", result.Analysis.Symbols)
	}
	if len(result.Dependencies) != 1 || result.Dependencies[0].Value != "http://schemas.microsoft.com/winfx/2006/xaml" {
		t.Fatalf("valid xmlns dependency should survive mismatched unrelated attributes: %+v", result.Dependencies)
	}
}
