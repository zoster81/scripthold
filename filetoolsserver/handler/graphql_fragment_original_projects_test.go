package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Independent GraphQL-Go and GraphQL.js source snapshots contain real fragment
// definitions and spreads, which must remain distinct in MCP navigation.
func TestGraphQLOriginalNamedFragmentNavigation(t *testing.T) {
	cases := []struct {
		name, env, relative, header, body, neighbor string
	}{
		{"GraphQL-Go reusable follower fragment",
			"SCRIPTHOLD_REAL_GRAPHQL_FRAGMENT_GO_ROOT",
			"kitchen-sink.graphql", "fragment frag on Follower {",
			"foo(size: $size, bar: $b", "namedQuery"},
		{"GraphQL.js reusable friend fragment",
			"SCRIPTHOLD_REAL_GRAPHQL_FRAGMENT_JS_ROOT",
			"files/graphql/kitchen-sink.graphql",
			"fragment frag on Friend @onFragmentDefinition {",
			"foo(", "queryName"},
	}
	configured := 0
	for _, tc := range cases {
		if os.Getenv(tc.env) != "" {
			configured++
		}
	}
	if configured == 0 {
		t.Skip("two original GraphQL project roots not configured")
	}
	if configured != len(cases) {
		t.Fatal("both independent GraphQL project roots must be set")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := os.Getenv(tc.env)
			path := filepath.Join(root, filepath.FromSlash(tc.relative))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("source fixture must be bounded regular file: %v %v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			if len(lines) < 45 || !strings.Contains(lines[44], tc.header) ||
				!strings.Contains(string(raw), tc.body) || !strings.Contains(string(raw), "...frag") {
				t.Fatalf("independent original GraphQL fragment oracle changed: %s", tc.relative)
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "graphql", MaxFiles: 1,
			})
			if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("GraphQL digest failed: %v %+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "graphql", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("GraphQL detailed outline failed: %v %+v", err, outline)
			}
			neighborFound := false
			for _, s := range outline.Symbols {
				if s.Name == tc.neighbor && string(s.Kind) == "operation" && s.NativeKind == "query" {
					neighborFound = true
				}
			}
			if !neighborFound {
				t.Errorf("adjacent original query missing: %s", tc.neighbor)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "graphql",
				Query: "frag", Match: "exact", MaxFiles: 1, MaxSymbols: 16,
				FileDetails: "relevant",
			})
			if err != nil {
				t.Fatal(err)
			}
			id := ""
			for _, s := range found.Symbols {
				if s.Name == "frag" && string(s.Kind) == "section" &&
					s.NativeKind == "fragment" && s.ParentQualifiedName == "" &&
					s.NameRange.Start.Line == 45 {
					id = s.ID
				}
			}
			inOutline := false
			for _, s := range outline.Symbols {
				if id != "" && s.ID == id {
					inOutline = true
				}
			}
			if id == "" || !inOutline || len(found.Files) != 1 {
				t.Fatalf("named fragment kind/owner/line absent: find=%+v outline=%+v", found.Symbols, outline.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("source fingerprint mismatch: %+v", file)
			}
			_, show, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "graphql", Encoding: file.Encoding,
				SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || show.Show == nil || !strings.Contains(show.Show.Text, tc.header) ||
				!strings.Contains(show.Show.Text, tc.body) {
				t.Fatalf("fingerprint-safe fragment body not recovered: %v %+v", err, show.Show)
			}
			_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "graphql", Query: "frag", Match: "exact", MaxFiles: 1, MaxResults: 32,
			})
			linked := false
			if query.Search != nil {
				for _, hit := range query.Search.Matches {
					if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint &&
						hit.Range.Start.Line == 45 {
						linked = true
					}
				}
			}
			if err != nil || query.Search == nil || query.Coverage.ResultsTruncated || !linked {
				t.Fatalf("structural search did not index exact fragment: %v %+v", err, query)
			}
		})
	}
}
