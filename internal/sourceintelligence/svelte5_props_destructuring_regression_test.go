package sourceintelligence

import (
	"context"
	"testing"
)

// Source-derived from Svelte Bits' original copy-code-button.svelte.
// The object keys and type-only property names are not local runtime bindings.
func TestSvelte5PropsDestructuringRuntimeBindings(t *testing.T) {
	source := `<script lang="ts">
 import type { HTMLButtonAttributes } from "svelte/elements";
 let {
   class: className,
   copyCode,
   copied = false,
   ...rest
 }: {
   copyCode: () => void;
   copied?: boolean;
   typeOnly?: number;
 } & HTMLButtonAttributes = $props();
</script>
<button onclick={copyCode} class={className}>{#if copied}{/if}</button>
`
	result, err := (SvelteAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("copy-code-button.svelte", source),
		testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"className": 4, "copyCode": 5, "copied": 6, "rest": 7}
	forbidden := map[string]bool{"class": true, "typeOnly": true, "HTMLButtonAttributes": true, "onclick": true}
	seen := make(map[string]int)
	for _, symbol := range result.Analysis.Symbols {
		seen[symbol.Name] = symbol.NameRange.Start.Line
		if forbidden[symbol.Name] {
			t.Errorf("object key, type-only or markup invented as runtime declaration: %+v", symbol)
		}
	}
	for name, line := range want {
		if seen[name] != line {
			t.Errorf("missing runtime binding %s:%d (got %d); symbols=%v", name, line, seen[name], seen)
		}
	}
	if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
		t.Errorf("bounded valid Svelte5 script unexpectedly partial: %+v", result.Analysis)
	}
}

func TestECMAScriptDestructuringUnsupportedPatternsStayPartial(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		wanted       []string
		forbidden    []string
	}{
		{"nested object", "const {nested: {inner}, safe} = source;\n", []string{"safe"}, []string{"nested", "inner"}},
		{"array pattern", "const [first, second] = pair;\n", nil, []string{"first", "second", "pair"}},
		{"multiple declarators", "const {primary} = source, secondary = another;\n", []string{"primary"}, []string{"secondary"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (TypeScriptAnalyzer{}).Analyze(context.Background(), testSourceDocument("partial.ts", tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			for _, sym := range result.Analysis.Symbols {
				seen[sym.Name] = true
			}
			for _, name := range tc.wanted {
				if !seen[name] {
					t.Errorf("missing supported binding %s: %v", name, seen)
				}
			}
			for _, name := range tc.forbidden {
				if seen[name] {
					t.Errorf("fabricated unsupported binding %s: %v", name, seen)
				}
			}
			if result.Analysis.CoverageComplete {
				t.Errorf("unsupported pattern falsely reported complete: %v", seen)
			}
		})
	}
}

func TestECMAScriptObjectDestructuringBindingAliases(t *testing.T) {
	source := `const {user: selectedUser, active = true, ...rest} = read();
const unrelated = left == right;
`
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{"typescript", TypeScriptAnalyzer{}},
		{"javascript", JavaScriptAnalyzer{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), testSourceDocument("bindings.ts", source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			for _, sym := range result.Analysis.Symbols {
				seen[sym.Name] = true
			}
			for _, name := range []string{"selectedUser", "active", "rest", "unrelated"} {
				if !seen[name] {
					t.Errorf("missing binding %s; symbols=%v", name, seen)
				}
			}
			for _, name := range []string{"user", "read", "left", "right"} {
				if seen[name] {
					t.Errorf("key or expression incorrectly indexed as declaration %s", name)
				}
			}
		})
	}
}
