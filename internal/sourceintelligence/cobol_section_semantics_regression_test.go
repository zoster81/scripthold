package sourceintelligence

import (
	"context"
	"testing"
)

func TestCOBOLQuotedProgramIDAndDivisionScopedSections(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "free format",
			source: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. 'HELLO'.\n" +
				"ENVIRONMENT DIVISION.\n" +
				"CONFIGURATION SECTION.\n" +
				"DATA DIVISION.\n" +
				"WORKING-STORAGE SECTION.\n" +
				"PROCEDURE DIVISION.\n" +
				"MAIN SECTION.\n",
		},
		{
			name: "fixed format",
			source: "       IDENTIFICATION DIVISION.\r\n" +
				"       PROGRAM-ID. 'HELLO'.\r\n" +
				"       ENVIRONMENT DIVISION.\r\n" +
				"       CONFIGURATION SECTION.\r\n" +
				"       PROCEDURE DIVISION.\r\n" +
				"       MAIN SECTION.\r\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (COBOLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("unexpected incomplete analysis: %+v", result.Analysis)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range map[string]SymbolKind{
				"HELLO":               SymbolKindModule,
				"HELLO.CONFIGURATION": SymbolKindSection,
				"HELLO.MAIN":          SymbolKindFunction,
			} {
				got, exists := symbols[name]
				if !exists || got.Kind != kind {
					t.Errorf("%q: present=%v, kind=%q, want=%q", name, exists, got.Kind, kind)
				}
			}
			if _, leaked := symbols["HELLO.WORKING-STORAGE"]; tc.name == "free format" && !leaked {
				t.Error("WORKING-STORAGE section is missing")
			} else if tc.name == "free format" && symbols["HELLO.WORKING-STORAGE"].Kind != SymbolKindSection {
				t.Errorf("WORKING-STORAGE should be section, got %q", symbols["HELLO.WORKING-STORAGE"].Kind)
			}
		})
	}
}
