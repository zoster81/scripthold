package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type originalCSource struct{ file, text string }
type originalCTarget struct {
	file, name, excerpt string
	line                int
}
type originalCScenario struct {
	project, query, unrelated string
	files                     []string
	oracle                    []originalCSource
	targets                   []originalCTarget
}

// The original source is only read to establish an independent oracle.
func TestSourceSymbolsOriginalCMaintenanceNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original C corpus unavailable")
	}
	for _, tc := range originalCScenarios() {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				p := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(p)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original path %s: %v", rel, err)
				}
				paths = append(paths, p)
			}
			for _, e := range tc.oracle {
				p := filepath.Join(root, filepath.FromSlash(e.file))
				b, err := os.ReadFile(p)
				if err != nil || !strings.Contains(string(b), e.text) {
					t.Fatalf("original oracle %s %q: %v", e.file, e.text, err)
				}
			}
			h := NewHandler([]string{root})
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "c", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("C digest err=%v failed=%+v result=%+v", err, failed, digest)
			}
			b, err := json.Marshal(digest)
			if err != nil || len(b) > 8192 {
				t.Fatalf("compact response bytes=%d err=%v", len(b), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(b), digest.CoverageComplete)
			checkCOriginalTargets(t, h, root, tc)
		})
	}
}

func checkCOriginalTargets(t *testing.T, h *Handler, root string, tc originalCScenario) {
	t.Helper()
	paths := make([]string, 0, len(tc.files))
	for _, rel := range tc.files {
		paths = append(paths, filepath.Join(root, filepath.FromSlash(rel)))
	}
	for _, want := range tc.targets {
		p := filepath.Join(root, filepath.FromSlash(want.file))
		failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{p}, Query: want.name, Match: "exact", Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 26,
		})
		if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
			t.Fatalf("C find %s: err=%v failed=%+v found=%+v", want.name, err, failed, found)
		}
		var id string
		for _, sym := range found.Symbols {
			if sym.Name == want.name && sym.NameRange.Start.Line == want.line {
				id = sym.ID
				break
			}
		}
		if id == "" {
			t.Fatalf("missing C declaration %s:%d %s found=%+v", want.file, want.line, want.name, found.Symbols)
		}
		if want.excerpt != "" {
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: p, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, Language: "c", Encoding: "utf-8", MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.excerpt) {
				t.Fatalf("fingerprint show %s: err=%v failed=%+v show=%+v", want.name, err, failed, shown.Show)
			}
		}
	}
	failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: paths, Query: tc.query, Match: "exact", Mode: "structural", Language: "c", Encoding: "utf-8", MaxResults: 30,
	})
	if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
		t.Fatalf("C structural search %s: err=%v failed=%+v search=%+v", tc.query, err, failed, search.Search)
	}
	failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))},
		Query: tc.query, Match: "exact", Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
	})
	if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
		t.Fatalf("unrelated source falsely defines %s: err=%v failed=%+v symbols=%+v", tc.query, err, failed, negative.Symbols)
	}
}
