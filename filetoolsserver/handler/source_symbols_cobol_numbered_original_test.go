package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The full upstream originals stay outside Git; incomplete opt-in fixtures fail.
func TestCOBOLNumberedOriginalMCPDeclarations(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_COBOL_NUMBERED_ROOT")
	if root == "" {
		t.Skip("full original COBOL corpus not configured")
	}
	for _, tc := range []struct {
		file      string
		want      int
		query     string
		queryLine int
	}{
		{"drujensen-fib.cbl", 5, "fibmain", 2},
		{"azac-cow.cbl", 15, "cow", 2},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			data, err := os.ReadFile(path)
			if err != nil || len(data) == 0 {
				t.Fatalf("configured original source missing/empty: %v", err)
			}
			ctx := context.Background()
			h := NewHandler([]string{root})
			failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "cobol", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || failed != nil || !outline.CoverageComplete || outline.Truncated ||
				len(outline.Files) != 1 || len(outline.Symbols) != tc.want {
				t.Fatalf("original detailed outline mismatch: err=%v failed=%v result=%+v", err, failed, outline)
			}
			fp := outline.Files[0].SourceFingerprint
			if fp == "" {
				t.Fatal("original source fingerprint missing")
			}
			if tc.file == "drujensen-fib.cbl" {
				expected := map[string]int{
					"fibmain": 2, "fibmain.WORKING-STORAGE": 5, "fib": 15,
					"fib.LOCAL-STORAGE": 18, "fib.LINKAGE": 21,
				}
				for _, sym := range outline.Symbols {
					line, ok := expected[sym.QualifiedName]
					if !ok || sym.NameRange.Start.Line != line {
						t.Errorf("original numbered COBOL identity: %+v", sym)
					}
				}
			}
			failed, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path},
				Language: "cobol", Encoding: "utf-8", MaxFiles: 1,
			})
			if err != nil || failed != nil || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint != fp {
				t.Fatalf("fingerprint-bound digest failed: %v", err)
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "cobol",
				Encoding: "utf-8", Query: tc.query, Match: "exact",
				MaxFiles: 1, MaxSymbols: 16, FileDetails: "relevant",
			})
			if err != nil || failed != nil || len(found.Symbols) != 1 ||
				found.Symbols[0].NameRange.Start.Line != tc.queryLine ||
				found.Symbols[0].ParentQualifiedName != "" {
				t.Fatalf("original top-level module find failed: err=%v result=%+v", err, found)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "cobol",
				Encoding: "utf-8", SymbolID: found.Symbols[0].ID,
				SourceFingerprint: fp, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(strings.ToLower(shown.Show.Text), "program-id.") {
				t.Fatalf("original verified source show failed: err=%v result=%+v", err, shown)
			}
			failed, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "cobol", Query: tc.query, Match: "exact",
				MaxFiles: 1, MaxResults: 16,
			})
			if err != nil || failed != nil || searched.Search == nil ||
				!searched.Coverage.CoverageComplete || len(searched.Search.Matches) != 1 {
				t.Fatalf("original structural search mismatch: err=%v result=%+v", err, searched)
			}
		})
	}
}
