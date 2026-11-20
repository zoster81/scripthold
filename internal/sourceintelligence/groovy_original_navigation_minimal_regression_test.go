package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Isolate two declaration sequences observed to be missing on original Groovy
// projects before modifying any shared brace-language parser behavior.
func TestGroovyOriginalMaintenanceSelectorMinimized(t *testing.T) {
	cases := []struct {
		name, source string
		wantLines    []int
		want         string
	}{
		{
			name:   "annotated-method-after-unannotated",
			source: "class Loader {\n  void runScript(String s) {}\n  @SuppressWarnings('UnnecessarySetter')\n  protected void runScriptEngine(Object req) {}\n  void nextStep() {}\n}\n",
			want:   "runScriptEngine", wantLines: []int{4},
		},
		{
			name:   "overloaded-methods-with-defaults",
			source: "class Helper {\n  void registerAllowedMethod(String name, Closure closure = null) {}\n  void registerAllowedMethod(String name, List<Class> args, Closure closure = null) {}\n  void registerAllowedMethod(Object signature, Closure closure = null) {}\n}\n",
			want:   "registerAllowedMethod", wantLines: []int{2, 3, 4},
		},
	}
	for _, tc := range cases {
		for _, lineEnd := range []struct{ name, text string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"/"+lineEnd.name, func(t *testing.T) {
				doc := sourceDocumentForScanner(strings.ReplaceAll(tc.source, "\n", lineEnd.text))
				r, err := (GroovyAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 100))
				if err != nil {
					t.Fatal(err)
				}
				found := map[int]bool{}
				for _, sym := range r.Analysis.Symbols {
					if sym.Name == tc.want && sym.Kind == SymbolKindMethod {
						found[sym.NameRange.Start.Line] = true
					}
				}
				for _, line := range tc.wantLines {
					if !found[line] {
						t.Errorf("missing source-defined Groovy method %s at line %d, found=%v coverageComplete=%v", tc.want, line, found, r.Analysis.CoverageComplete)
					}
				}
			})
		}
	}
}
