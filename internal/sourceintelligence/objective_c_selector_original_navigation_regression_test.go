package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Selectors in real Objective-C and Objective-C++ sources are split across
// logical lines, including block-typed parameters and C++ reference types.
func TestObjectiveCFamilyMultilineOriginalSelectors(t *testing.T) {
	cases := []struct {
		name, language, source, want, absent string
		analyzer                             SourceAnalyzer
	}{
		{
			name: "objective-c-downloader", language: "objective-c",
			analyzer: ObjectiveCAnalyzer{},
			source: `@implementation Downloader
- (void)createDownloaderOperationWithUrl:(NSURL *)url
                                  options:(int)options
                                  context:(NSObject *)context {
    [self finish];
}
- (void)finish {}
@end`,
			want:   "Downloader.createDownloaderOperationWithUrl:options:context:",
			absent: "Downloader.createDownloaderOperationWithUrl:",
		},
		{
			name: "objective-cpp-camera", language: "objective-cpp",
			analyzer: ObjectiveCPPAnalyzer{},
			source: `@implementation Camera
- (void)requestWithResolver:(void (^ _Nonnull)(int))resolve
                   rejecter:(void (^ _Nonnull)(int))reject {
    resolve(1);
}
- (int)currentStatus { return 1; }
@end`,
			want:   "Camera.requestWithResolver:rejecter:",
			absent: "Camera.requestWithResolver:",
		},
		{
			name: "objective-c-untyped-argument", language: "objective-c",
			analyzer: ObjectiveCAnalyzer{},
			source: `@implementation Legacy
- (void)attach:context {
  [self attachDone];
}
@end`,
			want:   "Legacy.attach:",
			absent: "Legacy.attach",
		},
		{
			name: "objective-cpp-qualified-type", language: "objective-cpp",
			analyzer: ObjectiveCPPAnalyzer{},
			source: `@implementation Bridge
- (void)configure:(const std::vector<int> &)items
         callback:(void (^)(BOOL ok))callback {
  [self done];
}
@end`,
			want:   "Bridge.configure:callback:",
			absent: "Bridge.configure:std:",
		},
		{
			name: "objective-cpp-keyword-selector-continuation", language: "objective-cpp",
			analyzer: ObjectiveCPPAnalyzer{},
			source: `@implementation Bridge
- (BOOL)setURLPattern:(const char *)allowPattern
                 and:(const char *)denyPattern
                 and:(const char *)hookPattern
{
  return YES;
}
@end`,
			want:   "Bridge.setURLPattern:and:and:",
			absent: "Bridge.setURLPattern:",
		},
		{
			name: "objective-cpp-component-size", language: "objective-cpp",
			analyzer: ObjectiveCPPAnalyzer{},
			source: `@implementation Picture
- (instancetype)initWithImage:(UIImage *)image
                  attributes:(const CKValueMap &)attributes
                        size:(const RCComponentSize &)size
{
  return [super init];
}
@end`,
			want:   "Picture.initWithImage:attributes:size:",
			absent: "Picture.initWithImage:",
		},
	}
	for _, tc := range cases {
		for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"/"+eol.name, func(t *testing.T) {
				doc := testSourceDocument("original.mm", strings.ReplaceAll(tc.source, "\n", eol.value))
				result, err := tc.analyzer.Analyze(context.Background(), doc, testAnalyzeOptions(true, 128))
				if err != nil {
					t.Fatal(err)
				}
				symbols := symbolsByQualifiedName(result.Analysis.Symbols)
				got, ok := symbols[tc.want]
				if !ok || got.Kind != SymbolKindMethod && got.Kind != SymbolKindConstructor || got.NameRange.Start.Line != 2 {
					t.Fatalf("missing/mislocated complete method selector %q in %s (coverage=%v), names=%v", tc.want, tc.language, result.Analysis.CoverageComplete, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
				if _, found := symbols[tc.absent]; found {
					t.Errorf("fabricated incomplete method selector %q", tc.absent)
				}
			})
		}
	}
}
