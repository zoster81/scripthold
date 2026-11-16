package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Task/protected bodies define behavior, not additional package-owned types or
// direct package methods. Unsupported internal scopes must remain partial.
func TestAdaOpaqueTaskProtectedBodyOwnership(t *testing.T) {
	const portable = "package body Demo is\n" +
		"  task body Worker is\n" +
		"  begin\n" +
		"    null;\n" +
		"  end Worker;\n" +
		"  protected body Guard is\n" +
		"    procedure Inner is\n" +
		"    begin\n" +
		"      null;\n" +
		"    end Inner;\n" +
		"  end Guard;\n" +
		"  procedure Outside is\n" +
		"  begin\n" +
		"    null;\n" +
		"  end Outside;\n" +
		"end Demo;\n"
	for _, eol := range []string{"LF", "CRLF"} {
		t.Run("portable-"+eol, func(t *testing.T) {
			s := portable
			if eol == "CRLF" {
				s = strings.ReplaceAll(s, "\n", "\r\n")
			}
			got := adaOpaqueAnalyze(t, s)
			assertAdaNoTypeAt(t, got, "Worker", 2)
			assertAdaNoTypeAt(t, got, "Guard", 6)
			assertAdaNoCallableAt(t, got, "Inner", 7)
			assertAdaCallable(t, got, "Outside", 12, "Demo")
		})
	}
	if path := os.Getenv("SCRIPTHOLD_REAL_ADA_CTAGS_BODY"); path != "" {
		t.Run("ctags-task-and-protected", func(t *testing.T) {
			content := adaOpaqueReadOriginal(t, path, 50000)
			if !strings.Contains(content, "task body Ma_Tasche is") ||
				!strings.Contains(content, "protected body Objet_Protege is") {
				t.Fatal("independent original source oracle changed")
			}
			got := adaOpaqueAnalyze(t, content)
			assertAdaNoTypeAt(t, got, "Ma_Tasche", 4)
			assertAdaNoTypeAt(t, got, "Objet_Protege", 52)
			assertAdaNoCallableAt(t, got, "Faire", 60)
			assertAdaNoCallableAt(t, got, "Tester", 66)
			assertAdaPackage(t, got, "Input_1")
		})
	}
	if path := os.Getenv("SCRIPTHOLD_REAL_ADA_CTAGS_SPEC"); path != "" {
		t.Run("ctags-task-protected-spec", func(t *testing.T) {
			content := adaOpaqueReadOriginal(t, path, 50000)
			if !strings.Contains(content, "task type Tasche_Type_3_T") ||
				!strings.Contains(content, "protected type Discriminant_Protege") {
				t.Fatal("independent Ada task/protected specification changed")
			}
			got := adaOpaqueAnalyze(t, content)
			assertAdaType(t, got, "Ma_Tasche", 4, "Input_1")
			assertAdaType(t, got, "Tasche_Type_2_T", 15, "Input_1")
			assertAdaType(t, got, "Tasche_Type_3_T", 22, "Input_1")
			assertAdaType(t, got, "Objet_Protege", 32, "Input_1")
			assertAdaType(t, got, "Discriminant_Protege", 48, "Input_1")
			assertAdaNoCallableAt(t, got, "Faire", 34)
			assertAdaNoCallableAt(t, got, "Tester", 35)
		})
	}
	if path := os.Getenv("SCRIPTHOLD_REAL_ADA_GUI_BODY"); path != "" {
		t.Run("ada-gui-protected", func(t *testing.T) {
			content := adaOpaqueReadOriginal(t, path, 50000)
			if !strings.Contains(content, "protected body Exception_Handler is") ||
				!strings.Contains(content, "procedure Activate_Exception_Handler") {
				t.Fatal("independent Ada GUI original source oracle changed")
			}
			got := adaOpaqueAnalyze(t, content)
			assertAdaType(t, got, "Exception_Handler", 398, "Ada_GUI.Gnoga")
			assertAdaNoTypeAt(t, got, "Exception_Handler", 404)
			assertAdaNoCallableAt(t, got, "Log", 399)
			assertAdaNoCallableAt(t, got, "Log", 405)
			assertAdaCallable(t, got, "Activate_Exception_Handler", 422, "Ada_GUI.Gnoga")
		})
	}
}

func adaOpaqueAnalyze(t *testing.T, source string) []NormalizedSymbol {
	t.Helper()
	result, err := (AdaAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("opaque.adb", source), testAnalyzeOptions(true, 2048))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.Truncated {
		t.Fatalf("unexpected truncation: %+v", result.Analysis)
	}
	return result.Analysis.Symbols
}

func adaOpaqueReadOriginal(t *testing.T, path string, limit int64) string {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit || info.Size() < 10 {
		t.Fatalf("independent bounded Ada file unavailable: %v %v", info, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertAdaNoTypeAt(t *testing.T, symbols []NormalizedSymbol, name string, line int) {
	t.Helper()
	for _, s := range symbols {
		if s.Name == name && s.Kind == SymbolKindType && s.NameRange.Start.Line == line {
			t.Errorf("Ada task/protected body falsely indexed as type: %+v", s)
		}
	}
}

func assertAdaNoCallableAt(t *testing.T, symbols []NormalizedSymbol, name string, line int) {
	t.Helper()
	for _, s := range symbols {
		if s.Name == name && s.Kind == SymbolKindFunction && s.NameRange.Start.Line == line {
			t.Errorf("nested Ada protected member falsely projected as direct callable: %+v", s)
		}
	}
}

func assertAdaCallable(t *testing.T, symbols []NormalizedSymbol, name string, line int, owner string) {
	t.Helper()
	for _, s := range symbols {
		if s.Name == name && s.Kind == SymbolKindFunction && s.NameRange.Start.Line == line &&
			s.ParentQualifiedName == owner && s.ID != "" {
			return
		}
	}
	t.Errorf("authored sibling Ada procedure not navigable %s at %d under %s; indexed=%v", name, line, owner, sortedSymbolQualifiedNames(symbols))
}

func assertAdaType(t *testing.T, symbols []NormalizedSymbol, name string, line int, owner string) {
	t.Helper()
	for _, s := range symbols {
		if s.Name == name && s.Kind == SymbolKindType && s.NameRange.Start.Line == line &&
			s.ParentQualifiedName == owner && s.ID != "" {
			return
		}
	}
	t.Errorf("authored Ada protected specification type missing %s at %d under %s", name, line, owner)
}

func assertAdaPackage(t *testing.T, symbols []NormalizedSymbol, name string) {
	t.Helper()
	for _, s := range symbols {
		if s.Name == name && s.Kind == SymbolKindPackage && s.ID != "" {
			return
		}
	}
	t.Errorf("Ada enclosing package not navigable: %s", name)
}
