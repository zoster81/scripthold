package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two independent protobuf implementations supplied these full authored .proto
// or embedded .proto test sources; no upstream code is executed by this test.
func TestProtoOriginalInlineEnumMCPNavigation(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker string
		members           []struct {
			qualified, spelling string
			line                int
		}
	}{
		{
			name: "buf-standalone", env: "SCRIPTHOLD_REAL_PROTO_INLINE_BUF",
			marker: "enum Foo { FOO_UNKNOWN = 1;}",
			members: []struct {
				qualified, spelling string
				line                int
			}{
				{"Foo.FOO_UNKNOWN", "FOO_UNKNOWN", 8},
				{"Bar.BAR_UNKNOWN", "BAR_UNKNOWN", 9},
			},
		},
		{
			name: "buf-multi", env: "SCRIPTHOLD_REAL_PROTO_INLINE_BUF_MULTI",
			marker: "enum TestEnum{ ZERO = 0; ONE = 1; }",
			members: []struct {
				qualified, spelling string
				line                int
			}{
				{"TestEnum.ZERO", "ZERO", 1},
				{"TestEnum.ONE", "ONE", 1},
			},
		},
		{
			name: "google-embedded", env: "SCRIPTHOLD_REAL_PROTO_INLINE_GOOGLE",
			marker: "enum Thud { VALUE = 0; }",
			members: []struct {
				qualified, spelling string
				line                int
			}{
				{"foo.Thud.VALUE", "VALUE", 4},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Protocol Buffers source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 30 || info.Size() > 1500 {
				t.Fatalf("authored original proto missing, non-regular or oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(body)
			if !strings.Contains(source, tc.marker) {
				t.Fatal("original protobuf source marker drift")
			}
			lines := strings.Split(source, "\n")
			h := NewHandler([]string{filepath.Dir(path)})
			failure, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "proto",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || out.Truncated || len(out.OutlineFiles) != 1 ||
				out.OutlineFiles[0].SourceFingerprint == "" {
				t.Fatalf("original proto MCP outline failed: %v %+v %+v", err, failure, out)
			}
			for _, want := range tc.members {
				col := strings.Index(lines[want.line-1], want.spelling) + 1
				if col < 1 {
					t.Fatalf("authored enum member %q missing at %d", want.spelling, want.line)
				}
				node := findProtoInlineOutline(out.OutlineFiles[0].Symbols, want.qualified)
				if node == nil || node.Kind != "constant" ||
					node.Line != want.line || node.Column != col {
					t.Errorf("protobuf member %q not navigable at %d:%d: %+v",
						want.qualified, want.line, col, node)
				}
			}
			failure, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "proto",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original proto MCP digest failed: %v %+v %+v", err, failure, digest)
			}
			total := 0
			for _, c := range digest.Digests[0].DeclarationCounts {
				if c.Kind == "constant" {
					total += c.Count
				}
			}
			if total != len(tc.members) {
				t.Fatalf("protobuf enum digest count %d; expected %d", total, len(tc.members))
			}
		})
	}
}

func findProtoInlineOutline(nodes []*SourceOutlineNode, qualified string) *SourceOutlineNode {
	for _, node := range nodes {
		if node.QualifiedName == qualified {
			return node
		}
		if match := findProtoInlineOutline(node.Children, qualified); match != nil {
			return match
		}
	}
	return nil
}
