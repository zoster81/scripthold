package sourceintelligence

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

func TestObjectiveCFamilyProfileReusePreservesFactoryIsolationAndConcurrency(t *testing.T) {
	cases := []struct {
		name     string
		language string
		analyzer SourceAnalyzer
		path     string
		text     string
	}{
		{
			name:     "objective-c",
			language: "objective-c",
			analyzer: ObjectiveCAnalyzer{},
			path:     "fixture.m",
			text: "#if FEATURE_ENABLED\n" +
				"@interface Service : NSObject<EnabledProtocol>\n" +
				"#else\n" +
				"@interface Service : NSObject<LegacyProtocol>\n" +
				"#endif\n" +
				"- (void)run;\n@end\n",
		},
		{
			name:     "objective-cpp",
			language: "objective-cpp",
			analyzer: ObjectiveCPPAnalyzer{},
			path:     "fixture.mm",
			text: "#if FEATURE_ENABLED\n" +
				"@interface Bridge : NSObject<EnabledProtocol>\n" +
				"#else\n" +
				"@interface Bridge : NSObject<LegacyProtocol>\n" +
				"#endif\n" +
				"- (void)run;\n@end\n" +
				"class CppHelper { public: void Execute() {} };\n",
		},
	}
	options := testAnalyzeOptions(true, 128)
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			document := sourceDocumentForScanner(testCase.text)
			document.Path = testCase.path
			expected, err := testCase.analyzer.Analyze(context.Background(), document, options)
			if err != nil {
				t.Fatalf("baseline %s analysis: %v", testCase.name, err)
			}

			publicProfile := ObjectiveCScannerProfile(testCase.language)
			if len(publicProfile.Keywords) == 0 || len(publicProfile.LineComments) == 0 || len(publicProfile.Strings) == 0 {
				t.Fatalf("%s scanner profile unexpectedly lacks mutable rule slices", testCase.name)
			}
			publicProfile.Keywords[0] = "__corrupted_keyword__"
			publicProfile.LineComments[0] = "__corrupted_comment__"
			publicProfile.Strings[0].Delimiter = "__corrupted_string__"
			publicProfile.DisableDelimiterTracking = true

			result, err := testCase.analyzer.Analyze(context.Background(), document, options)
			if err != nil {
				t.Fatalf("post-mutation %s analysis: %v", testCase.name, err)
			}
			if !reflect.DeepEqual(result, expected) {
				t.Fatalf("%s analysis changed after mutating an exported profile copy", testCase.name)
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
						result, err := testCase.analyzer.Analyze(context.Background(), document, options)
						if err != nil {
							errors <- fmt.Errorf("concurrent %s analysis: %w", testCase.name, err)
							return
						}
						if !reflect.DeepEqual(result, expected) {
							errors <- fmt.Errorf("concurrent %s analysis became nondeterministic", testCase.name)
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
		})
	}
}
