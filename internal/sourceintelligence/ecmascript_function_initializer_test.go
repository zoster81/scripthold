package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// A directly assigned function expression is callable, unlike a function
// nested in a call, object, conditional, or immediately invoked expression.
func TestECMAScriptDirectFunctionExpressionBindings(t *testing.T) {
	const source = "const direct = function (value) { return value };\n" +
		"const named = function inner(value) { return value };\n" +
		"const asyncDirect = async function (value) { return value };\n" +
		"const generator = function* () { yield 1 };\n" +
		"const fromCall = build(function (value) { return value });\n" +
		"const immediatelyInvoked = (function () { return 1 })();\n" +
		"const conditional = flag ? function () { return 1 } : function () { return 2 };\n" +
		"const compound = function () { return 1 } + fallback;\n" +
		"const settings = { handler: function () { return 1 } };\n" +
		"const after = 2;\n"
	for _, analyzer := range []SourceAnalyzer{
		JavaScriptAnalyzer{}, TypeScriptAnalyzer{}, FlowAnalyzer{}, JScriptNetAnalyzer{},
	} {
		t.Run(analyzer.Language(), func(t *testing.T) {
			for _, eol := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
				t.Run(eol.name, func(t *testing.T) {
					text := strings.ReplaceAll(source, "\n", eol.value)
					result, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 128))
					if err != nil {
						t.Fatal(err)
					}
					if result.Analysis.Truncated {
						t.Fatal("unexpected truncated analysis")
					}
					byName := symbolsByQualifiedName(result.Analysis.Symbols)
					for _, want := range []struct {
						name, native string
						kind         SymbolKind
					}{
						{"direct", "function-expression", SymbolKindFunction},
						{"named", "function-expression", SymbolKindFunction},
						{"asyncDirect", "function-expression", SymbolKindFunction},
						{"generator", "function-expression", SymbolKindFunction},
						{"fromCall", "const", SymbolKindConstant},
						{"immediatelyInvoked", "const", SymbolKindConstant},
						{"conditional", "const", SymbolKindConstant},
						{"compound", "const", SymbolKindConstant},
						{"settings", "const", SymbolKindConstant},
						{"after", "const", SymbolKindConstant},
					} {
						got, ok := byName[want.name]
						if !ok || got.Kind != want.kind || got.NativeKind != want.native {
							t.Errorf("%s: expected %s/%s, got %+v", want.name, want.kind, want.native, got)
						}
					}
					if _, ok := byName["inner"]; ok {
						t.Error("named function-expression inner identifier is not a standalone declaration")
					}
				})
			}
		})
	}
}

func TestTypeScriptDirectFunctionExpressionWithObjectReturnType(t *testing.T) {
	const source = "const measured = function (\n" +
		"  node: Node,\n" +
		"  width: number,\n" +
		"): {width: number; height: number} {\n" +
		"  return {width, height: 1};\n" +
		"};\n" +
		"const after = 3;\n"
	for _, eol := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(eol.name, func(t *testing.T) {
			text := strings.ReplaceAll(source, "\n", eol.value)
			result, err := (TypeScriptAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 96))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			if got, ok := byName["measured"]; !ok || got.Kind != SymbolKindFunction ||
				got.NativeKind != "function-expression" || got.NameRange.Start.Line != 1 ||
				got.DeclarationRange.End.Line != 6 {
				t.Errorf("multiline typed function expression not indexed correctly: %+v", got)
			}
			if got, ok := byName["after"]; !ok || got.Kind != SymbolKindConstant || got.NameRange.Start.Line != 7 {
				t.Errorf("following declaration not preserved: %+v", got)
			}
		})
	}
}
