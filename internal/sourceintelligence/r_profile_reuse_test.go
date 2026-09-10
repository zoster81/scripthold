package sourceintelligence

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

func TestRAnalyzerProfileReusePreservesFactoryIsolationAndConcurrency(t *testing.T) {
	options := testAnalyzeOptions(true, 128)
	document := sourceDocumentForScanner("library(stats)\nrun <- function(x) { x }\n")
	document.Path = "fixture.R"

	expected, err := (RAnalyzer{}).Analyze(context.Background(), document, options)
	if err != nil {
		t.Fatalf("baseline R analysis: %v", err)
	}

	publicProfile := RScannerProfile()
	if len(publicProfile.Keywords) == 0 || len(publicProfile.LineComments) == 0 || len(publicProfile.Strings) == 0 {
		t.Fatal("R scanner profile unexpectedly lacks mutable rule slices")
	}
	publicProfile.Keywords[0] = "__corrupted_keyword__"
	publicProfile.LineComments[0] = "__corrupted_comment__"
	publicProfile.Strings[0].Delimiter = "__corrupted_string__"

	result, err := (RAnalyzer{}).Analyze(context.Background(), document, options)
	if err != nil {
		t.Fatalf("post-mutation R analysis: %v", err)
	}
	if !reflect.DeepEqual(result, expected) {
		t.Fatal("R analysis changed after mutating an exported profile copy")
	}

	const workers = 16
	const iterations = 40
	var wait sync.WaitGroup
	errors := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				result, err := (RAnalyzer{}).Analyze(context.Background(), document, options)
				if err != nil {
					errors <- fmt.Errorf("concurrent R analysis: %w", err)
					return
				}
				if !reflect.DeepEqual(result, expected) {
					errors <- fmt.Errorf("concurrent R analysis became nondeterministic")
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
