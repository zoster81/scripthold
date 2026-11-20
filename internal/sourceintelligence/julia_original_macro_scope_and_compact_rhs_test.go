package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Read-only, opt-in full upstream originals, never copied into a tracked test.
func TestJuliaOriginalMacroWrappedFunctionPreservesModuleOwnership(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JULIA_OUTLINE_ROOT")
	if root == "" {
		t.Skip("set SCRIPTHOLD_REAL_JULIA_OUTLINE_ROOT to the two complete pinned originals")
	}
	data, err := os.ReadFile(filepath.Join(root, "julia-compiler-dev-tools.jl"))
	if err != nil || len(data) == 0 {
		t.Fatalf("configured full CompilerDevTools original missing or empty: %v", err)
	}
	doc := sourceDocumentForScanner(string(data))
	doc.Path = "julia-compiler-dev-tools.jl"
	result, err := (JuliaAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(result.Analysis.Symbols); got != 14 {
		t.Errorf("complete original supported declarations: got %d, want 14", got)
	}
	// The macro-wrapped function has its own 'end'; four authored declarations
	// after it remain inside the module that closes only at source line 81.
	for _, tc := range []struct {
		name string
		line int
	}{
		{"lookup_method_instance", 44},
		{"Compiler.transform_result_for_cache", 48},
		{"with_new_compiler", 65},
		{"with_new_compiler", 67},
	} {
		found := 0
		for _, sym := range result.Analysis.Symbols {
			if sym.Name != tc.name || sym.NameRange.Start.Line != tc.line {
				continue
			}
			found++
			if sym.ParentQualifiedName != "CompilerDevTools" ||
				sym.ParentID == "" ||
				sym.QualifiedName != "CompilerDevTools."+tc.name {
				t.Errorf("authored declaration %s at %d lost module parent: %+v", tc.name, tc.line, sym)
			}
		}
		if found != 1 {
			t.Errorf("original declaration %s at line %d: %d matches", tc.name, tc.line, found)
		}
	}
}

func TestJuliaOriginalMultilineCompactFunctionIncludesRHS(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JULIA_OUTLINE_ROOT")
	if root == "" {
		t.Skip("set SCRIPTHOLD_REAL_JULIA_OUTLINE_ROOT for both whole Julia originals")
	}
	data, err := os.ReadFile(filepath.Join(root, "exercism-annalyn-exemplar.jl"))
	if err != nil || len(data) == 0 {
		t.Fatalf("configured full Exercism Julia original missing or empty: %v", err)
	}
	result, err := (JuliaAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(string(data)), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Analysis.Symbols) != 4 {
		t.Errorf("original Exercism 4 authored compact functions, got %d", len(result.Analysis.Symbols))
	}
	found := 0
	for _, sym := range result.Analysis.Symbols {
		if sym.Name != "can_free_prisoner" || sym.NameRange.Start.Line != 27 {
			continue
		}
		found++
		if sym.NativeKind != "compact-function" || sym.ParentQualifiedName != "" ||
			sym.DeclarationRange.End.Line != 28 || sym.DeclarationRange.End.Column != 101 ||
			sym.SignatureRange == nil || sym.SignatureRange.End.Line != 28 ||
			sym.SignatureRange.End.Column != 101 ||
			!strings.Contains(sym.Signature, "dog_present && !archer_awake") {
			t.Errorf("complete authored multiline compact RHS omitted: %+v", sym)
		}
	}
	if found != 1 {
		t.Errorf("multiline compact function authored at 27 has %d identities", found)
	}
}

func TestJuliaMacroWrappedFunctionScopeVariants(t *testing.T) {
	for _, tc := range []struct{ name, source string }{
		{"nested-control", "module Demo\n@eval @noinline function generated(x)\n  if x\n    x\n  end\nend\nfunction after(x)\n  x\nend\nend\n"},
		{"inline-macro-end", "module Demo\n@eval @noinline function generated(x) x end\nfunction after(x)\n  x\nend\nend\n"},
		{"macro-compact-has-no-block-end", "module Demo\n@eval @noinline generated(x) = x\nfunction after(x)\n  x\nend\nend\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (JuliaAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 32))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Errorf("complete supported module shape unexpectedly partial")
			}
			found := 0
			for _, sym := range result.Analysis.Symbols {
				if sym.Name == "generated" {
					t.Errorf("unsupported macro expansion must not invent callable %+v", sym)
				}
				if sym.Name == "after" {
					found++
					if sym.QualifiedName != "Demo.after" || sym.ParentQualifiedName != "Demo" || sym.ParentID == "" {
						t.Errorf("valid named callable lost lexical module after macro: %+v", sym)
					}
				}
			}
			if found != 1 {
				t.Errorf("authored function after macro found %d times", found)
			}
		})
	}
}

func TestJuliaCompactFunctionRHSContinuationOnlyWhenIndented(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		endLine      int
	}{
		{"continued", "f(x) =\n   !x\nnext(x) = x\n", 2},
		{"continued-crlf", strings.ReplaceAll("f(x) =\n   !x\nnext(x) = x\n", "\n", "\r\n"), 2},
		{"missing-rhs-next-declaration", "f(x) =\nnext(x) = x\n", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (JuliaAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 32))
			if err != nil {
				t.Fatal(err)
			}
			fCount, nextCount := 0, 0
			for _, sym := range result.Analysis.Symbols {
				switch sym.Name {
				case "f":
					fCount++
					if sym.DeclarationRange.End.Line != tc.endLine ||
						sym.SignatureRange == nil || sym.SignatureRange.End.Line != tc.endLine {
						t.Errorf("compact declaration continuation wrong: %+v", sym)
					}
				case "next":
					nextCount++
					if sym.NameRange.Start.Line != 3 && tc.endLine == 2 ||
						sym.NameRange.Start.Line != 2 && tc.endLine == 1 {
						t.Errorf("subsequent authored function consumed: %+v", sym)
					}
				}
			}
			if fCount != 1 || nextCount != 1 {
				t.Errorf("expected one declaration each, f=%d next=%d", fCount, nextCount)
			}
		})
	}
}

func TestJuliaMacroFunctionEndAndCompactRHSBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, source string
	}{
		{"module-macro-scope", "module Demo\n" +
			"@eval @noinline function generated(x)\n" +
			"  x\nend\n" +
			"function after_macro(x)\n  x\nend\nend\n"},
		{"module-macro-scope-crlf", strings.ReplaceAll("module Demo\n@eval @noinline function generated(x)\n  x\nend\nfunction after_macro(x)\n  x\nend\nend\n", "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (JuliaAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 32))
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, sym := range result.Analysis.Symbols {
				if sym.Name == "after_macro" {
					count++
					if sym.ParentQualifiedName != "Demo" || sym.QualifiedName != "Demo.after_macro" || sym.ParentID == "" {
						t.Errorf("later function should retain module owner: %+v", sym)
					}
				}
				if sym.Name == "generated" {
					t.Errorf("macro-generated callable unsupported, not authored standalone declaration: %+v", sym)
				}
			}
			if count != 1 {
				t.Errorf("authored post-macro named function count=%d", count)
			}
		})
	}
}
