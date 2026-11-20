package sourceintelligence

import (
	"context"
	"testing"
)

func TestNimBlockInitializerKeepsLocalDeclarationsScoped(t *testing.T) {
	source := "const populationCount = block:\n" +
		"  var arr = 0\n" +
		"  proc countSetBits() =\n" +
		"    let detail = 1\n" +
		"  arr\n" +
		"proc after() =\n" +
		"  let value = 2\n"
	result, err := (NimAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 48))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid Nim block initializer incomplete: %+v", result.Analysis)
	}
	names := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"populationCount", "populationCount.arr", "populationCount.countSetBits", "populationCount.countSetBits.detail", "after", "after.value"} {
		if _, ok := names[name]; !ok {
			t.Errorf("missing Nim block-scoped declaration %q", name)
		}
	}
	for _, wrong := range []string{"arr", "countSetBits", "detail", "after.populationCount", "populationCount.after"} {
		if _, exists := names[wrong]; exists {
			t.Errorf("Nim initializer-local declaration misrepresented as %q", wrong)
		}
	}
}
