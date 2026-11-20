package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGraphQLPinnedWholeNamedQuerySourceNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_GRAPHQL_OUTLINE_ROOT")
	if root == "" {
		t.Skip("independent original GraphQL source not configured")
	}
	path := filepath.Join(root, "rust-fragments.graphql")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(source) != 394 {
		t.Fatalf("pinned source changed: %d", len(source))
	}
	ctx, h := context.Background(), NewHandler([]string{root})
	failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "graphql", MaxFiles: 1, MaxSymbols: 64,
	})
	if err != nil || failed != nil || outline.FilesParsed != 1 || outline.Truncated || len(outline.Symbols) != 6 {
		t.Fatalf("whole GraphQL original navigation unavailable: %v %+v", err, outline)
	}
	expected := map[string]int{"FragmentReference": 18, "SnakeCaseFragment": 22, "RecursiveFragmentQuery": 28}
	for _, symbol := range outline.Symbols {
		if symbol.NativeKind != "query" {
			continue
		}
		end, ok := expected[symbol.Name]
		if !ok || symbol.DeclarationRange.End.Line != end || symbol.SignatureRange.End.Line != end {
			t.Errorf("original query selection range not preserved: %+v", symbol)
		}
	}
	failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "graphql",
		Query: "RecursiveFragmentQuery", Match: "exact", MaxFiles: 1,
		MaxSymbols: 16, FileDetails: "relevant",
	})
	if err != nil || failed != nil || len(found.Symbols) != 1 || len(found.Files) != 1 {
		t.Fatalf("original query exact find failed: %v %+v", err, found)
	}
	query := found.Symbols[0]
	failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "show", Path: path, Language: "graphql", Encoding: found.Files[0].Encoding,
		SymbolID: query.ID, SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 4096,
	})
	if err != nil || failed != nil || shown.Show == nil ||
		!strings.Contains(shown.Show.Text, "recursive {") ||
		!strings.Contains(shown.Show.Text, "...RecursiveFragment") ||
		!strings.Contains(shown.Show.Text, "\n  }\n}") {
		t.Fatalf("fingerprint-bound original query excerpt truncated: %v %+v", err, shown.Show)
	}
}
