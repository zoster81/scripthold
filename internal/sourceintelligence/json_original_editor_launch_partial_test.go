package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Editor launch files are not uniformly strict JSON. Comments prevent
// complete JSON navigation; objects inside arrays are not given invented owners.
func TestJSONOriginalEditorLaunchFilesKeepIncompleteCoverage(t *testing.T) {
	check := func(t *testing.T, source, diagnostic string, expected map[int]string) {
		t.Helper()
		result, err := (JSONAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("launch.json", source), testAnalyzeOptions(true, 128))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.CoverageComplete || result.Analysis.Truncated {
			t.Fatalf("unsupported JSON source was incorrectly complete: %+v", result.Analysis)
		}
		foundDiagnostic := false
		for _, d := range result.Analysis.Diagnostics {
			if d.Code == diagnostic {
				foundDiagnostic = true
			}
		}
		if !foundDiagnostic {
			t.Fatalf("missing %s diagnostic for unsupported launch configuration: %+v",
				diagnostic, result.Analysis.Diagnostics)
		}
		seen := map[int]bool{}
		for _, sym := range result.Analysis.Symbols {
			if sym.Kind != SymbolKindKey || sym.Name == "name" ||
				sym.Name == "program" || sym.Name == "request" {
				t.Fatalf("unmodeled array or JSONC comment invented a key: %+v", sym)
			}
			if want, ok := expected[sym.NameRange.Start.Line]; ok {
				if sym.Name != want || sym.ParentQualifiedName != "" {
					t.Fatalf("unexpected root launch key: %+v, want %q", sym, want)
				}
				seen[sym.NameRange.Start.Line] = true
			}
		}
		if len(seen) != len(expected) || len(result.Analysis.Symbols) != len(expected) {
			t.Fatalf("incorrect partial launch keys: seen=%v expected=%v symbols=%+v",
				seen, expected, result.Analysis.Symbols)
		}
	}
	for _, tc := range []struct{ name, eol string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run("commented/"+tc.name, func(t *testing.T) {
			source := "{\n  // editor documentation\n  \"version\": \"0.2.0\"\n}\n"
			check(t, strings.ReplaceAll(source, "\n", tc.eol), "json-malformed", nil)
		})
		t.Run("array-objects/"+tc.name, func(t *testing.T) {
			source := "{\n  \"version\": \"0.2.0\",\n  \"configurations\": [{\"name\": \"Run\", \"program\": \"script.js\"}]\n}\n"
			check(t, strings.ReplaceAll(source, "\n", tc.eol),
				"json-unmodeled-array-objects", map[int]string{2: "version", 3: "configurations"})
		})
	}
	for _, tc := range []struct {
		name, env, anchor, diagnostic string
		expected                      map[int]string
	}{
		{
			name: "vue-original-jsonc", env: "SCRIPTHOLD_REAL_JSON_VUE_ROOT",
			anchor:     "// Use IntelliSense to learn about possible attributes.",
			diagnostic: "json-malformed",
		},
		{
			name: "svelte-original-json-array", env: "SCRIPTHOLD_REAL_JSON_SVELTE_ROOT",
			anchor:     "\"name\": \"Run sandbox\"",
			diagnostic: "json-unmodeled-array-objects",
			expected:   map[int]string{2: "version", 3: "configurations"},
		},
	} {
		if root := os.Getenv(tc.env); root != "" {
			t.Run(tc.name, func(t *testing.T) {
				path := filepath.Join(root, ".vscode", "launch.json")
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("bounded original editor config unavailable: %v %v", info, err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				source := string(data)
				if strings.Count(source, tc.anchor) != 1 {
					t.Fatal("original editor source anchor changed")
				}
				check(t, source, tc.diagnostic, tc.expected)
			})
		}
	}
}
