package sourceintelligence

import (
	"context"
	"testing"
)

// Derived from Swift Collections' Deque+Equatable.swift: the authored
// declaration is a symbolic operator, unlike comparison expressions in
// its body. No Swift project code is compiled or executed.
func TestSwiftSymbolicEqualityOperatorSourceNavigation(t *testing.T) {
	source := `extension Deque: Equatable where Element: Equatable {
  public static func ==(left: Self, right: Self) -> Bool {
    return left.count == right.count
  }
  public func count() -> Int { return 1 }
}
let same = first == second
`
	result, err := (SwiftAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	seenOperator, seenNeighbor := false, false
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Name == "==" && symbol.ParentQualifiedName == "Deque" &&
			symbol.NameRange.Start.Line == 2 && symbol.Kind == SymbolKindMethod {
			seenOperator = true
		}
		if symbol.Name == "count" && symbol.ParentQualifiedName == "Deque" &&
			symbol.NameRange.Start.Line == 5 && symbol.Kind == SymbolKindMethod {
			seenNeighbor = true
		}
		if symbol.Name == "left" || symbol.Name == "right" || symbol.Name == "first" || symbol.Name == "second" {
			t.Errorf("parameter or expression used as declaration: %+v", symbol)
		}
	}
	if !seenOperator || !seenNeighbor {
		t.Errorf("original-style equality declaration/neighbor missing: operator=%v neighbor=%v symbols=%+v",
			seenOperator, seenNeighbor, result.Analysis.Symbols)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Errorf("valid Swift declarations have misleading coverage: %+v", result.Analysis)
	}
}
