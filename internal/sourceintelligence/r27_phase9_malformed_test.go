package sourceintelligence

import (
	"context"
	"testing"
)

func TestR27Phase9MalformedSourceLowersCoverage(t *testing.T) {
	tests := []struct {
		name, path, text string
		analyzer         SourceAnalyzer
	}{
		{"fortran", "bad.f90", "subroutine good()\nend subroutine good\nvalue = \"unterminated\n", FortranAnalyzer{}},
		{"cobol", "bad.cbl", "IDENTIFICATION DIVISION.\nPROGRAM-ID. DEMO.\nPROCEDURE DIVISION.\nDISPLAY \"unterminated\n", COBOLAnalyzer{}},
		{"ada", "bad.adb", "procedure Good;\nValue := \"unterminated\n", AdaAnalyzer{}},
		{"matlab", "bad.m", "function good()\nend\nvalue = \"unterminated\n", MATLABAnalyzer{}},
		{"octave", "bad.m", "function good()\nendfunction\nvalue = \"unterminated\n", OctaveAnalyzer{}},
		{"julia", "bad.jl", "function good()\nend\nvalue = \"\"\"unterminated\n", JuliaAnalyzer{}},
		{"r", "bad.r", "good <- function(x) x\nvalue <- \"unterminated\n", RAnalyzer{}},
		{"haskell", "bad.hs", "good :: Int -> Int\ngood x = x\nvalue = \"unterminated\n", HaskellAnalyzer{}},
		{"ocaml", "bad.ml", "let good x = x\nlet value = \"unterminated\n", OCamlAnalyzer{}},
		{"common-lisp", "bad.lisp", "(defun good (x) x)\n(setq value \"unterminated\n", CommonLispAnalyzer{}},
		{"clojure", "bad.clj", "(defn good [x] x)\n(def value \"unterminated\n", ClojureAnalyzer{}},
		{"emacs-lisp", "bad.el", "(defun good (x) x)\n(setq value \"unterminated\n", EmacsLispAnalyzer{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), phase9TestDocument(tc.path, tc.text), phase3AnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
				t.Fatalf("%s malformed source reported complete: %+v", tc.name, result.Analysis)
			}
		})
	}
}
