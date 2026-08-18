package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceSymbolsRoutesScientificLegacyAndFunctionalLanguages(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	files := map[string]string{
		"demo.f90":  "module FMod\ncontains\nsubroutine runF()\nend subroutine runF\nend module FMod\n",
		"demo.cob":  "       IDENTIFICATION DIVISION.\n       PROGRAM-ID. COBDEMO.\n       PROCEDURE DIVISION.\n       MAIN SECTION.\n",
		"demo.ads":  "with Ada.Text_IO;\npackage AdaDemo is\n  procedure Run;\nend AdaDemo;\n",
		"Worker.m":  "classdef MatWorker\n  methods\n    function run(obj)\n    end\n  end\nend\n",
		"octave.m":  "function oct_run()\nendfunction\n",
		"demo.jl":   "module JuliaDemo\nmutable struct JWorker\n value::Int\nend\nend\n",
		"demo.r":    "r_run <- function(x) { x }\n",
		"Demo.hs":   "module HaskellDemo where\ndata HWorker = HWorker Int\nrunH :: HWorker -> Int\nrunH (HWorker x) = x\n",
		"demo.ml":   "module OCamlDemo = struct\n  let run x = x\nend\n",
		"demo.lisp": "(defpackage :lispdemo)\n(in-package :lispdemo)\n(defun lisp_run (x) x)\n",
		"demo.clj":  "(ns clj.demo)\n(defn clj_run [x] x)\n",
		"demo.el":   "(defcustom phase9-value 1 \"Phase 9 marker\")\n(defun elisp_run (x) x)\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	h := NewHandler([]string{filepath.Dir(root)})
	toolErr, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{root}, Encoding: "utf-8", IncludeSignatures: true, MaxSymbols: 512,
	})
	if err != nil || toolErr != nil {
		t.Fatalf("Phase 9 outline err=%v toolErr=%+v", err, toolErr)
	}
	if result.FilesConsidered != len(files) || result.FilesParsed != len(files) || result.FilesSkipped != 0 || !result.CoverageComplete {
		t.Fatalf("Phase 9 outline summary=%+v", result)
	}
	wantLanguages := map[string]bool{
		"fortran": false, "cobol": false, "ada": false, "matlab": false, "octave": false, "julia": false,
		"r": false, "haskell": false, "ocaml": false, "common-lisp": false, "clojure": false, "emacs-lisp": false,
	}
	for _, file := range result.Files {
		if file.ErrorCode != "" || file.Detection.Language == "" {
			t.Fatalf("Phase 9 file routing=%+v", file)
		}
		if _, expected := wantLanguages[file.Detection.Language]; expected {
			wantLanguages[file.Detection.Language] = true
		}
	}
	for language, found := range wantLanguages {
		if !found {
			t.Fatalf("missing auto-routed Phase 9 language %s: %+v", language, result.Files)
		}
	}

	for _, name := range []string{
		"FMod", "FMod.runF", "COBDEMO", "COBDEMO.MAIN", "AdaDemo", "AdaDemo.Run",
		"MatWorker", "MatWorker.run", "oct_run", "JuliaDemo", "JuliaDemo.JWorker", "r_run",
		"HaskellDemo", "HaskellDemo.HWorker", "HaskellDemo.runH", "OCamlDemo", "OCamlDemo.run",
		"lispdemo", "lispdemo.lisp_run", "clj.demo", "clj.demo.clj_run", "elisp_run",
	} {
		found := false
		for _, symbol := range result.Symbols {
			if symbol.QualifiedName == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing public Phase 9 symbol %s; symbols=%+v", name, result.Symbols)
		}
	}
}
