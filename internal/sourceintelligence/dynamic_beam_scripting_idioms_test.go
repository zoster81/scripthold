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

	t.Run("lua-long-bracket-context", func(t *testing.T) {
		text := "-- documentation marker [[ that is not a long comment\n" +
			"local first = \"nil --[[ LOOP:\\n\"\n" +
			"local second = \"^------- ]]\"\n" +
			"local opaque = [[ function Hidden() end ]]\n" +
			"local function visible() end\n"
		result, err := (LuaAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0 {
			t.Fatalf("valid Lua bracket-like text in comments/strings reported partial: %+v", result.Analysis)
		}
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		if !containsSortedString(names, "visible") || containsSortedString(names, "Hidden") {
			t.Fatalf("Lua lexical masking leaked/hid declarations: %v", names)
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

	t.Run("luau-interpolated-strings", func(t *testing.T) {
		text := "local function before() end\n" +
			"local watchedName = \"watched\"\n" +
			"local plain = `Entity's Position is {world:get(entity, Position)}`\n" +
			"local nested = `Hello {`from inside {\"a nested string\"}`}`\n" +
			"local escaped = `bracket = \\{, backtick = \\` = {'ok'}`\n" +
			"local function after() end\n"
		result, err := (LuauAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0 {
			t.Fatalf("valid Luau interpolated strings reported partial: %+v", result.Analysis)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		for _, name := range []string{"before", "after"} {
			if symbol, ok := byName[name]; !ok || symbol.Kind != SymbolKindFunction {
				t.Fatalf("Luau declaration %q = %+v exists=%v; symbols=%v", name, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
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

func TestRealWorldErlangCharacterAndMultilineStringFormsRemainOpaque(t *testing.T) {
	text := `-module(demo).
before() -> ok.
quote_chars() -> [$', $\", $\n].
legacy() -> <<"
line one
line two
">>.
-doc """
Documentation with "ordinary quotes" inside.
""".
after() -> ok.
`
	result, err := (ErlangAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("valid Erlang character/string forms reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"demo.before", "demo.quote_chars", "demo.legacy", "demo.after"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("missing %s; symbols=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
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
