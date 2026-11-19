package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Check complete immutable originals, not snippets or inferred compiler behavior.
func TestAdaPinnedWholeOriginalGenericPackageInstantiation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_ADA_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete independently pinned Ada originals not configured")
	}
	for _, tc := range []struct {
		file  string
		count int
	}{
		{"ctags-input2.ads", 5},
		{"sample-binary-search.adb", 6},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 1<<20 {
				t.Fatalf("original source unavailable or unsafe: %v %v", info, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (AdaAnalyzer{}).Analyze(context.Background(),
				testSourceDocument(path, string(b)), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != tc.count {
				t.Fatalf("full original Ada outline: %+v", result.Analysis)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			if tc.file == "ctags-input2.ads" {
				for _, name := range []string{"input_2", "input_2.Generator", "input_2.Unsigned", "input_2.Real", "input_2.Random"} {
					if _, ok := got[name]; !ok {
						t.Errorf("supported original Ada symbol %s missing", name)
					}
				}
				if _, ok := got["input_2.Random_Unsigned"]; ok {
					t.Error("generic formal function fabricated")
				}
				return
			}
			sym, ok := got["Binary_Search.Int_Vec"]
			want := "package Int_Vec is new\n     Ada.Containers.Vectors (Index_Type => Natural, Element_Type => Integer);"
			if !ok || sym.Kind != SymbolKindPackage || sym.NativeKind != "package" ||
				sym.ParentQualifiedName != "Binary_Search" ||
				sym.DeclarationRange.Start.Line != 10 ||
				sym.DeclarationRange.End.Line != 11 ||
				sym.DeclarationRange.End.Column != 78 ||
				sym.SignatureRange == nil || sym.SignatureRange.End != sym.DeclarationRange.End ||
				sym.Signature != want {
				t.Errorf("authored original complete package instantiation: %+v, exists=%v", sym, ok)
			}
			for _, name := range []string{"Binary_Search", "Binary_Search.Contains", "Binary_Search.Parse_Int", "Binary_Search.Parse_Sorted_List", "Binary_Search.Print_Usage"} {
				if _, exists := got[name]; !exists {
					t.Errorf("Ada original named scope %s missing", name)
				}
			}
		})
	}
}

func TestAdaMultilineGenericInstantiationSourceLFCRLF(t *testing.T) {
	for _, tc := range []struct{ name, eol string }{
		{"LF", "\n"}, {"CRLF", "\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := []string{
				"procedure Root is",
				"   package Instance is new",
				"     Containers.Library (Label => \"a;b\", Size => 3); -- comment ;",
				"   procedure Next;",
				"begin",
				"   null;",
				"end Root;",
				"",
			}
			text := strings.Join(lines, tc.eol)
			result, err := (AdaAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("guard.adb", text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			sym, ok := got["Root.Instance"]
			want := strings.Join([]string{
				"package Instance is new",
				"     Containers.Library (Label => \"a;b\", Size => 3);",
			}, tc.eol)
			if !ok || sym.Signature != want ||
				sym.DeclarationRange.End.Line != 3 ||
				sym.DeclarationRange.End.Column != 53 ||
				sym.SignatureRange == nil || sym.SignatureRange.End != sym.DeclarationRange.End {
				t.Errorf("complete multiline package source %+v exists=%v want=%q", sym, ok, want)
			}
			if _, ok := got["Root.Next"]; !ok {
				t.Error("following valid declaration consumed")
			}
		})
	}
}

func TestAdaUnterminatedGenericInstantiationNeverEatsNextDeclaration(t *testing.T) {
	source := "procedure Root is\n   package Broken is new\n   procedure Next;\nbegin\n   null;\nend Root;\n"
	result, err := (AdaAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("broken.adb", source), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete {
		t.Fatalf("missing semicolon must mark incomplete coverage: %+v", result.Analysis)
	}
	got := symbolsByQualifiedName(result.Analysis.Symbols)
	if sym, ok := got["Root.Broken"]; ok && sym.DeclarationRange.End.Line >= 3 {
		t.Errorf("malformed instantiation swallowed Next: %+v", sym)
	}
	if _, ok := got["Root.Next"]; !ok {
		t.Error("following Ada procedure missing")
	}
}
