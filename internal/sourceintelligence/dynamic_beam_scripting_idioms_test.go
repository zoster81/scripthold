package sourceintelligence

import (
	"context"
	"testing"
)

func TestCommonIdiomaticDeclarations(t *testing.T) {
	t.Run("lua-assigned-function", func(t *testing.T) {
		text := "local assigned = function(value) return value end\n"
		result, err := (LuaAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		if symbol, ok := symbolsByQualifiedName(result.Analysis.Symbols)["assigned"]; !ok || symbol.Kind != SymbolKindFunction {
			t.Fatalf("Lua assigned function = %+v exists=%v; symbols=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	})

	t.Run("luau-assigned-function", func(t *testing.T) {
		text := "local assigned = function(value: number): number return value end\n"
		result, err := (LuauAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		if symbol, ok := symbolsByQualifiedName(result.Analysis.Symbols)["assigned"]; !ok || symbol.Kind != SymbolKindFunction {
			t.Fatalf("Luau assigned function = %+v exists=%v; symbols=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	})

	t.Run("gleam-pub-opaque-type", func(t *testing.T) {
		text := "pub opaque type Secret { Secret(value: String) }\npub fn reveal(secret: Secret) { secret }\n"
		result, err := (GleamAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		if symbol, ok := byName["Secret"]; !ok || symbol.Kind != SymbolKindType {
			t.Fatalf("Gleam opaque type = %+v exists=%v; symbols=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
		if symbol, ok := byName["reveal"]; !ok || symbol.Kind != SymbolKindFunction {
			t.Fatalf("Gleam function after opaque type = %+v exists=%v; symbols=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	})
}

func TestCustomOpaqueConstructsReportIncompleteWhenUnterminated(t *testing.T) {
	tests := []struct {
		name     string
		analyzer SourceAnalyzer
		text     string
	}{
		{"perl-heredoc", PerlAnalyzer{}, "sub good {}\nmy $data = <<'EOF';\nsub Hidden {}\n"},
		{"lua-long-bracket", LuaAnalyzer{}, "function good() end\nlocal data = [[ function Hidden() end\n"},
		{"luau-long-bracket", LuauAnalyzer{}, "local function good() end\nlocal data = [=[ function Hidden() end\n"},
		{"groovy-dollar-slashy", GroovyAnalyzer{}, "def good() {}\ndef data = $/ class Hidden { }\n"},
		{"groovy-slashy", GroovyAnalyzer{}, "def good() {}\ndef data = / class Hidden { }\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
				t.Fatalf("unterminated custom opaque form reported complete: %+v", result.Analysis)
			}
			if containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), "Hidden") {
				t.Fatalf("unterminated custom opaque form leaked Hidden: %+v", result.Analysis.Symbols)
			}
		})
	}
}
