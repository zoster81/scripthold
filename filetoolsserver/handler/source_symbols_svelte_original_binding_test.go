package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Real originals are opt-in external fixtures and never committed to Git.
func TestSvelteOriginalFinalBindingMCPNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_SVELTE_OUTLINE_ROOT")
	if root == "" {
		t.Skip("set SCRIPTHOLD_REAL_SVELTE_OUTLINE_ROOT for the complete upstream Svelte corpus")
	}
	for _, tc := range []struct {
		file  string
		count int
		name  string
		line  int
	}{
		{"skeleton-item-content.svelte", 8, "props", 13},
		{"shadcn-copy-code-button.svelte", 2, "ctx", 10},
		{"flowbite-accordion-item.svelte", 28, "contentClass", 25},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			whole, err := os.ReadFile(path)
			if err != nil || len(whole) == 0 {
				t.Fatalf("original Svelte component missing or empty: %v", err)
			}
			ctx := context.Background()
			h := NewHandler([]string{root})
			failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "svelte", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 100,
			})
			if err != nil || failed != nil || len(outline.Files) != 1 ||
				!outline.CoverageComplete || outline.Truncated || len(outline.Symbols) != tc.count {
				t.Fatalf("whole original detailed outline: err=%v failed=%+v outline=%+v", err, failed, outline)
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" {
				t.Fatal("missing complete original source fingerprint")
			}
			count := 0
			for _, sym := range outline.Symbols {
				if sym.Name != tc.name {
					continue
				}
				count++
				if sym.NameRange.Start.Line != tc.line || sym.ParentQualifiedName != "" ||
					sym.QualifiedName != tc.name || sym.RegionID == "" {
					t.Errorf("wrong original authored symbol identity: %+v", sym)
				}
			}
			if count != 1 {
				t.Errorf("expected exactly one %s at authored line %d, got %d", tc.name, tc.line, count)
			}
			failed, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "svelte",
				Encoding: "utf-8", MaxFiles: 1,
			})
			if err != nil || failed != nil || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint != fingerprint {
				t.Fatalf("original digest fingerprint mismatch: %v", err)
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "svelte",
				Encoding: "utf-8", Query: tc.name, Match: "exact",
				MaxFiles: 1, MaxSymbols: 16, FileDetails: "relevant",
			})
			if err != nil || failed != nil || len(found.Symbols) != 1 ||
				found.Symbols[0].NameRange.Start.Line != tc.line {
				t.Fatalf("original exact authored binding find failed: err=%v found=%+v", err, found)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "svelte", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fingerprint, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.name) {
				t.Fatalf("fingerprint-verified original source excerpt missing %s: err=%v", tc.name, err)
			}
			failed, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "svelte", Query: tc.name, Match: "exact",
				MaxFiles: 1, MaxResults: 16,
			})
			if err != nil || failed != nil || searched.Search == nil ||
				!searched.Coverage.CoverageComplete || len(searched.Search.Matches) != 1 {
				t.Fatalf("structural search of original binding failed: err=%v result=%+v", err, searched)
			}
		})
	}
}
