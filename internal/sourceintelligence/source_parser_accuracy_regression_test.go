package sourceintelligence

import (
	"context"
	"os"
	"testing"
)

func TestSourceParserRealCorpusOracles(t *testing.T) {
	tests := []struct {
		env       string
		analyzer  SourceAnalyzer
		want      string
		kind      SymbolKind
		line      int
		absent    []string
	}{
		{"SCRIPTHOLD_ORACLE_DART_BOOL", DartAnalyzer{}, "bool", SymbolKindClass, 16, []string{"pragma"}},
		{"SCRIPTHOLD_ORACLE_APEX_FUTURE", ApexAnalyzer{}, "AtFutureRecipes.atFutureMethodWithCalloutPrivileges", SymbolKindMethod, 51, []string{"AtFutureRecipes.future"}},
		{"SCRIPTHOLD_ORACLE_D_SIMPLE", DAnalyzer{}, "test.simple.Struct", SymbolKindStruct, 7, []string{"test.simple.simple", "test.simple.stdio"}},
		{"SCRIPTHOLD_ORACLE_SOLIDITY_TYPES", SolidityAnalyzer{}, "DataTypes", SymbolKindModule, 4, []string{"solidity"}},
	}
	for _, tc := range tests {
		t.Run(tc.env, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent corpus oracle not configured")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result, err := tc.analyzer.Analyze(context.Background(), testSourceDocument(path, string(data)), testAnalyzeOptions(true, 2048))
			if err != nil {
				t.Fatal(err)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			sym, found := symbols[tc.want]
			if !found || sym.Kind != tc.kind || sym.DeclarationRange.Start.Line != tc.line {
				t.Errorf("real source %s: %q found=%v kind=%q line=%d want %q line=%d", tc.env, tc.want, found, sym.Kind, sym.DeclarationRange.Start.Line, tc.kind, tc.line)
			}
			for _, absent := range tc.absent {
				if sym, found := symbols[absent]; found {
					t.Errorf("real source %s falsely declares %q: %+v", tc.env, absent, sym)
				}
			}
		})
	}
}

func TestSourceParserDeclarationAccuracyRegressions(t *testing.T) {
	tests := []struct {
		name     string
		analyzer SourceAnalyzer
		source   string
		want     map[string]SymbolKind
		absent   []string
	}{
		{
			name:     "dart annotations preserve next declaration",
			analyzer: DartAnalyzer{},
			source:   "@pragma(\"vm:entry-point\")\nfinal class ProbeType {\n  int value = 1;\n}\nclass Other {}\n",
			want:     map[string]SymbolKind{"ProbeType": SymbolKindClass, "ProbeType.value": SymbolKindField, "Other": SymbolKindClass},
			absent:   []string{"pragma"},
		},
		{
			name:     "apex generic initializer is a field",
			analyzer: ApexAnalyzer{},
			source:   "public class Probe {\n  private static Set<String> cache = new Set<String>();\n  public static Boolean ready() { return true; }\n}\n",
			want:     map[string]SymbolKind{"Probe": SymbolKindClass, "Probe.cache": SymbolKindField, "Probe.ready": SymbolKindMethod},
			absent:   []string{"Probe.String"},
		},
		{
			name:     "arduino parenthesized object initialization is not a function",
			analyzer: ArduinoAnalyzer{},
			source:   "Widget object(1, 2);\nint counter = 0;\nvoid setup() { counter = 1; }\n",
			want:     map[string]SymbolKind{"object": SymbolKindVariable, "counter": SymbolKindVariable, "setup": SymbolKindFunction},
		},
		{
			name:     "cpp direct initializer remains a variable",
			analyzer: CPPAnalyzer{},
			source:   "Widget object(1, 2);\nWidget text(\"hello\");\nvoid run(int n);\n",
			want:     map[string]SymbolKind{"object": SymbolKindVariable, "text": SymbolKindVariable, "run": SymbolKindFunction},
		},
		{
			name:     "fsharp typed binding is a value",
			analyzer: FSharpAnalyzer{},
			source:   "module Probe\nlet value : int =\n    7\nlet square x =\n    x * x\n",
			want:     map[string]SymbolKind{"Probe": SymbolKindModule, "Probe.value": SymbolKindVariable, "Probe.square": SymbolKindFunction},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("unexpected incomplete analysis: %+v", result.Analysis)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range tc.want {
				if symbol, ok := got[name]; !ok || symbol.Kind != kind {
					t.Errorf("symbol %q: got %v (present=%v), want %s", name, symbol.Kind, ok, kind)
				}
			}
			for _, name := range tc.absent {
				if symbol, ok := got[name]; ok {
					t.Errorf("unexpected symbol %q: kind %s", name, symbol.Kind)
				}
			}
		})
	}
}
