package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreeBasicPinnedWholeOriginalDeclaredMethodNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_FREEBASIC_ORIGINAL_ROOT")
	if root == "" {
		t.Skip("pinned FreeBasic originals not configured")
	}
	path := filepath.Join(root, "freebasic-tiko-clsApp-9db5c84f.bi")
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "detailed",
		Language: "freebasic", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 128, IncludeSignatures: true,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated || len(outlined.Symbols) != 59 {
		t.Fatalf("whole pinned original: err=%v failed=%+v symbols=%d", err, failed, len(outlined.Symbols))
	}
	methodCount := 0
	var methodID string
	for _, s := range outlined.Symbols {
		if s.NameRange.Start.Line < 68 || s.NameRange.Start.Line > 83 {
			continue
		}
		methodCount++
		if string(s.Kind) != "method" || s.ParentQualifiedName != "clsApp" || s.NativeKind != "declare-function" {
			t.Errorf("line %d not a type-owned method: %+v", s.NameRange.Start.Line, s)
		}
		if s.Name == "GetProjectCompiler" {
			methodID = s.ID
		}
	}
	if methodCount != 16 || methodID == "" || len(outlined.Files) != 1 || outlined.Files[0].SourceFingerprint == "" {
		t.Fatalf("expected 16 real methods + fingerprint, got %d, id=%q", methodCount, methodID)
	}
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "freebasic", Encoding: "utf-8",
		Query: "GetProjectCompiler", Match: "exact", MaxFiles: 1, MaxSymbols: 16,
	})
	if err != nil || failed != nil || len(found.Symbols) != 1 || string(found.Symbols[0].Kind) != "method" {
		t.Fatalf("method find: err=%v failed=%+v found=%+v", err, failed, found.Symbols)
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: methodID, Language: "freebasic", Encoding: "utf-8",
		SourceFingerprint: outlined.Files[0].SourceFingerprint, MaxBytes: 4096,
	})
	if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "declare function GetProjectCompiler") {
		t.Fatalf("method show: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
	}
	failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Language: "freebasic", Encoding: "utf-8",
		Query: "GetProjectCompiler", Match: "exact", Mode: "structural", MaxResults: 16,
	})
	if err != nil || failed != nil || searched.Search == nil || len(searched.Search.Matches) == 0 {
		t.Fatalf("structural method search: err=%v failed=%+v searched=%+v", err, failed, searched.Search)
	}
}
