package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGraphQLOriginalInlineEnumMCPNavigation(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker, enumName string
		line                        int
		values                      []string
	}{
		{
			name:     "graphql-js",
			env:      "SCRIPTHOLD_REAL_GRAPHQL_INLINE_JS",
			marker:   "enum Hello { WO, RLD }",
			enumName: "Hello", line: 1, values: []string{"WO", "RLD"},
		},
		{
			name:     "graphql-java",
			env:      "SCRIPTHOLD_REAL_GRAPHQL_INLINE_JAVA",
			marker:   "enum Purpose { SECURITY EXECUTION }",
			enumName: "Purpose", line: 4, values: []string{"SECURITY", "EXECUTION"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original authored GraphQL SDL not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 20 || info.Size() > 1500 {
				t.Fatalf("original SDL unavailable, non-regular or oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(body)
			if !strings.Contains(source, tc.marker) {
				t.Fatal("original authored GraphQL SDL marker drift")
			}
			lines := strings.Split(source, "\n")
			handler := NewHandler([]string{filepath.Dir(path)})
			failure, out, err := handler.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "graphql",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || out.Truncated || len(out.OutlineFiles) != 1 ||
				out.OutlineFiles[0].SourceFingerprint == "" {
				t.Fatalf("original GraphQL inline outline failed: %v %+v %+v", err, failure, out)
			}
			for _, member := range tc.values {
				expectedCol := strings.Index(lines[tc.line-1], member) + 1
				if expectedCol < 1 {
					t.Fatalf("original enum member %q missing from exact source line", member)
				}
				qualified := tc.enumName + "." + member
				found := findGraphQLInlineOutline(out.OutlineFiles[0].Symbols, qualified)
				if found == nil || found.Kind != "constant" ||
					found.Line != tc.line || found.Column != expectedCol {
					t.Errorf("original GraphQL enum navigation %q wrong at %d:%d: %+v",
						qualified, tc.line, expectedCol, found)
				}
			}
			failure, digest, err := handler.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "graphql",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original GraphQL inline digest failed: %v %+v %+v", err, failure, digest)
			}
			constants := 0
			for _, count := range digest.Digests[0].DeclarationCounts {
				if count.Kind == "constant" {
					constants += count.Count
				}
			}
			if constants != len(tc.values) {
				t.Fatalf("original GraphQL inline enum digest count = %d, want %d",
					constants, len(tc.values))
			}
		})
	}
}

func findGraphQLInlineOutline(nodes []*SourceOutlineNode, qualified string) *SourceOutlineNode {
	for _, node := range nodes {
		if node.QualifiedName == qualified {
			return node
		}
		if child := findGraphQLInlineOutline(node.Children, qualified); child != nil {
			return child
		}
	}
	return nil
}
