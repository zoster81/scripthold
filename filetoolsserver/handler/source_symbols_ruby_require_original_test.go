package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Local MCP digest must not export a string inside a computed Ruby require
// expression as a source-known module dependency.
func TestRubyOriginalDynamicRequireDigestEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker string
	}{
		{"jit-benchmarks", "SCRIPTHOLD_REAL_RUBY_JIT_DC",
			"require File.join(File.dirname(__FILE__), \"tools\", \"cli\")"},
		{"amp", "SCRIPTHOLD_REAL_RUBY_AMP_BIN",
			"require File.join(File.dirname(follow_link(__FILE__)), \"..\", \"lib\", \"amp\")"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Ruby source not configured")
			}
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() < 100 || stat.Size() > 50000 {
				t.Fatalf("independent Ruby source missing/unbounded: %v %v", stat, err)
			}
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(bytes), tc.marker) {
				t.Fatal("original Ruby source oracle drift")
			}
			h, ctx := NewHandler([]string{filepath.Dir(path)}), context.Background()
			failure, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "ruby",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || digest.Truncated || len(digest.Digests) != 1 {
				t.Fatalf("Ruby original MCP digest unavailable: %v %+v %+v", err, failure, digest)
			}
			if len(digest.Digests[0].Dependencies) != 0 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("computed Ruby module path claimed as real dependency: %+v", digest.Digests[0])
			}
		})
	}
}
