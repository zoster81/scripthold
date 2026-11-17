package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDartMultipleDeclaratorsOriginalMCPNavigation(t *testing.T) {
	type expected struct {
		qualified string
		kind      string
		line      int
		excludes  string
	}
	for _, tc := range []struct {
		name, env, marker string
		wants             []expected
	}{
		{
			"flutter-cupertino", "SCRIPTHOLD_REAL_DART_MULTIDECL_FLUTTER",
			"const double kCupertinoFocusColorOpacity = 0.80,",
			[]expected{
				{"kCupertinoFocusColorOpacity", "constant", 40, "kCupertinoFocusColorBrightness"},
				{"kCupertinoFocusColorBrightness", "constant", 41, "kCupertinoFocusColorSaturation"},
				{"kCupertinoFocusColorSaturation", "constant", 42, ""},
				{"kCupertinoButtonTintedOpacityDark", "constant", 49, ""},
				{"kCupertinoButtonTapMoveSlop", "constant", 94, ""},
			},
		},
		{
			"dart-sdk-fields", "SCRIPTHOLD_REAL_DART_MULTIDECL_SDK",
			"var x = 2, y = 3, z, w = 2;",
			[]expected{
				{"A.x", "field", 10, "y ="},
				{"A.y", "field", 10, "z ="},
				{"A.z", "field", 10, ""},
				{"B.x", "field", 14, "y ="},
				{"B.y", "field", 14, "z,"},
				{"B.z", "field", 14, "w ="},
				{"B.w", "field", 14, ""},
				{"main", "function", 32, ""},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independently authored upstream Dart source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 12000 {
				t.Fatalf("original source missing/nonregular/oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("pinned independent Dart original marker drift")
			}
			lines := strings.Split(string(body), "\n")
			h := NewHandler([]string{filepath.Dir(path)})
			failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "dart",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || outlined.Truncated || len(outlined.OutlineFiles) != 1 ||
				outlined.OutlineFiles[0].SourceFingerprint == "" {
				t.Fatalf("Dart original outline failed: %v %+v %+v", err, failed, outlined)
			}
			file := outlined.OutlineFiles[0]
			nodes := flattenDartExtensionTypeOriginalOutline(file.Symbols)
			for _, want := range tc.wants {
				name := want.qualified
				if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
					name = name[dot+1:]
				}
				if want.line < 1 || want.line > len(lines) || !strings.Contains(lines[want.line-1], name) {
					t.Fatalf("source oracle drift for %s at line %d", want.qualified, want.line)
				}
				var found *SourceOutlineNode
				for _, node := range nodes {
					if node.QualifiedName == want.qualified && node.Line == want.line &&
						string(node.Kind) == want.kind {
						found = node
						break
					}
				}
				if found == nil {
					t.Errorf("original %s %q missing from MCP outline line %d", want.kind, want.qualified, want.line)
					continue
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "dart", Encoding: "utf-8",
					SymbolID: found.SymbolID, SourceFingerprint: file.SourceFingerprint,
					MaxBytes: 4096,
				})
				if err != nil || failed != nil || shown.Show == nil ||
					!strings.Contains(shown.Show.Text, name) {
					t.Errorf("fingerprint-bound show missing for %s: %v %+v", want.qualified, err, failed)
					continue
				}
				if want.excludes != "" && strings.Contains(shown.Show.Text, want.excludes) {
					t.Errorf("show %s incorrectly includes sibling %q", want.qualified, want.excludes)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "dart",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Errorf("original Dart digest failed: %v %+v", err, failed)
			}
		})
	}
}
