package sourceintelligence

import (
	"context"
	"testing"
)

// A protobuf message can nest types with identical short names. Fields within
// those types must not be exposed as fields of the enclosing message.
func TestProtoNestedMessageDeclarationsOwnTheirFields(t *testing.T) {
	source := "syntax = \"proto3\";\npackage demo;\n" +
		"message Outer {\n" +
		"  string label = 1;\n" +
		"  message Inner {\n" +
		"    string value = 1;\n" +
		"  }\n" +
		"  repeated Inner items = 2;\n" +
		"  oneof selected {\n" +
		"    string choice = 3;\n" +
		"  }\n" +
		"  enum Kind { NONE = 0; }\n" +
		"}\n" +
		"message Other {\n" +
		"  message Inner { int32 value = 1; }\n" +
		"}\n"
	result, err := (ProtoAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 80))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("unexpected partial proto analysis: %+v", result.Analysis)
	}
	got := symbolsByQualifiedName(result.Analysis.Symbols)
	want := map[string]SymbolKind{
		"demo":                   SymbolKindPackage,
		"demo.Outer":             SymbolKindType,
		"demo.Outer.label":       SymbolKindField,
		"demo.Outer.items":       SymbolKindField,
		"demo.Outer.choice":      SymbolKindField,
		"demo.Outer.Inner":       SymbolKindType,
		"demo.Outer.Inner.value": SymbolKindField,
		"demo.Outer.Kind":        SymbolKindEnum,
		"demo.Other":             SymbolKindType,
		"demo.Other.Inner":       SymbolKindType,
		"demo.Other.Inner.value": SymbolKindField,
	}
	for name, kind := range want {
		sym, ok := got[name]
		if !ok || sym.Kind != kind {
			t.Errorf("missing or incorrect protobuf ownership for %s: symbol=%+v exists=%v", name, sym, ok)
		}
	}
	for _, forbidden := range []string{
		"demo.Inner", "demo.Kind", "demo.Outer.value", "demo.Other.value",
		"demo.Outer.Inner.choice", "demo.Other.Inner.items",
	} {
		if sym, exists := got[forbidden]; exists {
			t.Errorf("fabricated protobuf declaration %s: %+v", forbidden, sym)
		}
	}
	for name := range want {
		if got[name].ID == "" {
			t.Errorf("no stable source selector for %s", name)
		}
	}
}

func TestProtoMalformedOuterBlockDoesNotInventTopLevelNestedType(t *testing.T) {
	source := "package demo;\nmessage Broken {\n  message Inner { string valid = 1; }\n"
	result, err := (ProtoAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 40))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
		t.Fatalf("unclosed proto outer scope claimed complete analysis: %+v", result.Analysis)
	}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Name == "Inner" && symbol.ParentQualifiedName == "demo" {
			t.Fatalf("nested type leaked to package after malformed outer declaration: %+v", symbol)
		}
	}
}
