package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// The anonymous typedef enum in curl/include/curl/header.h defines a type,
// not an object. Exercise the shared C/C++ parser with both line endings.
func TestCFamilyAnonymousEnumTypedefIsNotVariable(t *testing.T) {
	source := "typedef enum {\n  CURLHE_OK = 0,\n  CURLHE_BAD_ARGUMENT\n} CURLHcode;\nint after_type(void);\n"
	for _, language := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{{"c", CAnalyzer{}}, {"cpp", CPPAnalyzer{}}} {
		for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(language.name+"/"+eol.name, func(t *testing.T) {
				document := sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol.value))
				result, err := language.analyzer.Analyze(context.Background(), document, testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				symbols := symbolsByQualifiedName(result.Analysis.Symbols)
				alias, ok := symbols["CURLHcode"]
				if !ok || alias.Kind != SymbolKindAlias || alias.NameRange.Start.Line != 4 {
					t.Errorf("anonymous typedef alias missing or misclassified: %+v found=%v", alias, ok)
				}
				fn, ok := symbols["after_type"]
				if !ok || fn.Kind != SymbolKindFunction || fn.NameRange.Start.Line != 5 {
					t.Errorf("declaration following typedef missing: %+v found=%v", fn, ok)
				}
				for _, forbidden := range []string{"CURLHE_OK", "CURLHE_BAD_ARGUMENT", "enum", "typedef"} {
					if _, ok := symbols[forbidden]; ok {
						t.Errorf("false declaration %q: %+v", forbidden, symbols[forbidden])
					}
				}
			})
		}
	}
}
