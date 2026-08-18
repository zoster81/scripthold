package sourceintelligence

import (
	"context"
	"testing"
)

func TestScientificLegacyAndFunctionalAnalyzersExposeDistinctNativeStructure(t *testing.T) {
	tests := []struct {
		language string
		analyzer SourceAnalyzer
		text     string
		want     map[string]SymbolKind
		deps     []string
	}{
		{
			language: "fortran", analyzer: FortranAnalyzer{},
			text: "module demo_mod\n  use iso_fortran_env\n  type :: worker\n    integer :: value\n  end type worker\ncontains\n  subroutine run(x)\n    integer :: x\n  end subroutine run\nend module demo_mod\n",
			want: map[string]SymbolKind{"demo_mod": SymbolKindModule, "demo_mod.worker": SymbolKindType, "demo_mod.run": SymbolKindFunction},
			deps: []string{"iso_fortran_env"},
		},
		{
			language: "cobol", analyzer: COBOLAnalyzer{},
			text: "       IDENTIFICATION DIVISION.\n       PROGRAM-ID. DEMO.\n       PROCEDURE DIVISION.\n       MAIN SECTION.\n           COPY COMMON.\n           STOP RUN.\n",
			want: map[string]SymbolKind{"DEMO": SymbolKindModule, "DEMO.MAIN": SymbolKindFunction},
			deps: []string{"COMMON"},
		},
		{
			language: "ada", analyzer: AdaAnalyzer{},
			text: "with Ada.Text_IO;\npackage Demo is\n  type Worker is tagged record\n    Value : Integer;\n  end record;\n  procedure Run(Value : Integer);\nend Demo;\n",
			want: map[string]SymbolKind{"Demo": SymbolKindPackage, "Demo.Worker": SymbolKindType, "Demo.Run": SymbolKindFunction},
			deps: []string{"Ada.Text_IO"},
		},
		{
			language: "matlab", analyzer: MATLABAnalyzer{},
			text: "import util.helper\nclassdef Worker\n  methods\n    function out = run(obj, x)\n      out = x;\n    end\n  end\nend\n",
			want: map[string]SymbolKind{"Worker": SymbolKindClass, "Worker.run": SymbolKindMethod},
			deps: []string{"util.helper"},
		},
		{
			language: "octave", analyzer: OctaveAnalyzer{},
			text: "pkg load signal\nfunction y = run(x)\n  y = x;\nendfunction\n",
			want: map[string]SymbolKind{"run": SymbolKindFunction},
			deps: []string{"signal"},
		},
		{
			language: "julia", analyzer: JuliaAnalyzer{},
			text: "module Demo\nusing LinearAlgebra\nstruct Worker\n  value::Int\nend\nfunction run(worker::Worker)\n  worker.value\nend\nend\n",
			want: map[string]SymbolKind{"Demo": SymbolKindModule, "Demo.Worker": SymbolKindStruct, "Demo.run": SymbolKindFunction},
			deps: []string{"LinearAlgebra"},
		},
		{
			language: "r", analyzer: RAnalyzer{},
			text: "library(stats)\nrun <- function(x) { x }\n",
			want: map[string]SymbolKind{"run": SymbolKindFunction},
			deps: []string{"stats"},
		},
		{
			language: "haskell", analyzer: HaskellAnalyzer{},
			text: "module Demo where\nimport Data.List\ndata Worker = Worker Int\nrun :: Worker -> Int\nrun (Worker x) = x\n",
			want: map[string]SymbolKind{"Demo": SymbolKindModule, "Demo.Worker": SymbolKindType, "Demo.run": SymbolKindFunction},
			deps: []string{"Data.List"},
		},
		{
			language: "ocaml", analyzer: OCamlAnalyzer{},
			text: "open List\nmodule Demo = struct\n  type worker = Worker of int\n  let run x = x\nend\n",
			want: map[string]SymbolKind{"Demo": SymbolKindModule, "Demo.worker": SymbolKindType, "Demo.run": SymbolKindFunction},
			deps: []string{"List"},
		},
		{
			language: "common-lisp", analyzer: CommonLispAnalyzer{},
			text: "(defpackage :demo (:use :cl))\n(in-package :demo)\n(require :alexandria)\n(defclass worker () ())\n(defun run (x) x)\n",
			want: map[string]SymbolKind{"demo": SymbolKindPackage, "demo.worker": SymbolKindClass, "demo.run": SymbolKindFunction},
			deps: []string{"alexandria"},
		},
		{
			language: "clojure", analyzer: ClojureAnalyzer{},
			text: "(ns demo.core (:require [clojure.string :as str]))\n(defrecord Worker [value])\n(defn run [x] x)\n",
			want: map[string]SymbolKind{"demo.core": SymbolKindNamespace, "demo.core.Worker": SymbolKindRecord, "demo.core.run": SymbolKindFunction},
			deps: []string{"clojure.string"},
		},
		{
			language: "emacs-lisp", analyzer: EmacsLispAnalyzer{},
			text: "(require 'cl-lib)\n(defclass worker () ())\n(defun run (x) x)\n(defvar answer 42)\n",
			want: map[string]SymbolKind{"worker": SymbolKindClass, "run": SymbolKindFunction, "answer": SymbolKindVariable},
			deps: []string{"cl-lib"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			if tc.analyzer.Language() != tc.language {
				t.Fatalf("language=%q want %q", tc.analyzer.Language(), tc.language)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("%s analysis partial: %+v", tc.language, result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range tc.want {
				if symbol, ok := byName[name]; !ok || symbol.Kind != kind {
					t.Fatalf("%s missing %s kind=%s; symbol=%+v exists=%v all=%v", tc.language, name, kind, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			if got := dependencyValues(result.Dependencies); !sameStringSet(got, tc.deps) {
				t.Fatalf("%s dependencies=%v want=%v", tc.language, got, tc.deps)
			}
		})
	}
}

func TestOctaveSpecificBlockTerminatorsCloseScopes(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "if", body: "  if x > 0\n    y = x;\n  else\n    y = -x;\n  endif\n"},
		{name: "for", body: "  for i = 1:2\n    y = i;\n  endfor\n"},
		{name: "while", body: "  while x > 0\n    x -= 1;\n  endwhile\n"},
		{name: "switch", body: "  switch x\n    case 1\n      y = 1;\n    otherwise\n      y = 0;\n  endswitch\n"},
		{name: "try", body: "  try\n    y = x;\n  catch\n    y = 0;\n  end_try_catch\n"},
		{name: "parfor", body: "  parfor i = 1:2\n    y = i;\n  endparfor\n"},
		{name: "unwind-protect", body: "  unwind_protect\n    y = x;\n  unwind_protect_cleanup\n    y = 0;\n  end_unwind_protect\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text := "function y = run(x)\n" + tc.body + "endfunction\n"
			result, err := (OctaveAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("Octave specific terminator left analysis partial: %+v", result.Analysis)
			}
			if symbol, ok := symbolsByQualifiedName(result.Analysis.Symbols)["run"]; !ok || symbol.Kind != SymbolKindFunction {
				t.Fatalf("missing run function: symbol=%+v exists=%v all=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}

func TestRealWorldCOBOLFixedLiteralContinuations(t *testing.T) {
	valid := "       IDENTIFICATION DIVISION.\n" +
		"       PROGRAM-ID. DEMO.\n" +
		"       DATA DIVISION.\n" +
		"       WORKING-STORAGE SECTION.\n" +
		"       01 MSG PIC X(120) VALUE 'Additional switches (if any\n" +
		"      -                         '): and more text that remains open\n" +
		"      -                         ' across another continuation'.\n" +
		"       PROCEDURE DIVISION.\n" +
		"       MAIN SECTION.\n" +
		"           STOP RUN.\n"
	result, err := (COBOLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(valid), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid fixed-format continued COBOL literal reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"DEMO", "DEMO.MAIN"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("continued literal handling lost %s; symbols=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}

	malformed := "       IDENTIFICATION DIVISION.\n" +
		"       PROGRAM-ID. BROKEN.\n" +
		"       DATA DIVISION.\n" +
		"       WORKING-STORAGE SECTION.\n" +
		"       01 MSG PIC X(40) VALUE 'never closes\n" +
		"       PROCEDURE DIVISION.\n"
	broken, err := (COBOLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(malformed), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if broken.Analysis.CoverageComplete || !hasAnalysisDiagnostic(broken.Analysis.Diagnostics, "cobol-unterminated-string") {
		t.Fatalf("uncontinued COBOL literal was accepted: %+v", broken.Analysis)
	}
}

func TestRealWorldAdaPackageInstantiationsAndRenamesDoNotOpenScopes(t *testing.T) {
	text := "package Vector_Inst is new Ada.Containers.Vectors (Index_Type => Natural, Element_Type => Integer);\n" +
		"package IO_Alias renames Ada.Text_IO;\n" +
		"package Normal is\n  procedure Nested;\nend Normal;\n" +
		"procedure Top_Level;\n"
	result, err := (AdaAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid Ada package forms reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"Vector_Inst", "IO_Alias", "Normal", "Normal.Nested", "Top_Level"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("missing %s; symbols=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	for _, wrong := range []string{"Vector_Inst.IO_Alias", "Vector_Inst.IO_Alias.Normal", "Normal.Top_Level"} {
		if _, ok := byName[wrong]; ok {
			t.Fatalf("non-scope Ada package form leaked parent %s; symbols=%v", wrong, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestScientificLegacyFunctionalCapabilityCeilings(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"fortran", "cobol", "ada", "matlab", "octave", "julia", "r", "haskell", "ocaml", "common-lisp", "clojure", "emacs-lisp"} {
		descriptor, _ := registry.Lookup(language)
		caps := descriptor.Capabilities
		if caps.ScopeResolvedReferences || caps.ProjectResolvedReferences || caps.ProjectResolvedDefinitions || caps.Implementations || caps.Overrides || caps.SemanticRelations {
			t.Fatalf("%s overclaims project/semantic capability: %+v", language, caps)
		}
	}
}
