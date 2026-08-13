package sourceintelligence

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

var _ SourceAnalyzer = PythonAnalyzer{}

func TestPythonAnalyzerIndentationDecoratorsAndNestedDefinitions(t *testing.T) {
	text := `import os
from collections import abc

@decorator
class Box(Generic[T]):
    """class Fake:
        def wrong(self): pass
    """

    def __init__(self, value: T):
        self.value = value

    @property
    def value(self) -> T:
        return self._value

    async def compute(
        self,
        item: T,
    ) -> T:
        def nested(x: T) -> T:
            return x
        return nested(item)

def top_level(value: int) -> int:
    return value

# def Commented(): pass
fake = "def StringFake(): pass"
`
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.py"
	options := AnalyzeOptions{IncludeSignatures: true, Limits: SymbolBuilderLimits{MaxSymbols: 256, MaxSignatureBytes: 4096, MaxDiagnostics: 32}}
	result, err := (PythonAnalyzer{}).Analyze(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("Python analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	want := map[string]SymbolKind{
		"Box":                SymbolKindClass,
		"Box.__init__":       SymbolKindMethod,
		"Box.value":          SymbolKindMethod,
		"Box.compute":        SymbolKindMethod,
		"Box.compute.nested": SymbolKindFunction,
		"top_level":          SymbolKindFunction,
	}
	for qualified, kind := range want {
		symbol, ok := byName[qualified]
		if !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
		if symbol.Language != "python" || symbol.Analyzer != string(AnalyzerPython) || symbol.Evidence != SymbolEvidenceStructural {
			t.Fatalf("%s metadata = %+v", qualified, symbol)
		}
	}
	for _, forbidden := range []string{"Fake", "Box.wrong", "Commented", "StringFake"} {
		if _, exists := byName[forbidden]; exists {
			t.Fatalf("Python false positive %s", forbidden)
		}
	}
	if !containsString(byName["Box"].Modifiers, "decorated") || !containsString(byName["Box.value"].Modifiers, "decorated") || !containsString(byName["Box.compute"].Modifiers, "async") {
		t.Fatalf("decorator/async modifiers: Box=%v value=%v compute=%v", byName["Box"].Modifiers, byName["Box.value"].Modifiers, byName["Box.compute"].Modifiers)
	}
	if !strings.Contains(byName["Box.compute"].Signature, "async def compute(") || byName["Box.compute"].BodyRange == nil {
		t.Fatalf("multiline async signature/range = %+v", byName["Box.compute"])
	}
	if len(result.Dependencies) != 2 || result.Dependencies[0].Value != "os" || result.Dependencies[1].Value != "collections" {
		t.Fatalf("Python imports = %+v", result.Dependencies)
	}
	second, err := (PythonAnalyzer{}).Analyze(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, second) {
		t.Fatal("Python analyzer output is nondeterministic")
	}
}

func TestPythonAnalyzerPreservesRelativeImportLevel(t *testing.T) {
	text := `from .core import Command
from ..utils import helper
from . import local
import os.path
`
	document := sourceDocumentForScanner(text)
	document.Path = "imports.py"
	result, err := (PythonAnalyzer{}).Analyze(context.Background(), document, AnalyzeOptions{Limits: SymbolBuilderLimits{MaxSymbols: 32, MaxSignatureBytes: 1024, MaxDiagnostics: 8}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("relative import analysis unexpectedly partial: %+v", result.Analysis)
	}
	want := []string{".core", "..utils", ".", "os.path"}
	if len(result.Dependencies) != len(want) {
		t.Fatalf("dependencies = %+v, want %v", result.Dependencies, want)
	}
	for index, value := range want {
		if result.Dependencies[index].Value != value {
			t.Fatalf("dependency[%d] = %q, want %q; all=%+v", index, result.Dependencies[index].Value, value, result.Dependencies)
		}
	}
}

func TestPythonAnalyzerMalformedSourceKeepsEarlierDeclarations(t *testing.T) {
	text := `class Good:
    def ok(self):
        return 1

def visible():
    return 2

text = """unterminated
class Fake:
`
	document := sourceDocumentForScanner(text)
	document.Path = "broken.py"
	result, err := (PythonAnalyzer{}).Analyze(context.Background(), document, AnalyzeOptions{Limits: SymbolBuilderLimits{MaxSymbols: 32, MaxSignatureBytes: 1024, MaxDiagnostics: 8}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
		t.Fatalf("malformed Python did not lower coverage: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"Good", "Good.ok", "visible"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("malformed recovery lost %s: %v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	if _, exists := byName["Fake"]; exists {
		t.Fatal("class-looking text in unterminated triple string became a symbol")
	}
}
