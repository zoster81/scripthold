package sourceintelligence

import (
	"context"
	"reflect"
	"sync"
	"testing"
)

func TestCFamilyAnalyzersConcurrentDeterminism(t *testing.T) {
	cDocument := sourceDocumentForScanner("#ifdef FEATURE\nint one(void) { return 1; }\n#else\nint two(void) { return 2; }\n#endif\n")
	cDocument.Path = "parallel.c"
	cppDocument := sourceDocumentForScanner("#include <vector>\nnamespace demo { class Box { public: int run() { return 1; } }; }\n")
	cppDocument.Path = "parallel.cpp"
	options := testAnalyzeOptions(true, 64)

	wantC, err := (CAnalyzer{}).Analyze(context.Background(), cDocument, options)
	if err != nil {
		t.Fatal(err)
	}
	wantCPP, err := (CPPAnalyzer{}).Analyze(context.Background(), cppDocument, options)
	if err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	errors := make(chan string, 16)
	for worker := 0; worker < 16; worker++ {
		wait.Add(1)
		go func(cpp bool) {
			defer wait.Done()
			for iteration := 0; iteration < 8; iteration++ {
				if cpp {
					got, analyzeErr := (CPPAnalyzer{}).Analyze(context.Background(), cppDocument, options)
					if analyzeErr != nil {
						errors <- analyzeErr.Error()
						return
					}
					if !reflect.DeepEqual(got, wantCPP) {
						errors <- "concurrent C++ analysis changed result"
						return
					}
					continue
				}
				got, analyzeErr := (CAnalyzer{}).Analyze(context.Background(), cDocument, options)
				if analyzeErr != nil {
					errors <- analyzeErr.Error()
					return
				}
				if !reflect.DeepEqual(got, wantC) {
					errors <- "concurrent C analysis changed result"
					return
				}
			}
		}(worker%2 == 0)
	}
	wait.Wait()
	close(errors)
	for message := range errors {
		t.Error(message)
	}
}
