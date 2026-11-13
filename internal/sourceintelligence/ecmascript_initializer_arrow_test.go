package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// A nested arrow in an object, call or conditional expression does not turn
// the containing binding into a direct arrow-function declaration.
func TestECMAScriptVariableKindRequiresDirectArrowInitializer(t *testing.T) {
	const source = "const config = { handler: () => 1 };\n" +
		"const mapped = values.map(value => value);\n" +
		"const conditional = enabled ? () => 1 : () => 2;\n" +
		"const immediate = (value) => value;\n" +
		"const bare = value => value;\n" +
		"const asyncDirect = async (value) => value;\n" +
		"const grouped = (() => 1);\n" +
		"let settings = { refresh: (value) => value };\n" +
		"var legacy = { start: () => 1 };\n"
	for _, analyzer := range []SourceAnalyzer{
		JavaScriptAnalyzer{}, TypeScriptAnalyzer{}, FlowAnalyzer{}, JScriptNetAnalyzer{},
	} {
		t.Run(analyzer.Language(), func(t *testing.T) {
			for _, lineEndings := range []struct{ name, source string }{
				{"LF", source},
				{"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
			} {
				t.Run(lineEndings.name, func(t *testing.T) {
					document := sourceDocumentForScanner(lineEndings.source)
					result, err := analyzer.Analyze(context.Background(), document, testAnalyzeOptions(true, 96))
					if err != nil {
						t.Fatal(err)
					}
					if result.Analysis.Truncated {
						t.Fatalf("bounded ECMAScript source unexpectedly truncated: %+v", result.Analysis)
					}
					byName := symbolsByQualifiedName(result.Analysis.Symbols)
					for _, item := range []struct {
						name, native string
						kind         SymbolKind
					}{
						{"config", "const", SymbolKindConstant},
						{"mapped", "const", SymbolKindConstant},
						{"conditional", "const", SymbolKindConstant},
						{"immediate", "arrow-function", SymbolKindFunction},
						{"bare", "arrow-function", SymbolKindFunction},
						{"asyncDirect", "arrow-function", SymbolKindFunction},
						{"grouped", "arrow-function", SymbolKindFunction},
						{"settings", "let", SymbolKindVariable},
						{"legacy", "var", SymbolKindVariable},
					} {
						got, ok := byName[item.name]
						if !ok || got.NativeKind != item.native || got.Kind != item.kind {
							t.Errorf("binding %s classified incorrectly: %+v", item.name, got)
						}
					}
				})
			}
		})
	}
}

func TestTypeScriptTypedVariableArrowInitializerDisambiguation(t *testing.T) {
	const source = "const merelyTyped: (x: number) => number = makeFunction();\n" +
		"const directTyped: (x: number) => number = (x) => x;\n" +
		"const generic = <T,>(value: T): T => value;\n" +
		"const options: { format: (v: string) => string } = { format: (v) => v };\n"
	for _, analyzer := range []SourceAnalyzer{TypeScriptAnalyzer{}, FlowAnalyzer{}} {
		t.Run(analyzer.Language(), func(t *testing.T) {
			result, err := analyzer.Analyze(context.Background(),
				sourceDocumentForScanner(source), testAnalyzeOptions(true, 96))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, item := range []struct {
				name, native string
				kind         SymbolKind
			}{
				{"merelyTyped", "const", SymbolKindConstant},
				{"directTyped", "arrow-function", SymbolKindFunction},
				{"generic", "arrow-function", SymbolKindFunction},
				{"options", "const", SymbolKindConstant},
			} {
				got, ok := byName[item.name]
				if !ok || got.Kind != item.kind || got.NativeKind != item.native {
					t.Errorf("typed declaration %s is not kind-correct: %+v", item.name, got)
				}
			}
		})
	}
}
