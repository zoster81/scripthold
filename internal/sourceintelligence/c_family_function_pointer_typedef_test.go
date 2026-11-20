package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestCFamilyFunctionPointerTypedefName(t *testing.T) {
	source := "struct Clock {\n typedef void(*TIMERCALLBACK)(DWORD_PTR);\n TIMERCALLBACK callback;\n};\n"
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{"c", CAnalyzer{}}, {"cpp", CPPAnalyzer{}},
	} {
		for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"/"+eol.name, func(t *testing.T) {
				document := sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol.value))
				result, err := tc.analyzer.Analyze(context.Background(), document, testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				symbols := symbolsByQualifiedName(result.Analysis.Symbols)
				if s, ok := symbols["Clock.TIMERCALLBACK"]; !ok || s.Kind != SymbolKindAlias {
					t.Errorf("missing function-pointer alias TIMERCALLBACK: %+v", result.Analysis.Symbols)
				}
				if s, ok := symbols["Clock.DWORD_PTR"]; ok {
					t.Errorf("parameter type invented as alias: %+v", s)
				}
			})
		}
	}
}
