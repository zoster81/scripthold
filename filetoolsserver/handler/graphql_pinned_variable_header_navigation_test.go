package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Whole pinned GraphQL-Go kitchen sink retains all authored named operation
// selections in source navigation without inventing anonymous operations.
func TestGraphQLPinnedVariableHeaderOriginalNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_GRAPHQL_GO_AUDIT_ROOT")
	if root == "" {
		t.Skip("pinned original GraphQL-Go source not configured")
	}
	path := filepath.Join(root, "kitchen-sink.graphql")
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) != 823 {
		t.Fatalf("original changed: %v, bytes=%d", err, len(raw))
	}
	ctx, h := context.Background(), NewHandler([]string{root})
	rejected, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "graphql", MaxFiles: 1, MaxSymbols: 64,
	})
	if err != nil || rejected != nil || outline.Truncated || outline.FilesParsed != 1 || len(outline.Symbols) != 4 {
		t.Fatalf("whole original GraphQL outline changed: %v, %+v", err, outline)
	}
	expected := map[string]struct {
		kind       string
		start, end int
	}{
		"namedQuery":          {"query", 3, 22},
		"favPost":             {"mutation", 24, 30},
		"PostFavSubscription": {"subscription", 32, 43},
		"frag":                {"fragment", 45, 47},
	}
	for _, s := range outline.Symbols {
		w, ok := expected[s.Name]
		if !ok || s.NativeKind != w.kind || s.NameRange.Start.Line != w.start ||
			s.DeclarationRange.Start.Line != w.start || s.DeclarationRange.End.Line != w.end ||
			s.SignatureRange.End.Line != w.end {
			t.Errorf("whole original symbol excerpt differs: %+v", s)
		}
	}
	for _, name := range []string{"namedQuery", "PostFavSubscription"} {
		rejected, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{path}, Language: "graphql",
			Query: name, Match: "exact", MaxFiles: 1, MaxSymbols: 16, FileDetails: "relevant",
		})
		if err != nil || rejected != nil || len(found.Symbols) != 1 || len(found.Files) != 1 {
			t.Fatalf("%s exact find failed: %v %+v", name, err, found)
		}
		selected := found.Symbols[0]
		rejected, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
			Operation: "show", Path: path, Language: "graphql", Encoding: found.Files[0].Encoding,
			SymbolID: selected.ID, SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 8192,
		})
		if err != nil || rejected != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, name+"(") ||
			!strings.Contains(shown.Show.Text, "\n}") {
			t.Fatalf("%s source excerpt not fingerprint verified: %v %+v", name, err, shown.Show)
		}
		if name == "namedQuery" && !strings.Contains(shown.Show.Text, "...frag") {
			t.Fatalf("query selection references missing from original source show: %q", shown.Show.Text)
		}
		if name == "PostFavSubscription" && !strings.Contains(shown.Show.Text, "favSentence {") {
			t.Fatalf("subscription nested source lost: %q", shown.Show.Text)
		}
	}
}
