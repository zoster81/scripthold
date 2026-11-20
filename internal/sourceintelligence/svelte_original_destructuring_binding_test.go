package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The three byte-pinned upstream Svelte sources are optional external fixtures.
// They are read without execution; a configured but incomplete corpus is a failure.
func TestSvelteOriginalFinalDestructuredBindingAndControls(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_SVELTE_OUTLINE_ROOT")
	if root == "" {
		t.Skip("set SCRIPTHOLD_REAL_SVELTE_OUTLINE_ROOT to all three pinned Svelte originals")
	}
	type expected struct {
		name, native string
		kind         SymbolKind
		line         int
	}
	tests := []struct {
		file string
		want []expected
	}{
		{"skeleton-item-content.svelte", []expected{
			{"AccordionItemContentProps", "interface", SymbolKindInterface, 5},
			{"props", "const", SymbolKindConstant, 13},
			{"accordion", "const", SymbolKindConstant, 15},
			{"itemProps", "const", SymbolKindConstant, 16},
			{"element", "const", SymbolKindConstant, 18},
			{"children", "const", SymbolKindConstant, 18},
			{"rest", "const", SymbolKindConstant, 18},
			{"attributes", "const", SymbolKindConstant, 20},
		}},
		{"shadcn-copy-code-button.svelte", []expected{
			{"ctx", "const", SymbolKindConstant, 10},
			{"clipboard", "const", SymbolKindConstant, 12},
		}},
		{"flowbite-accordion-item.svelte", []expected{
			{"children", "let", SymbolKindVariable, 12},
			{"header", "let", SymbolKindVariable, 13},
			{"arrowup", "let", SymbolKindVariable, 14},
			{"arrowdown", "let", SymbolKindVariable, 15},
			{"headingTag", "let", SymbolKindVariable, 16},
			{"open", "let", SymbolKindVariable, 17},
			{"activeClass", "let", SymbolKindVariable, 18},
			{"inactiveClass", "let", SymbolKindVariable, 19},
			{"transitionType", "let", SymbolKindVariable, 20},
			{"transitionParams", "let", SymbolKindVariable, 21},
			{"className", "let", SymbolKindVariable, 22},
			{"classes", "let", SymbolKindVariable, 23},
			{"headerClass", "let", SymbolKindVariable, 24},
			{"contentClass", "let", SymbolKindVariable, 25},
			{"styling", "let", SymbolKindVariable, 44},
			{"ctx", "const", SymbolKindConstant, 47},
			{"ctxTransitionType", "const", SymbolKindConstant, 49},
			{"useTransition", "const", SymbolKindConstant, 51},
			{"theme", "const", SymbolKindConstant, 54},
			{"self", "const", SymbolKindConstant, 57},
			{"updateSingleSelection", "const", SymbolKindConstant, 59},
			{"handleToggle", "arrow-function", SymbolKindFunction, 65},
			{"base", "const", SymbolKindConstant, 69},
			{"button", "const", SymbolKindConstant, 69},
			{"content", "const", SymbolKindConstant, 69},
			{"active", "const", SymbolKindConstant, 69},
			{"inactive", "const", SymbolKindConstant, 69},
			{"buttonClass", "let", SymbolKindVariable, 71},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, tc.file))
			if err != nil || len(raw) == 0 {
				t.Fatalf("configured full original source absent/empty: %v", err)
			}
			doc := sourceDocumentForScanner(string(raw))
			doc.Path = tc.file
			result, err := (SvelteAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				count := 0
				for _, sym := range result.Analysis.Symbols {
					if sym.Name != want.name || sym.NameRange.Start.Line != want.line {
						continue
					}
					count++
					if sym.Kind != want.kind || sym.NativeKind != want.native ||
						sym.QualifiedName != want.name || sym.ParentID != "" || sym.ParentQualifiedName != "" {
						t.Errorf("wrong original source definition %s line %d: %+v", want.name, want.line, sym)
					}
					if want.name == "contentClass" && sym.NameRange.Start.Column != 5 {
						t.Errorf("contentClass original name starts at line 25 column 5, got %+v", sym.NameRange)
					}
				}
				if count != 1 {
					t.Errorf("authored %s line %d missing or duplicated (%d), actual=%v",
						want.name, want.line, count, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			if len(result.Analysis.Symbols) != len(tc.want) {
				t.Errorf("whole original symbol inventory=%d expected=%d, invented or omitted",
					len(result.Analysis.Symbols), len(tc.want))
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Errorf("whole original supported component unexpectedly incomplete/truncated")
			}
		})
	}
}
