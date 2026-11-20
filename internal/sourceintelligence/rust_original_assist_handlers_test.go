package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Rust-analyzer assist handlers contain source declarations, nested unit tests,
// and apparent functions inside documentation examples and raw string literals.
// Only authored Rust declarations in the supported navigation subset count.
func TestRustOriginalAnalyzerAssistHandlerNavigation(t *testing.T) {
	type expected struct {
		line   int
		name   string
		kind   SymbolKind
		parent string
	}
	check := func(t *testing.T, source string, wants []expected) {
		t.Helper()
		result, err := (RustAnalyzer{}).Analyze(context.Background(),
			sourceDocumentForScanner(source), testAnalyzeOptions(true, 128))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
			t.Fatalf("original Rust assist source unexpectedly incomplete: %+v", result.Analysis)
		}
		if len(result.Analysis.Symbols) != len(wants) {
			t.Fatalf("Rust declarations=%d want=%d: %v",
				len(result.Analysis.Symbols), len(wants), sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
		used := make([]bool, len(wants))
		for _, sym := range result.Analysis.Symbols {
			if sym.ID == "" || sym.NameRange.Start.Line < 1 {
				t.Fatalf("unsafe Rust declaration identity: %+v", sym)
			}
			if sym.Name == "main" || sym.Name == "dyn_eq" || sym.Name == "Phantom" ||
				sym.Name == "Foo" || sym.Name == "Test" {
				t.Fatalf("documented example or string fabricated a declaration: %+v", sym)
			}
			found := false
			for i, want := range wants {
				if !used[i] && sym.NameRange.Start.Line == want.line &&
					sym.Name == want.name && sym.Kind == want.kind &&
					sym.ParentQualifiedName == want.parent {
					used[i], found = true, true
					break
				}
			}
			if !found {
				t.Errorf("unexpected Rust declaration: name=%q kind=%s line=%d parent=%q",
					sym.Name, sym.Kind, sym.NameRange.Start.Line, sym.ParentQualifiedName)
			}
		}
		for i, present := range used {
			if !present {
				t.Errorf("missing original declaration: %+v", wants[i])
			}
		}
	}
	const portable = "// fn main() {}\npub fn flip() {}\n#[cfg(test)]\nmod tests {\n    #[test]\n    fn works() { let _text = \"fn Phantom() {}\"; }\n}\n"
	portableWants := []expected{
		{2, "flip", SymbolKindFunction, ""},
		{4, "tests", SymbolKindModule, ""},
		{6, "works", SymbolKindFunction, "tests"},
	}
	for _, tc := range []struct{ name, eol string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			check(t, strings.ReplaceAll(portable, "\n", tc.eol), portableWants)
		})
	}
	root := os.Getenv("SCRIPTHOLD_REAL_RUST_ANALYZER_ASSISTS_ROOT")
	if root == "" {
		return
	}
	for _, tc := range []struct {
		name, anchor string
		wants        []expected
	}{
		{
			name: "flip_binexpr.rs", anchor: "pub(crate) fn flip_binexpr(",
			wants: []expected{
				{24, "flip_binexpr", SymbolKindFunction, ""},
				{63, "FlipAction", SymbolKindEnum, ""},
				{72, "FlipAction", SymbolKindImplementation, ""},
				{73, "from", SymbolKindMethod, "FlipAction"},
				{117, "flip_range_expr", SymbolKindFunction, ""},
				{159, "tests", SymbolKindModule, ""},
				{165, "flip_binexpr_target_is_the_op", SymbolKindFunction, "tests"},
				{170, "flip_binexpr_not_applicable_for_assignment", SymbolKindFunction, "tests"},
				{175, "flip_binexpr_works_for_eq", SymbolKindFunction, "tests"},
				{180, "flip_binexpr_works_for_gt", SymbolKindFunction, "tests"},
				{185, "flip_binexpr_works_for_lteq", SymbolKindFunction, "tests"},
				{190, "flip_binexpr_works_for_complex_expr", SymbolKindFunction, "tests"},
				{199, "flip_binexpr_works_for_lhs_arith", SymbolKindFunction, "tests"},
				{208, "flip_binexpr_works_for_lhs_cmp", SymbolKindFunction, "tests"},
				{217, "flip_binexpr_works_inside_match", SymbolKindFunction, "tests"},
			},
		},
		{
			name: "flip_comma.rs", anchor: "pub(crate) fn flip_comma(",
			wants: []expected{
				{24, "flip_comma", SymbolKindFunction, ""},
				{60, "flip_tree", SymbolKindFunction, ""},
				{105, "tests", SymbolKindModule, ""},
				{111, "flip_comma_works_for_function_parameters", SymbolKindFunction, "tests"},
				{120, "flip_comma_target", SymbolKindFunction, "tests"},
				{125, "flip_comma_before_punct", SymbolKindFunction, "tests"},
				{134, "flip_comma_works", SymbolKindFunction, "tests"},
				{143, "flip_comma_not_applicable_for_macro_input", SymbolKindFunction, "tests"},
				{150, "flip_comma_attribute", SymbolKindFunction, "tests"},
				{164, "flip_comma_attribute_incomplete", SymbolKindFunction, "tests"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, "crates", "ide-assists", "src", "handlers", tc.name)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original Rust assist unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			if strings.Count(source, tc.anchor) != 1 {
				t.Fatal("original Rust assist source anchor changed")
			}
			check(t, source, tc.wants)
		})
	}
}
