package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCPPCLIPinnedWholeOriginalPropertySpanNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CPPCLI_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned C++/CLI source originals not configured")
	}
	path := filepath.Join(root, "cppcli-actor-f8c14d9f.h")
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "detailed",
		Language: "cpp-cli", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 128, IncludeSignatures: true,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated || len(outlined.Symbols) != 38 {
		t.Fatalf("whole pinned original detailed outline: err=%v failed=%+v count=%d", err, failed, len(outlined.Symbols))
	}
	wants := []struct {
		line int
		name string
	}{
		{42, "get"}, {48, "GetWorldBounds"}, {59, "get"}, {66, "get"},
		{74, "get"}, {82, "get"}, {91, "get"}, {102, "get"}, {112, "get"},
		{117, "UserData"}, {124, "get"}, {127, "UnmanagedOwner"},
	}
	var methodID string
	for _, want := range wants {
		found := false
		for _, s := range outlined.Symbols {
			if s.NameRange.Start.Line != want.line || s.Name != want.name {
				continue
			}
			found = true
			if s.DeclarationRange.Start.Line != want.line {
				t.Errorf("source %s line%d: declaration starts at %d not authored %d", want.name, want.line, s.DeclarationRange.Start.Line, want.line)
			}
			if want.name == "GetWorldBounds" {
				methodID = s.ID
			}
		}
		if !found {
			t.Errorf("source declaration missing %s at line%d", want.name, want.line)
		}
	}
	// Every authored managed block property is a real source owner, and
	// accessors with repeated names must remain attached to the correct owner.
	properties := map[string]int{
		"Disposed": 40, "Physics": 57, "Scene": 64, "Type": 72, "Name": 80,
		"Flags": 89, "DominanceGroup": 100, "OwnerClient": 110, "UnmanagedPointer": 122,
	}
	for name, line := range properties {
		qualified := "PhysX.Actor." + name
		var ownerID string
		for _, symbol := range outlined.Symbols {
			if symbol.QualifiedName == qualified && symbol.Kind == "property" &&
				symbol.NativeKind == "block-property" && symbol.NameRange.Start.Line == line &&
				symbol.ParentQualifiedName == "PhysX.Actor" {
				ownerID = symbol.ID
			}
		}
		if ownerID == "" {
			t.Errorf("missing authored property %s on line %d", qualified, line)
			continue
		}
		want := 1
		if name == "Name" || name == "Flags" || name == "DominanceGroup" || name == "OwnerClient" {
			want = 2
		}
		found := 0
		for _, symbol := range outlined.Symbols {
			if symbol.ParentID == ownerID && symbol.ParentQualifiedName == qualified &&
				(symbol.Name == "get" || symbol.Name == "set") {
				found++
			}
		}
		if found != want {
			t.Errorf("%s has %d owned accessors, want %d", qualified, found, want)
		}
	}
	if methodID == "" || len(outlined.Files) != 1 || outlined.Files[0].SourceFingerprint == "" {
		t.Fatal("missing authentic method or source fingerprint")
	}
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "cpp-cli", Encoding: "utf-8",
		Query: "GetWorldBounds", Match: "exact", MaxFiles: 1, MaxSymbols: 8,
	})
	if err != nil || failed != nil || len(found.Symbols) != 1 ||
		found.Symbols[0].DeclarationRange.Start.Line != 48 {
		t.Fatalf("MCP find wrong authored method source start: err=%v failed=%+v found=%+v", err, failed, found.Symbols)
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: methodID, Language: "cpp-cli", Encoding: "utf-8",
		SourceFingerprint: outlined.Files[0].SourceFingerprint, MaxBytes: 2048,
	})
	if err != nil || failed != nil || shown.Show == nil ||
		!strings.HasPrefix(strings.TrimSpace(shown.Show.Text), "Bounds3 GetWorldBounds();") ||
		strings.Contains(shown.Show.Text, "</summary>") {
		t.Fatalf("MCP show includes previous property or XML comment: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
	}
	failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Language: "cpp-cli", Encoding: "utf-8",
		Query: "GetWorldBounds", Match: "exact", Mode: "structural", MaxResults: 8,
	})
	if err != nil || failed != nil || searched.Search == nil || len(searched.Search.Matches) != 1 {
		t.Fatalf("structural source search: err=%v failed=%+v search=%+v", err, failed, searched.Search)
	}
}

func TestCPPCLIIndependentWholeMetaStillHasSixDeclarations(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CPPCLI_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned C++/CLI source originals not configured")
	}
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{filepath.Join(root, "cppcli-meta-5f5b92b8.h")}, Presentation: "detailed",
		Language: "cpp-cli", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || len(outlined.Symbols) != 6 {
		t.Fatalf("complete independent original changed: err=%v failed=%+v count=%d", err, failed, len(outlined.Symbols))
	}
}
