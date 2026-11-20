package sourceintelligence

import (
	"context"
	"testing"
)

func TestFreeBasicQualifiedMethodNamesAndPropertyPrototypes(t *testing.T) {
	text := "Type JsonItem\n" +
		"  Declare Property Value() As String\n" +
		"  Declare Property Value(ByRef newValue As String)\n" +
		"  Declare Function AddItem(newValue As String) As Boolean\n" +
		"End Type\n" +
		"Property JsonItem.Value(ByRef newValue As String)\n" +
		"  Select Case newValue\n" +
		"  Case \"null\"\n" +
		"  End Select\n" +
		"End Property\n" +
		"Function JsonItem.AddItem(newValue As String) As Boolean\n" +
		"  Return True\n" +
		"End Function\n"
	result, err := (FreeBasicAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	type expected struct {
		name, qualified string
		kind            SymbolKind
		line            int
	}
	want := []expected{
		{"Value", "JsonItem.Value", SymbolKindProperty, 2},
		{"Value", "JsonItem.Value", SymbolKindProperty, 3},
		{"AddItem", "JsonItem.AddItem", SymbolKindMethod, 4},
		{"Value", "JsonItem.Value", SymbolKindProperty, 6},
		{"AddItem", "JsonItem.AddItem", SymbolKindFunction, 11},
	}
	for _, w := range want {
		found := false
		for _, s := range result.Analysis.Symbols {
			if s.Name == w.name && s.QualifiedName == w.qualified && s.Kind == w.kind &&
				s.DeclarationRange.Start.Line == w.line && s.NameRange.Start.Line == w.line {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing %s (%s) at line %d; got symbols: %+v", w.name, w.qualified, w.line, result.Analysis.Symbols)
		}
	}
}
