package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Full pinned upstream originals are read; no R code is executed.
func TestRPinnedWholeOriginalSymbolNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_R_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two immutable original R sources are not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file, name     string
		count, endLine int
	}{
		{"cli-ansi.R", "make_ansi_style", 20, 298},
		{"dplyr-rename.R", "rename_with.data.frame", 4, 112},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded complete R original unavailable: %v %v", info, err)
			}
			failed, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "r", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 4096,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 || len(outline.Symbols) != tc.count {
				t.Fatalf("full R outline: err=%v failed=%+v count=%d want=%d", err, failed, len(outline.Symbols), tc.count)
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" {
				t.Fatal("original fingerprint missing")
			}
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "r", Encoding: "utf-8",
				Query: tc.name, Match: "exact", MaxFiles: 1, MaxSymbols: 4096,
			})
			if err != nil || failed != nil || len(found.Symbols) != 1 ||
				found.Symbols[0].Name != tc.name ||
				found.Symbols[0].DeclarationRange.End.Line != tc.endLine ||
				found.Symbols[0].DeclarationRange.End.Column != 2 {
				t.Fatalf("full R body range: err=%v failed=%+v found=%+v", err, failed, found.Symbols)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "r", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fingerprint, MaxBytes: 16384,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.name+" <- function(") ||
				!strings.Contains(shown.Show.Text, "}") {
				t.Fatalf("R fingerprint-confirmed source body: err=%v failed=%+v", err, failed)
			}
			failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "r", Encoding: "utf-8", Query: tc.name, Match: "exact",
				MaxFiles: 1, MaxResults: 4096,
			})
			if err != nil || failed != nil || searched.Search == nil ||
				len(searched.Search.Matches) != 1 {
				t.Fatalf("R structural search: err=%v failed=%+v", err, failed)
			}
		})
	}
}
