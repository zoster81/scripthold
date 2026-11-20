package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The original OBS Studio pthread header opens and closes extern "C" under
// separate #if defined(__cplusplus) guards, including many intervening
// preprocessor branches. One complete textual guard is not a compile state.
func TestCFamilyOriginalOBSPthreadsConditionalLinkage(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	original := filepath.Join(root, "real-projects", "c-obsproject-obs-studio", "deps", "w32-pthreads", "implement.h")
	doc, err := OpenSourceDocument(context.Background(), original, OpenDocumentOptions{RequestedEncoding: "utf-8", MaxFileBytes: 1024 * 1024, MaxDecodedCharacters: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{{"c", CAnalyzer{}}, {"cpp", CPPAnalyzer{}}} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.analyzer.Analyze(context.Background(), doc, testAnalyzeOptions(true, 1024))
			if err != nil {
				t.Fatal(err)
			}
			if !got.Analysis.CoverageComplete || got.Analysis.Truncated {
				t.Fatalf("original OBS conditional linkage is balanced; got partial: %+v", got.Analysis.Diagnostics)
			}
			names := symbolsByQualifiedName(got.Analysis.Symbols)
			for _, fn := range []struct {
				name string
				line int
			}{{"ptw32_is_attr", 605}, {"ptw32_processInitialize", 619}, {"ptw32_new", 627}, {"ptw32_threadStart", 644}} {
				symbol, ok := names[fn.name]
				if !ok || symbol.Kind != SymbolKindFunction || symbol.ParentQualifiedName != "" || symbol.NameRange.Start.Line != fn.line {
					t.Errorf("incorrect source-derived function %q: %+v found=%v", fn.name, symbol, ok)
				}
			}
		})
	}
}

// Equivalent exact 'defined' syntax has one stable state; unrelated boolean
// expressions and changing macros must not be over-correlated.
func TestCFamilyExactDefinedGuardsAcrossIncludes(t *testing.T) {
	for _, tc := range []struct{ name, first, second string }{
		{"parenthesized", "#if defined(__cplusplus)", "#if defined (__cplusplus)"},
		{"mixed-if-ifdef", "#ifdef __cplusplus", "#if defined(__cplusplus)"},
		{"bare-operand", "#if defined __cplusplus", "#ifdef __cplusplus"},
		{"negated", "#if !defined(__cplusplus)", "#if ! defined (__cplusplus)"},
	} {
		for _, lang := range []struct {
			name     string
			analyzer SourceAnalyzer
		}{{"c", CAnalyzer{}}, {"cpp", CPPAnalyzer{}}} {
			for _, lineEnd := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
				t.Run(tc.name+"/"+lang.name+"/"+lineEnd.name, func(t *testing.T) {
					s := tc.first + "\nextern \"C\" {\n#endif\n" +
						"int inside(void);\n#include \"platform.h\"\n" +
						"#ifdef OTHER\nint optional(void);\n#endif\n" +
						tc.second + "\n}\n#endif\nint after(void);\n"
					got, err := lang.analyzer.Analyze(context.Background(), sourceDocumentForScanner(strings.ReplaceAll(s, "\n", lineEnd.value)), testAnalyzeOptions(true, 64))
					if err != nil {
						t.Fatal(err)
					}
					if !got.Analysis.CoverageComplete {
						t.Fatalf("equivalent macro states must balance conditional linkage: %+v", got.Analysis.Diagnostics)
					}
					names := symbolsByQualifiedName(got.Analysis.Symbols)
					for _, name := range []string{"inside", "after"} {
						if sym, ok := names[name]; !ok || sym.Kind != SymbolKindFunction || sym.ParentQualifiedName != "" {
							t.Errorf("missing/misnested function %s: %+v found=%v", name, sym, ok)
						}
					}
				})
			}
		}
	}
}

// An exact defined predicate can share a macro state with #ifdef; compound
// or malformed expressions must retain conservative, separate behavior.
func TestCFamilyExactDefinedPredicateBoundaries(t *testing.T) {
	for _, tc := range []struct {
		text, wantName string
		wantPositive   bool
	}{
		{"defined(FLAG)", "FLAG", true},
		{"defined ( FLAG )", "FLAG", true},
		{"defined FLAG", "FLAG", true},
		{"!defined(FLAG)", "FLAG", false},
		{"! defined ( FLAG )", "FLAG", false},
		{"defined(FLAG) || defined(OTHER)", "", false},
		{"defined(FLAG) && OTHER", "", false},
		{"defined(FLAG) /* trailing condition */", "", false},
		{"defined(FLAG", "", false},
		{"defined((FLAG))", "", false},
		{"defined FLAG EXTRA", "", false},
		{"definedFLAG", "", false},
		{"!!defined(FLAG)", "", false},
		{"defined(_VALID123)", "_VALID123", true},
		{"defined(3INVALID)", "", false},
	} {
		name, positive := cFamilyExactDefinedCondition(tc.text)
		if name != tc.wantName || name != "" && positive != tc.wantPositive {
			t.Errorf("%q: got %q positive=%v, want %q positive=%v", tc.text, name, positive, tc.wantName, tc.wantPositive)
		}
	}

	versions := map[string]int{"FLAG": 1}
	macroIf, _ := cFamilyConditionalState("#if", "defined(FLAG)", versions, 2, 3)
	macroIfdef, _ := cFamilyConditionalState("#ifdef", "FLAG", versions, 2, 3)
	if macroIf != macroIfdef || macroIf == "" {
		t.Errorf("defined(FLAG) and #ifdef FLAG must share a state: %q vs %q", macroIf, macroIfdef)
	}
	if differentEpoch, _ := cFamilyConditionalState("#if", "defined(FLAG)", versions, 3, 3); differentEpoch == macroIf {
		t.Error("a new include epoch may have changed an ordinary macro")
	}
	versions["FLAG"]++
	if changed, _ := cFamilyConditionalState("#if", "defined(FLAG)", versions, 2, 3); changed == macroIf {
		t.Error("macro mutation must invalidate correlation")
	}
	cppFirst, _ := cFamilyConditionalState("#if", "defined(__cplusplus)", versions, 0, 1)
	cppLater, _ := cFamilyConditionalState("#ifdef", "__cplusplus", versions, 12, 45)
	if cppFirst != cppLater || cppFirst == "" {
		t.Errorf("compiler-defined __cplusplus must remain stable across includes: %q != %q", cppFirst, cppLater)
	}
}
