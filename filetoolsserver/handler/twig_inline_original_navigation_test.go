package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Read-only original-source navigation on three independent pinned upstream
// files. Never execute a third-party template as part of these tests.
func TestTwigPinnedInlineBlockFullOriginalNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_TWIG_OUTLINE_ROOT")
	if root == "" {
		t.Skip("three complete original Twig templates not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file, target, excerpt string
		expected              int
	}{
		{"symfony-admin-layout.html.twig", "header_navigation_links", "{% block header_navigation_links %}", 2},
		{"easyadmin-menu.html.twig", "render_menu_badge", "{% macro render_menu_badge(badge) %}", 4},
		{"symfony-blog-index.html.twig", "body_id", "{% block body_id 'blog_index' %}", 3},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() > 1<<20 {
				t.Fatalf("pinned original source must be bounded regular file: stat=%v err=%v", stat, err)
			}
			failed, detailed, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "twig", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 4096,
			})
			if err != nil || failed != nil || detailed.Truncated || !detailed.CoverageComplete ||
				detailed.FilesParsed != 1 || len(detailed.Files) != 1 || len(detailed.Symbols) != tc.expected {
				t.Fatalf("incomplete authored original Twig outline: err=%v failed=%+v returned=%d expected=%d",
					err, failed, len(detailed.Symbols), tc.expected)
			}
			for _, sym := range detailed.Symbols {
				if sym.ParentID != "" || sym.ParentQualifiedName != "" || sym.QualifiedName != sym.Name {
					t.Errorf("unexpected top-level original block scope: %+v", sym)
				}
			}
			fingerprint := detailed.Files[0].SourceFingerprint
			if fingerprint == "" {
				t.Fatal("missing original source fingerprint")
			}
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "twig",
				Encoding: "utf-8", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || failed != nil || len(found.Symbols) != 1 ||
				found.Symbols[0].Name != tc.target {
				t.Fatalf("Twig original exact find failed: err=%v failed=%+v count=%d",
					err, failed, len(found.Symbols))
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "twig", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fingerprint, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("Twig original fingerprint show failed: err=%v failed=%+v", err, failed)
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "twig", Encoding: "utf-8",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxResults: 64,
			})
			if err != nil || failed != nil || search.Search == nil ||
				len(search.Search.Matches) != 1 {
				t.Fatalf("Twig original structural search failed: err=%v failed=%+v", err, failed)
			}
		})
	}
}
