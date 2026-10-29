package sourceintelligence

import (
	"context"
	"testing"
)

func TestBraceSharedStatementConsumption(t *testing.T) {
	tests := []struct {
		name     string
		analyzer SourceAnalyzer
		source   string
		want     map[string]SymbolKind
		absent   []string
	}{
		{
			name:     "D module and import are not variables",
			analyzer: DAnalyzer{},
			source:   "module demo.core;\nimport std.stdio;\nstruct Widget { int count; }\nint active;\n",
			want:     map[string]SymbolKind{"demo.core": SymbolKindModule, "demo.core.Widget": SymbolKindStruct, "demo.core.active": SymbolKindVariable},
			absent:   []string{"demo.core.core", "demo.core.stdio"},
		},
		{
			name:     "D unittest assertions are calls, not function declarations",
			analyzer: DAnalyzer{},
			source:   "module demo;\nint before() { return 1; }\nunittest {\n  assert(before() == 1);\n  assert(true);\n}\nvoid after() {}\n",
			want:     map[string]SymbolKind{"demo": SymbolKindModule, "demo.before": SymbolKindFunction, "demo.after": SymbolKindFunction},
			absent:   []string{"demo.assert", "demo.true", "demo.unittest"},
		},
		{
			name:     "D static assert is a compile-time check, not a function declaration",
			analyzer: DAnalyzer{},
			source:   "module demo;\nstatic assert(1 == 1, \"valid\");\nint usable() { return 1; }\n",
			want:     map[string]SymbolKind{"demo": SymbolKindModule, "demo.usable": SymbolKindFunction},
			absent:   []string{"demo.assert", "demo.static"},
		},
		{
			name:     "Solidity pragma does not become variable",
			analyzer: SolidityAnalyzer{},
			source:   "pragma solidity ^0.8.0;\nimport \"VaultBase.sol\";\ncontract Vault { uint amount; function pay() public {} }\n",
			want:     map[string]SymbolKind{"Vault": SymbolKindClass, "Vault.amount": SymbolKindField, "Vault.pay": SymbolKindMethod},
			absent:   []string{"solidity", "VaultBase", "VaultBase.sol"},
		},
		{
			name:     "Dart annotation preserves class and its next sibling",
			analyzer: DartAnalyzer{},
			source:   "@pragma(\"vm:entry-point\")\nfinal class Visible { int count = 1; }\nclass Sibling {}\n",
			want:     map[string]SymbolKind{"Visible": SymbolKindClass, "Visible.count": SymbolKindField, "Sibling": SymbolKindClass},
			absent:   []string{"pragma"},
		},
		{
			name:     "Apex annotation preserves member and its sibling",
			analyzer: ApexAnalyzer{},
			source:   "public class Ops {\n  @future(callout=true)\n  public static void dispatch(String url) {}\n  public static void next() {}\n}\n",
			want:     map[string]SymbolKind{"Ops": SymbolKindClass, "Ops.dispatch": SymbolKindMethod, "Ops.next": SymbolKindMethod},
			absent:   []string{"Ops.future"},
		},
		{
			name:     "Apex qualified annotation with nested arguments",
			analyzer: ApexAnalyzer{},
			source:   "public class Ops {\n  @TestVisible(mode=Factory.make(1))\n  public void verify() {}\n}\n",
			want:     map[string]SymbolKind{"Ops": SymbolKindClass, "Ops.verify": SymbolKindMethod},
			absent:   []string{"Ops.TestVisible", "Ops.make"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("unexpected incomplete analysis: %+v", result.Analysis)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for qualifiedName, kind := range tc.want {
				symbol, ok := got[qualifiedName]
				if !ok || symbol.Kind != kind {
					t.Errorf("symbol %q: present=%v, kind=%q; want %q", qualifiedName, ok, symbol.Kind, kind)
				}
			}
			for _, qualifiedName := range tc.absent {
				if symbol, exists := got[qualifiedName]; exists {
					t.Errorf("unexpected symbol %q: kind=%q", qualifiedName, symbol.Kind)
				}
			}
		})
	}
}
