package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Independent GitHub Actions workflows expose exact YAML key occurrences,
// while sequence-item identities remain deliberately unmodeled.
func TestYAMLOriginalWorkflowMatrixKeysArePartialButNavigable(t *testing.T) {
	check := func(t *testing.T, source string, want map[int]string) {
		t.Helper()
		result, err := (YAMLAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("ci.yml", source), testAnalyzeOptions(true, 512))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.CoverageComplete || result.Analysis.Truncated {
			t.Fatalf("workflow sequences cannot claim complete structural ownership: %+v", result.Analysis)
		}
		seen := make(map[int]bool, len(want))
		for _, symbol := range result.Analysis.Symbols {
			if symbol.Kind != SymbolKindKey || strings.Contains(symbol.Name, "fabricated") {
				t.Fatalf("workflow produced an unproven YAML key: %+v", symbol)
			}
			if expected, ok := want[symbol.NameRange.Start.Line]; ok && symbol.QualifiedName == expected {
				seen[symbol.NameRange.Start.Line] = true
			}
		}
		if len(seen) != len(want) {
			t.Fatalf("workflow YAML authored keys missing: seen=%v expected=%v", seen, want)
		}
		foundPartial := false
		for _, diagnostic := range result.Analysis.Diagnostics {
			if diagnostic.Code == "yaml-unmodeled-sequence" {
				foundPartial = true
			}
		}
		if !foundPartial {
			t.Fatal("unmodeled workflow sequences omitted their coverage warning")
		}
	}
	fixture := "jobs:\n  test:\n    strategy:\n      matrix:\n        include:\n        - build: pinned\n          os: ubuntu\n        - build: stable\n          os: windows\n    steps:\n    - run: |\n        echo 'fabricated: not-a-key'\n    timeout-minutes: 10\n"
	want := map[int]string{
		1: "jobs", 2: "jobs.test", 5: "jobs.test.strategy.matrix.include",
		6: "jobs.test.strategy.matrix.include.build", 7: "jobs.test.strategy.matrix.include.os",
		8: "jobs.test.strategy.matrix.include.build", 9: "jobs.test.strategy.matrix.include.os",
		11: "jobs.test.steps.run", 13: "jobs.test.timeout-minutes",
	}
	for _, tc := range []struct{ name, eol string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			check(t, strings.ReplaceAll(fixture, "\n", tc.eol), want)
		})
	}
	for _, tc := range []struct {
		env, file, anchor string
		want              map[int]string
	}{
		{
			env: "SCRIPTHOLD_REAL_YAML_ZOXIDE_ROOT", file: "ci.yml",
			anchor: "    - name: Run lints + tests",
			want: map[int]string{
				16: "jobs", 17: "jobs.ci", 21: "jobs.ci.strategy.matrix",
				23: "jobs.ci.steps", 57: "jobs.ci.steps.run",
			},
		},
		{
			env: "SCRIPTHOLD_REAL_YAML_RIPGREP_ROOT", file: "ci.yml",
			anchor: "        - build: stable-riscv64",
			want: map[int]string{
				30: "jobs", 31: "jobs.test", 53: "jobs.test.strategy.matrix.include",
				54:  "jobs.test.strategy.matrix.include.build",
				57:  "jobs.test.strategy.matrix.include.build",
				207: "jobs.wasm", 221: "jobs.rustfmt",
				234: "jobs.docs", 248: "jobs.fuzz_testing",
			},
		},
	} {
		if root := os.Getenv(tc.env); root != "" {
			t.Run(tc.env, func(t *testing.T) {
				path := filepath.Join(root, tc.file)
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("original workflow unavailable or oversized: %v %v", info, err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				source := string(data)
				if strings.Count(source, tc.anchor) != 1 {
					t.Fatal("original YAML workflow source anchor changed")
				}
				check(t, source, tc.want)
			})
		}
	}
}
