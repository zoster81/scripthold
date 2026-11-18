package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Complete upstream Objective-C++ originals guard previously incorrect
// destructor names without broadening the explicitly supported grammar.
func TestObjectiveCPPPinnedWholeOriginalSymbolsAndDestructors(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_OBJECTIVE_CPP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("independent complete Objective-C++ originals not configured")
	}
	type expected struct {
		name, qualified, kind string
		line                  int
		column                int
	}
	for _, tc := range []struct {
		file    string
		entries []expected
	}{
		{"fastbuild-osmenu.mm", []expected{
			{"MenuItem", "MenuItem", "class", 15, 0},
			{"MenuItem", "MenuItem@implementation", "implementation", 20, 0},
			{"SelectorCallback:", "MenuItem.SelectorCallback:", "method", 21, 0},
			{"MenuOSX_Create", "MenuOSX_Create", "function", 30, 0},
			{"MenuOSX_Destroy", "MenuOSX_Destroy", "function", 41, 0},
			{"MenuOSX_AddItem", "MenuOSX_AddItem", "function", 50, 0},
		}},
		{"llvm-ivar-objects.mm", []expected{
			{"NSObject", "NSObject", "class", 9, 0},
			{"release", "NSObject.release", "method", 12, 0},
			{"printf", "printf", "function", 15, 0},
			{"count", "count", "variable", 17, 0},
			{"X", "X", "struct", 18, 0},
			{"X", "X.X", "constructor", 19, 0},
			{"~X", "X.~X", "destructor", 20, 3},
			{"value", "X.value", "field", 21, 0},
			{"Y", "Y", "struct", 24, 0},
			{"Y", "Y.Y", "constructor", 25, 0},
			{"~Y", "Y.~Y", "destructor", 26, 3},
			{"value", "Y.value", "field", 27, 0},
			{"Super", "Super", "class", 30, 0},
			{"finalize", "Super.finalize", "method", 35, 0},
			{"A", "A", "class", 38, 0},
			{"print", "A.print", "method", 45, 0},
			{"finalize", "A.finalize", "method", 46, 0},
			{"Super", "Super@implementation", "implementation", 49, 0},
			{"print", "Super.print", "method", 50, 0},
			{"finalize", "Super.finalize", "method", 55, 0},
			{"A", "A@implementation", "implementation", 58, 0},
			{"print", "A.print", "method", 59, 0},
			{"finalize", "A.finalize", "method", 67, 0},
			{"main", "main", "function", 70, 0},
			{"S", "S", "class", 76, 0},
			{"operator=", "S.operator=", "operator", 78, 0},
			{"I", "I", "class", 81, 0},
			{"position", "I.position", "property", 84, 0},
			{"I", "I@implementation", "implementation", 87, 0},
			{"test3", "test3", "namespace", 92, 0},
			{"S", "test3.S", "struct", 92, 0},
			{"S", "test3.S.S", "constructor", 92, 0},
			{"B", "B@implementation", "implementation", 93, 0},
			{"test4", "test4", "namespace", 99, 0},
			{"S", "test4.S", "struct", 99, 0},
			{"~S", "test4.S.~S", "destructor", 99, 30},
			{"C", "C@implementation", "implementation", 100, 0},
		}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded pinned complete original invalid: info=%v err=%v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (ObjectiveCPPAnalyzer{}).Analyze(context.Background(),
				testSourceDocument(path, string(data)), testAnalyzeOptions(true, 4096))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Errorf("complete original must retain full supported source coverage: %+v", result.Analysis)
			}
			got := result.Analysis.Symbols
			if len(got) != len(tc.entries) {
				t.Fatalf("original emitted %d structural symbols, expected %d: %v",
					len(got), len(tc.entries), sortedSymbolQualifiedNames(got))
			}
			for i, want := range tc.entries {
				s := got[i]
				if s.Name != want.name || s.QualifiedName != want.qualified ||
					string(s.Kind) != want.kind || s.NameRange.Start.Line != want.line ||
					s.DeclarationRange.Start.Line != want.line {
					t.Errorf("complete source symbol %d got=%+v expected=%+v", i, s, want)
				}
				if want.column != 0 &&
					(s.NameRange.Start.Column != want.column ||
						s.NameRange.End.Column != want.column+len(want.name)) {
					t.Errorf("authored tilde destructor must retain exact source name range: %+v", s)
				}
				if s.Name == "m_Window" || s.Name == "alloc" || s.Name == "init" ||
					s.Name == "@synthesize" {
					t.Errorf("unsupported ivar/untyped selector or synthesis fabricated as supported declaration: %+v", s)
				}
			}
		})
	}
}
