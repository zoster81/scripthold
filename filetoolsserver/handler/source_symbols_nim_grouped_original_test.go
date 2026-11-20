package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The local MCP digest emits one dependency per authored Nim grouped-import
// member and anchors its range to the member's original source tokens.
func TestNimOriginalGroupedImportDigest(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker string
		line              int
		members           []string
	}{
		{
			name: "bu", env: "SCRIPTHOLD_REAL_NIM_GROUP_BU",
			marker: "import std/[math, random], memfiles as mf, cligen/sysUt",
			line:   11, members: []string{"math", "random"},
		},
		{
			name: "compiler", env: "SCRIPTHOLD_REAL_NIM_GROUP_COMPILER",
			marker: "import std / [os, strutils, sets]",
			line:   34, members: []string{"os", "strutils", "sets"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Nim grouped source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 16000 {
				t.Fatalf("original Nim grouped source unavailable or oversized: %v %v", info, err)
			}
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(source), tc.marker) {
				t.Fatal("original Nim grouped import oracle drift")
			}
			originalLines := strings.Split(string(source), "\n")
			originalLine := originalLines[tc.line-1]
			h := NewHandler([]string{filepath.Dir(path)})
			failure, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "nim",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || out.Truncated || len(out.Digests) != 1 ||
				out.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original Nim group MCP digest failed: %v %+v %+v", err, failure, out)
			}
			for _, dep := range out.Digests[0].Dependencies {
				if dep.Range.Start.Line == tc.line &&
					(strings.Contains(dep.Value, "[") || strings.Contains(dep.Value, "]")) {
					t.Fatalf("grouped Nim import was invented as one module: %+v", dep)
				}
			}
			for _, member := range tc.members {
				column := strings.Index(originalLine, member) + 1
				if column == 0 {
					t.Fatalf("source member %q missing from original line", member)
				}
				found := false
				for _, dep := range out.Digests[0].Dependencies {
					if dep.Value != "std/"+member {
						continue
					}
					if dep.Kind != "import" || dep.Alias != "" ||
						dep.Range.Start.Line != tc.line || dep.Range.End.Line != tc.line ||
						dep.Range.Start.Column != column ||
						dep.Range.End.Column-dep.Range.Start.Column != len(member) {
						t.Fatalf("incorrect Nim grouped import module/range: %+v", dep)
					}
					found = true
				}
				if !found {
					t.Fatalf("original grouped Nim module std/%s missing: %+v", member, out.Digests[0].Dependencies)
				}
			}
		})
	}
}
