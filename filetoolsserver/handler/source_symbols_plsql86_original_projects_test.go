package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Additional independent original projects exercise task-to-source navigation,
// not complete PL/SQL grammar or execution of either upstream project.
func TestPLSQL86IndependentOriginalProjectNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, owner, expectedExcerpt, related, negative string
		files                                                        []string
		targetLines                                                  []int
		relatedLines                                                 []int
	}{
		{
			name:   "utPLSQL between-string implementation and paired package tests",
			env:    "SCRIPTHOLD_REAL_PLSQL86_UT_ROOT",
			files:  []string{"betwnstr.sql", "test_betwnstr.pks", "test_betwnstr.pkb"},
			target: "betwnstr", owner: "", targetLines: []int{1},
			expectedExcerpt: "function betwnstr", related: "zero_start_position",
			relatedLines: []int{9, 8}, negative: "to_equal",
		},
		{
			name:   "Oracle Text cleanup package and unrelated section setup",
			env:    "SCRIPTHOLD_REAL_PLSQL86_ORACLE_ROOT",
			files:  []string{"sec3.sql", "demo.sql"},
			target: "cleanup_sections", owner: "sectionutils", targetLines: []int{2, 7},
			expectedExcerpt: "function cleanup_sections(", related: "sectionutils",
			relatedLines: []int{1}, negative: "add_field_section",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := os.Getenv(tc.env)
			if root == "" {
				t.Skip("original pinned project sources not configured")
			}
			paths := make([]string, 0, len(tc.files))
			for _, name := range tc.files {
				path := filepath.Join(root, name)
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("cannot independently inspect original %s: %v", name, err)
				}
				if len(raw) == 0 {
					t.Fatalf("empty original source %s", name)
				}
				paths = append(paths, path)
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "plsql",
				Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != len(paths) ||
				len(digest.OverviewFiles) != len(paths) {
				t.Fatalf("project digest failed: err=%v files=%d coverage=%v digests=%d",
					err, digest.FilesParsed, digest.CoverageComplete, len(digest.OverviewFiles))
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: paths, Language: "plsql",
				Presentation: "detailed", MaxFiles: len(paths), MaxSymbols: 128,
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated {
				t.Fatalf("project outline failed: err=%v coverage=%v truncated=%v",
					err, outline.CoverageComplete, outline.Truncated)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: paths, Language: "plsql",
				Query: tc.target, Match: "exact", FileDetails: "relevant",
				MaxFiles: len(paths), MaxSymbols: 32,
			})
			if err != nil || !found.CoverageComplete || found.Truncated ||
				len(found.Symbols) != len(tc.targetLines) {
				t.Fatalf("wrong original edit target: err=%v coverage=%v matches=%+v",
					err, found.CoverageComplete, found.Symbols)
			}
			files := make(map[string]SourceSymbolsFile, len(found.Files))
			for _, f := range found.Files {
				files[filepath.Clean(f.Path)] = f
			}
			seen := map[int]bool{}
			for _, sym := range found.Symbols {
				line := sym.NameRange.Start.Line
				if sym.Name != tc.target || sym.ParentQualifiedName != tc.owner ||
					sym.ID == "" || !strings.HasSuffix(strings.ToLower(sym.Path), ".sql") {
					t.Fatalf("fabricated or incorrectly owned declaration: %+v", sym)
				}
				seen[line] = true
				file, ok := files[filepath.Clean(sym.Path)]
				if !ok || file.SourceFingerprint == "" || file.Encoding == "" {
					t.Fatalf("missing exact source identity for %+v: %+v", sym, found.Files)
				}
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: sym.Path, Language: "plsql",
					Encoding: file.Encoding, SymbolID: sym.ID,
					SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
				})
				if err != nil || shown.Show == nil ||
					!strings.Contains(strings.ToLower(shown.Show.Text), strings.ToLower(tc.expectedExcerpt)) {
					t.Fatalf("missing fingerprint-verified source excerpt: err=%v show=%+v", err, shown.Show)
				}
			}
			for _, line := range tc.targetLines {
				if !seen[line] {
					t.Fatalf("original source declaration line %d absent: %+v", line, found.Symbols)
				}
			}
			_, near, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: paths, Language: "plsql",
				Query: tc.related, Match: "exact", MaxFiles: len(paths), MaxSymbols: 32,
			})
			if err != nil || !near.CoverageComplete || len(near.Symbols) != len(tc.relatedLines) {
				t.Fatalf("related original code not navigable: err=%v symbols=%+v", err, near.Symbols)
			}
			relatedSeen := map[int]bool{}
			for _, sym := range near.Symbols {
				relatedSeen[sym.NameRange.Start.Line] = true
			}
			for _, line := range tc.relatedLines {
				if !relatedSeen[line] {
					t.Fatalf("related original declaration at %d missing", line)
				}
			}
			_, irrelevant, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: paths, Language: "plsql",
				Query: tc.negative, Match: "exact", MaxFiles: len(paths), MaxSymbols: 32,
			})
			if err != nil || !irrelevant.CoverageComplete || len(irrelevant.Symbols) != 0 {
				t.Fatalf("call-only distractor projected as declaration: err=%v symbols=%+v", err, irrelevant.Symbols)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: paths,
				Language: "plsql", Query: tc.target, Match: "exact",
				MaxFiles: len(paths), MaxResults: 32,
			})
			if err != nil || search.Search == nil || !search.Coverage.CoverageComplete ||
				search.Coverage.ResultsTruncated || len(search.Search.Matches) != len(tc.targetLines) {
				t.Fatalf("structural original-source search mismatch: err=%v response=%+v", err, search)
			}
			t.Logf("verified project navigation across %d complete original reads; %d target declarations",
				len(paths), len(found.Symbols))
		})
	}
}
