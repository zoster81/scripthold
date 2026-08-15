package sourceintelligence

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestR27Phase9LegacyFormatsPreserveFixedAndFreeStructure(t *testing.T) {
	t.Run("fortran-fixed-continuation-offsets", func(t *testing.T) {
		text := "C     SUBROUTINE Fake()\r\n      MODULE LEGACY\r\n      USE ISO_C_BINDING\r\n      SUBROUTINE\r\n     & RUN(X)\r\n      END SUBROUTINE RUN\r\n      END MODULE LEGACY\r\n"
		document := phase9TestDocument("legacy.f", text)
		result, err := (FortranAnalyzer{}).Analyze(context.Background(), document, phase3AnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		for _, want := range []string{"LEGACY", "LEGACY.RUN"} {
			if _, ok := byName[want]; !ok {
				t.Fatalf("missing %s: %v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
		if _, leaked := byName["Fake"]; leaked {
			t.Fatalf("fixed-form comment leaked Fake: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
		run := byName["LEGACY.RUN"]
		_, nameRange, _, _ := run.SourceOffsets()
		if got := document.Text[nameRange.Start:nameRange.End]; got != "RUN" {
			t.Fatalf("continued Fortran name range points to %q, want RUN: %+v", got, nameRange)
		}
		if got := dependencyValues(result.Dependencies); !sameStringSet(got, []string{"ISO_C_BINDING"}) {
			t.Fatalf("fixed Fortran dependencies=%v", got)
		}
	})

	t.Run("fortran-type-spec-is-not-type-definition", func(t *testing.T) {
		text := "module demo\n  type(worker) :: item\ncontains\n  subroutine run()\n  end subroutine run\nend module demo\n"
		result, err := (FortranAnalyzer{}).Analyze(context.Background(), phase9TestDocument("demo.f90", text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if _, leaked := symbolsByQualifiedName(result.Analysis.Symbols)["demo.item"]; leaked {
			t.Fatalf("Fortran type-spec variable was overclaimed as a type: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	})

	t.Run("cobol-fixed-comment-and-free-format", func(t *testing.T) {
		fixed := "      * PROGRAM-ID. FAKE.\r\n       IDENTIFICATION DIVISION.\r\n       PROGRAM-ID. REAL.\r\n       PROCEDURE DIVISION.\r\n       MAIN SECTION.\r\n           COPY COMMON.\r\n"
		fixedResult, err := (COBOLAnalyzer{}).Analyze(context.Background(), phase9TestDocument("fixed.cob", fixed), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		fixedNames := symbolsByQualifiedName(fixedResult.Analysis.Symbols)
		if _, ok := fixedNames["REAL"]; !ok {
			t.Fatalf("fixed COBOL missing REAL: %v", sortedSymbolQualifiedNames(fixedResult.Analysis.Symbols))
		}
		if _, leaked := fixedNames["FAKE"]; leaked {
			t.Fatalf("fixed COBOL indicator comment leaked FAKE")
		}

		free := "IDENTIFICATION DIVISION.\nPROGRAM-ID. FREEDEMO.\nPROCEDURE DIVISION.\n*> COPY FAKE.\nMAIN SECTION.\nCOPY COMMON.\n"
		freeResult, err := (COBOLAnalyzer{}).Analyze(context.Background(), phase9TestDocument("free.cbl", free), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := symbolsByQualifiedName(freeResult.Analysis.Symbols)["FREEDEMO"]; !ok {
			t.Fatalf("free-form COBOL missing program: %v", sortedSymbolQualifiedNames(freeResult.Analysis.Symbols))
		}
		if got := dependencyValues(freeResult.Dependencies); !sameStringSet(got, []string{"COMMON"}) {
			t.Fatalf("free COBOL dependencies=%v", got)
		}
	})
}

func TestR27Phase9DynamicAndFunctionalBoundariesStayConservative(t *testing.T) {
	t.Run("matlab-transpose-and-control-scopes", func(t *testing.T) {
		text := "classdef Worker\n  methods\n    function out = first(obj, data)\n      value = data';\n      if value\n        out = value;\n      end\n    end\n    function out = second(obj, x)\n      out = x;\n    end\n  end\nend\n"
		result, err := (MATLABAnalyzer{}).Analyze(context.Background(), phase9TestDocument("Worker.m", text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete {
			t.Fatalf("valid MATLAB transpose/control source lowered coverage: %+v", result.Analysis.Diagnostics)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		for _, want := range []string{"Worker", "Worker.first", "Worker.second"} {
			if _, ok := byName[want]; !ok {
				t.Fatalf("MATLAB scope missing %s: %v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
	})

	t.Run("haskell-qualified-import-and-equation", func(t *testing.T) {
		text := "module Demo where\nimport qualified Data.Map as M\ndata Worker = Worker Int\nrun (Worker x) = x\nanswer = 42\npromoted = 'Worker\n"
		result, err := (HaskellAnalyzer{}).Analyze(context.Background(), phase9TestDocument("Demo.hs", text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete {
			t.Fatalf("valid Haskell promoted constructor lowered coverage: %+v", result.Analysis.Diagnostics)
		}
		if got := dependencyValues(result.Dependencies); !sameStringSet(got, []string{"Data.Map"}) {
			t.Fatalf("Haskell dependencies=%v", got)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		if symbol, ok := byName["Demo.run"]; !ok || symbol.Kind != SymbolKindFunction {
			t.Fatalf("Haskell equation function=%+v exists=%v", symbol, ok)
		}
		if symbol, ok := byName["Demo.answer"]; !ok || symbol.Kind != SymbolKindVariable {
			t.Fatalf("Haskell value binding=%+v exists=%v", symbol, ok)
		}
	})

	t.Run("ocaml-value-versus-function", func(t *testing.T) {
		text := "module Demo = struct\n  let answer = 42\n  let run x = x\nend\n"
		result, err := (OCamlAnalyzer{}).Analyze(context.Background(), phase9TestDocument("demo.ml", text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		if symbol, ok := byName["Demo.answer"]; !ok || symbol.Kind != SymbolKindVariable {
			t.Fatalf("OCaml value binding=%+v exists=%v", symbol, ok)
		}
		if symbol, ok := byName["Demo.run"]; !ok || symbol.Kind != SymbolKindFunction {
			t.Fatalf("OCaml function binding=%+v exists=%v", symbol, ok)
		}
	})

	t.Run("julia-compact-function", func(t *testing.T) {
		text := "module Demo\ncompact(x) = x\nadjoint = matrix'\nmacro tagged(ex)\n  ex\nend\nend\n"
		result, err := (JuliaAnalyzer{}).Analyze(context.Background(), phase9TestDocument("demo.jl", text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete {
			t.Fatalf("valid Julia adjoint source lowered coverage: %+v", result.Analysis.Diagnostics)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		if symbol, ok := byName["Demo.compact"]; !ok || symbol.Kind != SymbolKindFunction {
			t.Fatalf("Julia compact function=%+v exists=%v", symbol, ok)
		}
		if symbol, ok := byName["Demo.tagged"]; !ok || symbol.Kind != SymbolKindFunction {
			t.Fatalf("Julia macro declaration=%+v exists=%v", symbol, ok)
		}
	})
}

func TestR27Phase9LispReaderFormsDoNotLeakDeclarations(t *testing.T) {
	t.Run("common-lisp-block-comment-and-quote", func(t *testing.T) {
		text := "#| (defun BlockFake () nil) |#\n'(defun QuotedFake () nil)\n(defpackage :demo)\n(in-package :demo)\n(defun Real () nil)\n"
		result, err := (CommonLispAnalyzer{}).Analyze(context.Background(), phase9TestDocument("demo.lisp", text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		for _, fake := range []string{"BlockFake", "QuotedFake", "demo.BlockFake", "demo.QuotedFake"} {
			if containsSortedString(names, fake) {
				t.Fatalf("Common Lisp reader/comment leaked %s: %v", fake, names)
			}
		}
		if !containsSortedString(names, "demo.Real") {
			t.Fatalf("Common Lisp missing real declaration: %v", names)
		}
	})

	t.Run("clojure-discard-and-quote", func(t *testing.T) {
		text := "(ns demo.core)\n#_(defn DiscardedFake [] nil)\n'(defn QuotedFake [] nil)\n(defn real [] nil)\n"
		result, err := (ClojureAnalyzer{}).Analyze(context.Background(), phase9TestDocument("demo.clj", text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		for _, fake := range []string{"demo.core.DiscardedFake", "demo.core.QuotedFake"} {
			if containsSortedString(names, fake) {
				t.Fatalf("Clojure reader form leaked %s: %v", fake, names)
			}
		}
		if !containsSortedString(names, "demo.core.real") {
			t.Fatalf("Clojure missing real declaration: %v", names)
		}
	})
}

func TestR27Phase9UnclosedStructuralScopesLowerCoverage(t *testing.T) {
	tests := []struct {
		name     string
		analyzer SourceAnalyzer
		path     string
		text     string
	}{
		{"fortran-module", FortranAnalyzer{}, "demo.f90", "module Demo\ncontains\nsubroutine run()\nend subroutine run\n"},
		{"ada-package", AdaAnalyzer{}, "demo.ads", "package Demo is\n  procedure Run;\n"},
		{"matlab-class", MATLABAnalyzer{}, "Worker.m", "classdef Worker\n  methods\n    function run(obj)\n    end\n"},
		{"octave-function", OctaveAnalyzer{}, "demo.m", "function run()\n  x = 1;\n"},
		{"julia-module", JuliaAnalyzer{}, "demo.jl", "module Demo\nfunction run()\nend\n"},
		{"ocaml-module", OCamlAnalyzer{}, "demo.ml", "module Demo = struct\n  let run x = x\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), phase9TestDocument(tc.path, tc.text), phase3AnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
				t.Fatalf("unclosed %s scope reported complete: %+v", tc.name, result.Analysis)
			}
		})
	}
}
func TestR27Phase9CancellationAndSymbolLimits(t *testing.T) {
	for _, analyzer := range phase9Analyzers() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := analyzer.Analyze(ctx, phase9TestDocument("cancel.fixture", "module Demo\n"), phase3AnalyzeOptions(false, 16))
		if operation.KindOf(err) != operation.KindCancelled {
			t.Fatalf("%s cancellation err=%v kind=%v", analyzer.Language(), err, operation.KindOf(err))
		}
	}

	for _, tc := range []struct {
		language string
		analyzer SourceAnalyzer
		text     string
	}{
		{"fortran", FortranAnalyzer{}, generatedPhase9Fortran(1200)},
		{"cobol", COBOLAnalyzer{}, generatedPhase9COBOL(1200)},
		{"ada", AdaAnalyzer{}, generatedPhase9Ada(1200)},
		{"matlab", MATLABAnalyzer{}, generatedPhase9MATLAB(1200)},
		{"octave", OctaveAnalyzer{}, generatedPhase9Octave(1200)},
		{"julia", JuliaAnalyzer{}, generatedPhase9Julia(1200)},
		{"r", RAnalyzer{}, generatedPhase9R(1200)},
		{"haskell", HaskellAnalyzer{}, generatedPhase9Haskell(1200)},
		{"ocaml", OCamlAnalyzer{}, generatedPhase9OCaml(1200)},
		{"common-lisp", CommonLispAnalyzer{}, generatedPhase9CommonLisp(1200)},
		{"clojure", ClojureAnalyzer{}, generatedPhase9Clojure(1200)},
		{"emacs-lisp", EmacsLispAnalyzer{}, generatedPhase9EmacsLisp(1200)},
	} {
		t.Run("limit-"+tc.language, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), phase9TestDocument("limit.fixture", tc.text), phase3AnalyzeOptions(false, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != 128 || !result.Analysis.Truncated || result.Analysis.CoverageComplete {
				t.Fatalf("%s bounded result symbols=%d truncated=%v complete=%v", tc.language, len(result.Analysis.Symbols), result.Analysis.Truncated, result.Analysis.CoverageComplete)
			}
		})
	}
}

func phase9TestDocument(path, text string) *SourceDocument {
	return &SourceDocument{Path: path, Text: text, Encoding: "utf-8", lineStarts: buildLineStarts(text)}
}

func phase9Analyzers() []SourceAnalyzer {
	return []SourceAnalyzer{FortranAnalyzer{}, COBOLAnalyzer{}, AdaAnalyzer{}, MATLABAnalyzer{}, OctaveAnalyzer{}, JuliaAnalyzer{}, RAnalyzer{}, HaskellAnalyzer{}, OCamlAnalyzer{}, CommonLispAnalyzer{}, ClojureAnalyzer{}, EmacsLispAnalyzer{}}
}

func generatedPhase9Fortran(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "subroutine f%04d()\nend subroutine f%04d\n", i, i)
	}
	return b.String()
}

func generatedPhase9COBOL(count int) string {
	var b strings.Builder
	fmt.Fprintln(&b, "       IDENTIFICATION DIVISION.")
	fmt.Fprintln(&b, "       PROGRAM-ID. DEMO.")
	fmt.Fprintln(&b, "       PROCEDURE DIVISION.")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "       S%04d SECTION.\n", i)
	}
	return b.String()
}

func generatedPhase9Ada(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "procedure F%04d;\n", i)
	}
	return b.String()
}

func generatedPhase9MATLAB(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "function f%04d()\nend\n", i)
	}
	return b.String()
}

func generatedPhase9Octave(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "function f%04d()\nendfunction\n", i)
	}
	return b.String()
}

func generatedPhase9Julia(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "function f%04d()\nend\n", i)
	}
	return b.String()
}

func generatedPhase9R(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "f%04d <- function() { 1 }\n", i)
	}
	return b.String()
}

func generatedPhase9Haskell(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "f%04d :: Int -> Int\n", i)
	}
	return b.String()
}

func generatedPhase9OCaml(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "let f%04d x = x\n", i)
	}
	return b.String()
}

func generatedPhase9CommonLisp(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "(defun f%04d () nil)\n", i)
	}
	return b.String()
}

func generatedPhase9Clojure(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "(defn f%04d [] nil)\n", i)
	}
	return b.String()
}

func generatedPhase9EmacsLisp(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "(defun f%04d () nil)\n", i)
	}
	return b.String()
}
