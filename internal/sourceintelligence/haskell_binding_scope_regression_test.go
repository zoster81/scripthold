package sourceintelligence

import (
	"context"
	"testing"
)

func TestHaskellModuleReexportsAreNotDeclarations(t *testing.T) {
	const source = "module Example (\n" +
		"  module Other,\n" +
		"  module Another.Core,\n" +
		"  main\n" +
		"  ) where\n" +
		"import Other\n" +
		"import Another.Core\n" +
		"main :: Int\n" +
		"main = 1\n"
	result, err := (HaskellAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("reexport scope unexpectedly partial: %+v", result.Analysis)
	}
	if len(result.Analysis.Symbols) != 2 {
		t.Fatalf("module reexports are not new modules: got %+v", result.Analysis.Symbols)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	if module, ok := byName["Example"]; !ok || module.Kind != SymbolKindModule {
		t.Errorf("expected original declared module Example: %+v", byName)
	}
	if main, ok := byName["Example.main"]; !ok || main.Kind != SymbolKindFunction {
		t.Errorf("expected top-level function Example.main: %+v", byName)
	}
}

func TestHaskellLayoutScopeRegression(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   map[string]SymbolKind
		absent []string
	}{
		{
			name: "do-block and where bindings stay local",
			source: "module Demo where\n" +
				"run n = do\n" +
				"  let doubled x = x + x\n" +
				"      answer = doubled n\n" +
				"  pure answer\n" +
				"top x = inner x\n" +
				"  where\n" +
				"    inner y = y + 1\n" +
				"outside = 42\n",
			want: map[string]SymbolKind{
				"Demo": SymbolKindModule, "Demo.run": SymbolKindFunction,
				"Demo.top": SymbolKindFunction, "Demo.outside": SymbolKindVariable,
			},
			absent: []string{"Demo.let", "Demo.doubled", "Demo.answer", "Demo.inner"},
		},
		{
			name: "class and instance methods retain declaration visibility",
			source: "module Demo where\n" +
				"class Printable a where\n" +
				"  render :: a -> String\n" +
				"  size :: a -> Int\n" +
				"instance Printable Int where\n" +
				"  render n = show n\n" +
				"  instanceHelper n = n\n" +
				"  value n = do\n" +
				"    let internal = n\n" +
				"    pure internal\n" +
				"after = 1\n",
			want: map[string]SymbolKind{
				"Demo.Printable": SymbolKindInterface,
				"Demo.render":    SymbolKindFunction, "Demo.size": SymbolKindFunction,
				"Demo.instanceHelper": SymbolKindFunction, "Demo.value": SymbolKindFunction,
				"Demo.after": SymbolKindVariable,
			},
			absent: []string{"Demo.let", "Demo.internal"},
		},
		{
			name: "consistently indented top-level declarations are retained",
			source: "module Demo where\n" +
				"  answer :: Int\n" +
				"  answer = 42\n" +
				"  square x = x * x\n",
			want: map[string]SymbolKind{
				"Demo": SymbolKindModule, "Demo.answer": SymbolKindFunction,
				"Demo.square": SymbolKindFunction,
			},
		},
		{
			name: "CR-only lines preserve top-level and class members",
			source: "module Demo where\r" +
				"work n = do\r" +
				"  let x = n\r" +
				"      nested = n\r" +
				"  pure x\r" +
				"class Named a where\r" +
				"  named :: a -> String\r" +
				"after = 7\r",
			want: map[string]SymbolKind{
				"Demo": SymbolKindModule, "Demo.work": SymbolKindFunction,
				"Demo.Named": SymbolKindInterface, "Demo.named": SymbolKindFunction,
				"Demo.after": SymbolKindVariable,
			},
			absent: []string{"Demo.let", "Demo.nested"},
		},
		{
			name: "tab-indented top-level bindings remain available",
			source: "module Demo where\r\n" +
				"\tfirst x = x\r\n" +
				"\tsecond = 2\r\n",
			want: map[string]SymbolKind{
				"Demo": SymbolKindModule, "Demo.first": SymbolKindFunction,
				"Demo.second": SymbolKindVariable,
			},
		},
		{
			name: "existing unindented module grammar stays visible",
			source: "module Network.UDP where\n" +
				"import Data.List\n" +
				"data Packet = Packet Int\n" +
				"ping :: Packet -> Int\n" +
				"ping (Packet n) = n\n",
			want: map[string]SymbolKind{
				"Network.UDP": SymbolKindModule, "Network.UDP.Packet": SymbolKindType,
				"Network.UDP.ping": SymbolKindFunction,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (HaskellAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 96))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("unexpected incomplete analysis: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range tc.want {
				symbol, found := byName[name]
				if !found || symbol.Kind != kind {
					t.Errorf("%q: present=%v, kind=%q; want=%q", name, found, symbol.Kind, kind)
				}
			}
			for _, name := range tc.absent {
				if symbol, found := byName[name]; found {
					t.Errorf("local binding leaked as declaration %q: %q", name, symbol.Kind)
				}
			}
		})
	}
}
