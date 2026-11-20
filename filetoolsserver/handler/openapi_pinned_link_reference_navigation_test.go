package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// One full Git-pinned OpenAPI Specification Link Object example: six authored
// path operations, three direct schema types and four nondeclaration references.
func TestOpenAPIPinnedWholeLinkReferenceNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_OPENAPI_LINK_EXAMPLE_ROOT")
	if root == "" {
		t.Skip("official pinned OpenAPI original not configured")
	}
	path := filepath.Join(root, "openapi-links-original.yaml")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != 5280 {
		t.Fatalf("pinned upstream source changed: %v %v", info, err)
	}
	ctx, h := context.Background(), NewHandler([]string{root})
	rejected, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "openapi",
		MaxFiles: 1, MaxSymbols: 128,
	})
	if err != nil || rejected != nil || outline.FilesParsed != 1 || outline.Truncated ||
		!outline.CoverageComplete || len(outline.Symbols) != 9 {
		t.Fatalf("original whole Link Object structural inventory mismatch: %v %+v", err, outline)
	}
	expected := map[string]struct {
		kind string
		line int
	}{
		"getUserByName":               {"operation", 8},
		"getRepositoriesByOwner":      {"operation", 27},
		"getRepository":               {"operation", 48},
		"getPullRequestsByRepository": {"operation", 72},
		"getPullRequestsById":         {"operation", 103},
		"mergePullRequest":            {"operation", 132},
		"user":                        {"schema", 179}, "repository": {"schema", 186}, "pullrequest": {"schema", 193},
	}
	for _, sym := range outline.Symbols {
		w, ok := expected[sym.Name]
		if !ok || sym.NativeKind != w.kind || sym.NameRange.Start.Line != w.line ||
			sym.DeclarationRange.Start.Line != w.line || sym.ParentQualifiedName != "" {
			t.Errorf("unexpected/misplaced source identity: %+v expected %+v", sym, w)
		}
	}
	for _, name := range []string{"getRepositoriesByOwner", "mergePullRequest", "repository"} {
		rejected, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{path}, Language: "openapi", Query: name,
			Match: "exact", MaxFiles: 1, MaxSymbols: 16, FileDetails: "relevant",
		})
		if err != nil || rejected != nil || len(found.Symbols) != 1 || len(found.Files) != 1 {
			t.Fatalf("original find %s includes reference/sibling: %v %+v", name, err, found)
		}
		sym, file := found.Symbols[0], found.Files[0]
		if sym.NameRange.Start.Line != expected[name].line {
			t.Errorf("original location %s wrong: %+v", name, sym)
		}
		rejected, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
			Operation: "show", Path: path, Language: "openapi", Encoding: file.Encoding,
			SymbolID: sym.ID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
		})
		if err != nil || rejected != nil || shown.Show == nil ||
			!strings.Contains(shown.Show.Text, name) {
			t.Fatalf("fingerprint-verified original show %s failed: %v %+v", name, err, shown.Show)
		}
		rejected, queried, err := h.SourceQuery(ctx, nil, SourceQueryInput{
			Operation: "search", Mode: "structural", Paths: []string{path},
			Query: name, Match: "exact", Language: "openapi", MaxFiles: 1, MaxResults: 16,
		})
		if err != nil || rejected != nil || queried.Search == nil ||
			len(queried.Search.Matches) != 1 ||
			queried.Search.Matches[0].SymbolID != sym.ID {
			t.Errorf("original structural query duplicated reference %s: %v %+v", name, err, queried)
		}
	}
}
