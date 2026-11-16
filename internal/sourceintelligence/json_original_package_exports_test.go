package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original Vue and Svelte package manifests contain nested conditional exports,
// repeated object keys, punctuation-bearing paths, and non-key script strings.
// Their authored key locations remain distinct without resolving Node exports.
func TestJSONOriginalVueSveltePackageExportNavigation(t *testing.T) {
	type expected struct {
		name, qualified, parent string
	}
	check := func(t *testing.T, source string, wants map[int]expected) {
		t.Helper()
		result, err := (JSONAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("package.json", source), testAnalyzeOptions(true, 512))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
			t.Fatalf("strict original JSON manifest unexpectedly incomplete: %+v", result.Analysis)
		}
		seen := make(map[int]bool, len(wants))
		for _, sym := range result.Analysis.Symbols {
			if sym.Kind != SymbolKindKey || sym.ID == "" || sym.NameRange.Start.Line < 1 {
				t.Fatalf("JSON source navigation fabricated an unsafe declaration: %+v", sym)
			}
			if strings.HasPrefix(sym.Name, "node scripts/") ||
				strings.HasPrefix(sym.Name, "./src/") ||
				strings.HasPrefix(sym.Name, "workspace:") {
				t.Fatalf("manifest value fabricated a JSON key: %+v", sym)
			}
			if want, ok := wants[sym.NameRange.Start.Line]; ok && sym.Name == want.name {
				if sym.Name != want.name || sym.QualifiedName != want.qualified ||
					sym.ParentQualifiedName != want.parent {
					t.Errorf("original JSON key at line %d: got %+v, want %+v",
						sym.NameRange.Start.Line, sym, want)
				} else {
					seen[sym.NameRange.Start.Line] = true
				}
			}
		}
		if len(seen) != len(wants) {
			t.Fatalf("original JSON keys absent: got=%v want=%v", seen, wants)
		}
	}
	const portable = "{\n  \"exports\": {\n    \".\": {\n      \"import\": {\"types\": \"./a.d.mts\", \"default\": \"./a.mjs\"},\n      \"require\": {\"types\": \"./a.d.ts\"}\n    },\n    \"./compiler\": {\"import\": \"./compiler.mjs\"}\n  }\n}\n"
	portableWants := map[int]expected{
		2: {"exports", "exports", ""},
		3: {".", "exports..", "exports"},
		4: {"import", "exports...import", "exports.."},
		5: {"require", "exports...require", "exports.."},
		7: {"./compiler", "exports../compiler", "exports"},
	}
	for _, tc := range []struct{ name, eol string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			check(t, strings.ReplaceAll(portable, "\n", tc.eol), portableWants)
		})
	}
	for _, tc := range []struct {
		name, env, rel, anchor string
		wants                  map[int]expected
	}{
		{
			name: "vue-root", env: "SCRIPTHOLD_REAL_JSON_VUE_ROOT", rel: "package.json",
			anchor: "\"packageManager\": \"pnpm@11.19.0\"",
			wants: map[int]expected{
				6:  {"scripts", "scripts", ""},
				8:  {"build", "scripts.build", "scripts"},
				64: {"devDependencies", "devDependencies", ""},
			},
		},
		{
			name: "vue-package", env: "SCRIPTHOLD_REAL_JSON_VUE_ROOT", rel: "packages/vue/package.json",
			anchor: "\"./compiler-sfc\": {",
			wants: map[int]expected{
				19: {"exports", "exports", ""},
				20: {".", "exports..", "exports"},
				21: {"import", "exports...import", "exports.."},
				23: {"node", "exports...import.node", "exports...import"},
				28: {"node", "exports...require.node", "exports...require"},
				46: {"./compiler-sfc", "exports../compiler-sfc", "exports"},
				49: {"browser", "exports../compiler-sfc.import.browser", "exports../compiler-sfc.import"},
				70: {"./package.json", "exports../package.json", "exports"},
			},
		},
		{
			name: "svelte-root", env: "SCRIPTHOLD_REAL_JSON_SVELTE_ROOT", rel: "package.json",
			anchor: "\"name\": \"svelte-monorepo\"",
			wants: map[int]expected{
				16: {"scripts", "scripts", ""},
				22: {"changeset:version", "scripts.changeset:version", "scripts"},
				28: {"devDependencies", "devDependencies", ""},
			},
		},
		{
			name: "svelte-package", env: "SCRIPTHOLD_REAL_JSON_SVELTE_ROOT", rel: "packages/svelte/package.json",
			anchor: "\"./internal/flags/tracing\": {",
			wants: map[int]expected{
				22:  {"exports", "exports", ""},
				23:  {".", "exports..", "exports"},
				25:  {"worker", "exports...worker", "exports.."},
				41:  {"./compiler", "exports../compiler", "exports"},
				43:  {"require", "exports../compiler.require", "exports../compiler"},
				68:  {"./internal/flags/tracing", "exports../internal/flags/tracing", "exports"},
				113: {"imports", "imports", ""},
				116: {"#compiler", "imports.#compiler", "imports"},
				140: {"scripts", "scripts", ""},
			},
		},
	} {
		if root := os.Getenv(tc.env); root != "" {
			t.Run(tc.name, func(t *testing.T) {
				path := filepath.Join(root, filepath.FromSlash(tc.rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("bounded original manifest unavailable: %v %v", info, err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				source := string(data)
				if strings.Count(source, tc.anchor) != 1 {
					t.Fatal("original manifest source anchor changed")
				}
				check(t, source, tc.wants)
			})
		}
	}
}
