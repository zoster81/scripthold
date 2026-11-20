package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The local MCP digest must preserve Nim module and alias as independent
// source evidence rather than inventing one concatenated module name.
func TestNimOriginalAliasedImportDigest(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker, module, alias string
		line                             int
	}{
		{
			name: "bu", env: "SCRIPTHOLD_REAL_NIM_ALIAS_BU",
			marker: "memfiles as mf", module: "memfiles", alias: "mf", line: 11,
		},
		{
			name: "compiler", env: "SCRIPTHOLD_REAL_NIM_ALIAS_COMPILER",
			marker: "import lineinfos as astli", module: "lineinfos", alias: "astli", line: 36,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("original independent Nim alias source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 ||
				info.Size() > 16000 {
				t.Fatalf("original Nim alias source missing or oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("original Nim alias oracle drift")
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failure, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "nim",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || out.Truncated || len(out.Digests) != 1 ||
				out.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original Nim MCP digest failed: %v %+v %+v", err, failure, out)
			}
			found := false
			for _, dep := range out.Digests[0].Dependencies {
				if strings.Contains(dep.Value, "asmf") || strings.Contains(dep.Value, "asastli") {
					t.Fatalf("Nim imported local alias folded into module dependency: %+v", dep)
				}
				if dep.Value != tc.module {
					continue
				}
				if dep.Kind != "import" || dep.Alias != tc.alias ||
					dep.Range.Start.Line != tc.line || dep.Range.End.Line != tc.line ||
					dep.Range.End.Column-dep.Range.Start.Column != len(tc.module) {
					t.Fatalf("Nim MCP alias location mismatch for %s: %+v", tc.name, dep)
				}
				found = true
			}
			if !found {
				t.Fatalf("missing original Nim module dependency %q as %q: %+v",
					tc.module, tc.alias, out.Digests[0].Dependencies)
			}
		})
	}
}
