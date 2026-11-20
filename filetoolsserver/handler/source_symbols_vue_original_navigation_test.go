package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type originalVueTarget struct {
	file, name, excerpt string
	line                int
}

type originalVueEvidence struct {
	file, text string
}

type originalVueTask struct {
	project, query, target, unrelated string
	files                             []string
	evidence                          []originalVueEvidence
	targets                           []originalVueTarget
}

// Original Vue projects are inspected as read-only source; no npm commands run.
func TestSourceSymbolsOriginalVueTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Vue corpus unavailable")
	}
	cases := []originalVueTask{
		{
			project:   "vue-panjiachen-vue-element-admin",
			query:     "handleSizeChange",
			target:    "src/components/Pagination/index.vue",
			unrelated: "src/components/BackToTop/index.vue",
			files: []string{
				"src/components/Pagination/index.vue",
				"src/views/table/complex-table.vue",
				"src/components/BackToTop/index.vue",
				"src/components/HeaderSearch/index.vue",
			},
			evidence: []originalVueEvidence{
				{"src/components/Pagination/index.vue", "@size-change=\"handleSizeChange\""},
				{"src/components/Pagination/index.vue", "@current-change=\"handleCurrentChange\""},
				{"src/components/Pagination/index.vue", "default: 20"},
				{"src/components/Pagination/index.vue", "this.$emit('pagination', { page: this.currentPage, limit: val })"},
				{"src/views/table/complex-table.vue", "@pagination=\"getList\""},
				{"src/views/table/complex-table.vue", ":limit.sync=\"listQuery.limit\""},
				{"src/components/BackToTop/index.vue", "methods: {"},
			},
			targets: []originalVueTarget{
				{"src/components/Pagination/index.vue", "handleSizeChange", "this.$emit('pagination', { page: this.currentPage, limit: val })", 77},
				{"src/components/Pagination/index.vue", "handleCurrentChange", "this.$emit('pagination', { page: val, limit: this.pageSize })", 83},
			},
		},
		{
			project:   "vue-elementplus",
			query:     "handleChange",
			target:    "packages/components/pagination/src/components/sizes.vue",
			unrelated: "packages/components/pagination/src/components/total.vue",
			files: []string{
				"packages/components/pagination/src/components/sizes.vue",
				"packages/components/pagination/src/components/jumper.vue",
				"packages/components/pagination/src/components/total.vue",
				"packages/components/pagination/src/components/prev.vue",
			},
			evidence: []originalVueEvidence{
				{"packages/components/pagination/src/components/sizes.vue", "@change=\"handleChange\""},
				{"packages/components/pagination/src/components/sizes.vue", "pagination.handleSizeChange?.(Number(val))"},
				{"packages/components/pagination/src/components/sizes.ts", "default: () => mutable([10, 20, 30, 40, 50, 100] as const)"},
				{"packages/components/pagination/src/components/jumper.vue", "changeEvent?.(val)"},
				{"packages/components/pagination/__tests__/pagination.test.tsx", "describe('Pagination', () => {"},
			},
			targets: []originalVueTarget{
				{"packages/components/pagination/src/components/sizes.vue", "handleChange", "pagination.handleSizeChange?.(Number(val))", 65},
				{"packages/components/pagination/src/components/jumper.vue", "handleInput", "userInput.value = val ? +val : ''", 41},
				{"packages/components/pagination/src/components/jumper.vue", "handleChange", "changeEvent?.(val)", 45},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				p := filepath.Join(root, filepath.FromSlash(rel))
				requireOriginalVueTextFile(t, p)
				paths = append(paths, p)
			}
			for _, oracle := range tc.evidence {
				p := filepath.Join(root, filepath.FromSlash(oracle.file))
				requireOriginalVueTextFile(t, p)
				body, err := os.ReadFile(p)
				if err != nil || !strings.Contains(string(body), oracle.text) {
					t.Fatalf("independent original source %s %q err=%v", oracle.file, oracle.text, err)
				}
			}
			h := NewHandler([]string{root})
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "vue", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("compact digest err=%v failed=%+v result=%+v", err, failed, digest)
			}
			encoded, err := json.Marshal(digest)
			if err != nil || len(encoded) > 8192 {
				t.Fatalf("compact digest bytes=%d err=%v", len(encoded), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(encoded), digest.CoverageComplete)
			for _, want := range tc.targets {
				p := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{p}, Query: want.name, Match: "exact",
					Language: "vue", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 24,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("find %s err=%v failed=%+v result=%+v", want.name, err, failed, found)
				}
				id := ""
				for _, symbol := range found.Symbols {
					if symbol.Name == want.name && symbol.NameRange.Start.Line == want.line {
						id = symbol.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("original declaration %s:%d %s missing", want.file, want.line, want.name)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: p, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint,
					Language: "vue", Encoding: "utf-8", MaxBytes: 16384,
				})
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.excerpt) {
					t.Fatalf("fingerprint show %s err=%v failed=%+v show=%+v", want.name, err, failed, shown.Show)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: tc.query, Match: "exact", Mode: "structural",
				Language: "vue", Encoding: "utf-8", MaxResults: 25,
			})
			if err != nil || failed != nil || search.Search == nil {
				t.Fatalf("structural search %s err=%v failed=%+v result=%+v", tc.query, err, failed, search.Search)
			}
			primary := filepath.Join(root, filepath.FromSlash(tc.target))
			seen := false
			for _, match := range search.Search.Matches {
				if match.Name == tc.query && filepath.Clean(match.Path) == primary {
					seen = true
				}
			}
			if !seen {
				t.Fatalf("structural search omitted primary %s: %+v", tc.target, search.Search.Matches)
			}
			failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))},
				Query: tc.query, Match: "exact", Language: "vue", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 15,
			})
			if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated source defines %s err=%v failed=%+v symbols=%+v", tc.query, err, failed, negative.Symbols)
			}
		})
	}
}

func requireOriginalVueTextFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("original file %s unavailable or not regular: %v", path, err)
	}
}
