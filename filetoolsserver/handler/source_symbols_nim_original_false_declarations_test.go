package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Complete upstream Nim source files are external opt-in fixtures only.
func TestNimOriginalNoPhantomMCPNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_NIM_OUTLINE_ROOT")
	if root == "" {
		t.Skip("set SCRIPTHOLD_REAL_NIM_OUTLINE_ROOT for the two pinned original Nim files")
	}
	for _, tc := range []struct {
		file  string
		want  int
		query string
		line  int
	}{
		{"nim-new-dmodule.nim", 5, "MinValue", 5},
		{"ws-echo.nim", 4, "cb", 3},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			raw, err := os.ReadFile(path)
			if err != nil || len(raw) == 0 {
				t.Fatalf("missing/empty configured upstream original: %v", err)
			}
			ctx := context.Background()
			h := NewHandler([]string{root})
			failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "nim", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || len(outline.Files) != 1 ||
				outline.Truncated || len(outline.Symbols) != tc.want {
				t.Fatalf("original detailed Nim outline: err=%v failed=%+v output=%+v", err, failed, outline)
			}
			fp := outline.Files[0].SourceFingerprint
			if fp == "" {
				t.Fatal("original source fingerprint missing")
			}
			for _, sym := range outline.Symbols {
				if sym.Name == "else" || sym.Name == "a" {
					t.Errorf("invented field or parameter-derived callable: %+v", sym)
				}
			}
			failed, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path},
				Language: "nim", Encoding: "utf-8", MaxFiles: 1,
			})
			if err != nil || failed != nil || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint != fp {
				t.Fatalf("original digest/fingerprint mismatch: %v", err)
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "nim",
				Encoding: "utf-8", Query: tc.query, Match: "exact",
				MaxFiles: 1, MaxSymbols: 16, FileDetails: "relevant",
			})
			if err != nil || failed != nil || len(found.Symbols) != 1 ||
				found.Symbols[0].NameRange.Start.Line != tc.line {
				t.Fatalf("original exact Nim module/callable find: err=%v result=%+v", err, found)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "nim", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fp, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.query) {
				t.Fatalf("original source-authored show failed: err=%v result=%+v", err, shown)
			}
			failed, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "nim", Query: tc.query, Match: "exact",
				MaxFiles: 1, MaxResults: 16,
			})
			if err != nil || failed != nil || search.Search == nil ||
				len(search.Search.Matches) != 1 {
				t.Fatalf("original structural Nim search mismatch: err=%v result=%+v", err, search)
			}
		})
	}
}
