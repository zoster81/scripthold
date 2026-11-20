package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestAutoHotkeyNestedLocalFunctionsAndControlFlow(t *testing.T) {
	const source = `class JSON {
    static parse(text) {
        if (text) {
            Loop (2) {
                text := StrReplace(text, "x")
            }
        }
        return text
        UC(value) {
            return value
        }
    }
    static stringify(obj) {
        while (obj) {
            obj := ""
        }
        switch Type(obj) {
            case "String":
                return obj
        }
        CO(value) {
            return value
        }
        ES(value) {
            return value
        }
    }
}
regular(value) {
    return value
    step(v) {
        return v
    }
}
`
	for _, tc := range []struct{ name, text string }{
		{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (AutoHotkeyAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("complete source unexpectedly partial: %+v", result.Analysis)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range map[string]SymbolKind{
				"JSON":              SymbolKindClass,
				"JSON.parse":        SymbolKindMethod,
				"JSON.parse.UC":     SymbolKindFunction,
				"JSON.stringify":    SymbolKindMethod,
				"JSON.stringify.CO": SymbolKindFunction,
				"JSON.stringify.ES": SymbolKindFunction,
				"regular":           SymbolKindFunction,
				"regular.step":      SymbolKindFunction,
			} {
				symbol, ok := got[name]
				if !ok || symbol.Kind != kind {
					t.Errorf("expected %s kind %s: found=%v symbol=%+v", name, kind, ok, symbol)
				}
			}
			for _, fake := range []string{"JSON.parse.if", "JSON.parse.Loop",
				"JSON.stringify.while", "JSON.stringify.switch", "JSON.parse.text",
				"JSON.stringify.obj", "JSON.stringify.StrReplace"} {
				if _, exists := got[fake]; exists {
					t.Errorf("invented declaration %s", fake)
				}
			}
		})
	}
}
