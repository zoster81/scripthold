package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Groovy-only script initialization and callable recognition must not
// suppress a real user-defined method merely because its name is also a DSL call.
func TestGroovyJenkinsScriptDeclarationsAndDSLCallBoundaries(t *testing.T) {
	const source = `def list = [
    hook(name: 'demo'),
]
def callback = { -> stage('inner') { echo 'ok' } }
def enabled = true
stage('outside') { echo 'yes' }
void stage(String name, Closure block) { block() }
class Fixture {
  Fixture(String name) {}
  def run(String value) { println value }
  int count = 0
}
def after = 2
`
	for _, tc := range []struct{ name, source string }{
		{"lf", source},
		{"crlf", strings.ReplaceAll(source, "\n", "\r\n")},
		{"utf8-bom", "\uFEFF" + source},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := (GroovyAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !out.Analysis.CoverageComplete || out.Analysis.Truncated {
				t.Errorf("bounded well-formed source unexpectedly incomplete")
			}
			want := []struct {
				qualified string
				kind      SymbolKind
				line      int
			}{
				{"list", SymbolKindVariable, 1},
				{"callback", SymbolKindVariable, 4},
				{"enabled", SymbolKindVariable, 5},
				{"stage", SymbolKindFunction, 7},
				{"Fixture", SymbolKindClass, 8},
				{"Fixture.Fixture", SymbolKindConstructor, 9},
				{"Fixture.run", SymbolKindMethod, 10},
				{"Fixture.count", SymbolKindField, 11},
				{"after", SymbolKindVariable, 13},
			}
			if len(out.Analysis.Symbols) != len(want) {
				t.Errorf("got %d symbols, want %d: %+v", len(out.Analysis.Symbols), len(want),
					sortedSymbolQualifiedNames(out.Analysis.Symbols))
			}
			for _, w := range want {
				found := false
				for _, sym := range out.Analysis.Symbols {
					if sym.QualifiedName == w.qualified && sym.Kind == w.kind &&
						sym.NameRange.Start.Line == w.line {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing authored declaration %s (%s) line %d", w.qualified, w.kind, w.line)
				}
			}
			for _, sym := range out.Analysis.Symbols {
				if sym.Name == "hook" || sym.Name == "echo" || sym.Name == "block" {
					t.Errorf("expression/DSL call indexed as declaration: %+v", sym)
				}
			}
		})
	}
}
