package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObjectiveCPinnedOriginalNoPhantomMethodsNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_OBJECTIVEC_OUTLINE_ROOT")
	if root == "" {
		t.Skip("Objective-C whole original not configured")
	}
	path := filepath.Join(root, "ctags-objc-signature.m")
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "detailed",
		Language: "objective-c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 128,
		IncludeSignatures: true,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated || len(outlined.Symbols) != 13 {
		t.Fatalf("whole original: err=%v failed=%+v symbols=%d", err, failed, len(outlined.Symbols))
	}
	var id string
	for _, symbol := range outlined.Symbols {
		if strings.HasPrefix(symbol.Name, "idarg") {
			t.Errorf("original argument reported as method: %+v", symbol)
		}
		if symbol.Name == "doVoidReturn" && symbol.NameRange.Start.Line == 3 {
			id = symbol.ID
		}
	}
	if id == "" || len(outlined.Files) != 1 || outlined.Files[0].SourceFingerprint == "" {
		t.Fatal("missing genuine typed method and fingerprint")
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: id, Language: "objective-c",
		Encoding: "utf-8", SourceFingerprint: outlined.Files[0].SourceFingerprint, MaxBytes: 4096,
	})
	if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "- (void)doVoidReturn") {
		t.Fatalf("source excerpt changed: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
	}
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "objective-c",
		Encoding: "utf-8", Query: "idargExplicit", Match: "exact", MaxFiles: 1, MaxSymbols: 8,
	})
	if err != nil || failed != nil || len(found.Symbols) != 0 {
		t.Fatalf("phantom find result: err=%v failed=%+v found=%+v", err, failed, found.Symbols)
	}
	failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Language: "objective-c",
		Encoding: "utf-8", Query: "idargExplicit", Match: "exact",
		Mode: "structural", MaxResults: 8,
	})
	if err != nil || failed != nil || searched.Search == nil || len(searched.Search.Matches) != 0 {
		t.Fatalf("phantom structural search: err=%v failed=%+v result=%+v", err, failed, searched.Search)
	}
}
