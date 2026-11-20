package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCOBOLQualifiedOriginalProcedureParagraphs(t *testing.T) {
	cases := []struct {
		name, env, symbol string
		line              int
	}{
		{"Cisp", "SCRIPTHOLD_REAL_COBOL_CISP", "MAIN-PROCEDURE", 40},
		{"CobolCraft", "SCRIPTHOLD_REAL_COBOL_OPTIMIZER", "Optimize", 68},
		{"Webbol", "SCRIPTHOLD_REAL_COBOL_WEBBOL", "MAIN-LOGIC", 36},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("qualified original COBOL source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatal("original COBOL source must be regular and <=1 MiB")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			doc := sourceDocumentForScanner(string(data))
			doc.Path = path
			result, err := (COBOLAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("partial: %+v", result.Analysis)
			}
			for _, sym := range result.Analysis.Symbols {
				if sym.Name == tc.symbol && sym.Kind == SymbolKindFunction && sym.NativeKind == "paragraph" && sym.NameRange.Start.Line == tc.line {
					return
				}
			}
			t.Errorf("original COBOL paragraph %q line %d absent, freeFormat=%v, symbols=%v", tc.symbol, tc.line, cobolLooksFreeForm(string(data)), sortedSymbolQualifiedNames(result.Analysis.Symbols))
		})
	}
}

func TestCOBOLProcedureParagraphsAfterUsingAndDataLFCRLF(t *testing.T) {
	cases := []struct{ name, text string }{
		{"free", "IDENTIFICATION DIVISION.\nPROGRAM-ID. DEMO.\nDATA DIVISION.\nWORKING-STORAGE SECTION.\n01 DATA-NAME PIC X.\nPROCEDURE DIVISION USING DATA-NAME.\nMAIN-PARA.\n DISPLAY 'hello'.\nNEXT-PARA.\n GOBACK.\n"},
		{"fixed", "       IDENTIFICATION DIVISION.\n       PROGRAM-ID. DEMO.\n       DATA DIVISION.\n       WORKING-STORAGE SECTION.\n       01 DATA-NAME PIC X.\n       PROCEDURE DIVISION.\n       MAIN-PARA.\n           DISPLAY 'hello'.\n       NEXT-PARA.\n           GOBACK.\n"},
	}
	for _, tc := range cases {
		for _, eol := range []string{"\n", "\r\n"} {
			t.Run(tc.name+"-"+strings.ReplaceAll(eol, "\r", "R"), func(t *testing.T) {
				code := strings.ReplaceAll(tc.text, "\n", eol)
				result, err := (COBOLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(code), testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
					t.Fatalf("partial: %+v", result.Analysis)
				}
				byName := symbolsByQualifiedName(result.Analysis.Symbols)
				for _, name := range []string{"DEMO.MAIN-PARA", "DEMO.NEXT-PARA"} {
					got, ok := byName[name]
					if !ok || got.Kind != SymbolKindFunction || got.NativeKind != "paragraph" {
						t.Errorf("missing %q, got=%+v, complete=%v", name, got, result.Analysis.CoverageComplete)
					}
				}
				if _, falseSymbol := byName["DEMO.DATA-NAME"]; falseSymbol {
					t.Error("data item treated as procedure")
				}
			})
		}
	}
}
