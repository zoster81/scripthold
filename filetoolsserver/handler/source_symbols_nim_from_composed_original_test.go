package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Composed Nim selective imports identify one lexical module dependency,
// anchored to the whole authored quoted-root / suffix source expression.
func TestNimOriginalComposedFromDigest(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker, module, spelling string
		line                                int
	}{
		{
			name: "compiler", env: "SCRIPTHOLD_REAL_NIM_COMPOSED_COMPILER",
			marker:   "from \"../dist/nimony/src/lib\" / bif import BifModule, containsSym",
			module:   "../dist/nimony/src/lib/bif",
			spelling: "\"../dist/nimony/src/lib\" / bif", line: 40,
		},
		{
			name: "nimble", env: "SCRIPTHOLD_REAL_NIM_COMPOSED_NIMBLE",
			marker:   "from \"$nim\" / compiler/nimblecmd import getPathVersionChecksum",
			module:   "$nim/compiler/nimblecmd",
			spelling: "\"$nim\" / compiler/nimblecmd", line: 12,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent authored Nim composed-from source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 24000 {
				t.Fatalf("original Nim source unavailable or oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(body)
			if !strings.Contains(source, tc.marker) {
				t.Fatal("composed Nim original-source oracle drift")
			}
			lines := strings.Split(source, "\n")
			column := strings.Index(lines[tc.line-1], tc.spelling) + 1
			if column < 1 {
				t.Fatal("original composed module spelling not found")
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failure, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "nim",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || out.Truncated || len(out.Digests) != 1 ||
				out.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original Nim composed MCP digest failed: %v %+v %+v", err, failure, out)
			}
			found := false
			for _, dep := range out.Digests[0].Dependencies {
				if dep.Range.Start.Line != tc.line {
					continue
				}
				if dep.Value != tc.module || dep.Kind != "import" || dep.Alias != "" ||
					dep.Range.End.Line != tc.line ||
					dep.Range.Start.Column != column ||
					dep.Range.End.Column-dep.Range.Start.Column != len(tc.spelling) {
					t.Fatalf("Nim composed selective import fabricated module or range: %+v", dep)
				}
				if found {
					t.Fatalf("duplicate source unit from a single Nim composed import")
				}
				found = true
			}
			if !found {
				t.Fatalf("Nim composed from module missing: %q, got %+v",
					tc.module, out.Digests[0].Dependencies)
			}
		})
	}
}
