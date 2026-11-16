package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Source symbols must expose the imported Dart URI and its local prefix as
// distinct source-backed fields, not infer the prefix from runtime behavior.
func TestDartOriginalImportPrefixDigestEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker, target, prefix string
		line                              int
	}{
		{"google-photos", "SCRIPTHOLD_REAL_DART_GPH",
			"import 'package:gpth/interactive.dart' as interactive;",
			"package:gpth/interactive.dart", "interactive", 4},
		{"dartx", "SCRIPTHOLD_REAL_DART_DARTX",
			"import 'package:collection/collection.dart' as collection;",
			"package:collection/collection.dart", "collection", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Dart source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 20000 {
				t.Fatalf("original Dart source missing/unbounded: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("original Dart prefix oracle drift")
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failure, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "dart", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || out.Truncated || len(out.Digests) != 1 ||
				out.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Dart MCP digest unavailable: %v %+v %+v", err, failure, out)
			}
			found := false
			for _, dep := range out.Digests[0].Dependencies {
				if dep.Value == tc.target && dep.Alias == tc.prefix &&
					dep.Range.Start.Line == tc.line && dep.Range.End.Line == tc.line {
					found = true
				}
			}
			if !found {
				t.Fatalf("Dart URI and authored local prefix missing: %+v", out.Digests[0].Dependencies)
			}
		})
	}
}
