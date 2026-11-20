package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLuauPinnedWholeOriginalFalseTypesExcludedFromNavigation(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_LUAU_OUTLINE_FILE")
	if path == "" {
		t.Skip("pinned whole Luau original not configured")
	}
	root := filepath.Dir(path)
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "detailed",
		Language: "luau", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 128, IncludeSignatures: true,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated || len(outlined.Symbols) != 20 {
		t.Fatalf("20 source-authored declarations: err=%v failed=%+v count=%d", err, failed, len(outlined.Symbols))
	}
	var nodeID string
	for _, symbol := range outlined.Symbols {
		if symbol.Name == "string" || symbol.Name == "nodeType" {
			t.Errorf("field invented as Luau type: %+v", symbol)
		}
		if symbol.Name == "Node" {
			nodeID = symbol.ID
		}
	}
	if nodeID == "" || len(outlined.Files) != 1 || outlined.Files[0].SourceFingerprint == "" {
		t.Fatal("missing real Node type and fingerprint")
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: nodeID, Language: "luau",
		Encoding: "utf-8", SourceFingerprint: outlined.Files[0].SourceFingerprint, MaxBytes: 4096,
	})
	if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "export type Node") {
		t.Fatalf("authored type show failed: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
	}
	for _, name := range []string{"string", "nodeType"} {
		failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{path}, Language: "luau", Encoding: "utf-8",
			Query: name, Match: "exact", MaxFiles: 1, MaxSymbols: 20,
		})
		if err != nil || failed != nil || len(found.Symbols) != 0 {
			t.Fatalf("phantom find %s: err=%v failed=%+v count=%d", name, err, failed, len(found.Symbols))
		}
		failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: []string{path}, Language: "luau", Encoding: "utf-8",
			Query: name, Match: "exact", Mode: "structural", MaxResults: 20,
		})
		if err != nil || failed != nil || searched.Search == nil || len(searched.Search.Matches) != 0 {
			t.Fatalf("phantom search %s: err=%v failed=%+v search=%+v", name, err, failed, searched.Search)
		}
	}
}
