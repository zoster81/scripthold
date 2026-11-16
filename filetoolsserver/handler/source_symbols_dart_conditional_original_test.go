package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Conditional Dart import/export has runtime-specific URI alternatives. The
// MCP digest must not report complete dependency coverage for those files.
func TestDartOriginalConditionalURIDigestHonestCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker, defaultURI string
		line                          int
	}{
		{"hive", "SCRIPTHOLD_REAL_DART_COND_HIVE",
			"if (dart.library.html) 'package:hive/src/impl/isolate_stub.dart'",
			"dart:isolate", 6},
		{"dart-sass", "SCRIPTHOLD_REAL_DART_COND_SASS",
			"if (dart.library.io) 'io/vm.dart'",
			"io/interface.dart", 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Dart source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 10000 {
				t.Fatalf("original conditional Dart source unavailable/unbounded: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("original conditional URI oracle drift")
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failure, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "dart", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || out.Truncated || len(out.Digests) != 1 ||
				out.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Dart conditional MCP digest unavailable: %v %+v %+v", err, failure, out)
			}
			if out.CoverageComplete || out.Digests[0].CoverageComplete {
				t.Fatalf("MCP falsely claims all Dart conditional URI alternatives resolved: %+v", out.Digests[0])
			}
			found := false
			for _, dep := range out.Digests[0].Dependencies {
				if dep.Value == tc.defaultURI && dep.Range.Start.Line == tc.line {
					found = true
				}
			}
			if !found {
				t.Fatalf("authored default Dart URI absent: %+v", out.Digests[0].Dependencies)
			}
		})
	}
}
