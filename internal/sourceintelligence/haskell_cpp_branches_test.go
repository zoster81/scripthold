package sourceintelligence

import (
	"context"
	"testing"
)

func TestHaskellCPPBranchesAndInstanceLayoutRegression(t *testing.T) {
	source := "module Example where\n" +
		"#if FIRST\n" +
		"value :: Int\n" +
		"value = 1\n" +
		"#else\n" +
		"value :: Int\n" +
		"value = 2\n" +
		"#endif\n" +
		"class Printable a where\n" +
		"  render :: a -> String\n" +
		"instance Printable Int where\n" +
		"  render x = show x\n" +
		"#if SECOND\n" +
		"  extra = 3\n" +
		"#else\n" +
		"  extra = 4\n" +
		"#endif\n" +
		"  Constructor x <> Other y = x\n" +
		"after = 5\n"
	result, err := (HaskellAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
		t.Fatalf("unexpected incomplete Haskell CPP analysis: %+v", result.Analysis)
	}
	want := map[int]string{
		1: "Example", 3: "value", 6: "value", 9: "Printable", 10: "render",
		12: "render", 14: "extra", 16: "extra", 19: "after",
	}
	if len(result.Analysis.Symbols) != len(want) {
		t.Errorf("got %d symbols, expected %d: %+v", len(result.Analysis.Symbols), len(want), result.Analysis.Symbols)
	}
	for _, symbol := range result.Analysis.Symbols {
		line := symbol.NameRange.Start.Line
		name, ok := want[line]
		if !ok || symbol.Name != name {
			t.Errorf("invented/misidentified Haskell CPP symbol line=%d name=%s expected=%s exists=%v", line, symbol.Name, name, ok)
		}
		delete(want, line)
	}
	for line, name := range want {
		t.Errorf("lost CPP declaration %d:%s", line, name)
	}
}

func TestHaskellCPPBranchScopeGrouping(t *testing.T) {
	events := haskellCPPBranchEvents("#if A\n#if B\n#else\n#endif\n#else\n#endif\n")
	scopes := []string{"1/", "1/2/", "1/3/", "1/", "4/", ""}
	if len(events) != len(scopes) {
		t.Fatalf("branch events=%v, want %v", events, scopes)
	}
	for i, event := range events {
		if event.scope != scopes[i] {
			t.Errorf("event %d scope=%q, want %q", i, event.scope, scopes[i])
		}
	}
	seen := map[string]map[string]struct{}{}
	haskellRememberCPPBinding(seen, "x", "1/2/")
	if !haskellSeenInCPPAncestor(seen, "x", "1/2/") || haskellSeenInCPPAncestor(seen, "x", "1/3/") || haskellSeenInCPPAncestor(seen, "x", "4/") {
		t.Fatalf("CPP sibling branch binding misidentified: %+v", seen)
	}
	haskellRememberCPPBinding(seen, "y", "1/")
	if !haskellSeenInCPPAncestor(seen, "y", "1/2/") || haskellSeenInCPPAncestor(seen, "y", "4/") {
		t.Fatalf("CPP inherited scope error: %+v", seen)
	}
}
