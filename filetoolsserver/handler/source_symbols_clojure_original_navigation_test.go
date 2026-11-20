package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type originalClojureText struct{ file, text string }
type originalClojureTarget struct {
	file, name string
	line       int
}
type originalClojureCase struct {
	project, query, unrelated string
	files                     []string
	source                    []originalClojureText
	targets                   []originalClojureTarget
}

// Read-only original projects; no original project code is executed.
func TestSourceSymbolsOriginalClojureMaintenanceNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Clojure corpus unavailable")
	}
	for _, tc := range originalClojureCases() {
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
				Operation: "digest", Paths: paths, Language: "clojure", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("compact digest err=%v failed=%+v result=%+v", err, failed, digest)
			}
			b, err := json.Marshal(digest)
			if err != nil || len(b) > 8192 {
				t.Fatalf("compact bytes=%d err=%v", len(b), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(b), digest.CoverageComplete)
			checkOriginalClojureNavigation(t, h, root, paths, tc)
		})
	}
}

// Read-only, opt-in complete original files with source-first authored symbol counts.
func TestClojurePinnedCompleteOriginalDetailedOutline(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CLOJURE_OUTLINE_ROOT")
	if root == "" {
		t.Skip("set SCRIPTHOLD_REAL_CLOJURE_OUTLINE_ROOT for both pinned full originals")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file  string
		count int
	}{
		{"kde-clojure.clj", 17},
		{"ctags-metadata.clj", 6},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			original, err := os.ReadFile(path)
			if err != nil || len(original) == 0 {
				t.Fatalf("configured complete original missing or empty: %v", err)
			}
			failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "clojure", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || failed != nil || len(out.Files) != 1 ||
				out.Truncated || !out.CoverageComplete || len(out.Symbols) != tc.count {
				t.Fatalf("full original detailed outline: err=%v failed=%+v result=%+v", err, failed, out)
			}
			fp := out.Files[0].SourceFingerprint
			if fp == "" {
				t.Fatal("original source fingerprint unavailable")
			}
			if tc.file == "kde-clojure.clj" {
				apostrophe := 0
				for _, sym := range out.Symbols {
					if sym.NameRange.Start.Line == 43 && sym.Name == "=" {
						t.Errorf("expression under metadata invented direct binding: %+v", sym)
					}
					if sym.NameRange.Start.Line == 18 {
						apostrophe++
						if sym.Name != "fizz'" || sym.QualifiedName != "highlighting.fizz'" ||
							sym.NameRange.Start.Column != 6 || sym.NameRange.End.Column != 11 ||
							sym.Signature != "(def fizz'" {
							t.Errorf("source-authored apostrophe identity and span: %+v", sym)
						}
					}
				}
				if apostrophe != 1 {
					t.Fatalf("expected one declaration at authored line 18, got %d", apostrophe)
				}
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "clojure", Encoding: "utf-8",
					Query: "fizz'", Match: "exact", MaxFiles: 1, MaxSymbols: 12,
				})
				if err != nil || failed != nil || len(found.Symbols) != 1 ||
					found.Symbols[0].NameRange.Start.Line != 18 {
					t.Fatalf("exact source-authorized name lookup: err=%v failed=%+v found=%+v", err, failed, found)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "clojure", Encoding: "utf-8",
					SymbolID: found.Symbols[0].ID, SourceFingerprint: fp, MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil ||
					!strings.Contains(shown.Show.Text, "(def fizz'") {
					t.Fatalf("fingerprint-bound authored source excerpt: err=%v failed=%+v shown=%+v", err, failed, shown)
				}
				failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{path},
					Language: "clojure", Encoding: "utf-8", Query: "fizz'",
					Match: "exact", MaxFiles: 1, MaxResults: 12,
				})
				if err != nil || failed != nil || search.Search == nil ||
					len(search.Search.Matches) != 1 {
					t.Fatalf("source-intelligence structural find: err=%v failed=%+v result=%+v", err, failed, search)
				}
			}
		})
	}
}

func checkOriginalClojureNavigation(t *testing.T, h *Handler, root string, paths []string, tc originalClojureCase) {
	t.Helper()
	for _, want := range tc.targets {
		p := filepath.Join(root, filepath.FromSlash(want.file))
		failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{p}, Query: want.name, Match: "exact", Language: "clojure", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 25,
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
		failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "show", Path: p, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, Language: "clojure", Encoding: "utf-8", MaxBytes: 8192,
		})
		if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.name) {
			t.Fatalf("fingerprint show %s err=%v failed=%+v show=%+v", want.name, err, failed, shown.Show)
		}
	}
	failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: paths, Query: tc.query, Match: "exact", Mode: "structural", Language: "clojure", Encoding: "utf-8", MaxResults: 25,
	})
	if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
		t.Fatalf("structural search %s err=%v failed=%+v search=%+v", tc.query, err, failed, search.Search)
	}
	failed, neg, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))},
		Query: tc.query, Match: "exact", Language: "clojure", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 15,
	})
	if err != nil || failed != nil || neg.Truncated || len(neg.Symbols) != 0 {
		t.Fatalf("unrelated file falsely defines %s err=%v failed=%+v symbols=%+v", tc.query, err, failed, neg.Symbols)
	}
}
