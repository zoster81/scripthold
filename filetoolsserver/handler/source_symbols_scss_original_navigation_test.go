package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type originalSCSSText struct{ file, text string }
type originalSCSSTarget struct {
	file, name string
	line       int
}
type originalSCSSCase struct {
	project, query, unrelated string
	files                     []string
	source                    []originalSCSSText
	targets                   []originalSCSSTarget
}

// Read-only original projects; no original project code is executed.
func TestSourceSymbolsOriginalSCSSMaintenanceNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original SCSS corpus unavailable")
	}
	for _, tc := range originalSCSSCases() {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				p := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(p)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original file %s: %v", rel, err)
				}
				paths = append(paths, p)
			}
			for _, e := range tc.source {
				p := filepath.Join(root, filepath.FromSlash(e.file))
				info, err := os.Lstat(p)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original oracle %s: %v", e.file, err)
				}
				b, err := os.ReadFile(p)
				if err != nil || !strings.Contains(string(b), e.text) {
					t.Fatalf("source oracle %s %q err=%v", e.file, e.text, err)
				}
			}
			h := NewHandler([]string{root})
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "scss", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("compact digest err=%v failed=%+v result=%+v", err, failed, digest)
			}
			b, err := json.Marshal(digest)
			if err != nil || len(b) > 8192 {
				t.Fatalf("compact bytes=%d err=%v", len(b), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(b), digest.CoverageComplete)
			checkOriginalSCSSNavigation(t, h, root, paths, tc)
		})
	}
}

func checkOriginalSCSSNavigation(t *testing.T, h *Handler, root string, paths []string, tc originalSCSSCase) {
	t.Helper()
	for _, want := range tc.targets {
		p := filepath.Join(root, filepath.FromSlash(want.file))
		failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{p}, Query: want.name, Match: "exact", Language: "scss", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 25,
		})
		if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
			t.Fatalf("exact find %s err=%v failed=%+v result=%+v", want.name, err, failed, found)
		}
		var id string
		for _, sym := range found.Symbols {
			if sym.Name == want.name && sym.NameRange.Start.Line == want.line {
				id = sym.ID
				break
			}
		}
		if id == "" {
			t.Fatalf("source-confirmed function %s:%d %s missing: %+v", want.file, want.line, want.name, found.Symbols)
		}
		maxShowBytes := 8192
		failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "show", Path: p, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, Language: "scss", Encoding: "utf-8", MaxBytes: maxShowBytes,
		})
		if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.name) {
			t.Fatalf("fingerprint show %s err=%v failed=%+v show=%+v", want.name, err, failed, shown.Show)
		}
	}
	failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: paths, Query: tc.query, Match: "exact", Mode: "structural", Language: "scss", Encoding: "utf-8", MaxResults: 25,
	})
	if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
		t.Fatalf("structural search %s err=%v failed=%+v search=%+v", tc.query, err, failed, search.Search)
	}
	failed, neg, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))},
		Query: tc.query, Match: "exact", Language: "scss", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 15,
	})
	if err != nil || failed != nil || neg.Truncated || len(neg.Symbols) != 0 {
		t.Fatalf("unrelated file falsely defines %s err=%v failed=%+v symbols=%+v", tc.query, err, failed, neg.Symbols)
	}
}
