package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An except list excludes imported members; the MCP source dependency digest
// must identify only the authored module, with its exact lexical source span.
func TestNimOriginalExceptImportDigest(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker, module, spelling string
		line                                int
	}{
		{
			name: "nimble", env: "SCRIPTHOLD_REAL_NIM_EXCEPT_NIMBLE",
			marker: "import system except TResult",
			module: "system", spelling: "system", line: 7,
		},
		{
			name: "compiler", env: "SCRIPTHOLD_REAL_NIM_EXCEPT_COMPILER",
			marker:   "import \"../dist/nimony/src/lib/nifcore\" except symName, strVal, poolSym, poolStr, lineInfoFile",
			module:   "../dist/nimony/src/lib/nifcore",
			spelling: "\"../dist/nimony/src/lib/nifcore\"", line: 39,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Nim except source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 ||
				info.Size() > 16000 {
				t.Fatalf("original Nim except source missing/oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(body)
			if !strings.Contains(source, tc.marker) {
				t.Fatal("original Nim except source marker drift")
			}
			lines := strings.Split(source, "\n")
			column := strings.Index(lines[tc.line-1], tc.spelling) + 1
			if column < 1 {
				t.Fatal("authored Nim except module source token not present")
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failure, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "nim",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || out.Truncated || len(out.Digests) != 1 ||
				out.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Nim except original MCP digest failed: %v %+v %+v", err, failure, out)
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
					t.Fatalf("Nim except invented dependency or wrong authored range: %+v", dep)
				}
				if found {
					t.Fatalf("duplicate Nim except source dependency on line %d", tc.line)
				}
				found = true
			}
			if !found {
				t.Fatalf("missing Nim except source module %q: %+v", tc.module, out.Digests[0].Dependencies)
			}
		})
	}
}
