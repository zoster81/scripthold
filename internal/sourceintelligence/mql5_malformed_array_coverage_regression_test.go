package sourceintelligence

import (
	"context"
	"testing"
)

// The original TheMarket.mq5 has a top-level signals[] initializer without
// a semicolon before its authored OnInit/OnDeinit/OnTick declarations.
// This synthetic reproducer never compiles or executes third-party code.
func TestMQL5MalformedTopLevelArrayRetainsHonestNavigationCoverage(t *testing.T) {
	header := "string signals[] = {\n    // other entries omitted\n    \"SAP LONG OPEN SL @ 20000\",\n}"
	for _, tc := range []struct {
		name, suffix     string
		shouldBeComplete bool
	}{
		{name: "missing semicolon", suffix: "", shouldBeComplete: false},
		{name: "terminated array", suffix: ";", shouldBeComplete: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := header + tc.suffix +
				"\nint OnInit() { return(INIT_SUCCEEDED); }\n" +
				"void OnDeinit(const int reason) {}\n" +
				"void OnTick() {}\n"
			result, err := (MQL5Analyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			seen := make(map[string]bool)
			for _, symbol := range result.Analysis.Symbols {
				seen[symbol.QualifiedName] = true
				if symbol.Name == "INIT_SUCCEEDED" {
					t.Errorf("return expression fabricated as declaration: %+v", symbol)
				}
			}
			if tc.shouldBeComplete {
				for _, wanted := range []string{"OnInit", "OnDeinit", "OnTick"} {
					if !seen[wanted] {
						t.Errorf("complete source missed authored declaration %q; symbols=%v", wanted, seen)
					}
				}
				if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
					t.Errorf("valid original-form initializer unexpectedly partial: %+v", result.Analysis)
				}
				return
			}
			for _, wanted := range []string{"OnInit", "OnDeinit", "OnTick"} {
				if !seen[wanted] {
					t.Errorf("malformed initializer swallowed following authored %q: %v", wanted, seen)
				}
			}
			if result.Analysis.CoverageComplete {
				t.Errorf("unterminated top-level initializer must report incomplete coverage: %+v", result.Analysis)
			}
		})
	}
}
