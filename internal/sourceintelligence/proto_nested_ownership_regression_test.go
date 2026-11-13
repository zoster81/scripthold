package sourceintelligence

import (
	"context"
	"strings"
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

func TestProtoEnumValuesNestedOwnershipAndComments(t *testing.T) {
	source := "syntax = \"proto3\";\npackage demo;\n" +
		"message Outer {\n" +
		"  string STATUS = 1;\n" +
		"  enum State {\n" +
		"    ZERO = 0;\n" +
		"    NEGATIVE = -1;\n" +
		"    // PHANTOM = 99;\n" +
		"    option allow_alias = true;\n" +
		"  }\n" +
		"  enum OtherState {\n" +
		"    ZERO = 0;\n" +
		"  }\n" +
		"  int32 NEGATIVE = 2;\n" +
		"}\n" +
		"message Unrelated { string value = 1; }\n"
	for _, tc := range []struct{ name, text string }{
		{"lf", source},
		{"crlf", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (ProtoAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 80))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("bounded enum schema parsed partially: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, expected := range []struct {
				name, native string
				kind         SymbolKind
				line         int
			}{
				{"demo.Outer.State.ZERO", "enum-value", SymbolKindConstant, 6},
				{"demo.Outer.State.NEGATIVE", "enum-value", SymbolKindConstant, 7},
				{"demo.Outer.OtherState.ZERO", "enum-value", SymbolKindConstant, 12},
				{"demo.Outer.NEGATIVE", "field", SymbolKindField, 14},
				{"demo.Outer.STATUS", "field", SymbolKindField, 4},
			} {
				got, exists := byName[expected.name]
				if !exists || got.Kind != expected.kind || got.NativeKind != expected.native ||
					got.NameRange.Start.Line != expected.line || got.ID == "" {
					t.Errorf("unexpected Protobuf enum/field declaration %s: %+v", expected.name, got)
				}
			}
			for _, forbidden := range []string{
				"demo.Outer.State.PHANTOM", "demo.Outer.State.allow_alias",
				"demo.Outer.State.STATUS", "demo.Outer.NEGATIVE.ZERO",
				"demo.Outer.ZERO", "demo.ZERO",
			} {
				if symbol, exists := byName[forbidden]; exists {
					t.Errorf("false enum member or owner %s: %+v", forbidden, symbol)
				}
			}
		})
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
