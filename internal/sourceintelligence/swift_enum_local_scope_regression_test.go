package sourceintelligence

import (
	"context"
	"testing"
)

func TestSwiftEnumCaseAndLexicalLocalFunctionDeclarations(t *testing.T) {
	source := `enum State: String {
    case ready, waiting
    case finished
    var isReady: Bool {
        switch self {
        case .ready: return true
        default: return false
        }
    }
}
func outer() {
    func inner() -> Int {
        func nested() -> Int { 1 }
        return nested()
    }
    let misleading = "func invented() {} case phantom"
    _ = inner()
}
struct Service {
    func run() {
        func helper() {}
        helper()
    }
}
`
	result, err := (SwiftAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("unexpectedly incomplete source: %+v", result.Analysis)
	}
	expected := map[string]struct {
		kind           SymbolKind
		native, parent string
	}{
		"State":              {SymbolKindEnum, "enum", ""},
		"State.ready":        {SymbolKindField, "case", "State"},
		"State.waiting":      {SymbolKindField, "case", "State"},
		"State.finished":     {SymbolKindField, "case", "State"},
		"State.isReady":      {SymbolKindProperty, "var", "State"},
		"outer":              {SymbolKindFunction, "func-definition", ""},
		"outer.inner":        {SymbolKindFunction, "func-definition", "outer"},
		"outer.inner.nested": {SymbolKindFunction, "func-definition", "outer.inner"},
		"Service":            {SymbolKindStruct, "struct", ""},
		"Service.run":        {SymbolKindMethod, "func-definition", "Service"},
		"Service.run.helper": {SymbolKindFunction, "func-definition", "Service.run"},
	}
	actual := symbolsByQualifiedName(result.Analysis.Symbols)
	if len(actual) != len(expected) {
		t.Errorf("wrong number of declarations got=%v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	for name, e := range expected {
		s, exists := actual[name]
		if !exists || s.Kind != e.kind || s.NativeKind != e.native || s.ParentQualifiedName != e.parent {
			t.Errorf("incorrect source declaration %s: got %+v; expected %+v", name, s, e)
		}
	}
	for name := range actual {
		if _, exists := expected[name]; !exists {
			t.Errorf("fabricated Swift symbol %s", name)
		}
	}
}
