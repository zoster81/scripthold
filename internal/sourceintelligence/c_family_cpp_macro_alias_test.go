package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestCPPUnresolvedStandaloneMacroPreservesFollowingDeclarations(t *testing.T) {
	for _, tc := range []struct{ name, eol string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			source := "NAMESPACE_OPEN(ROOT)\n" +
				"using another::imported_type;\n" +
				"NAMESPACE_OPEN(inner)\n" +
				"template <typename H>\n" +
				"using is_holder = std::is_same<H, imported_type>;\n" +
				"NAMESPACE_CLOSE(inner)\n" +
				"NAMESPACE_CLOSE(ROOT)\n" +
				"namespace real {\n" +
				"  template <typename T> using value_type = T;\n" +
				"  void actual_method(int arg);\n" +
				"}\n"
			doc := sourceDocumentForScanner(strings.ReplaceAll(source, "\n", tc.eol))
			result, err := (CPPAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 80))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete {
				t.Error("unknown namespace macro expansions must not claim fully correct ownership")
			}
			want := map[string]SymbolKind{
				"is_holder":          SymbolKindAlias,
				"real":               SymbolKindNamespace,
				"real.value_type":    SymbolKindAlias,
				"real.actual_method": SymbolKindFunction,
			}
			seenWarnings := false
			for _, diagnostic := range result.Analysis.Diagnostics {
				if strings.Contains(diagnostic.Code, "unresolved") && strings.Contains(diagnostic.Code, "macro") {
					seenWarnings = true
				}
			}
			if !seenWarnings {
				t.Errorf("unknown macro scope must be reported explicitly: %+v", result.Analysis.Diagnostics)
			}
			for _, s := range result.Analysis.Symbols {
				if strings.HasPrefix(s.Name, "NAMESPACE_OPEN") || strings.HasPrefix(s.Name, "NAMESPACE_CLOSE") {
					t.Errorf("macro invocation fabricated as function: %+v", s)
				}
				if kind, ok := want[s.QualifiedName]; ok {
					if s.Kind != kind {
						t.Errorf("%s is %s, expected %s", s.QualifiedName, s.Kind, kind)
					}
					delete(want, s.QualifiedName)
				}
			}
			for name := range want {
				t.Errorf("actual C++ declaration %q missing", name)
			}
		})
	}
}

func TestCPPStandaloneMacroCounterexamples(t *testing.T) {
	source := "void REGISTER_CALLBACK(int argument);\n" +
		"namespace actual { template <class T> struct Node { T value; }; }\n" +
		"template <typename T> using Wrapper = actual::Node<T>;\n" +
		"int ordinary_function(int value) { return value; }\n"
	out, err := (CPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 80))
	if err != nil {
		t.Fatal(err)
	}
	if !out.Analysis.CoverageComplete {
		t.Fatalf("valid declarations must not be mistaken for macro wrappers: %+v", out.Analysis.Diagnostics)
	}
	wanted := map[string]SymbolKind{
		"REGISTER_CALLBACK": SymbolKindFunction,
		"actual":            SymbolKindNamespace,
		"actual.Node":       SymbolKindStruct,
		"actual.Node.value": SymbolKindField,
		"Wrapper":           SymbolKindAlias,
		"ordinary_function": SymbolKindFunction,
	}
	for _, s := range out.Analysis.Symbols {
		if kind, ok := wanted[s.QualifiedName]; ok {
			if s.Kind != kind {
				t.Errorf("%s: got %s want %s", s.QualifiedName, s.Kind, kind)
			}
			delete(wanted, s.QualifiedName)
		}
	}
	for name := range wanted {
		t.Errorf("valid declaration missing: %s", name)
	}
}
