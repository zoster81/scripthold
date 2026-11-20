package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The entire upstream originals are optional, read-only external fixtures.
func TestJuliaOriginalScopeAndCompactMCPNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JULIA_OUTLINE_ROOT")
	if root == "" {
		t.Skip("set SCRIPTHOLD_REAL_JULIA_OUTLINE_ROOT for both complete Julia originals")
	}
	for _, tc := range []struct {
		file          string
		count         int
		name          string
		line, endLine int
		owner         string
	}{
		{"julia-compiler-dev-tools.jl", 14, "lookup_method_instance", 44, 44, "CompilerDevTools"},
		{"exercism-annalyn-exemplar.jl", 4, "can_free_prisoner", 27, 28, ""},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			raw, err := os.ReadFile(path)
			if err != nil || len(raw) == 0 {
				t.Fatalf("configured full original missing/empty: %v", err)
			}
			h := NewHandler([]string{root})
			ctx := context.Background()
			failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "julia", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 100,
			})
			if err != nil || failed != nil || len(outline.Files) != 1 || outline.Truncated ||
				!outline.CoverageComplete || len(outline.Symbols) != tc.count {
				t.Fatalf("whole original detailed Julia outline: err=%v failed=%+v result=%+v", err, failed, outline)
			}
			fp := outline.Files[0].SourceFingerprint
			if fp == "" {
				t.Fatal("missing full source fingerprint")
			}
			foundCount := 0
			for _, sym := range outline.Symbols {
				if sym.Name == tc.name && sym.NameRange.Start.Line == tc.line {
					foundCount++
					if sym.ParentQualifiedName != tc.owner || sym.DeclarationRange.End.Line != tc.endLine {
						t.Errorf("source owner or continuation wrong: %+v", sym)
					}
				}
			}
			if foundCount != 1 {
				t.Fatalf("authored %s found %d times", tc.name, foundCount)
			}
			failed, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "julia", Encoding: "utf-8",
				MaxFiles: 1,
			})
			if err != nil || failed != nil || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint != fp {
				t.Fatalf("original digest fingerprint mismatch: %v", err)
			}
			failed, match, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "julia", Encoding: "utf-8",
				Query: tc.name, Match: "exact", MaxFiles: 1, MaxSymbols: 16, FileDetails: "relevant",
			})
			if err != nil || failed != nil || len(match.Symbols) != 1 ||
				match.Symbols[0].NameRange.Start.Line != tc.line ||
				match.Symbols[0].ParentQualifiedName != tc.owner {
				t.Fatalf("original exact find mismatch: err=%v result=%+v", err, match)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "julia", Encoding: "utf-8",
				SymbolID: match.Symbols[0].ID, SourceFingerprint: fp, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.name) {
				t.Fatalf("original source fingerprint-bound show mismatch: err=%v result=%+v", err, shown)
			}
			failed, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "julia", Query: tc.name, Match: "exact", MaxFiles: 1, MaxResults: 16,
			})
			if err != nil || failed != nil || searched.Search == nil ||
				len(searched.Search.Matches) != 1 {
				t.Fatalf("original structural search mismatch: err=%v result=%+v", err, searched)
			}
		})
	}
}
