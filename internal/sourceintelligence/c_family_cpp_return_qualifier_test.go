package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// A qualified return type is not an authored owner qualifier. In contrast,
// the qualifier immediately before a method name is part of its declaration.
func TestCPPQualifiedReturnTypeDoesNotInventMemberScope(t *testing.T) {
	source := "namespace API {\n" +
		"class Timer {\n" +
		"public:\n" +
		" static std::string Big(std::string title) { return title; }\n" +
		" std::string elapsed() const { return {}; }\n" +
		"};\n" +
		"}\n" +
		"std::ostream& operator<<(std::ostream& out, API::Timer const& t) { return out; }\n" +
		"void API::Timer::reset() {}\n"
	for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(eol.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol.value))
			result, err := (CPPAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Fatalf("valid cpp source partial: %+v", result.Analysis.Diagnostics)
			}
			want := map[string]SymbolKind{
				"API.Timer.Big":     SymbolKindMethod,
				"API.Timer.elapsed": SymbolKindMethod,
				"operator<<":        SymbolKindOperator,
				"API.Timer.reset":   SymbolKindMethod,
			}
			for _, symbol := range result.Analysis.Symbols {
				if strings.Contains(symbol.QualifiedName, ".std.") || strings.HasPrefix(symbol.QualifiedName, "std.") {
					t.Errorf("return-type namespace invented as function owner: %+v", symbol)
				}
				if kind, exists := want[symbol.QualifiedName]; exists {
					if symbol.Kind != kind {
						t.Errorf("%s has kind %s want %s", symbol.QualifiedName, symbol.Kind, kind)
					}
					delete(want, symbol.QualifiedName)
				}
			}
			for name := range want {
				t.Errorf("original function missing: %s", name)
			}
		})
	}
}
