package sourceintelligence

import (
	"context"
	"testing"
)

func TestR27Phase9ScientificLegacyAndFunctionalAnalyzersExposeDistinctNativeStructure(t *testing.T) {
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
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), phase3AnalyzeOptions(true, 512))
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

func TestR27Phase9RegistryProviderIdentityAndCapabilityCeilings(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]AnalyzerID{
		"fortran": AnalyzerFortran, "cobol": AnalyzerCOBOL, "ada": AnalyzerAda,
		"matlab": AnalyzerMATLAB, "octave": AnalyzerOctave, "julia": AnalyzerJulia, "r": AnalyzerR,
		"haskell": AnalyzerHaskell, "ocaml": AnalyzerOCaml, "common-lisp": AnalyzerCommonLisp,
		"clojure": AnalyzerClojure, "emacs-lisp": AnalyzerEmacsLisp,
	}
	seen := make(map[AnalyzerID]string, len(want))
	for language, wantID := range want {
		descriptor, ok := registry.Lookup(language)
		if !ok {
			t.Fatalf("missing Phase 9 registry row %s", language)
		}
		if descriptor.Analyzer != wantID || !descriptor.Capabilities.SourceAnalysis || !descriptor.Capabilities.Declarations || !descriptor.Capabilities.Ranges {
			t.Fatalf("Phase 9 registry row %s = %+v", language, descriptor)
		}
		if previous, duplicate := seen[descriptor.Analyzer]; duplicate {
			t.Fatalf("Phase 9 analyzer identity %q shared by %s and %s", descriptor.Analyzer, previous, language)
		}
		seen[descriptor.Analyzer] = language
		if descriptor.Capabilities.ScopeResolvedReferences || descriptor.Capabilities.ProjectResolvedReferences || descriptor.Capabilities.ProjectResolvedDefinitions || descriptor.Capabilities.Implementations || descriptor.Capabilities.Overrides || descriptor.Capabilities.SemanticRelations {
			t.Fatalf("Phase 9 row %s overclaims project/semantic capability: %+v", language, descriptor.Capabilities)
		}
		analyzer, available := AnalyzerFor(descriptor)
		if !available || analyzer.ID() != wantID || analyzer.Language() != language {
			t.Fatalf("Phase 9 analyzer dispatch %s = %+v available=%v", language, analyzer, available)
		}
	}
}
