package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The local MCP digest reports source modules, not selected Nim imported names.
func TestNimOriginalFromImportDigest(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker, module string
		line                      int
	}{
		{"aporia", "SCRIPTHOLD_REAL_NIM_FROM_APORIA",
			"from gdk2 import WINDOW_TYPE_HINT_MENU", "gdk2", 2},
		{"cligen", "SCRIPTHOLD_REAL_NIM_FROM_CLIGEN",
			"from std/strutils import toUpperAscii", "std/strutils", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("original independent Nim source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 50 || info.Size() > 16000 {
				t.Fatalf("original Nim source unavailable or unbounded: %v %v", info, err)
			}
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(source), tc.marker) {
				t.Fatal("Nim source oracle drift")
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failure, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "nim", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || out.Truncated || len(out.Digests) != 1 ||
				out.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original Nim MCP digest unavailable: %v %+v %+v", err, failure, out)
			}
			found := false
			for _, dep := range out.Digests[0].Dependencies {
				if strings.Contains(dep.Value, " import ") || strings.Contains(dep.Value, "toUpperAscii") ||
					strings.Contains(dep.Value, "WINDOW_TYPE_HINT_MENU") {
					t.Fatalf("fabricated Nim imported-symbol dependency: %+v", dep)
				}
				if dep.Value == tc.module && dep.Kind == "import" && dep.Alias == "" &&
					dep.Range.Start.Line == tc.line && dep.Range.End.Line == tc.line {
					found = true
				}
			}
			if !found {
				t.Fatalf("Nim original source-unit dependency missing: %+v", out.Digests[0].Dependencies)
			}
		})
	}
}
