package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type cobolOriginalDeclaration struct {
	name, native, owner string
	kind                SymbolKind
	line                int
}

// Exact original source bytes are opt-in external fixtures, never committed.
// A specified but missing corpus is a failing test setup.
func TestCOBOLNumberedOriginalProgramsAndControl(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_COBOL_NUMBERED_ROOT")
	if root == "" {
		t.Skip("set SCRIPTHOLD_REAL_COBOL_NUMBERED_ROOT to complete pinned COBOL sources")
	}
	cases := []struct {
		file  string
		fixed bool
		want  []cobolOriginalDeclaration
	}{
		{"drujensen-fib.cbl", true, []cobolOriginalDeclaration{
			{"fibmain", "program-id", "", SymbolKindModule, 2},
			{"WORKING-STORAGE", "section", "fibmain", SymbolKindSection, 5},
			{"fib", "program-id", "", SymbolKindModule, 15},
			{"LOCAL-STORAGE", "section", "fib", SymbolKindSection, 18},
			{"LINKAGE", "section", "fib", SymbolKindSection, 21},
		}},
		{"azac-cow.cbl", false, []cobolOriginalDeclaration{
			{"cow", "program-id", "", SymbolKindModule, 2},
			{"working-storage", "section", "cow", SymbolKindSection, 8},
			{"bad-query-error", "paragraph", "cow", SymbolKindFunction, 75},
			{"web-header", "paragraph", "cow", SymbolKindFunction, 80},
			{"getquery", "program-id", "", SymbolKindModule, 101},
			{"input-output", "section", "getquery", SymbolKindSection, 105},
			{"file", "section", "getquery", SymbolKindSection, 111},
			{"working-storage", "section", "getquery", SymbolKindSection, 115},
			{"linkage", "section", "getquery", SymbolKindSection, 161},
			{"showvars", "program-id", "", SymbolKindModule, 200},
			{"working-storage", "section", "showvars", SymbolKindSection, 203},
			{"linkage", "section", "showvars", SymbolKindSection, 207},
			{"checkquery", "program-id", "", SymbolKindModule, 241},
			{"working-storage", "section", "checkquery", SymbolKindSection, 244},
			{"linkage", "section", "checkquery", SymbolKindSection, 271},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, tc.file))
			if err != nil {
				t.Fatalf("original source missing/unreadable: %v", err)
			}
			if tc.fixed && cobolLooksFreeForm(string(data)) {
				t.Error("six-digit sequence-only blank records incorrectly classified as free-form")
			}
			doc := sourceDocumentForScanner(string(data))
			doc.Path = tc.file
			result, err := (COBOLAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Errorf("original source unexpectedly incomplete/truncated")
			}
			symbols := result.Analysis.Symbols
			for _, want := range tc.want {
				matched := 0
				for _, got := range symbols {
					if got.Name != want.name || got.Kind != want.kind ||
						got.NativeKind != want.native || got.NameRange.Start.Line != want.line {
						continue
					}
					matched++
					if got.ParentQualifiedName != want.owner || got.QualifiedName !=
						qualifiedCOBOLTestName(want.owner, want.name) {
						t.Errorf("wrong owner for %s line %d: %+v", want.name, want.line, got)
					}
					if (want.owner != "") != (got.ParentID != "") {
						t.Errorf("wrong parent reference for %s line %d: %+v", want.name, want.line, got)
					}
				}
				if matched != 1 {
					t.Errorf("expected one %s/%s at line %d owner %q; got %d; all=%v",
						want.kind, want.name, want.line, want.owner, matched,
						sortedSymbolQualifiedNames(symbols))
				}
			}
			if len(symbols) != len(tc.want) {
				t.Errorf("original symbols=%d want=%d; invented or omitted symbols", len(symbols), len(tc.want))
			}
		})
	}
}

func qualifiedCOBOLTestName(owner, name string) string {
	if owner == "" {
		return name
	}
	return owner + "." + name
}

func TestCOBOLSequenceOnlyFixedRecordsDoNotForceFreeFormat(t *testing.T) {
	const original = "000010 IDENTIFICATION DIVISION.\n" +
		"000020 PROGRAM-ID. DEMO.\n" +
		"000030\n" +
		"000040 DATA DIVISION.\n" +
		"000050 WORKING-STORAGE SECTION.\n" +
		"000060 PROCEDURE DIVISION.\n" +
		"000070     CALL \"DEMO\" USING BY VALUE X.\n" +
		"000080 END PROGRAM DEMO.\n"
	for _, tc := range []struct{ name, code string }{
		{"lf", original},
		{"crlf", strings.ReplaceAll(original, "\n", "\r\n")},
		{"utf8-bom", "\uFEFF" + original},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(tc.code)
			if cobolLooksFreeForm(doc.Text) {
				t.Error("valid fixed numbered original source was classified as free-form")
			}
			result, err := (COBOLAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 32))
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"DEMO", "DEMO.WORKING-STORAGE"}
			if len(result.Analysis.Symbols) != len(want) {
				t.Errorf("expected exactly two authored symbols, got %v",
					sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			found := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, name := range want {
				if _, ok := found[name]; !ok {
					t.Errorf("missing %s: %v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
		})
	}
}
