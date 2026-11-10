package sourceintelligence

import (
	"context"
	"testing"
)

// Classic ASP keeps "jscript" as its authored embedded language, while the
// registered JavaScript descriptor supplies structural-project lookup keys.
func TestClassicASPJScriptSymbolsReachProjectStructuralSearch(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	source := "<%@ Language=\"VBScript\" %>\n" +
		"<script language=\"javascript\" runat=\"server\">\n" +
		"function run(){ return related(); }\n" +
		"function related(){ return 1; }\n" +
		"var note = 'function fake()';\n" +
		"</script>\n"
	facts := projectResolverFacts(t, ClassicASPAnalyzer{}, "project/session.asp", source)
	byName := symbolsByQualifiedName(facts.Analysis.Analysis.Symbols)
	for _, name := range []string{"run", "related"} {
		sym, ok := byName[name]
		if !ok || sym.Language != "jscript" || sym.Analyzer != string(AnalyzerClassicASP) {
			t.Fatalf("embedded JScript declaration unavailable: %q %+v", name, sym)
		}
	}
	if _, hasFake := byName["fake"]; hasFake {
		t.Fatal("fabricated a function from an embedded JavaScript string")
	}
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{facts}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"run", "related"} {
		got, err := model.StructuralSearch(context.Background(), ProjectSearchOptions{
			Query: name, Match: ProjectSearchExact, MaxResults: 16,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Matches) != 1 || got.Matches[0].Name != name ||
			got.Matches[0].SymbolID != byName[name].ID ||
			got.Matches[0].Language != "jscript" ||
			got.Matches[0].SourceFingerprint != facts.SourceFingerprint {
			t.Fatalf("embedded JScript project-search identity mismatch for %q: %+v", name, got)
		}
	}
}
