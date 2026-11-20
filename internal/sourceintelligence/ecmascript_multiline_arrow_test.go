package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestECMAScriptMultilineVariableInitializerKind(t *testing.T) {
	cases := []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{"javascript", JavaScriptAnalyzer{}},
		{"typescript", TypeScriptAnalyzer{}},
		{"flow", FlowAnalyzer{}},
		{"jscriptnet", JScriptNetAnalyzer{}},
	}
	const ordinary = "const handler =\n  (event) => event;\n" +
		"const nested =\n  consume((event) => event);\n" +
		"const invoked =\n  (() => 12)();\n" +
		"const text =\n  'a value';\n" +
		"const next = 1;\n"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, eol := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
				t.Run(eol.name, func(t *testing.T) {
					input := strings.ReplaceAll(ordinary, "\n", eol.value)
					result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(input), testAnalyzeOptions(true, 96))
					if err != nil {
						t.Fatal(err)
					}
					if result.Analysis.Truncated {
						t.Fatalf("unexpected truncation")
					}
					m := symbolsByQualifiedName(result.Analysis.Symbols)
					for _, want := range []struct {
						name, native string
						kind         SymbolKind
					}{
						{"handler", "arrow-function", SymbolKindFunction},
						{"nested", "const", SymbolKindConstant},
						{"invoked", "const", SymbolKindConstant},
						{"text", "const", SymbolKindConstant},
						{"next", "const", SymbolKindConstant},
					} {
						s, ok := m[want.name]
						if !ok || s.Kind != want.kind || s.NativeKind != want.native {
							t.Errorf("%s: expected %s %s; got %+v", want.name, want.kind, want.native, s)
						}
					}
				})
			}
		})
	}
	const generic = "const toThenable =\n" +
		"  <Result, Input>(\n" +
		"    fn: (input: Input) => Result | Promise<Result>,\n" +
		"  ) =>\n" +
		"  (input: Input): Result => {\n" +
		"    return fn(input)\n" +
		"  }\n" +
		"const after = 1;\n"
	for _, eol := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run("typescript-generic-"+eol.name, func(t *testing.T) {
			input := strings.ReplaceAll(generic, "\n", eol.value)
			result, err := (TypeScriptAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(input), testAnalyzeOptions(true, 96))
			if err != nil {
				t.Fatal(err)
			}
			m := symbolsByQualifiedName(result.Analysis.Symbols)
			s, ok := m["toThenable"]
			if !ok || s.Kind != SymbolKindFunction || s.NativeKind != "arrow-function" ||
				s.NameRange.Start.Line != 1 || s.DeclarationRange.End.Line != 7 {
				t.Errorf("multiline generic arrow not correctly indexed: %+v", s)
			}
			if after, ok := m["after"]; !ok || after.Kind != SymbolKindConstant || after.NameRange.Start.Line != 8 {
				t.Errorf("following declaration lost: %+v", after)
			}
		})
	}
}
