package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Both Free Pascal and Delphi permit class-prefixed declarations and
// implementations. Router authorization methods must not silently disappear.
func TestPascalDelphiClassMethodNavigation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{"pascal", PascalAnalyzer{}}, {"delphi", DelphiAnalyzer{}},
	} {
		for _, ending := range []string{"\n", "\r\n"} {
			label := "LF"
			if ending == "\r\n" {
				label = "CRLF"
			}
			t.Run(tc.name+"/"+label, func(t *testing.T) {
				text := strings.Join([]string{
					"unit Router;",
					"interface",
					"type",
					"  TRouter = class",
					"  public",
					"    class procedure MethodNotAllowed; static;",
					"    class function Service: TRouter;",
					"    procedure Route;",
					"  end;",
					"implementation",
					"class procedure TRouter.MethodNotAllowed;",
					"begin",
					"end;",
					"class function TRouter.Service: TRouter;",
					"begin",
					"end;",
					"procedure TRouter.Route;",
					"begin",
					"end;",
					"end.",
				}, ending) + ending
				result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 100))
				if err != nil {
					t.Fatal(err)
				}
				if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
					t.Fatalf("valid class methods reported partial: %+v", result.Analysis)
				}
				byName := map[string][]NormalizedSymbol{}
				for _, sym := range result.Analysis.Symbols {
					byName[sym.QualifiedName] = append(byName[sym.QualifiedName], sym)
				}
				for _, w := range []struct {
					qualified               string
					declaration, definition int
				}{
					{"Router.TRouter.MethodNotAllowed", 6, 11},
					{"Router.TRouter.Service", 7, 14},
				} {
					declarations := byName[w.qualified]
					if len(declarations) != 2 {
						t.Errorf("expected class method pair %s, got %+v", w.qualified, declarations)
						continue
					}
					hits := map[int]bool{}
					for _, sym := range declarations {
						if sym.Kind != SymbolKindMethod || !strings.Contains(strings.Join(sym.Modifiers, ","), "class") {
							t.Errorf("class method kind/modifier: %+v", sym)
						}
						hits[sym.DeclarationRange.Start.Line] = true
					}
					if !hits[w.declaration] || !hits[w.definition] {
						t.Errorf("bad class method lines %s: %v", w.qualified, hits)
					}
				}
				if len(byName["Router.TRouter.Route"]) != 2 {
					t.Errorf("ordinary instance method missing: %+v", byName["Router.TRouter.Route"])
				}
				if len(byName["Router.TRouter.class"]) != 0 {
					t.Errorf("false class keyword: %+v", byName["Router.TRouter.class"])
				}
			})
		}
	}
}
