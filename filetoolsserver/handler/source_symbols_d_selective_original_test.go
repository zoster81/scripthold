package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The MCP digest must return authored D module paths, not selective member names
// appended to those paths. Function-local imports are a distinct scope.
func TestDSelectiveOriginalModuleImportDigest(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker, module string
		line                      int
		localOnly                 bool
	}{
		{"yes", "SCRIPTHOLD_REAL_D_SELECTIVE_YES",
			"import std.stdio : writeln;", "std.stdio", 1, false},
		{"powernex", "SCRIPTHOLD_REAL_D_SELECTIVE_POWERNEX",
			"import core.time : msecs;", "core.time", 6, false},
		{"tsv-local", "SCRIPTHOLD_REAL_D_SELECTIVE_TSV",
			"import std.process : escapeShellCommand, executeShell;", "", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original D source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 20000 {
				t.Fatalf("original D source unavailable/unbounded: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("original D dependency oracle drift")
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failure, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "d", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || out.Truncated || len(out.Digests) != 1 ||
				out.Digests[0].SourceFingerprint == "" {
				t.Fatalf("D original MCP digest unavailable: %v %+v %+v", err, failure, out)
			}
			found := false
			for _, dep := range out.Digests[0].Dependencies {
				if strings.ContainsAny(dep.Value, ":,=;") {
					t.Fatalf("invented D module dependency: %+v", dep)
				}
				if dep.Value == tc.module && dep.Range.Start.Line == tc.line {
					found = true
				}
			}
			if tc.localOnly {
				if len(out.Digests[0].Dependencies) != 0 {
					t.Fatalf("local-only D imports projected as top-level: %+v", out.Digests[0].Dependencies)
				}
			} else if !found {
				t.Fatalf("authored D module import %q line=%d absent: %+v", tc.module, tc.line, out.Digests[0].Dependencies)
			}
		})
	}
}
