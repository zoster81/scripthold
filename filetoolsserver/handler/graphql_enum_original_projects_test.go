package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two independent original GraphQL projects expose enum members for changing
// role gates and display options; field references are not member definitions.
func TestGraphQLOriginalEnumValueNavigation(t *testing.T) {
	type member struct {
		name   string
		parent string
		line   int
		source string
	}
	cases := []struct {
		name, env, relative, neighbor string
		members                       []member
	}{
		{
			name:     "GraphQL-Go site and annotated values",
			env:      "SCRIPTHOLD_REAL_GRAPHQL_ENUM_GO_ROOT",
			relative: "schema-kitchen-sink.graphql",
			neighbor: "InputType",
			members: []member{
				{"DESKTOP", "Site", 39, "DESKTOP"},
				{"MOBILE", "Site", 40, "MOBILE"},
				{"ANNOTATED_VALUE", "AnnotatedEnum", 44, "ANNOTATED_VALUE @onEnumValue"},
				{"OTHER_VALUE", "AnnotatedEnum", 45, "OTHER_VALUE"},
			},
		},
		{
			name:     "gqlgen authorization roles",
			env:      "SCRIPTHOLD_REAL_GRAPHQL_ENUM_GQLGEN_ROOT",
			relative: "files/graphql/schema.graphql",
			neighbor: "Todo",
			members: []member{
				{"ADMIN", "Role", 9, "ADMIN"},
				{"OWNER", "Role", 10, "OWNER"},
			},
		},
	}
	count := 0
	for _, tc := range cases {
		if os.Getenv(tc.env) != "" {
			count++
		}
	}
	if count == 0 {
		t.Skip("two independent original GraphQL enum projects not configured")
	}
	if count != len(cases) {
		t.Fatal("both independent GraphQL enum roots must be set")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := os.Getenv(tc.env)
			path := filepath.Join(root, filepath.FromSlash(tc.relative))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("expected bounded regular original source: %v %v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			for _, m := range tc.members {
				if m.line > len(lines) || strings.TrimSpace(lines[m.line-1]) != m.source ||
					!strings.Contains(string(raw), "enum "+m.parent+" {") &&
						!strings.Contains(string(raw), "enum "+m.parent+" @onEnum {") {
					t.Fatalf("independent original value oracle changed: %s:%d %s.%s",
						tc.relative, m.line, m.parent, m.name)
				}
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "graphql", MaxFiles: 1,
			})
			if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original source digest: %v %+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "graphql", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("original source outline: %v %+v", err, outline)
			}
			neighbor := false
			for _, s := range outline.Symbols {
				if s.Name == tc.neighbor && string(s.Kind) == "type" {
					neighbor = true
				}
			}
			if !neighbor {
				t.Errorf("neighboring ordinary type missing: %s", tc.neighbor)
			}
			for _, m := range tc.members {
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "graphql",
					Query: m.name, Match: "exact", MaxFiles: 1, MaxSymbols: 16,
					FileDetails: "relevant",
				})
				if err != nil {
					t.Fatal(err)
				}
				id := ""
				for _, s := range found.Symbols {
					if s.Name == m.name && string(s.Kind) == "constant" &&
						s.NativeKind == "enum-value" && s.ParentQualifiedName == m.parent &&
						s.NameRange.Start.Line == m.line {
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
					t.Fatalf("enum value not safely located: %s.%s line %d %+v",
						m.parent, m.name, m.line, found.Symbols)
				}
				file := found.Files[0]
				if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
					t.Fatalf("source fingerprint mismatch for %s.%s: %+v", m.parent, m.name, file)
				}
				_, show, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "graphql", Encoding: file.Encoding,
					SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 1024,
				})
				if err != nil || show.Show == nil || !strings.Contains(show.Show.Text, m.source) {
					t.Fatalf("fingerprint-safe enum value excerpt missing: %s.%s %v %+v",
						m.parent, m.name, err, show.Show)
				}
				_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{path},
					Language: "graphql", Query: m.name, Match: "exact", MaxFiles: 1, MaxResults: 32,
				})
				linked := false
				if query.Search != nil {
					for _, hit := range query.Search.Matches {
						if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint &&
							hit.Range.Start.Line == m.line {
							linked = true
						}
					}
				}
				if err != nil || query.Search == nil || query.Coverage.ResultsTruncated || !linked {
					t.Fatalf("structural enum lookup wrong: %s.%s %v %+v", m.parent, m.name, err, query)
				}
			}
		})
	}
}
