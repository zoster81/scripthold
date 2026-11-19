package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCPPCLIPinnedWholeOriginalFinalizerNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CPPCLI_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned whole C++/CLI originals not configured")
	}
	path := filepath.Join(root, "cppcli-actor-f8c14d9f.h")
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "detailed",
		Language: "cpp-cli", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 128, IncludeSignatures: true,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated || len(outlined.Symbols) != 38 {
		t.Fatalf("whole original C++/CLI outline: err=%v failed=%+v count=%d", err, failed, len(outlined.Symbols))
	}
	var finalizerID string
	for _, s := range outlined.Symbols {
		if s.NameRange.Start.Line != 37 {
			continue
		}
		if s.Name != "Actor" || string(s.Kind) != "method" || s.NativeKind != "finalizer-declaration" ||
			s.ParentQualifiedName != "PhysX.Actor" || !strings.Contains(s.Signature, "!Actor()") {
			t.Errorf("authored finalizer wrongly classified: %+v", s)
		}
		finalizerID = s.ID
	}
	if finalizerID == "" || len(outlined.Files) != 1 || outlined.Files[0].SourceFingerprint == "" {
		t.Fatal("missing authentic finalizer identity/fingerprint")
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: finalizerID, Language: "cpp-cli", Encoding: "utf-8",
		SourceFingerprint: outlined.Files[0].SourceFingerprint, MaxBytes: 2048,
	})
	if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "!Actor();") {
		t.Fatalf("source-backed finalizer show: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
	}
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "cpp-cli", Encoding: "utf-8",
		Query: "Actor", Match: "exact", MaxFiles: 1, MaxSymbols: 20,
	})
	if err != nil || failed != nil {
		t.Fatalf("real method find: err=%v failed=%+v", err, failed)
	}
	finalizerFound := false
	for _, s := range found.Symbols {
		if s.NameRange.Start.Line == 37 && s.NativeKind == "finalizer-declaration" && string(s.Kind) == "method" {
			finalizerFound = true
		}
	}
	if !finalizerFound {
		t.Fatalf("finalizer absent from method find: %+v", found.Symbols)
	}
	failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Language: "cpp-cli", Encoding: "utf-8",
		Query: "Actor", Match: "exact", Mode: "structural", MaxResults: 20,
	})
	if err != nil || failed != nil || searched.Search == nil || len(searched.Search.Matches) == 0 {
		t.Fatalf("structural finalizer search: err=%v failed=%+v search=%+v", err, failed, searched.Search)
	}
}

func TestCPPCLIPinnedIndependentMetaWholeOriginalNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CPPCLI_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned whole C++/CLI originals not configured")
	}
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{filepath.Join(root, "cppcli-meta-5f5b92b8.h")}, Presentation: "detailed",
		Language: "cpp-cli", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated || len(outlined.Symbols) != 6 {
		t.Fatalf("independent C++/CLI whole original: err=%v failed=%+v count=%d", err, failed, len(outlined.Symbols))
	}
}
