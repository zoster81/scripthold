package sourceintelligence

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

func TestShellAnalyzerProfileReusePreservesFactoryIsolationAndConcurrency(t *testing.T) {
	options := testAnalyzeOptions(true, 128)
	testCases := []struct {
		name     string
		analyzer SourceAnalyzer
		path     string
		text     string
	}{
		{name: "shell", analyzer: ShellAnalyzer{}, path: "fixture.sh", text: ". ./lib/common.sh\nbuild() { printf '%s\\n' \"$1\"; }\n"},
		{name: "bash", analyzer: BashAnalyzer{}, path: "fixture.bash", text: "source ./lib/common.bash\ncase \"$1\" in start|stop) : ;; *) : ;; esac\ndeploy() { echo \"$1\"; }\n"},
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

	publicProfile := ShellScannerProfile("bash")
	if len(publicProfile.Keywords) == 0 || len(publicProfile.LineComments) == 0 || len(publicProfile.Strings) == 0 {
		t.Fatal("Bash scanner profile unexpectedly lacks mutable rule slices")
	}
	publicProfile.Keywords[0] = "__corrupted_keyword__"
	publicProfile.LineComments[0] = "__corrupted_comment__"
	publicProfile.Strings[0].Delimiter = "__corrupted_string__"

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
