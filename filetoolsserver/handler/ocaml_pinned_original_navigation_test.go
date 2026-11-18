package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Verify local MCP navigation on entire, immutable upstream OCaml originals.
func TestOCamlPinnedWholeOriginalNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_OCAML_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two original OCaml files are not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file, target              string
		count, endLine, endColumn int
	}{
		{"ocaml-compiler-scheduling.ml", "fundecl", 1, 30, 18},
		{"exercism-acronym.ml", "acronym", 3, 12, 25},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			file, err := os.Lstat(path)
			if err != nil || !file.Mode().IsRegular() || file.Size() > 1<<20 {
				t.Fatalf("original OCaml file unavailable or unsafe: %v %v", file, err)
			}
			failed, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "ocaml", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 1024,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Symbols) != tc.count || len(outline.Files) != 1 {
				t.Fatalf("complete original detailed outline: err=%v failed=%+v count=%d", err, failed, len(outline.Symbols))
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" {
				t.Fatal("missing original fingerprint")
			}
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "ocaml",
				Encoding: "utf-8", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxSymbols: 1024,
			})
			if err != nil || failed != nil || len(found.Symbols) != 1 ||
				found.Symbols[0].DeclarationRange.End.Line != tc.endLine ||
				found.Symbols[0].DeclarationRange.End.Column != tc.endColumn {
				t.Fatalf("entire original exact find: err=%v failed=%+v found=%+v", err, failed, found.Symbols)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "ocaml", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fingerprint, MaxBytes: 16384,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, "let "+tc.target+" ") ||
				(tc.target == "acronym" && !strings.Contains(shown.Show.Text, "|> String.of_char_list")) {
				t.Fatalf("fingerprint-confirmed OCaml full source: err=%v failed=%+v", err, failed)
			}
			failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "ocaml", Encoding: "utf-8", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 1024,
			})
			if err != nil || failed != nil || searched.Search == nil ||
				len(searched.Search.Matches) != 1 {
				t.Fatalf("original OCaml structural search: err=%v failed=%+v", err, failed)
			}
		})
	}
}
