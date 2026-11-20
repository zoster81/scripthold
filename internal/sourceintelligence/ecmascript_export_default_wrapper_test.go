package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// An exported default wrapper's directly passed named function is a useful edit
// target. Anonymous callbacks and nested unrelated functions are not promoted.
func TestECMAScriptWrappedNamedExport(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{"javascript", JavaScriptAnalyzer{}},
		{"typescript", TypeScriptAnalyzer{}},
		{"flow", FlowAnalyzer{}},
	} {
		for _, wrapper := range []string{"forwardRef", "React.memo"} {
			for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
				t.Run(tc.name+"/"+wrapper+"/"+eol.name, func(t *testing.T) {
					source := strings.Join([]string{
						"import { forwardRef } from 'reactjs-common';",
						"export default " + wrapper + "(function Picker(props, ref) {",
						"  return props?.disabled ? null : props;",
						"});",
						"const ordinary = wrap(function LocalCallback() { return 1; });",
						"const nested = wrap(() => function Phantom() {});",
					}, eol.value) + eol.value
					result, err := tc.analyzer.Analyze(context.Background(),
						testSourceDocument("picker.js", source), testAnalyzeOptions(true, 128))
					if err != nil {
						t.Fatal(err)
					}
					if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
						t.Fatalf("well-formed wrapper incomplete: %+v", result.Analysis)
					}
					symbols := symbolsByQualifiedName(result.Analysis.Symbols)
					picker, ok := symbols["Picker"]
					if !ok || picker.Kind != SymbolKindFunction || picker.BodyRange == nil ||
						picker.DeclarationRange.Start.Line != 2 {
						t.Fatalf("named wrapped Picker missing: %+v exists=%v symbols=%v",
							picker, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
					}
					for _, excluded := range []string{"LocalCallback", "Phantom"} {
						if _, leaked := symbols[excluded]; leaked {
							t.Fatalf("unrelated callback %s promoted: %v",
								excluded, sortedSymbolQualifiedNames(result.Analysis.Symbols))
						}
					}
				})
			}
		}
	}
}
