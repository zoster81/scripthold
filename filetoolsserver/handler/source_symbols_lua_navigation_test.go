package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The task oracles are read from original projects; never execute them.
func TestSourceSymbolsOriginalLuaNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Lua corpus not configured")
	}
	for _, tc := range []struct {
		project, primary, query, excerpt, distractor string
		line                                         int
		files, evidence                              []string
	}{
		{
			project: "lua-nvim-lua-kickstart.nvim",
			files:   []string{"lua/kickstart/health.lua", "init.lua", "lua/kickstart/plugins/lint.lua"},
			primary: "lua/kickstart/health.lua", query: "check_external_reqs", line: 22,
			excerpt: "vim.fn.executable(exe)", distractor: "lua/kickstart/plugins/lint.lua",
			evidence: []string{"'git', 'make', 'unzip', 'rg'", "check_external_reqs()", "require 'kickstart.plugins.lint'", "BufWritePost"},
		},
		{
			project: "lua-nvim-telescope-telescope.nvim",
			files:   []string{"lua/telescope/sorters.lua", "lua/tests/automated/sorters_spec.lua", "lua/telescope/algos/fzy.lua"},
			primary: "lua/telescope/sorters.lua", query: "sorters.get_substr_matcher", line: 533,
			excerpt: "vim.o.smartcase", distractor: "lua/telescope/algos/fzy.lua",
			evidence: []string{"sorters.get_substr_matcher = function()", "case.expected_score", "function fzy.has_match"},
		},
	} {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			h := NewHandler([]string{root})
			paths := make([]string, 0, len(tc.files))
			all := ""
			for _, rel := range tc.files {
				p := filepath.Join(root, filepath.FromSlash(rel))
				f, err := os.Lstat(p)
				if err != nil || !f.Mode().IsRegular() {
					t.Fatalf("original %s: %v", rel, err)
				}
				data, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				all += string(data)
				paths = append(paths, p)
			}
			for _, e := range tc.evidence {
				if !strings.Contains(all, e) {
					t.Fatalf("original oracle changed %q", e)
				}
			}
			if !strings.Contains(all, tc.excerpt) {
				t.Fatalf("original body oracle changed %q", tc.excerpt)
			}
			failed, d, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "lua", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || d.Truncated || d.FilesParsed != len(paths) {
				t.Fatalf("digest err=%v tool=%+v result=%+v", err, failed, d)
			}
			t.Logf("files=%d symbols=%d reportedComplete=%v", d.FilesParsed, d.SymbolCount, d.CoverageComplete)
			p := filepath.Join(root, filepath.FromSlash(tc.primary))
			failed, f, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{p}, Query: tc.query, Match: "exact", Language: "lua", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 10,
			})
			if err != nil || failed != nil || len(f.Files) != 1 {
				t.Fatalf("find %s: %v %+v", tc.query, err, failed)
			}
			id := ""
			for _, s := range f.Symbols {
				if s.Name == tc.query && s.DeclarationRange.Start.Line == tc.line {
					id = s.ID
					break
				}
			}
			if id == "" {
				t.Fatalf("missing original declaration %s:%d: %+v", tc.query, tc.line, f.Symbols)
			}
			failed, show, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: p, SymbolID: id, SourceFingerprint: f.Files[0].SourceFingerprint, Language: "lua", Encoding: "utf-8", MaxBytes: 16384,
			})
			if err != nil || failed != nil || show.Show == nil || !strings.Contains(show.Show.Text, tc.query) {
				t.Fatalf("verified body %s: %v %+v", tc.query, err, failed)
			}
			failed, q, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: tc.query, Match: "exact", Mode: "structural", Language: "lua", Encoding: "utf-8", MaxResults: 10,
			})
			if err != nil || failed != nil || q.Search == nil || len(q.Search.Matches) == 0 {
				t.Fatalf("structural search %s: %v %+v", tc.query, err, failed)
			}
			failed, x, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.distractor))}, Query: tc.query, Match: "exact", Language: "lua", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 5,
			})
			if err != nil || failed != nil || len(x.Symbols) != 0 {
				t.Fatalf("false alternative %s: %v %+v", tc.query, err, x.Symbols)
			}
		})
	}
}
