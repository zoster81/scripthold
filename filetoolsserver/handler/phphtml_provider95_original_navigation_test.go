package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// The source scopes come from distinct original Git projects. These tests
// verify bounded navigation, not inferred PHP hook/call dependencies.
func TestPHPHTMLProvider95OriginalNavigation(t *testing.T) {
	cases := []struct {
		name, env, editID, excerpt, oracle, callOnly string
		editLine                                     int
		neighbors                                    map[string]int
	}{
		{
			name: "WordPress 404 search page markup", env: "SCRIPTHOLD_REAL_PHPHTML_WORDPRESS",
			editID: "primary", editLine: 12, excerpt: `<div id="primary">`,
			oracle: "get_search_form();", callOnly: "get_search_form",
			neighbors: map[string]int{"content": 13, "post-0": 15},
		},
		{
			name: "Storefront header and cart hook markup", env: "SCRIPTHOLD_REAL_PHPHTML_STOREFRONT_HEADER",
			editID: "masthead", editLine: 30, excerpt: `<header id="masthead"`,
			oracle: "do_action( 'storefront_header' );", callOnly: "do_action",
			neighbors: map[string]int{"page": 27, "content": 63},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_PHPHTML_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("original PHP/HTML source not configured")
			}
			if root == "" || path == "" {
				t.Fatal("original PHP/HTML source only partially configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("expected bounded regular original source file: info=%v err=%v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tc.excerpt) || !strings.Contains(string(raw), tc.oracle) {
				t.Fatal("original task-specific source evidence mismatch")
			}

			h, ctx := NewHandler([]string{root}), context.Background()
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "php-html", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("PHP/HTML file digest unusable: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "php-html",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || outline.FilesParsed != 1 || outline.Truncated ||
				!outline.CoverageComplete {
				t.Fatalf("PHP/HTML outline incomplete: err=%v outline=%+v", err, outline)
			}
			id := ""
			seen := map[string]int{}
			for _, symbol := range outline.Symbols {
				if symbol.Kind != "entity" {
					continue
				}
				if symbol.Name == tc.editID && symbol.NameRange.Start.Line == tc.editLine &&
					symbol.ID != "" {
					id = symbol.ID
				}
				if _, ok := tc.neighbors[symbol.Name]; ok {
					if symbol.NameRange.Start.Line == tc.neighbors[symbol.Name] {
						seen[symbol.Name]++
					}
				}
				// PHP/HTML comment text is not a real HTML element.
				if tc.editID == "masthead" && symbol.Name == "content" &&
					symbol.NameRange.Start.Line == 5 {
					t.Errorf("fabricated HTML id from PHP doc comment: %+v", symbol)
				}
			}
			if id == "" {
				t.Errorf("source-anchored edit element %s:%d unavailable: %+v", tc.editID, tc.editLine, outline.Symbols)
			}
			for name, line := range tc.neighbors {
				if seen[name] != 1 {
					t.Errorf("expected one neighboring HTML id %s on line %d, saw %d", name, line, seen[name])
				}
			}
			if id == "" {
				return
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "php-html",
				Query: tc.editID, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
				FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("PHP/HTML exact find failed: err=%v result=%+v", err, found)
			}
			foundID := false
			for _, symbol := range found.Symbols {
				if symbol.ID == id && symbol.NameRange.Start.Line == tc.editLine {
					foundID = true
				}
			}
			file := found.Files[0]
			if !foundID || file.SourceFingerprint != digest.Digests[0].SourceFingerprint ||
				file.Encoding == "" {
				t.Fatalf("PHP/HTML safe selector inconsistent: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "php-html",
				Encoding: file.Encoding, SymbolID: id,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("PHP/HTML exact authored element not retrieved: err=%v show=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "php-html", Query: tc.editID, Match: "exact",
				MaxFiles: 1, MaxResults: 128,
			})
			if err != nil || search.Search == nil || search.Coverage.ResultsTruncated {
				t.Fatalf("PHP/HTML structural lookup failed: err=%v result=%+v", err, search)
			}
			linked := false
			for _, hit := range search.Search.Matches {
				if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("structural lookup does not link exact PHP/HTML ID: %+v", search.Search)
			}
			// Embedded PHP calls are not declarations or proven call edges.
			_, calls, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "php-html",
				Query: tc.callOnly, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || len(calls.Symbols) != 0 {
				t.Fatalf("PHP call fabricated as a declaration: err=%v symbols=%+v", err, calls.Symbols)
			}
		})
	}
}
