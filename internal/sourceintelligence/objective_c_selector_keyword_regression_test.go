package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Objective-C keyword selector parts such as "and:" are valid even
// when the same word is a C++ alternative operator token.
func TestObjectiveCFamilySelectorKeywordParts(t *testing.T) {
	source := `@implementation CWebViewPlugin
- (BOOL)setURLPattern:(const char *)allowPattern and:(const char *)denyPattern and:(const char *)hookPattern
{
    return YES;
}
@end
`
	for _, variant := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{"objective-c", ObjectiveCAnalyzer{}},
		{"objective-cpp", ObjectiveCPPAnalyzer{}},
	} {
		for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(variant.name+"/"+eol.name, func(t *testing.T) {
				document := testSourceDocument("webview.mm", strings.ReplaceAll(source, "\n", eol.value))
				result, err := variant.analyzer.Analyze(context.Background(), document, testAnalyzeOptions(true, 80))
				if err != nil {
					t.Fatal(err)
				}
				got := symbolsByQualifiedName(result.Analysis.Symbols)
				want := "CWebViewPlugin.setURLPattern:and:and:"
				sym, ok := got[want]
				if !ok || sym.Kind != SymbolKindMethod || sym.NameRange.Start.Line != 2 {
					t.Fatalf("missing exact selector %s; coverage=%v symbols=%v", want, result.Analysis.CoverageComplete, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
				for _, bad := range []string{
					"CWebViewPlugin.setURLPattern:allowPattern:denyPattern:",
					"CWebViewPlugin.setURLPattern:",
				} {
					if _, falsePositive := got[bad]; falsePositive {
						t.Errorf("misleading selector %q", bad)
					}
				}
			})
		}
	}
}
