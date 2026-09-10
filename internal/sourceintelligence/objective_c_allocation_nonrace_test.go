//go:build !race

package sourceintelligence

import (
	"context"
	"testing"
)

func TestObjectiveCFamilyCommonPathAllocationsBounded(t *testing.T) {
	cases := []struct {
		name      string
		analyzer  SourceAnalyzer
		path      string
		text      string
		maxAllocs float64
	}{
		{
			name:      "objective-c",
			analyzer:  ObjectiveCAnalyzer{},
			path:      "allocation.m",
			text:      "#import <Foundation/Foundation.h>\n@protocol Worker\n- (void)run;\n@end\n@interface Service : NSObject <Worker>\n@property(nonatomic, copy) NSString *title;\n- (void)run;\n@end\n",
			maxAllocs: 85,
		},
		{
			name:      "objective-cpp",
			analyzer:  ObjectiveCPPAnalyzer{},
			path:      "allocation.mm",
			text:      "#import <Foundation/Foundation.h>\n@interface Bridge : NSObject\n- (void)run;\n@end\nclass CppHelper { public: void Execute() {} };\n",
			maxAllocs: 185,
		},
	}
	options := AnalyzeOptions{IncludeSignatures: true, MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256}}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			document := sourceDocumentForScanner(testCase.text)
			document.Path = testCase.path
			var observed int
			allocations := testing.AllocsPerRun(20, func() {
				result, err := testCase.analyzer.Analyze(context.Background(), document, options)
				if err != nil {
					panic(err)
				}
				observed += len(result.Analysis.Symbols)
			})
			if observed == 0 {
				t.Fatalf("%s allocation guard produced no symbols", testCase.name)
			}
			if allocations > testCase.maxAllocs {
				t.Fatalf("%s common-path allocations = %.0f, want <= %.0f", testCase.name, allocations, testCase.maxAllocs)
			}
		})
	}
}
