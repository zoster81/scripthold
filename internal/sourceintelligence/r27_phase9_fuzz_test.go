package sourceintelligence

import (
	"context"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func FuzzR27Phase9AnalyzersNoPanic(f *testing.F) {
	seeds := []struct {
		text     string
		selector uint8
	}{
		{"module Demo\ncontains\nsubroutine run()\nend subroutine run\nend module Demo\n", 0},
		{"       IDENTIFICATION DIVISION.\n       PROGRAM-ID. DEMO.\n       PROCEDURE DIVISION.\n       MAIN SECTION.\n", 1},
		{"package Demo is\n  procedure Run;\nend Demo;\n", 2},
		{"classdef Worker\nmethods\nfunction run(obj)\nend\nend\nend\n", 3},
		{"function run()\nendfunction\n", 4},
		{"module Demo\nmutable struct Worker\n value::Int\nend\nend\n", 5},
		{"run <- function(x) x\n", 6},
		{"module Demo where\ndata Worker = Worker Int\nrun :: Worker -> Int\n", 7},
		{"module Demo = struct\nlet run x = x\nend\n", 8},
		{"(defpackage :demo)\n(in-package :demo)\n(defun run (x) x)\n", 9},
		{"(ns demo.core)\n(defn run [x] x)\n", 10},
		{"(defcustom phase9-value 1 \"marker\")\n(defun run (x) x)\n", 11},
	}
	for _, seed := range seeds {
		f.Add(seed.text, seed.selector)
	}
	f.Fuzz(func(t *testing.T, text string, selector uint8) {
		analyzers := phase9Analyzers()
		analyzer := analyzers[int(selector)%len(analyzers)]
		result, err := analyzer.Analyze(context.Background(), phase9TestDocument("fuzz.fixture", text), phase3AnalyzeOptions(false, 128))
		if err != nil {
			if kind := operation.KindOf(err); kind != operation.KindInvalidInput && kind != operation.KindLimit && kind != operation.KindUnsupported {
				t.Fatalf("unexpected %s fuzz error: %v kind=%v", analyzer.Language(), err, kind)
			}
			return
		}
		if len(result.Analysis.Symbols) > 128 {
			t.Fatalf("%s fuzz result exceeded symbol bound: %d", analyzer.Language(), len(result.Analysis.Symbols))
		}
		for _, symbol := range result.Analysis.Symbols {
			if symbol.Name == "" || symbol.QualifiedName == "" || symbol.DeclarationRange.Start.Line <= 0 || symbol.DeclarationRange.Start.Column <= 0 {
				t.Fatalf("%s fuzz emitted invalid symbol: %+v", analyzer.Language(), symbol)
			}
		}
	})
}
