package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreeBasicPinnedOriginalTypeOwnedDeclareFunctions(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_FREEBASIC_ORIGINAL_ROOT")
	if root == "" {
		t.Skip("pinned FreeBasic originals not configured")
	}
	original := filepath.Join(root, "freebasic-tiko-clsApp-9db5c84f.bi")
	b, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 5747 {
		t.Fatalf("unexpected original bytes: %d", len(b))
	}
	expected := []string{
		"IsValidDocumentPointer", "RemoveAllSelectionAttributes", "CreateEmptyDocument",
		"AddNewDocument", "RemoveDocument", "RemoveAllDocuments", "GetDocumentCount",
		"GetDocumentPtrByWindow", "GetDocumentPtrByFilename", "GetMainDocumentPtr",
		"GetResourceDocumentPtr", "GetSourceDocumentPtr", "GetHeaderDocumentPtr",
		"SaveProject", "ProjectSetFileType", "GetProjectCompiler",
	}
	for _, input := range []struct{ name, source string }{
		{"git-lf", string(b)}, {"worktree-crlf", strings.ReplaceAll(string(b), "\n", "\r\n")},
	} {
		t.Run(input.name, func(t *testing.T) {
			result, err := (FreeBasicAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("clsApp.bi", input.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 59 {
				t.Fatalf("expected all 59 source-authored symbols, got %d (complete=%v)", len(result.Analysis.Symbols), result.Analysis.CoverageComplete)
			}
			kinds := map[SymbolKind]int{}
			for _, s := range result.Analysis.Symbols {
				kinds[s.Kind]++
			}
			if kinds[SymbolKindType] != 1 || kinds[SymbolKindField] != 42 {
				t.Errorf("expected one type and 42 fields: %+v", kinds)
			}
			for i, name := range expected {
				line := 68 + i
				found := false
				for _, s := range result.Analysis.Symbols {
					if s.NameRange.Start.Line != line {
						continue
					}
					found = true
					if s.Name != name || s.Kind != SymbolKindMethod || s.NativeKind != "declare-function" ||
						s.ParentQualifiedName != "clsApp" || s.QualifiedName != "clsApp."+name {
						t.Errorf("original line %d: expected type-owned method %q, got name=%q kind=%q native=%q owner=%q qualified=%q",
							line, name, s.Name, s.Kind, s.NativeKind, s.ParentQualifiedName, s.QualifiedName)
					}
				}
				if !found {
					t.Errorf("missing original declaration %q at line %d", name, line)
				}
			}
		})
	}
}

func TestFreeBasicPinnedOriginalUnownedFunctionsRemainFunctions(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_FREEBASIC_ORIGINAL_ROOT")
	if root == "" {
		t.Skip("pinned FreeBasic originals not configured")
	}
	b, err := os.ReadFile(filepath.Join(root, "freebasic-chessbrain-time-3eaf3c.bi"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 5189 {
		t.Fatalf("unexpected original size: %d", len(b))
	}
	result, err := (FreeBasicAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("time.bi", string(b)), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 26 {
		t.Fatalf("expected complete original 26 symbols, got %d complete=%v", len(result.Analysis.Symbols), result.Analysis.CoverageComplete)
	}
	n := 0
	for _, s := range result.Analysis.Symbols {
		if s.Kind == SymbolKindFunction {
			n++
			if s.ParentQualifiedName != "" {
				t.Errorf("free function with unexpected owner: %+v", s)
			}
		}
	}
	if n != 8 {
		t.Fatalf("expected eight source-authored free routines, got %d", n)
	}
}

func TestFreeBasicMemberDeclareScopeVsFreeRoutine(t *testing.T) {
	source := "Type Box\n Declare Function MethodA() As Integer\n Declare Sub MethodB()\nEnd Type\nDeclare Function GlobalA() As Integer\nNamespace Scope\n Declare Function GlobalInNamespace() As Integer\nEnd Namespace\nFunction Box.MethodA() As Integer\nEnd Function\n"
	result, err := (FreeBasicAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("mixed.bas", source), testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	wants := map[string]SymbolKind{"Box.MethodA": SymbolKindMethod, "Box.MethodB": SymbolKindMethod, "GlobalA": SymbolKindFunction, "Scope.GlobalInNamespace": SymbolKindFunction}
	for _, s := range result.Analysis.Symbols {
		if want, ok := wants[s.QualifiedName]; ok && s.DeclarationRange.Start.Line <= 7 {
			if s.Kind != want {
				t.Errorf("%s expected %s, got %s", s.QualifiedName, want, s.Kind)
			}
			delete(wants, s.QualifiedName)
		}
	}
	if len(wants) > 0 {
		t.Errorf("missing declared names: %+v", wants)
	}
}
