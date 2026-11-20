package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test against two complete independent Ada originals through the local
// handler, without using a connected server or executing source programs.
func TestAdaPinnedWholeOriginalPackageNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_ADA_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two independent whole Ada original projects not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file, name, native, snippet string
		count, line, col            int
	}{
		{"ctags-input2.ads", "Random", "function", "function Random (G: Generator) return Real;", 5, 8, 12},
		{"sample-binary-search.adb", "Int_Vec", "package", "package Int_Vec is new\n     Ada.Containers.Vectors (Index_Type => Natural, Element_Type => Integer);", 6, 10, 12},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 1<<20 {
				t.Fatalf("entire original Ada fixture unavailable or unsafe: %v %v", info, err)
			}
			failed, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "ada", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 || len(outline.Symbols) != tc.count {
				t.Fatalf("entire Ada detailed outline: err=%v failed=%+v symbols=%+v", err, failed, outline.Symbols)
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" {
				t.Fatal("missing full original source fingerprint")
			}
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "ada", Encoding: "utf-8",
				Query: tc.name, Match: "exact", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || len(found.Symbols) != 1 {
				t.Fatalf("Ada original exact find: err=%v failed=%+v symbols=%+v", err, failed, found.Symbols)
			}
			sym := found.Symbols[0]
			if sym.Name != tc.name || sym.NativeKind != tc.native ||
				sym.NameRange.Start.Line != tc.line || sym.NameRange.Start.Column != tc.col ||
				sym.SignatureRange == nil || sym.Signature != tc.snippet {
				t.Fatalf("Ada authored full signature and identity: %+v", sym)
			}
			if tc.name == "Int_Vec" &&
				(sym.DeclarationRange.End.Line != 11 || sym.DeclarationRange.End.Column != 78 ||
					sym.SignatureRange.End != sym.DeclarationRange.End) {
				t.Fatalf("complete Ada package instantiation range: %+v", sym)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "ada", Encoding: "utf-8",
				SymbolID: sym.ID, SourceFingerprint: fingerprint, MaxBytes: 16384,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.snippet) {
				t.Fatalf("original Ada fingerprint-bound show: err=%v failed=%+v", err, failed)
			}
			failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "ada", Encoding: "utf-8", Query: tc.name, Match: "exact",
				MaxFiles: 1, MaxResults: 256,
			})
			if err != nil || failed != nil || searched.Search == nil || len(searched.Search.Matches) != 1 {
				t.Fatalf("original Ada structural search: err=%v failed=%+v", err, failed)
			}
		})
	}
}
