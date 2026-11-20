package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Full unmodified GraphQL.js kitchen-sink source: three named directive-bearing
// operations, one reusable fragment, two anonymous selection negative controls.
func TestGraphQLPinnedWholeDirectiveOperationNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_GRAPHQL_FRAGMENT_JS_ROOT")
	if root == "" {
		t.Skip("pinned GraphQL.js original root not configured")
	}
	path := filepath.Join(root, "files", "graphql", "kitchen-sink.graphql")
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) != 1024 {
		t.Fatalf("pinned upstream file changed: %v, bytes=%d", err, len(raw))
	}
	ctx, h := context.Background(), NewHandler([]string{root})
	rejected, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "graphql", MaxFiles: 1, MaxSymbols: 64,
	})
	if err != nil || rejected != nil || outline.Truncated || outline.FilesParsed != 1 || len(outline.Symbols) != 4 {
		t.Fatalf("whole original symbol inventory changed: %v %+v", err, outline)
	}
	expected := map[string]struct {
		kind       string
		start, end int
	}{
		"queryName":             {"query", 1, 20},
		"likeStory":             {"mutation", 22, 28},
		"StoryLikeSubscription": {"subscription", 30, 43},
		"frag":                  {"fragment", 45, 56},
	}
	for _, s := range outline.Symbols {
		w, ok := expected[s.Name]
		if !ok || s.NativeKind != w.kind || s.DeclarationRange.Start.Line != w.start ||
			s.NameRange.Start.Line != w.start || s.DeclarationRange.End.Line != w.end ||
			s.SignatureRange.End.Line != w.end {
			t.Errorf("whole original mismatch %+v", s)
		}
	}
	for _, name := range []string{"queryName", "likeStory", "StoryLikeSubscription"} {
		rejected, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{path}, Language: "graphql", Query: name, Match: "exact",
			MaxFiles: 1, MaxSymbols: 16, FileDetails: "relevant",
		})
		if err != nil || rejected != nil || len(found.Symbols) != 1 || len(found.Files) != 1 {
			t.Fatalf("%s original exact find failed: %v %+v", name, err, found)
		}
		selected := found.Symbols[0]
		rejected, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
			Operation: "show", Path: path, Language: "graphql", Encoding: found.Files[0].Encoding,
			SymbolID: selected.ID, SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 4096,
		})
		if err != nil || rejected != nil || shown.Show == nil ||
			!strings.Contains(shown.Show.Text, name) || !strings.Contains(shown.Show.Text, "\n}") {
			t.Fatalf("%s verified source excerpt truncated: %v %+v", name, err, shown.Show)
		}
		switch name {
		case "queryName":
			if !strings.Contains(shown.Show.Text, "...frag @onFragmentSpread") {
				t.Error("query source selections lost")
			}
		case "likeStory":
			if !strings.Contains(shown.Show.Text, "@onMutation") {
				t.Error("mutation directive lost")
			}
		case "StoryLikeSubscription":
			if !strings.Contains(shown.Show.Text, "@onVariableDefinition") ||
				!strings.Contains(shown.Show.Text, "@onSubscription") {
				t.Error("multiline annotated variable source lost")
			}
		}
	}
}
