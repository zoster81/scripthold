package sourceintelligence

import (
	"context"
	"testing"
)

// A namespace alias is a real declaration, unlike a using-namespace import.
// Original C++/CLI project evidence: namespace fs = std::filesystem;
func TestCFamilyNamespaceAliasAndNamespaceImport(t *testing.T) {
	source := "namespace fs = std::filesystem;\n" +
		"using namespace System;\n" +
		"namespace Demo { namespace inner = std::chrono; class Box {}; }\n" +
		"namespace missing = ;\n"
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{"cpp", CPPAnalyzer{}}, {"cpp-cli", CPPCLIAnalyzer{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]SymbolKind{"fs": SymbolKindAlias, "Demo": SymbolKindNamespace, "Demo.inner": SymbolKindAlias, "Demo.Box": SymbolKindClass}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range want {
				s, ok := got[name]
				if !ok || s.Kind != kind {
					t.Errorf("%s missing/wrong kind=%s exists=%v, want %s", name, s.Kind, ok, kind)
				}
			}
			for _, forbidden := range []string{"System", "missing", "std", "chrono", "filesystem"} {
				if _, ok := got[forbidden]; ok {
					t.Errorf("false declaration %q", forbidden)
				}
			}
			if len(result.Analysis.Symbols) != len(want) {
				t.Errorf("got %d symbols, want %d: %v", len(result.Analysis.Symbols), len(want), sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}
