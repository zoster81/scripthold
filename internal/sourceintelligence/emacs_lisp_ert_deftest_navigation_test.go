package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestEmacsLispERTDeclarationsAreNavigableWithoutFabricatedForms(t *testing.T) {
	lines := []string{
		"(defun before-test () t)",
		"(ert-deftest simple-test () (should t))",
		"(ert-deftest regex-derived-case () (should (equal 1 1)))",
		"; (ert-deftest commented-fake () t)",
		"'(ert-deftest quoted-fake () t)",
		"`(ert-deftest backquoted-fake () t)",
		"(other-package/ert-deftest foreign-fake () t)",
		"(ERT-DEFTEST uppercase-fake () t)",
		"(ert-deftest \"string-fake\" () t)",
		"(ert-deftest :keyword-fake () t)",
		"(ert-deftest 'quoted-name-fake () t)",
		"(defun wrapper () (ert-deftest nested-fake () t))",
		"(defun after-test () t)",
	}
	for _, tc := range []struct{ name, eol string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			source := strings.Join(lines, tc.eol) + tc.eol
			result, err := (EmacsLispAnalyzer{}).Analyze(context.Background(), testSourceDocument("suite.el", source), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Fatalf("balanced Emacs Lisp source reported partial: %+v", result.Analysis.Diagnostics)
			}
			found := make(map[string]NormalizedSymbol)
			for _, symbol := range result.Analysis.Symbols {
				found[symbol.Name] = symbol
			}
			for name, line := range map[string]int{"simple-test": 2, "regex-derived-case": 3} {
				sym, exists := found[name]
				if !exists || sym.Kind != SymbolKindFunction || sym.NativeKind != "ert-deftest" || sym.NameRange.Start.Line != line {
					t.Fatalf("source-declared ERT test %s:%d not navigable: %+v", name, line, sym)
				}
			}
			for _, normal := range []string{"before-test", "wrapper", "after-test"} {
				if _, ok := found[normal]; !ok {
					t.Fatalf("normal Emacs Lisp callable %s lost", normal)
				}
			}
			for _, fake := range []string{"commented-fake", "quoted-fake", "backquoted-fake", "foreign-fake", "uppercase-fake", "string-fake", ":keyword-fake", "quoted-name-fake", "nested-fake"} {
				if sym, exists := found[fake]; exists {
					t.Fatalf("fabricated ERT test %s: %+v", fake, sym)
				}
			}
		})
	}
}
