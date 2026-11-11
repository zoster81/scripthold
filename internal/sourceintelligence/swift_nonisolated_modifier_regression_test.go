package sourceintelligence

import (
	"context"
	"testing"
)

func TestSwiftUnknownParameterizedNonisolatedModifierIsPartial(t *testing.T) {
	source := "struct Demo {\n" +
		"  nonisolated(unknown) var ghost: Int = 1\n" +
		"  var valid: Int = 2\n" +
		"}\n"
	result, err := (SwiftAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete {
		t.Fatalf("unknown parameterized modifier must not claim complete coverage: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	if _, exists := byName["Demo.ghost"]; exists {
		t.Errorf("unsupported modifier invented a valid declaration: %+v", byName)
	}
	if valid, exists := byName["Demo.valid"]; !exists || valid.Kind != SymbolKindProperty {
		t.Errorf("valid sibling lost after unsupported qualifier: %+v", byName)
	}
}

func TestSwiftParameterizedNonisolatedModifier(t *testing.T) {
	source := "struct Demo {\n" +
		"  static nonisolated(unsafe) var state: Int = 1\n" +
		"  nonisolated func read() -> Int { state }\n" +
		"  nonisolated(nonsending) func compute() -> Int { 1 }\n" +
		"}\n"
	result, err := (SwiftAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("unexpected partial structural coverage: %+v", result.Analysis)
	}
	expected := map[string]SymbolKind{
		"Demo":         SymbolKindStruct,
		"Demo.state":   SymbolKindProperty,
		"Demo.read":    SymbolKindMethod,
		"Demo.compute": SymbolKindMethod,
	}
	actual := symbolsByQualifiedName(result.Analysis.Symbols)
	if len(actual) != len(expected) {
		t.Errorf("unexpected fabricated/missing declarations, got %+v", result.Analysis.Symbols)
	}
	for name, kind := range expected {
		symbol, found := actual[name]
		if !found || symbol.Kind != kind {
			t.Errorf("missing or mistyped %s: %+v", name, symbol)
		}
	}
}
