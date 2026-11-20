package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Groovy import aliases are local names; the MCP dependency target must
// remain the authored fully qualified type, with the alias kept separately.
func TestGroovyOriginalImportAliasDigestEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker string
		line              int
	}{
		{"jyreport", "SCRIPTHOLD_REAL_GROOVY_JYREPORT", "import java.sql.Date as SqlDate", 3},
		{"matrix", "SCRIPTHOLD_REAL_GROOVY_MATRIX", "import java.sql.Date as SqlDate", 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Groovy source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 20000 {
				t.Fatalf("source missing/unbounded: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("original Groovy alias oracle drift")
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failure, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "groovy", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || out.Truncated || len(out.Digests) != 1 ||
				out.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Groovy original MCP digest unavailable: %v %+v %+v", err, failure, out)
			}
			found := false
			for _, dep := range out.Digests[0].Dependencies {
				if strings.Contains(dep.Value, "asSqlDate") {
					t.Fatalf("invented Groovy import target: %+v", dep)
				}
				if dep.Value == "java.sql.Date" && dep.Alias == "SqlDate" && dep.Range.Start.Line == tc.line {
					found = true
				}
			}
			if !found {
				t.Fatalf("authored Groovy import+alias missing from MCP output %+v", out.Digests[0].Dependencies)
			}
		})
	}
}
