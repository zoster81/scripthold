package sourceintelligence

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestCOBOLFixedFormatMixedAlphanumericSequenceNumbers(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(fmt.Sprintf("eol-%d", len(eol)), func(t *testing.T) {
			source := strings.Join([]string{
				"       IDENTIFICATION DIVISION.",
				"       PROGRAM-ID. DEMO.",
				"       DATA DIVISION.",
				"       WORKING-STORAGE SECTION.",
				"GC0909 01  ACCOUNT-ID PIC X(8).",
				"GC0909",
				"SCROLL* COMMENTED DISPLAY 'must never be scanned",
				"      * Comments with an unmatched ' quote are not source.",
				"       PROCEDURE DIVISION.",
				"       MAIN.",
				"           DISPLAY 'hello'.",
				"",
			}, eol)
			if cobolLooksFreeForm(source) {
				t.Fatal("mixed alphanumeric sequence column must not force free-format routing")
			}
			result, err := (COBOLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0 {
				t.Fatalf("fixed-format sequence lines yielded incomplete coverage: %+v", result.Analysis.Diagnostics)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			if symbols["DEMO"].Kind != SymbolKindModule || symbols["DEMO.MAIN"].Kind != SymbolKindFunction {
				t.Fatalf("expected program and paragraph: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			if _, falseSymbol := symbols["DEMO.ACCOUNT-ID"]; falseSymbol {
				t.Fatal("data field must not become a procedure declaration")
			}
		})
	}
}

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
