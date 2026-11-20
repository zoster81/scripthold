package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCPinnedWholeOriginalTaggedTypedefNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_C_OUTLINE_ROOT")
	if root == "" {
		t.Skip("whole pinned C source corpus not configured")
	}
	path := filepath.Join(root, "c-ctags-keyword-8495a124.c")
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "detailed",
		Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 128, IncludeSignatures: true,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated || len(outlined.Symbols) != 24 {
		t.Fatalf("whole original outline: err=%v failed=%+v count=%d", err, failed, len(outlined.Symbols))
	}
	var aliasID string
	for _, symbol := range outlined.Symbols {
		if symbol.Name != "hashEntry" {
			continue
		}
		if string(symbol.Kind) != "alias" || symbol.NativeKind != "typedef" ||
			symbol.DeclarationRange.Start.Line != 27 || symbol.DeclarationRange.Start.Column != 1 ||
			symbol.NameRange.Start.Line != 32 || symbol.NameRange.Start.Column != 3 {
			t.Errorf("wrong authored typedef identity or origin: %+v", symbol)
		}
		aliasID = symbol.ID
	}
	if aliasID == "" || len(outlined.Files) != 1 || outlined.Files[0].SourceFingerprint == "" {
		t.Fatal("missing authored alias + fingerprint")
	}
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "c", Encoding: "utf-8",
		Query: "hashEntry", Match: "exact", MaxFiles: 1, MaxSymbols: 8,
	})
	if err != nil || failed != nil || len(found.Symbols) != 1 || string(found.Symbols[0].Kind) != "alias" {
		t.Fatalf("find original typedef: err=%v failed=%+v found=%+v", err, failed, found.Symbols)
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: aliasID, Language: "c", Encoding: "utf-8",
		SourceFingerprint: outlined.Files[0].SourceFingerprint, MaxBytes: 4096,
	})
	if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "typedef struct sHashEntry") ||
		!strings.Contains(shown.Show.Text, "} hashEntry;") {
		t.Fatalf("full authored typedef span: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
	}
	failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Language: "c", Encoding: "utf-8",
		Query: "hashEntry", Match: "exact", Mode: "structural", MaxResults: 12,
	})
	if err != nil || failed != nil || searched.Search == nil || len(searched.Search.Matches) == 0 {
		t.Fatalf("structural alias search: err=%v failed=%+v search=%+v", err, failed, searched.Search)
	}
}

func TestCPinnedWholeOriginalNegativeControlsAndIndependentCurl(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_C_OUTLINE_ROOT")
	if root == "" {
		t.Skip("whole pinned C source corpus not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file  string
		count int
		name  string
	}{
		{"c-ctags-numarray-8495a124.c", 0, ""},
		{"c-curl-get-line-8eb978b9.c", 1, "Curl_get_line"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{filepath.Join(root, tc.file)}, Presentation: "detailed",
				Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated || len(outlined.Symbols) != tc.count {
				t.Fatalf("whole pinned control count: err=%v failed=%+v count=%d", err, failed, len(outlined.Symbols))
			}
			if tc.count > 0 && outlined.Symbols[0].Name != tc.name {
				t.Errorf("whole authored function wrong: %+v", outlined.Symbols[0])
			}
		})
	}
}
