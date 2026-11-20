package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The local MCP digest must expose only authored literal require targets.
// Format strings from dynamic module loads are not valid dependency facts.
func TestLuaOriginalRequireDigestDependencyEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, env, needle string
		expected          []string
	}{
		{"overseer.nvim", "SCRIPTHOLD_REAL_LUA_OVERSEER",
			"require(string.format(\"overseer.strategy.%s\", name))", []string{"overseer.util"}},
		{"zenbones.nvim", "SCRIPTHOLD_REAL_LUA_ZENBONES",
			"require(string.format(\"lualine.themes.%s_%s\", name, ness))", []string{"lush", "zenbones.term"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Lua source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 100000 {
				t.Fatalf("original Lua file unavailable/unbounded: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.needle) {
				t.Fatal("independent Lua source oracle drift")
			}
			ctx, h := context.Background(), NewHandler([]string{filepath.Dir(path)})
			failure, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "lua",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Lua MCP digest unavailable: %v %+v %+v", err, failure, digest)
			}
			got := make([]string, 0, len(digest.Digests[0].Dependencies))
			for _, dep := range digest.Digests[0].Dependencies {
				got = append(got, dep.Value)
			}
			if len(got) != len(tc.expected) {
				t.Fatalf("Lua MCP dependencies=%q want exactly %q", got, tc.expected)
			}
			for i := range tc.expected {
				if got[i] != tc.expected[i] {
					t.Fatalf("Lua MCP dependencies=%q want exactly %q", got, tc.expected)
				}
			}
		})
	}
}
