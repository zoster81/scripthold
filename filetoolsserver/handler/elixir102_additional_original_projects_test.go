package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type elixir102Target struct {
	file, name, owner, native, excerpt string
	line                               int
}

// Independent immutable Git originals were read before development handler
// queries. Cross-file calls and Mix settings are source facts, not execution.
func TestElixir102AdditionalOriginalProjectNavigation(t *testing.T) {
	mal := os.Getenv("SCRIPTHOLD_REAL_ELIXIR102_MAL_ROOT")
	samples := os.Getenv("SCRIPTHOLD_REAL_ELIXIR102_SAMPLES_ROOT")
	if mal == "" && samples == "" {
		t.Skip("additional original Elixir projects not configured")
	}
	if mal == "" || samples == "" {
		t.Fatal("both independent original Elixir project roots are required")
	}
	cases := []struct {
		name, root, rootSubdir, negativeFile, negative string
		code, sources                                  []string
		evidence                                       []struct{ file, text string }
		targets                                        []elixir102Target
	}{
		{
			name: "MAL Elixir lexical lookup and Mix entrypoint",
			root: mal, rootSubdir: filepath.Join("impls", "elixir"),
			code:         []string{"lib/mal/env.ex", "lib/mal/core.ex", "lib/mix/tasks/stepA_mal.ex", "mix.exs"},
			sources:      []string{"lib/mal/env.ex", "lib/mal/core.ex", "lib/mix/tasks/stepA_mal.ex", "mix.exs"},
			negativeFile: "lib/mal/env.ex", negative: "quick_sort",
			evidence: []struct{ file, text string }{
				{"lib/mal/env.ex", "map.outer && find(map.outer, key)"},
				{"lib/mal/core.ex", "def namespace do"},
				{"lib/mix/tasks/stepA_mal.ex", "env = Mal.Env.new()"},
				{"lib/mix/tasks/stepA_mal.ex", "Mal.Env.merge(env, Mal.Core.namespace)"},
				{"lib/mix/tasks/stepA_mal.ex", "let_env = Mal.Env.new(env)"},
				{"mix.exs", "default_task: \"stepA_mal\""},
				{"mix.exs", "main_module: Mix.Tasks.StepAMal"},
			},
			targets: []elixir102Target{
				{"lib/mal/env.ex", "find", "Mal.Env", "def", "def find(pid, key) do", 36},
				{"lib/mal/env.ex", "get", "Mal.Env", "def", "def get(pid, key) do", 54},
				{"lib/mal/core.ex", "namespace", "Mal.Core", "def", "def namespace do", 5},
				{"lib/mix/tasks/stepA_mal.ex", "run", "Mix.Tasks.StepAMal", "def", "def run(args) do", 10},
				{"lib/mix/tasks/stepA_mal.ex", "eval_list", "Mix.Tasks.StepAMal", "defp", "defp eval_list([{:symbol, \"let*\"}", 170},
				{"mix.exs", "escript", "Mal.Mixfile", "def", "def escript do", 15},
			},
		},
		{
			name: "Sample Programs alternative Elixir sort implementations",
			root: samples, rootSubdir: filepath.Join("archive", "e", "elixir"),
			code:         []string{"merge-sort.ex", "quick-sort.ex"},
			sources:      []string{"merge-sort.ex", "quick-sort.ex", "README.md"},
			negativeFile: "quick-sort.ex", negative: "merge",
			evidence: []struct{ file, text string }{
				{"merge-sort.ex", "merge(merge_sort(left), merge_sort(right))"},
				{"quick-sort.ex", "quick_sort(left) ++ [x] ++ quick_sort(right)"},
				{"README.md", "Merge Sort in Elixir"},
				{"README.md", "Quick Sort in Elixir"},
				{"README.md", "This page is auto-generated. DO NOT EDIT!"},
			},
			targets: []elixir102Target{
				{"merge-sort.ex", "merge_sort", "MergeSort", "def", "def merge_sort(list) do", 8},
				{"merge-sort.ex", "merge", "MergeSort", "defp", "defp merge([x | left], [y | right], acc) do", 17},
				{"merge-sort.ex", "parse_list", "MergeSort", "def", "def parse_list(str) do", 26},
				{"quick-sort.ex", "quick_sort", "QuickSort", "def", "def quick_sort([x | xs]) do", 8},
				{"quick-sort.ex", "parse_list", "QuickSort", "def", "def parse_list(str) do", 16},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := make(map[string]string, len(tc.sources))
			for _, rel := range tc.sources {
				path := filepath.Join(tc.root, tc.rootSubdir, filepath.FromSlash(rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("unsafe or oversized original %s: info=%v err=%v", rel, info, err)
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				content[rel] = string(raw)
			}
			for _, edge := range tc.evidence {
				if !strings.Contains(content[edge.file], edge.text) {
					t.Fatalf("independent original Elixir source oracle missing %s %q", edge.file, edge.text)
				}
			}
			for _, target := range tc.targets {
				lines := strings.Split(content[target.file], "\n")
				if target.line < 1 || target.line > len(lines) ||
					!strings.Contains(lines[target.line-1], target.native+" "+target.name) ||
					!strings.Contains(lines[target.line-1], target.excerpt) {
					t.Fatalf("Elixir original source declaration mismatch %s:%d %s", target.file, target.line, target.name)
				}
			}
			ctx, h := context.Background(), NewHandler([]string{tc.root})
			outlineIDs := make(map[string]map[string]bool)
			for _, rel := range tc.code {
				path := filepath.Join(tc.root, tc.rootSubdir, filepath.FromSlash(rel))
				_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "digest", Presentation: "compact", Paths: []string{path},
					Language: "elixir", MaxFiles: 1,
				})
				if err != nil || digest.Truncated || digest.FilesParsed != 1 || len(digest.OverviewFiles) != 1 {
					t.Fatalf("original Elixir digest unusable %s: err=%v result=%+v", rel, err, digest)
				}
				_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "outline", Presentation: "detailed", Paths: []string{path},
					Language: "elixir", MaxFiles: 1, MaxSymbols: 1024,
				})
				if err != nil || outline.Truncated || outline.FilesParsed != 1 || len(outline.Files) != 1 {
					t.Fatalf("original Elixir outline unusable %s: err=%v result=%+v", rel, err, outline)
				}
				ids := make(map[string]bool)
				for _, symbol := range outline.Symbols {
					ids[symbol.ID] = true
				}
				outlineIDs[rel] = ids
			}
			for _, target := range tc.targets {
				path := filepath.Join(tc.root, tc.rootSubdir, filepath.FromSlash(target.file))
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "elixir",
					Query: target.name, Match: "exact", MaxFiles: 1,
					MaxSymbols: 256, FileDetails: "relevant",
				})
				if err != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("original Elixir find failed %s:%d: err=%v result=%+v", target.file, target.line, err, found)
				}
				id := ""
				for _, sym := range found.Symbols {
					if sym.Path == path && sym.Name == target.name &&
						sym.NativeKind == target.native && sym.ParentQualifiedName == target.owner &&
						sym.NameRange.Start.Line == target.line {
						id = sym.ID
					}
				}
				if id == "" || !outlineIDs[target.file][id] {
					t.Fatalf("Elixir symbol missing or wrong owner/overload %s:%d %s: %+v", target.file, target.line, target.name, found.Symbols)
				}
				file := found.Files[0]
				if file.SourceFingerprint == "" || file.Encoding == "" {
					t.Fatalf("safe source selector absent: %+v", file)
				}
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "elixir", Encoding: file.Encoding,
					SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
				})
				if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target.excerpt) {
					t.Fatalf("Elixir fingerprint-bound source missing %s:%d: err=%v shown=%+v", target.file, target.line, err, shown.Show)
				}
				_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{path}, Language: "elixir",
					Query: target.name, Match: "exact", MaxFiles: 1, MaxResults: 256,
				})
				linked := false
				if query.Search != nil {
					for _, hit := range query.Search.Matches {
						if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint &&
							hit.Range.Start.Line == target.line {
							linked = true
						}
					}
				}
				if err != nil || query.Search == nil || query.Coverage.ResultsTruncated || !linked {
					t.Fatalf("Elixir structural search not linked %s:%d: err=%v query=%+v", target.file, target.line, err, query)
				}
			}
			_, absent, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(tc.root, tc.rootSubdir, filepath.FromSlash(tc.negativeFile))},
				Language: "elixir", Query: tc.negative, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || absent.Truncated || len(absent.Symbols) != 0 {
				t.Fatalf("fabricated unrelated Elixir declaration %s: err=%v symbols=%+v", tc.negative, err, absent.Symbols)
			}
			t.Logf("original Elixir navigation: %d files, %d positive declaration targets", len(tc.sources), len(tc.targets))
		})
	}
}
