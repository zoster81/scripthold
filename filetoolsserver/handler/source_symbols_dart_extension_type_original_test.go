package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDartExtensionTypeOriginalMCPNavigation(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker string
		want              []struct {
			name string
			line int
		}
	}{
		{
			"dart-core", "SCRIPTHOLD_REAL_DART_EXTENSION_CORE",
			"extension type Navigator(JSObject _) {",
			[]struct {
				name string
				line int
			}{{"Navigator", 11}},
		},
		{
			"flutter-web-host", "SCRIPTHOLD_REAL_DART_EXTENSION_FLUTTER_HOST",
			"extension type _TestRunner(JSObject _) implements JSObject {",
			[]struct {
				name string
				line int
			}{{"_TestRunner", 17}, {"_JSApi", 29}},
		},
		{
			"dart-sdk-inline-const", "SCRIPTHOLD_REAL_DART_EXTENSION_SDK_INLINE",
			"extension type const ExtensionType1(int it) {}",
			[]struct {
				name string
				line int
			}{{"ExtensionType1", 1}, {"ExtensionType2", 2}, {"ExtensionType3", 3},
				{"ExtensionType4", 4}, {"ExtensionType5", 22}, {"ExtensionType6", 23}, {"ExtensionType7", 24}},
		},
		{
			"dart-sdk-generic-const", "SCRIPTHOLD_REAL_DART_EXTENSION_SDK_GENERIC",
			"extension type const ExtensionType<T>(T id) {}",
			[]struct {
				name string
				line int
			}{{"ExtensionType", 9}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independently authored upstream Dart original not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 16000 {
				t.Fatalf("original Dart source missing/nonregular/oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(body)
			if !strings.Contains(source, tc.marker) {
				t.Fatal("authored Dart original source marker drift")
			}
			lines := strings.Split(source, "\n")
			h := NewHandler([]string{filepath.Dir(path)})
			failed, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "dart",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || result.Truncated || len(result.OutlineFiles) != 1 ||
				result.OutlineFiles[0].SourceFingerprint == "" {
				t.Fatalf("original Dart outline unavailable: err=%v failure=%+v result=%+v", err, failed, result)
			}
			nodes := flattenDartExtensionTypeOriginalOutline(result.OutlineFiles[0].Symbols)
			for _, wanted := range tc.want {
				if wanted.line < 1 || wanted.line > len(lines) {
					t.Fatalf("authored Dart oracle line outside original source: %+v", wanted)
				}
				col := strings.Index(lines[wanted.line-1], "extension type") + 1
				if col < 1 || !strings.Contains(lines[wanted.line-1], wanted.name) {
					t.Fatalf("authored Dart declaration %q missing from line %d", wanted.name, wanted.line)
				}
				var found *SourceOutlineNode
				for _, node := range nodes {
					if node.Kind == "type" && node.QualifiedName == wanted.name &&
						node.Line == wanted.line && node.Column == col {
						found = node
						break
					}
				}
				if found == nil {
					t.Errorf("Dart extension type %q missing from public outline at %d:%d", wanted.name, wanted.line, col)
					continue
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "dart", Encoding: "utf-8",
					SymbolID: found.SymbolID, SourceFingerprint: result.OutlineFiles[0].SourceFingerprint,
					MaxBytes: 4096,
				})
				if err != nil || failed != nil || shown.Show == nil ||
					!strings.Contains(shown.Show.Text, "extension type") ||
					!strings.Contains(shown.Show.Text, wanted.name) {
					t.Errorf("Dart authored extension type %q fingerprint show missing: %v %+v %+v", wanted.name, failed, err, shown.Show)
				}
			}
			for _, node := range nodes {
				if node.QualifiedName == "type" || node.QualifiedName == "const" ||
					strings.HasPrefix(node.QualifiedName, "type.") {
					t.Errorf("Dart contextual keyword invented as public declaration: %+v", node)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "dart",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Dart original digest missing: err=%v failed=%+v digest=%+v", err, failed, digest)
			}
			count := 0
			for _, category := range digest.Digests[0].DeclarationCounts {
				if category.Kind == "type" {
					count += category.Count
				}
			}
			if count < len(tc.want) {
				t.Errorf("Dart extension type digest inventory missing: got %d need %d", count, len(tc.want))
			}
		})
	}
}

func flattenDartExtensionTypeOriginalOutline(nodes []*SourceOutlineNode) []*SourceOutlineNode {
	out := make([]*SourceOutlineNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, node)
		out = append(out, flattenDartExtensionTypeOriginalOutline(node.Children)...)
	}
	return out
}
