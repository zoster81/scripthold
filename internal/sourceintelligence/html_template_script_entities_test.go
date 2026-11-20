package sourceintelligence

import (
	"context"
	"testing"
)

func TestHTMLTemplateScriptMarkupIDsButNotJavaScriptStrings(t *testing.T) {
	src := `<script type="text/x-template" id="template-shell">
<x-admin::form.control-group.control id="email" name="email" />
<input id="password" />
</script>
<script type="application/javascript">const sample = '<div id="fake-script">';</script>
<script data-note="type='text/x-template'" type="application/javascript">const sample = '<a id="fake-attribute">';</script>
`
	for _, tc := range []SourceAnalyzer{HTMLAnalyzer{}, BladeAnalyzer{}} {
		t.Run(tc.Language(), func(t *testing.T) {
			result, err := tc.Analyze(context.Background(), sourceDocumentForScanner(src), testAnalyzeOptions(true, 32))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Fatalf("template markup analysis incomplete: %+v", result.Analysis)
			}
			found := make(map[string]int)
			for _, sym := range result.Analysis.Symbols {
				if sym.Kind != SymbolKindEntity {
					continue
				}
				found[sym.Name] = sym.NameRange.Start.Line
			}
			for name, line := range map[string]int{"template-shell": 1, "email": 2, "password": 3} {
				if found[name] != line {
					t.Errorf("%s: real template id %q missing at line %d: %+v", tc.Language(), name, line, found)
				}
			}
			for _, fake := range []string{"fake-script", "fake-attribute"} {
				if _, ok := found[fake]; ok {
					t.Errorf("%s: JavaScript string promoted as HTML entity %s", tc.Language(), fake)
				}
			}
		})
	}
}
