package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// An explicit class-template instantiation requests code generation for an
// existing type. Its argument types do not declare variables or new classes.
func TestCPPExplicitClassTemplateInstantiationDoesNotDeclareMembers(t *testing.T) {
	const source = "template <typename T>\n" +
		"struct Box { T value; };\n" +
		"template class Box<int>;\n" +
		"extern template struct Box<long>;\n" +
		"template class EXPORT_API Named::Sink<Named::Mutex>;\n" +
		"namespace Named { struct Mutex {}; }\n" +
		"void after() {}\n"
	for _, tc := range []struct{ name, eol string }{
		{"lf", "\n"}, {"crlf", "\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(strings.ReplaceAll(source, "\n", tc.eol))
			out, err := (CPPAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 96))
			if err != nil {
				t.Fatal(err)
			}
			if !out.Analysis.CoverageComplete {
				t.Fatalf("valid explicit template instantiation marked incomplete: %+v", out.Analysis.Diagnostics)
			}
			count := map[string]int{}
			for _, s := range out.Analysis.Symbols {
				count[s.QualifiedName]++
				if s.QualifiedName == "int" || s.QualifiedName == "long" ||
					s.QualifiedName == "Mutex" || s.QualifiedName == "Named.Mutex" && s.Kind == SymbolKindVariable ||
					s.Kind == SymbolKindVariable && (s.Name == "int" || s.Name == "long" || s.Name == "Mutex") {
					t.Errorf("template argument invented as declaration: %+v", s)
				}
			}
			for _, name := range []string{"Box", "Box.value", "Named", "Named.Mutex", "after"} {
				if count[name] != 1 {
					t.Errorf("expected one %s, got %d; symbols=%v", name, count[name], sortedSymbolQualifiedNames(out.Analysis.Symbols))
				}
			}
			if count["Named.Sink"] != 0 {
				t.Errorf("explicit instantiation invented a new class")
			}
		})
	}
}
