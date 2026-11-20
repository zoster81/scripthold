package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// A direct parenthesized arrow remains a function after a TypeScript type
// assertion, while a call or conditional containing an arrow does not.
func TestTypeScriptAssertedDirectArrowInitializer(t *testing.T) {
	const source = "type Callable = (value: number) => number;\n" +
		"export const createStore = ((createState) =>\n" +
		"  createState ? createStoreImpl(createState) : createStoreImpl) as CreateStore\n" +
		"const asserted = ((value: number) => value) as Callable;\n" +
		"const satisfied = ((value: number) => value) satisfies Callable;\n" +
		"const grouped = (((value: number) => value)) as Callable;\n" +
		"const immediate = (() => 3)();\n" +
		"const nestedCall = consume((value: number) => value) as Callable;\n" +
		"const groupedCall = (consume((value: number) => value)) as Callable;\n" +
		"const groupedConditional = (enabled ? (() => 1) : (() => 2)) as Callable;\n" +
		"const trailingOperator = ((value: number) => value) as Callable || fallback;\n" +
		"const conditional = enabled ? ((value: number) => value) : other;\n"
	for _, tc := range []struct{ name, source string }{
		{"LF", source},
		{"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (TypeScriptAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 96))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated {
				t.Fatalf("unexpected truncated scan: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, item := range []struct {
				name, native string
				kind         SymbolKind
			}{
				{"createStore", "arrow-function", SymbolKindFunction},
				{"asserted", "arrow-function", SymbolKindFunction},
				{"satisfied", "arrow-function", SymbolKindFunction},
				{"grouped", "arrow-function", SymbolKindFunction},
				{"immediate", "const", SymbolKindConstant},
				{"nestedCall", "const", SymbolKindConstant},
				{"groupedCall", "const", SymbolKindConstant},
				{"groupedConditional", "const", SymbolKindConstant},
				{"trailingOperator", "const", SymbolKindConstant},
				{"conditional", "const", SymbolKindConstant},
			} {
				got, ok := byName[item.name]
				if !ok || got.Kind != item.kind || got.NativeKind != item.native {
					t.Errorf("binding %s is not kind-correct: %+v (present %v)", item.name, got, ok)
				}
			}
		})
	}
}
