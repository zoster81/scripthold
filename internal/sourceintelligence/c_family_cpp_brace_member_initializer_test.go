package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestCPPBraceInitializedMemberMustNotConsumeFollowingMethod(t *testing.T) {
	const source = "namespace Example {\n" +
		"class Timer {\n" +
		" std::size_t cycles{1};\n" +
		" static std::string Simple(std::string title, std::string time) { return title + time; }\n" +
		" void next() {}\n" +
		"};\n" +
		"class Widget { public: explicit Widget(int value) : value_{value} {} int value_{0}; };\n" +
		"}\n" +
		"Example::Widget global{7};\n"
	for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(eol.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol.value))
			out, err := (CPPAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 96))
			if err != nil {
				t.Fatal(err)
			}
			if !out.Analysis.CoverageComplete {
				t.Fatalf("valid C++ brace initialization incomplete: %+v", out.Analysis.Diagnostics)
			}
			want := map[string]SymbolKind{
				"Example.Timer.cycles":  SymbolKindField,
				"Example.Timer.Simple":  SymbolKindMethod,
				"Example.Timer.next":    SymbolKindMethod,
				"Example.Widget.value_": SymbolKindField,
				"global":                SymbolKindVariable,
			}
			for _, s := range out.Analysis.Symbols {
				if kind, ok := want[s.QualifiedName]; ok {
					if s.Kind != kind {
						t.Errorf("%s kind %s instead of %s", s.QualifiedName, s.Kind, kind)
					}
					delete(want, s.QualifiedName)
				}
				if s.ParentQualifiedName == "Example.Timer" && (s.Name == "title" || s.Name == "time") {
					t.Errorf("method parameter or initializer invented as field: %+v", s)
				}
			}
			for name := range want {
				t.Errorf("braced C++ declaration missing: %s", name)
			}
		})
	}
}
