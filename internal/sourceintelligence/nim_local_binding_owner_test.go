package sourceintelligence

import (
	"context"
	"testing"
)

func TestNimLocalBindingsKeepCallableOwnership(t *testing.T) {
	source := "func binarySearch*[T](a: openArray[T], value: T): int =\n" +
		"  var left = 0\n" +
		"  while left < a.len:\n" +
		"    let mid = left + 1\n" +
		"    return mid\n" +
		"  return -1\n" +
		"  proc helper() =\n" +
		"    let inner = 1\n" +
		"var sharedCounter = 0\n" +
		"proc other*() =\n" +
		"  let message = \"ok\"\n"
	result, err := (NimAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 48))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("well-formed Nim source incompletely indexed: %+v", result.Analysis)
	}
	names := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"binarySearch", "binarySearch.left", "binarySearch.mid", "binarySearch.helper", "binarySearch.helper.inner", "sharedCounter", "other", "other.message"} {
		if _, ok := names[name]; !ok {
			t.Errorf("Nim binding or proper owner %s missing: %+v", name, result.Analysis.Symbols)
		}
	}
	for _, wrong := range []string{"left", "mid", "message", "inner", "binarySearch.inner", "binarySearch.sharedCounter", "binarySearch.other"} {
		if _, exists := names[wrong]; exists {
			t.Errorf("Nim local variable indexed under wrong owner %s", wrong)
		}
	}
}
