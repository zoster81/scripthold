package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Independent original-source assertion: a let name ends at its authored
// identifier, and a parenthesized symbolic operator is a function identity.
func TestFSharpPinnedWholeOriginalLetNameBoundaries(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_FSHARP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("complete independent FSharp originals not configured")
	}
	cases := []struct {
		file  string
		count int
	}{
		{"elmish-program.fs", 24},
		{"fsharp-data-csv-extensions.fs", 13},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 1<<20 {
				t.Fatalf("unsafe original source %v %v", info, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (FSharpAnalyzer{}).Analyze(context.Background(), testSourceDocument(path, string(b)), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != tc.count {
				t.Fatalf("whole original FSharp outline %+v", result.Analysis)
			}
			byLine := make(map[int]NormalizedSymbol)
			for _, s := range result.Analysis.Symbols {
				byLine[s.NameRange.Start.Line] = s
			}
			if tc.file == "fsharp-data-csv-extensions.fs" {
				s, ok := byLine[116]
				if !ok || s.Name != "?" || s.Kind != SymbolKindFunction || s.NativeKind != "let" ||
					s.QualifiedName != "FSharp.Data.CsvExtensions.?" ||
					s.NameRange.Start.Column != 10 || s.NameRange.End.Column != 11 {
					t.Errorf("full original symbolic let (?) function identity: %+v exists=%v", s, ok)
				}
				return
			}
			for _, e := range []struct {
				line int
				name string
				col  int
			}{
				{55, "withSubscription", 25}, {60, "mapSubscription", 24}, {65, "withConsoleTrace", 25},
				{88, "withTrace", 18}, {98, "withErrorHandler", 25}, {103, "withTermination", 24},
				{108, "mapTermination", 23}, {113, "mapErrorHandler", 24}, {118, "onError", 16},
				{122, "withSetState", 21}, {128, "setState", 17}, {132, "view", 13},
				{136, "init", 13}, {140, "update", 15}, {144, "map", 12},
				{160, "runWithDispatch", 24}, {208, "runWith", 16}, {211, "run", 12},
			} {
				s, ok := byLine[e.line]
				if !ok || s.Name != e.name || s.Kind != SymbolKindFunction || s.NameRange.End.Column != e.col {
					t.Errorf("original Elmish let authored name on line %d: %+v exists=%v", e.line, s, ok)
				}
			}
		})
	}
}

// Keep simple bindings, modifiers, nested lets, comments and line endings
// stable while recovering the directly authored symbolic operator.
func TestFSharpLetSymbolicAndNormalBoundariesLFCRLF(t *testing.T) {
	for _, tc := range []struct{ name, eol string }{
		{"LF", "\n"}, {"CRLF", "\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := []string{
				"module Demo =",
				"    let (?) (row: string) (field: string) = row",
				"    let functionName item = item",
				"    let inline applied input = input",
				"    let rec loop x = x",
				"    let scalar = 42",
				"    let printed = \"let (?) fake = 2\"",
				"    // let (?) fake = 2",
				"    let outer x =",
				"        let inner y = y",
				"        inner x",
				"",
			}
			text := strings.Join(lines, tc.eol)
			result, err := (FSharpAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("demo.fs", text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 8 {
				t.Fatalf("valid FSharp module and seven let declarations: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, expectation := range []struct {
				name   string
				kind   SymbolKind
				line   int
				column int
			}{
				{"Demo.?", SymbolKindFunction, 2, 10},
				{"Demo.functionName", SymbolKindFunction, 3, 9},
				{"Demo.applied", SymbolKindFunction, 4, 16},
				{"Demo.loop", SymbolKindFunction, 5, 13},
				{"Demo.scalar", SymbolKindVariable, 6, 9},
				{"Demo.printed", SymbolKindVariable, 7, 9},
				{"Demo.outer", SymbolKindFunction, 9, 9},
			} {
				s, ok := byName[expectation.name]
				if !ok || s.Kind != expectation.kind || s.NameRange.Start.Line != expectation.line ||
					s.NameRange.Start.Column != expectation.column ||
					s.NameRange.End.Line != expectation.line ||
					s.NameRange.End.Column != expectation.column+len(s.Name) {
					t.Errorf("source-authored let %+v: symbol=%+v exists=%v", expectation, s, ok)
				}
			}
			for _, fake := range []string{"Demo.inner", "Demo.fake", "Demo.row", "Demo.field"} {
				if _, ok := byName[fake]; ok {
					t.Errorf("invented nested/comment/parameter symbol %s", fake)
				}
			}
		})
	}
}
