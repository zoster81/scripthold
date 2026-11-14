package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original GraphQL-Go and gqlgen schemas contain directive definitions used by
// named operations/fields; annotations must not be indexed as definitions.
func TestGraphQLOriginalSchemaDirectiveNavigation(t *testing.T) {
	type declaration struct {
		name   string
		line   int
		header string
		body   string
	}
	cases := []struct {
		name, env, relative, neighbor string
		want                          []declaration
	}{
		{
			"GraphQL-Go schema locations", "SCRIPTHOLD_REAL_GRAPHQL_DIRECTIVE_GO_ROOT",
			"schema-kitchen-sink.graphql", "Foo",
			[]declaration{
				{"skip", 65, "directive @skip(", "FRAGMENT_SPREAD"},
				{"include", 67, "directive @include(", "INLINE_FRAGMENT"},
			},
		},
		{
			"gqlgen authorization and resolver metadata", "SCRIPTHOLD_REAL_GRAPHQL_DIRECTIVE_GQLGEN_ROOT",
			"files/graphql/schema.graphql", "MyQuery",
			[]declaration{
				{"goField", 1, "directive @goField(", "FIELD_DEFINITION"},
				{"hasRole", 47, "directive @hasRole(", "FIELD_DEFINITION"},
				{"user", 48, "directive @user(", "MUTATION | QUERY | FIELD"},
			},
		},
	}
	configured := 0
	for _, c := range cases {
		if os.Getenv(c.env) != "" {
			configured++
		}
	}
	if configured == 0 {
		t.Skip("two independent original GraphQL directive schemas not configured")
	}
	if configured != len(cases) {
		t.Fatal("both independent original GraphQL roots must be configured")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := os.Getenv(tc.env)
			path := filepath.Join(root, filepath.FromSlash(tc.relative))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("expected bounded regular original file: %v %v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			for _, decl := range tc.want {
				if decl.line > len(lines) || !strings.Contains(lines[decl.line-1], decl.header) ||
					!strings.Contains(string(raw), decl.body) {
					t.Fatalf("independent source oracle changed: %s:%d %s", tc.relative, decl.line, decl.name)
				}
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "graphql", MaxFiles: 1,
			})
			if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original digest failed: %v %+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "graphql", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("original outline failed: %v %+v", err, outline)
			}
			neighbor := false
			for _, s := range outline.Symbols {
				if s.Name == tc.neighbor && string(s.Kind) == "type" {
					neighbor = true
				}
			}
			if !neighbor {
				t.Errorf("neighboring type %q missing", tc.neighbor)
			}
			for _, decl := range tc.want {
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "graphql", Query: decl.name,
					Match: "exact", MaxFiles: 1, MaxSymbols: 16, FileDetails: "relevant",
				})
				if err != nil {
					t.Fatal(err)
				}
				id := ""
				for _, s := range found.Symbols {
					if s.Name == decl.name && string(s.Kind) == "section" && s.NativeKind == "directive" &&
						s.ParentQualifiedName == "" && s.NameRange.Start.Line == decl.line {
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
					t.Fatalf("directive not indexed correctly: %s:%d %+v", decl.name, decl.line, found.Symbols)
				}
				file := found.Files[0]
				if file.SourceFingerprint != digest.Digests[0].SourceFingerprint || file.Encoding == "" {
					t.Fatalf("fingerprint mismatch for %q: %+v", decl.name, file)
				}
				_, show, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "graphql", Encoding: file.Encoding,
					SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
				})
				if err != nil || show.Show == nil || !strings.Contains(show.Show.Text, decl.header) ||
					!strings.Contains(show.Show.Text, decl.body) {
					t.Fatalf("verified directive show wrong: %s %v %+v", decl.name, err, show.Show)
				}
				_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{path},
					Language: "graphql", Query: decl.name, Match: "exact", MaxFiles: 1, MaxResults: 32,
				})
				linked := false
				if query.Search != nil {
					for _, hit := range query.Search.Matches {
						if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint &&
							hit.Range.Start.Line == decl.line {
							linked = true
						}
					}
				}
				if err != nil || query.Search == nil || query.Coverage.ResultsTruncated || !linked {
					t.Fatalf("structural directive search wrong: %s %v %+v", decl.name, err, query)
				}
			}
		})
	}
}
