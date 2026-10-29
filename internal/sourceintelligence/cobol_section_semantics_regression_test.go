package sourceintelligence

import (
	"context"
	"testing"
)

func TestCOBOLProcedureParagraphsKeepDeclarationIdentity(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "free-format",
			source: "IDENTIFICATION DIVISION.\nPROGRAM-ID. DEMO.\nDATA DIVISION.\nWORKING-STORAGE SECTION.\n01 val pic 9.\nPROCEDURE DIVISION.\nMAIN.\n  CONTINUE.\n  DISPLAY 'main.'.\nHANDLERS SECTION.\nNEXT-PARA.\n  GOBACK.\n",
		},
		{
			name:   "fixed-format",
			source: "       IDENTIFICATION DIVISION.\r\n       PROGRAM-ID. DEMO.\r\n       PROCEDURE DIVISION.\r\n       MAIN.\r\n           CONTINUE.\r\n       HANDLERS SECTION.\r\n       NEXT-PARA.\r\n           GOBACK.\r\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (COBOLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range map[string]SymbolKind{
				"DEMO":                    SymbolKindModule,
				"DEMO.MAIN":               SymbolKindFunction,
				"DEMO.HANDLERS":           SymbolKindFunction,
				"DEMO.HANDLERS.NEXT-PARA": SymbolKindFunction,
			} {
				got, ok := symbols[name]
				if !ok || got.Kind != kind {
					t.Errorf("paragraph/section %q: got=%+v present=%v, want kind %q", name, got, ok, kind)
				}
			}
			for child, parent := range map[string]string{
				"DEMO.MAIN":               "DEMO",
				"DEMO.HANDLERS":           "DEMO",
				"DEMO.HANDLERS.NEXT-PARA": "DEMO.HANDLERS",
			} {
				if got, ok := symbols[child]; ok && got.ParentQualifiedName != parent {
					t.Errorf("%q parent=%q, want %q", child, got.ParentQualifiedName, parent)
				}
			}
			for _, forbidden := range []string{"DEMO.CONTINUE", "DEMO.GOBACK", "DEMO.main.", "DEMO.HANDLERS.GOBACK"} {
				if _, ok := symbols[forbidden]; ok {
					t.Errorf("COBOL statement became declaration: %q", forbidden)
				}
			}
			if !result.Analysis.CoverageComplete {
				t.Errorf("bounded source unexpectedly partial: %+v", result.Analysis.Diagnostics)
			}
		})
	}
}

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
