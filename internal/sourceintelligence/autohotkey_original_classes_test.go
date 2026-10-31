package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestAutoHotkeyQualifiedOriginalClasses(t *testing.T) {
	tests := []struct {
		env    string
		symbol string
		line   int
		member string
	}{
		{"SCRIPTHOLD_REAL_AHK_RUNANY_JSON", "JSON", 1, "JSON.Load"},
		{"SCRIPTHOLD_REAL_AHK_EXILE_CUSTOMFONT", "CustomFont", 17, "CustomFont.AddFromFile"},
	}
	for _, tc := range tests {
		t.Run(tc.symbol, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("set " + tc.env + " to the pinned original source")
			}
			stat, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if stat.Size() > 1<<20 {
				t.Fatal("original exceeds 1 MiB")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			document := sourceDocumentForScanner(string(data))
			document.Path = path
			result, err := (AutoHotkeyAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 1024))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("partial original analysis: %+v", result.Analysis)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			class, ok := got[tc.symbol]
			if !ok || class.Kind != SymbolKindClass || class.NameRange.Start.Line != tc.line {
				t.Errorf("%s missing/wrong: %+v exists=%v; expected class line %d; names=%v", tc.symbol, class, ok, tc.line, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			if sym, ok := got[tc.member]; !ok || (sym.Kind != SymbolKindClass && sym.Kind != SymbolKindMethod) {
				t.Errorf("%s member missing or wrong kind %+v exists=%v", tc.member, sym, ok)
			}
		})
	}
}

func TestAutoHotkeyClassesBraceOnNextLine(t *testing.T) {
	const original = "class JSON\n{\n  class Load extends JSON.Functor\n  {\n    Call(self, text) { return text }\n  }\n}\nClass CustomFont\n{\n  AddFromFile(file) { return file }\n}\n"
	for _, tc := range []struct{ name, source string }{{"LF", original}, {"CRLF", strings.ReplaceAll(original, "\n", "\r\n")}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (AutoHotkeyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("partial %v", result.Analysis)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range map[string]SymbolKind{"JSON": SymbolKindClass, "JSON.Load": SymbolKindClass, "JSON.Load.Call": SymbolKindMethod, "CustomFont": SymbolKindClass, "CustomFont.AddFromFile": SymbolKindMethod} {
				s, ok := got[name]
				if !ok || s.Kind != kind {
					t.Errorf("%s: %+v ok=%v want=%s all=%v", name, s, ok, kind, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
		})
	}
}
