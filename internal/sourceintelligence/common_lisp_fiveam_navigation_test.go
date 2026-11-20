package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestCommonLispFiveAMTestDeclarationsRequireExplicitPackageAndSuite(t *testing.T) {
	lines := []string{
		"(defpackage :demo.test (:use :cl :fiveam))",
		"(in-package :demo.test)",
		"(def-suite config-tests :description \"A real suite\")",
		"(in-suite config-tests)",
		"(test merge-config--deep (is (= 1 1)))",
		"(test parse-config (is t))",
		"(defun normal-helper () t)",
		"; (test commented-fake (is t))",
		"'(test quoted-fake (is t))",
		"`(test backquoted-fake (is t))",
		"(foreign:test qualified-fake (is t))",
		"(test \"string-fake\" (is t))",
		"(test :keyword-fake (is t))",
		"(test 'quoted-name-fake (is t))",
		"(defun wrapper () (test nested-fake (is t)))",
		"(defpackage :unrelated (:use :cl))",
		"(in-package :unrelated)",
		"(def-suite unrelated-suite)",
		"(in-suite unrelated-suite)",
		"(test unrelated-fake (is t))",
		"(defpackage :later (:use :cl :fiveam))",
		"(in-package :later)",
		"(test no-active-suite-fake (is t))",
		"\"(defpackage :pretend (:use :fiveam))\"",
	}
	for _, tc := range []struct{ name, eol string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			source := strings.Join(lines, tc.eol) + tc.eol
			result, err := (CommonLispAnalyzer{}).Analyze(context.Background(), testSourceDocument("suite.lisp", source), testAnalyzeOptions(false, 80))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Fatalf("balanced Common Lisp scan incomplete: %+v", result.Analysis.Diagnostics)
			}
			found := make(map[string]NormalizedSymbol)
			for _, sym := range result.Analysis.Symbols {
				found[sym.Name] = sym
			}
			for name, want := range map[string]struct {
				line   int
				kind   SymbolKind
				native string
			}{
				"config-tests":       {3, SymbolKindSection, "def-suite"},
				"merge-config--deep": {5, SymbolKindFunction, "test"},
				"parse-config":       {6, SymbolKindFunction, "test"},
				"normal-helper":      {7, SymbolKindFunction, "defun"},
				"wrapper":            {15, SymbolKindFunction, "defun"},
			} {
				sym, ok := found[name]
				if !ok || sym.Kind != want.kind || sym.NativeKind != want.native || sym.NameRange.Start.Line != want.line {
					t.Fatalf("missing independently authored %s:%d: %+v", name, want.line, sym)
				}
			}
			for _, name := range []string{"commented-fake", "quoted-fake", "backquoted-fake", "qualified-fake",
				"string-fake", ":keyword-fake", "quoted-name-fake", "nested-fake", "unrelated-suite", "unrelated-fake", "no-active-suite-fake"} {
				if sym, ok := found[name]; ok {
					t.Fatalf("false FiveAM symbol %s: %+v", name, sym)
				}
			}
		})
	}
}
