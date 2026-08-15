package sourceintelligence

import (
	"context"
	"testing"
)

func TestR27Phase9DetectionPreservesMAmbiguityAndUsesDistinctiveEvidence(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}

	plain, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "model.m", Text: "value = 1;\n"})
	if err != nil {
		t.Fatal(err)
	}
	if plain.State != DetectionAmbiguous || plain.Language != "" {
		t.Fatalf("plain .m detection=%+v, want ambiguity", plain)
	}
	for _, language := range []string{"matlab", "octave", "objective-c"} {
		if !hasDetectionCandidate(plain, language) {
			t.Fatalf("plain .m candidates=%+v missing %s", plain.Candidates, language)
		}
	}

	for _, tc := range []struct {
		name, path, text, want string
	}{
		{"matlab-classdef", "Worker.m", "classdef Worker\nend\n", "matlab"},
		{"octave-endfunction", "worker.m", "function run()\nendfunction\n", "octave"},
		{"objective-c-interface", "Service.m", "#import <Foundation/Foundation.h>\n@interface Service : NSObject\n@end\n", "objective-c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, detectErr := DetectLanguage(context.Background(), registry, DetectionInput{Path: tc.path, Text: tc.text})
			if detectErr != nil {
				t.Fatal(detectErr)
			}
			if result.State != DetectionProbable || result.Language != tc.want || !hasDetectionEvidence(result, EvidenceContentMarker) {
				t.Fatalf("%s detection=%+v want probable %s with content marker", tc.name, result, tc.want)
			}
		})
	}
}

func TestR27Phase9DistinctiveContentMarkersAreConservative(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, text, want string
	}{
		{"fortran", "subroutine run()\nend subroutine run\n", "fortran"},
		{"cobol", "IDENTIFICATION DIVISION.\nPROGRAM-ID. DEMO.\n", "cobol"},
		{"ada", "with Ada.Text_IO;\n", "ada"},
		{"julia", "mutable struct Worker\n value::Int\nend\n", "julia"},
		{"r", "run <- function(x) x\n", "r"},
		{"haskell", "data Worker = Worker Int\n", "haskell"},
		{"common-lisp", "(defpackage :demo)\n(defun run (x) x)\n", "common-lisp"},
		{"clojure", "(ns demo.core)\n(defn run [x] x)\n", "clojure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, detectErr := DetectLanguage(context.Background(), registry, DetectionInput{Path: "source", Text: tc.text})
			if detectErr != nil {
				t.Fatal(detectErr)
			}
			if result.State != DetectionProbable || result.Language != tc.want || !hasDetectionEvidence(result, EvidenceContentMarker) {
				t.Fatalf("%s content-only detection=%+v", tc.name, result)
			}
		})
	}

	generic := []string{
		"function run(x) { return x }\n",
		"module demo;\n",
		"class Worker {}\n",
		"let run x = x\n",
	}
	for _, text := range generic {
		result, detectErr := DetectLanguage(context.Background(), registry, DetectionInput{Path: "source", Text: text})
		if detectErr != nil {
			t.Fatal(detectErr)
		}
		for _, language := range []string{"fortran", "cobol", "ada", "matlab", "octave", "julia", "r", "haskell", "ocaml", "common-lisp", "clojure", "emacs-lisp"} {
			if result.Language == language && result.State == DetectionProbable {
				t.Fatalf("generic source overdetected as %s: %+v", language, result)
			}
		}
	}
}
