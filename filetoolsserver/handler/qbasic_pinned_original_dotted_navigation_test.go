package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The complete pinned original is an independent source oracle, not generated
// from the current handler response.
func TestQBasicPinnedOriginalDottedNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_QBASIC_OUTLINE_ROOT")
	if root == "" {
		t.Skip("complete QBasic original not configured")
	}
	path := filepath.Join(root, "advgen-AG.QBI")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("missing pinned original: %v", err)
	}
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "qbasic", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 200,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated || len(outlined.Symbols) != 116 {
		t.Fatalf("complete original QBasic outline: err=%v failed=%+v count=%d", err, failed, len(outlined.Symbols))
	}
	var id string
	for _, symbol := range outlined.Symbols {
		if symbol.Name == "C.IFLOC" {
			if symbol.QualifiedName != "C.IFLOC" || symbol.NameRange.Start.Line != 140 ||
				symbol.NameRange.End.Column-symbol.NameRange.Start.Column != len("C.IFLOC") {
				t.Fatalf("source-authored dotted constant changed: %+v", symbol)
			}
			id = symbol.ID
		}
	}
	if id == "" || len(outlined.Files) != 1 || outlined.Files[0].SourceFingerprint == "" {
		t.Fatal("missing source-authored dotted symbol or source fingerprint")
	}
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "qbasic", Encoding: "utf-8",
		Query: "C.IFLOC", Match: "exact", MaxFiles: 1, MaxSymbols: 10,
	})
	if err != nil || failed != nil || !found.CoverageComplete || len(found.Symbols) != 1 ||
		found.Symbols[0].Name != "C.IFLOC" {
		t.Fatalf("exact dotted lookup: err=%v failed=%+v matches=%+v", err, failed, found.Symbols)
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: id,
		SourceFingerprint: outlined.Files[0].SourceFingerprint,
		Language:          "qbasic", Encoding: "utf-8", MaxBytes: 4096,
	})
	if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "CONST C.IFLOC") {
		t.Fatalf("fingerprint-verified source excerpt: err=%v failed=%+v show=%+v", err, failed, shown.Show)
	}
	failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Language: "qbasic", Encoding: "utf-8",
		Query: "C.IFLOC", Match: "exact", Mode: "structural", MaxResults: 10,
	})
	if err != nil || failed != nil || searched.Search == nil || len(searched.Search.Matches) != 1 ||
		searched.Search.Matches[0].Name != "C.IFLOC" {
		t.Fatalf("original structural search: err=%v failed=%+v search=%+v", err, failed, searched.Search)
	}
}
