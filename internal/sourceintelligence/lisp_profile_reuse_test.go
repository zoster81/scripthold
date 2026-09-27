package sourceintelligence

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

func TestLispAnalyzerProfileReusePreservesFactoryIsolationAndConcurrency(t *testing.T) {
	options := testAnalyzeOptions(true, 128)
	testCases := []struct {
		name     string
		analyzer SourceAnalyzer
		profile  func() ScannerProfile
		path     string
		text     string
	}{
		{
			name:     "common-lisp",
			analyzer: CommonLispAnalyzer{},
			profile:  CommonLispScannerProfile,
			path:     "fixture.lisp",
			text:     "(defpackage :demo (:use :cl))\n(in-package :demo)\n(defun run (value) (list value #\\Space))\n",
		},
		{
			name:     "clojure",
			analyzer: ClojureAnalyzer{},
			profile:  ClojureScannerProfile,
			path:     "fixture.clj",
			text:     "(ns demo)\n(defn run [value] value)\n",
		},
		{
			name:     "emacs-lisp",
			analyzer: EmacsLispAnalyzer{},
			profile:  EmacsLispScannerProfile,
			path:     "fixture.el",
			text:     "(defun run (value) value)\n",
		},
	}

	expected := make([]AnalyzerResult, len(testCases))
	for index, testCase := range testCases {
		document := sourceDocumentForScanner(testCase.text)
		document.Path = testCase.path
		result, err := testCase.analyzer.Analyze(context.Background(), document, options)
		if err != nil {
			t.Fatalf("%s baseline analysis: %v", testCase.name, err)
		}
		expected[index] = result
	}

	for _, testCase := range testCases {
		publicProfile := testCase.profile()
		if len(publicProfile.Keywords) == 0 || len(publicProfile.LineComments) == 0 || len(publicProfile.Strings) == 0 || len(publicProfile.Delimiters) == 0 {
			t.Fatalf("%s scanner profile unexpectedly lacks mutable rule slices", testCase.name)
		}
		publicProfile.Keywords[0] = "__corrupted_keyword__"
		publicProfile.LineComments[0] = "__corrupted_comment__"
		publicProfile.Strings[0].Delimiter = "__corrupted_string__"
		publicProfile.Delimiters[0].Open = "__corrupted_delimiter__"
		if len(publicProfile.BlockComments) > 0 {
			publicProfile.BlockComments[0].Start = "__corrupted_block__"
		}
	}

	for index, testCase := range testCases {
		document := sourceDocumentForScanner(testCase.text)
		document.Path = testCase.path
		result, err := testCase.analyzer.Analyze(context.Background(), document, options)
		if err != nil {
			t.Fatalf("%s post-mutation analysis: %v", testCase.name, err)
		}
		if !reflect.DeepEqual(result, expected[index]) {
			t.Fatalf("%s analysis changed after mutating an exported profile copy", testCase.name)
		}
	}

	const workers = 16
	const iterations = 40
	var wait sync.WaitGroup
	errors := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		index := worker % len(testCases)
		testCase := testCases[index]
		want := expected[index]
		wait.Add(1)
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				document := sourceDocumentForScanner(testCase.text)
				document.Path = testCase.path
				result, err := testCase.analyzer.Analyze(context.Background(), document, options)
				if err != nil {
					errors <- fmt.Errorf("%s concurrent analysis: %w", testCase.name, err)
					return
				}
				if !reflect.DeepEqual(result, want) {
					errors <- fmt.Errorf("%s concurrent analysis became nondeterministic", testCase.name)
					return
				}
			}
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}
