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
