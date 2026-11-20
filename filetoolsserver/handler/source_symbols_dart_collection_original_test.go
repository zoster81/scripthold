package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDartCollectionLiteralOriginalMCPNavigation(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker string
		wants             []struct {
			name, kind string
			line       int
			show       bool
		}
	}{
		{
			"flutter-build-source", "SCRIPTHOLD_REAL_DART_COLLECTION_FLUTTER",
			"const _validLeadingDirectoryPlaceholders = <String>{",
			[]struct {
				name, kind string
				line       int
				show       bool
			}{
				{"_validLeadingDirectoryPlaceholders", "constant", 10, true},
				{"ExtensionArtifact", "type", 27, true},
				{"ExtensionHostArtifact", "type", 37, true},
				{"ExtensionSource", "class", 41, false},
			},
		},
		{
			"dart-sdk-dds-validation", "SCRIPTHOLD_REAL_DART_COLLECTION_SDK",
			"const _allowedSecFetchSites = <String>{'same-origin', 'none'};",
			[]struct {
				name, kind string
				line       int
				show       bool
			}{
				{"_allowedSecFetchSites", "constant", 82, true},
				{"isAllowedSecFetchSite", "function", 100, true},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent complete original Dart source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 16000 {
				t.Fatalf("original source missing/nonregular/oversized: %v %v", info, err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(original), tc.marker) {
				t.Fatal("authored original Dart collection marker drift")
			}
			lines := strings.Split(string(original), "\n")
			h := NewHandler([]string{filepath.Dir(path)})
			failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "dart",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || outlined.Truncated || len(outlined.OutlineFiles) != 1 ||
				outlined.OutlineFiles[0].SourceFingerprint == "" {
				t.Fatalf("original Dart outline unavailable: %v %+v %+v", err, failed, outlined)
			}
			file := outlined.OutlineFiles[0]
			nodes := flattenDartExtensionTypeOriginalOutline(file.Symbols)
			for _, want := range tc.wants {
				if want.line < 1 || want.line > len(lines) ||
					!strings.Contains(lines[want.line-1], want.name) {
					t.Fatalf("authored original oracle line drift: %+v", want)
				}
				var found *SourceOutlineNode
				for _, node := range nodes {
					if node.QualifiedName == want.name && node.Line == want.line &&
						string(node.Kind) == want.kind {
						found = node
						break
					}
				}
				if found == nil {
					t.Errorf("authored %s %q not indexed at line %d", want.kind, want.name, want.line)
					continue
				}
				if !want.show {
					continue
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "dart", Encoding: "utf-8",
					SymbolID: found.SymbolID, SourceFingerprint: file.SourceFingerprint,
					MaxBytes: 4096,
				})
				if err != nil || failed != nil || shown.Show == nil ||
					!strings.Contains(shown.Show.Text, want.name) {
					t.Errorf("fingerprint-bound original show for %s unavailable: %v %+v", want.name, err, failed)
				}
			}
			for _, node := range nodes {
				if node.QualifiedName == "ExtensionSource" && node.Kind == "function" {
					t.Errorf("collection literal fabricated a function from later class: %+v", node)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "dart",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Errorf("original Dart digest unavailable: %v %+v", err, failed)
			}
		})
	}
}
