package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDartExternalGetterOriginalMCPNavigation(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker string
		names             []string
	}{
		{"dart-core", "SCRIPTHOLD_REAL_DART_EXTENSION_CORE", "external String? get userAgent;",
			[]string{"Navigator.userAgent", "Navigator.appVersion", "navigator"}},
		{"flutter", "SCRIPTHOLD_REAL_DART_EXTENSION_FLUTTER_HOST", "external JSFunction get resume;",
			[]string{"testRunner", "_JSApi.resume", "_JSApi.restartCurrent"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent complete original source not configured")
			}
			body, err := os.ReadFile(path)
			if err != nil || len(body) < 100 || len(body) > 16000 || !strings.Contains(string(body), tc.marker) {
				t.Fatalf("original source unavailable or drifted: %v", err)
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "dart",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || outlined.Truncated || len(outlined.OutlineFiles) != 1 {
				t.Fatalf("Dart original outline failure: %v %+v", err, failed)
			}
			file := outlined.OutlineFiles[0]
			nodes := flattenDartExtensionTypeOriginalOutline(file.Symbols)
			for _, qualified := range tc.names {
				var getter *SourceOutlineNode
				for _, node := range nodes {
					if node.QualifiedName == qualified && node.Kind == "property" {
						getter = node
						break
					}
				}
				if getter == nil {
					t.Errorf("external getter %s absent or not property in MCP outline", qualified)
					continue
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "dart", Encoding: "utf-8",
					SymbolID: getter.SymbolID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 512,
				})
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "get ") {
					t.Errorf("fingerprint-bound getter show unavailable for %s: %v %+v", qualified, err, failed)
				}
			}
		})
	}
}
