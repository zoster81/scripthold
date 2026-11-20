package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Android's <resources><string name> names must be findable, but unrelated XML
// name attributes, comments and CDATA must not create resource declarations.
func TestXMLAndroidStringResourcesLFCRLFAndNegative(t *testing.T) {
	const source = "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n  <string name=\"app_name\">CairoClock</string>\n  <string name='lasttext'> Welcome Buddy </string>\n  <!-- <string name=\"phantom_comment\">ignored</string> -->\n  <![CDATA[<string name=\"phantom_cdata\">fake</string>]]>\n  <item name=\"phantom_item\">Not a string</item>\n  <string android:name=\"phantom_namespaced\">not named</string>\n</resources>\n"
	for _, tc := range []struct{ name, text string }{{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (XMLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("partial result %+v", result.Analysis)
			}
			found := map[string]NormalizedSymbol{}
			for _, sym := range result.Analysis.Symbols {
				found[sym.Name] = sym
			}
			for _, w := range []struct {
				name string
				line int
			}{{"app_name", 3}, {"lasttext", 4}} {
				sym, ok := found[w.name]
				if !ok || sym.Kind != SymbolKindResource || sym.NativeKind != "android-string" || sym.NameRange.Start.Line != w.line {
					t.Errorf("missing XML Android resource %s: %+v present=%v", w.name, sym, ok)
				}
			}
			for _, name := range []string{"phantom_comment", "phantom_cdata", "phantom_item", "phantom_namespaced"} {
				if _, ok := found[name]; ok {
					t.Errorf("invented resource %s", name)
				}
			}
		})
	}
	const unrelated = "<config><string name=\"not_a_resource\">ok</string><user name=\"alice\"/></config>"
	result, err := (XMLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(unrelated), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	for _, sym := range result.Analysis.Symbols {
		if sym.Name == "not_a_resource" || sym.Name == "alice" {
			t.Errorf("non-resource XML name indexed: %s", sym.Name)
		}
	}
}
