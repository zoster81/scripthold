package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Inspect the complete immutable original, including its compound setter name.
func TestCommonLispPinnedWholeOriginalCompoundSetterName(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_COMMON_LISP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two immutable Common Lisp originals are not configured")
	}
	for _, tc := range []struct {
		file  string
		count int
	}{
		{"quicklisp-config.lisp", 3},
		{"slime-match.lisp", 14},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original source unavailable or unsafe: %v %v", info, err)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (CommonLispAnalyzer{}).Analyze(context.Background(),
				testSourceDocument(path, string(content)), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != tc.count {
				t.Fatalf("whole original incomplete or declarations missing: %+v", result.Analysis)
			}
			if tc.file != "quicklisp-config.lisp" {
				return
			}
			found := false
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Name == "setf" {
					t.Errorf("false truncated compound setter identity: %+v", symbol)
				}
				if symbol.Name != "(setf config-value)" {
					continue
				}
				found = true
				if symbol.QualifiedName != symbol.Name ||
					symbol.NameRange.Start.Line != 28 || symbol.NameRange.Start.Column != 8 ||
					symbol.NameRange.End.Line != 28 || symbol.NameRange.End.Column != 27 {
					t.Errorf("compound setter name/qualified/range drift: %+v", symbol)
				}
			}
			if !found {
				t.Errorf("missing source-defined compound function identity (setf config-value)")
			}
		})
	}
}

func TestCommonLispCompoundSetterNameLFCRLF(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(strings.ReplaceAll(eol, "\r", "CR"), func(t *testing.T) {
			source := strings.Join([]string{
				"(defun (setf config-value) (v path)",
				"  (list v path))",
				"(defun ordinary (x) x)",
				"",
			}, eol)
			result, err := (CommonLispAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("setter.lisp", source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != 2 || !result.Analysis.CoverageComplete {
				t.Fatalf("unexpected functions or coverage: %+v", result.Analysis)
			}
			first, second := result.Analysis.Symbols[0], result.Analysis.Symbols[1]
			if first.Name != "(setf config-value)" || first.QualifiedName != first.Name ||
				first.NameRange.Start.Line != 1 || first.NameRange.Start.Column != 8 ||
				first.NameRange.End.Column != 27 || first.DeclarationRange.End.Line != 2 ||
				second.Name != "ordinary" || second.DeclarationRange.Start.Line != 3 {
				t.Errorf("incorrect compound setter or ordinary defun: first=%+v second=%+v", first, second)
			}
		})
	}
}

func TestCommonLispCompoundSetterNameBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, source, want string
		valid              bool
	}{
		{"case-insensitive operator", "(DEFUN (SETF Foo) (x) x)\n(defun sibling () nil)\n", "(setf Foo)", true},
		{"multiline compound name", "(defun (setf\n  config-value) (x) x)\n(defun sibling () nil)\n", "(setf config-value)", true},
		{"missing setter target", "(defun (setf) (x) x)\n(defun sibling () nil)\n", "", false},
		{"extra setter target", "(defun (setf foo bar) (x) x)\n(defun sibling () nil)\n", "", false},
		{"nested setter target", "(defun (setf (foo)) (x) x)\n(defun sibling () nil)\n", "", false},
		{"unsupported compound operator", "(defun (custom foo) (x) x)\n(defun sibling () nil)\n", "", false},
		{"oversized setter target", "(defun (setf " + strings.Repeat("x", 1025) + ") (x) x)\n(defun sibling () nil)\n", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (CommonLispAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("shape.lisp", tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			got := result.Analysis.Symbols
			if tc.valid {
				if !result.Analysis.CoverageComplete || len(got) != 2 ||
					got[0].Name != tc.want || got[0].QualifiedName != tc.want ||
					!strings.HasPrefix(got[0].Signature, "(DEFUN ") && !strings.HasPrefix(got[0].Signature, "(defun ") ||
					!strings.Contains(got[0].Signature, ")") ||
					got[1].Name != "sibling" {
					t.Fatalf("valid compound setter and sibling: %+v", result.Analysis)
				}
			} else if result.Analysis.CoverageComplete || len(got) != 1 || got[0].Name != "sibling" {
				t.Fatalf("malformed or unsupported compound name must not invent identity or claim full coverage: %+v", result.Analysis)
			}
		})
	}
}
