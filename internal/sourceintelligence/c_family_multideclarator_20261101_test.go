package sourceintelligence

import (
	"context"
	"testing"
)

// Independently grounded by real MQL5 declarations in Argus' ADXTrendStrengthEA.mq5.
func TestCFamilyMultiDeclaratorGlobalVariables(t *testing.T) {
	cases := []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{"c", CAnalyzer{}},
		{"cpp", CPPAnalyzer{}},
		{"mql4", MQL4Analyzer{}},
		{"mql5", MQL5Analyzer{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := "int adx_handle, ema_handle, atr_handle;\n" +
				"int initialized = 1, second = 2, trailing;\n" +
				"// int ghost, phantom;\n" +
				"void OnStart() { int local; }\n"
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("incomplete analysis of unambiguous declarations: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, line := range map[string]int{
				"adx_handle": 1, "ema_handle": 1, "atr_handle": 1,
				"initialized": 2, "second": 2, "trailing": 2,
			} {
				symbol, ok := byName[name]
				if !ok || symbol.Kind != SymbolKindVariable || symbol.DeclarationRange.Start.Line != line {
					t.Errorf("variable %s = %+v exists=%v, want variable on line %d", name, symbol, ok, line)
				}
			}
			for _, forbidden := range []string{"ghost", "phantom", "local", "int"} {
				if _, exists := byName[forbidden]; exists {
					t.Errorf("non-global declaration %q escaped scope", forbidden)
				}
			}
		})
	}
}

func TestCFamilyMultiDeclaratorNestedSyntaxAndMemberOwnership(t *testing.T) {
	text := "int first[2], second[3];\n" +
		"int *left, *right;\n" +
		"int (*callback)(int), next;\n" +
		"int initialized=func(1,2), trailing=7;\n" +
		"struct Holder { int x, y; };\n" +
		"// int ghost, phantom;\n"
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{{"c", CAnalyzer{}}, {"cpp", CPPAnalyzer{}}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("unexpected partial coverage: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range map[string]SymbolKind{
				"first": SymbolKindVariable, "second": SymbolKindVariable,
				"left": SymbolKindVariable, "right": SymbolKindVariable,
				"callback": SymbolKindVariable, "next": SymbolKindVariable,
				"initialized": SymbolKindVariable, "trailing": SymbolKindVariable,
				"Holder": SymbolKindStruct, "Holder.x": SymbolKindField, "Holder.y": SymbolKindField,
			} {
				sym, ok := byName[name]
				if !ok || sym.Kind != kind {
					t.Errorf("%s = %+v, exists=%v, want %s", name, sym, ok, kind)
				}
			}
			for _, forbidden := range []string{"int", "ghost", "phantom", "func", "2", "3"} {
				if _, ok := byName[forbidden]; ok {
					t.Errorf("false declaration %q", forbidden)
				}
			}
			if got, want := len(result.Analysis.Symbols), 11; got != want {
				t.Errorf("got %d declarations, want %d: %v", got, want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}

func TestCFamilyMultiDeclaratorComparisonInitializer(t *testing.T) {
	text := "int lower = value < limit, upper = 7;\n" +
		"int greater = value > limit, after = 9;\n"
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{{"c", CAnalyzer{}}, {"cpp", CPPAnalyzer{}}, {"mql5", MQL5Analyzer{}}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			names := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, want := range []string{"lower", "upper", "greater", "after"} {
				if s, ok := names[want]; !ok || s.Kind != SymbolKindVariable {
					t.Errorf("missing true variable %s: %+v exists=%v", want, s, ok)
				}
			}
			if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 4 {
				t.Errorf("comparison is an initializer, not a template: complete=%v symbols=%v", result.Analysis.CoverageComplete, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}
