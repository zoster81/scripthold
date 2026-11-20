package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestElixirPinnedDirectDefimplOwnerNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_ELIXIR_PHOENIX_HTML_ROOT")
	if root == "" {
		t.Skip("independent pinned Phoenix HTML original not configured")
	}
	path := filepath.Join(root, "phoenix-html-form-data.ex")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != 6078 {
		t.Fatalf("pinned original source mismatch: %v %v", info, err)
	}
	ctx, h := context.Background(), NewHandler([]string{root})
	rejected, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "elixir",
		MaxFiles: 1, MaxSymbols: 64,
	})
	if err != nil || rejected != nil || outlined.FilesParsed != 1 || outlined.Truncated ||
		len(outlined.Symbols) != 13 {
		t.Fatalf("whole source inventory failed: %v %+v", err, outlined)
	}
	const qualified = "Phoenix.HTML.FormData.Map"
	var ownerID string
	childCount := 0
	for _, sym := range outlined.Symbols {
		if sym.QualifiedName == qualified {
			if sym.NativeKind != "defimpl" || sym.NameRange.Start.Line != 79 {
				t.Errorf("wrong direct implementation: %+v", sym)
			}
			ownerID = sym.ID
		}
	}
	if ownerID == "" {
		t.Fatal("missing authored implementation scope")
	}
	for _, sym := range outlined.Symbols {
		if sym.ParentQualifiedName == qualified {
			childCount++
			if sym.ParentID != ownerID {
				t.Errorf("wrong implementation parent link: %+v", sym)
			}
		}
	}
	if childCount != 7 {
		t.Errorf("got %d owned function declarations, want seven", childCount)
	}
	rejected, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "elixir",
		Query: qualified, Match: "qualified", MaxFiles: 1, MaxSymbols: 16, FileDetails: "relevant",
	})
	if err != nil || rejected != nil || len(found.Symbols) != 1 || len(found.Files) != 1 {
		t.Fatalf("source-qualified original implementation find failed: %v %+v", err, found)
	}
	file := found.Files[0]
	rejected, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "show", Path: path, Language: "elixir", Encoding: file.Encoding,
		SymbolID: found.Symbols[0].ID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
	})
	if err != nil || rejected != nil || shown.Show == nil ||
		!strings.Contains(shown.Show.Text, "defimpl Phoenix.HTML.FormData, for: Map do") {
		t.Fatalf("fingerprint-verified original owner show failed: %v %+v", err, shown.Show)
	}
	rejected, queried, err := h.SourceQuery(ctx, nil, SourceQueryInput{
		Operation: "search", Mode: "structural", Paths: []string{path}, Language: "elixir",
		Query: qualified, Match: "exact", MaxFiles: 1, MaxResults: 16,
	})
	if err != nil || rejected != nil || queried.Search == nil {
		t.Fatalf("original owner structural query failed: %v %+v", err, queried)
	}
	present := false
	for _, m := range queried.Search.Matches {
		if m.QualifiedName == qualified && m.SymbolID == ownerID {
			present = true
		}
	}
	if !present {
		t.Fatal("original implementation owner missing from structural search")
	}
}
