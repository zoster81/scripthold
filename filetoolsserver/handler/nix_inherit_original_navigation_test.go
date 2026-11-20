package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNixWholeOriginalInheritedLetNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_NIX_FLAKE_PARTS_ROOT")
	if root == "" {
		t.Skip("whole original flake-parts source not configured")
	}
	path := filepath.Join(root, "transposition.nix")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(source) != 4588 {
		t.Fatalf("original Nix source changed: %d bytes", len(source))
	}
	ctx := context.Background()
	h := NewHandler([]string{root})
	failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "nix", MaxFiles: 1, MaxSymbols: 128,
	})
	if err != nil || failed != nil || outline.CoverageComplete || outline.Truncated {
		t.Fatalf("whole Nix original should remain partial but navigable: %v %+v", err, outline)
	}
	wants := map[string]int{
		"filterAttrs": 5, "mapAttrs": 6, "mkOption": 7,
		"types": 8, "escapeNixIdentifier": 11,
	}
	for name, line := range wants {
		found := false
		for _, symbol := range outline.Symbols {
			if symbol.Name == name && symbol.NameRange.Start.Line == line &&
				symbol.Kind == "variable" && symbol.NativeKind == "inherited-binding" &&
				symbol.ParentQualifiedName == "" {
				found = true
			}
		}
		if !found {
			t.Errorf("missing original inherited name %s at line %d", name, line)
		}
	}
	failed, selected, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "nix",
		Query: "escapeNixIdentifier", Match: "exact", MaxFiles: 1,
		MaxSymbols: 16, FileDetails: "relevant",
	})
	if err != nil || failed != nil || len(selected.Symbols) != 1 || len(selected.Files) != 1 {
		t.Fatalf("original inherited name exact find failed: %v %+v", err, selected)
	}
	symbol := selected.Symbols[0]
	if symbol.NameRange.Start.Line != 11 {
		t.Fatalf("incorrect original inherited line: %+v", symbol)
	}
	failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "show", Path: path, Language: "nix",
		Encoding: selected.Files[0].Encoding, SymbolID: symbol.ID,
		SourceFingerprint: selected.Files[0].SourceFingerprint, MaxBytes: 2048,
	})
	if err != nil || failed != nil || shown.Show == nil ||
		!strings.Contains(shown.Show.Text, "escapeNixIdentifier") {
		t.Fatalf("fingerprint-bound inherited show failed: %v %+v", err, shown.Show)
	}
}
