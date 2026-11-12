package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// OCaml binding groups use "and" for subsequent function or type
// declarations. Index actual declarations, not a parameter or a reference.
func TestOCamlTopLevelAndDeclarationGroups(t *testing.T) {
	const source = "(* and orphan x = x is not a declaration *)\n" +
		"let rec format out x =\n" +
		"  match x with\n" +
		"  | Some v -> v\n" +
		"  | None -> 0\n" +
		"and format_field out (name, x) = format out x\n" +
		"and format_tail out x = format_field out (\"field\", x)\n" +
		"let next v = v\n" +
		"type 'a wrapped = Wrapped of 'a\n" +
		"and fallback = Fallback\n" +
		"let after v = v\n"
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(map[string]string{"\n": "lf", "\r\n": "crlf"}[eol], func(t *testing.T) {
			result, err := (OCamlAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol)),
				testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("balanced lexical source unexpectedly partial: %+v", result.Analysis)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			want := map[string]SymbolKind{
				"format": SymbolKindFunction, "format_field": SymbolKindFunction,
				"format_tail": SymbolKindFunction, "next": SymbolKindFunction,
				"wrapped": SymbolKindType, "fallback": SymbolKindType,
				"after": SymbolKindFunction,
			}
			for name, kind := range want {
				if sym, exists := got[name]; !exists || sym.Kind != kind {
					t.Errorf("%q: got=%+v exists=%v want kind=%s", name, sym, exists, kind)
				}
			}
			for _, ghost := range []string{"orphan", "out", "name", "x", "a"} {
				if _, found := got[ghost]; found {
					t.Errorf("non-declaration %q indexed as a symbol", ghost)
				}
			}
		})
	}
}
