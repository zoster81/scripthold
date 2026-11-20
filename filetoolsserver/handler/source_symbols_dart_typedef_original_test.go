package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDartTypedefOriginalMCPNavigation(t *testing.T) {
	type want struct {
		name              string
		line              int
		excerpt, excludes string
	}
	for _, tc := range []struct {
		name, env, marker string
		wants             []want
	}{
		{
			"flutter-foundation", "SCRIPTHOLD_REAL_DART_TYPEDEF_FLUTTER",
			"typedef ValueChanged<T> = void Function(T value);",
			[]want{
				{"ValueChanged", 17, "void Function(T value)", "ValueSetter"},
				{"ValueGetter", 39, "T Function()", "IterableFilter"},
				{"AsyncCallback", 52, "Future<void> Function()", "AsyncValueSetter"},
				{"AsyncValueGetter", 69, "Future<T> Function()", "CachingIterable"},
			},
		},
		{
			"dart-sdk-annotated", "SCRIPTHOLD_REAL_DART_TYPEDEF_SDK",
			"typedef void G();",
			[]want{
				{"F", 10, "void Function()", "typedef void G()"},
				{"G", 13, "typedef void G()", "main()"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent complete Dart original not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 15000 {
				t.Fatalf("invalid original source: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("source marker drift")
			}
			lines := strings.Split(string(body), "\n")
			h := NewHandler([]string{filepath.Dir(path)})
			failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "dart",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || out.Truncated || len(out.OutlineFiles) != 1 ||
				out.OutlineFiles[0].SourceFingerprint == "" {
				t.Fatalf("Dart original outline: %v %+v %+v", err, failed, out)
			}
			file := out.OutlineFiles[0]
			nodes := flattenDartExtensionTypeOriginalOutline(file.Symbols)
			for _, w := range tc.wants {
				if w.line < 1 || w.line > len(lines) || !strings.Contains(lines[w.line-1], w.name) {
					t.Fatalf("authored source drift %+v", w)
				}
				var found *SourceOutlineNode
				for _, node := range nodes {
					if node.QualifiedName == w.name && node.Line == w.line && string(node.Kind) == "alias" {
						found = node
						break
					}
				}
				if found == nil {
					t.Errorf("Dart original typedef %s absent/misclassified in MCP outline at %d", w.name, w.line)
					continue
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "dart", Encoding: "utf-8",
					SymbolID: found.SymbolID, SourceFingerprint: file.SourceFingerprint,
					MaxBytes: 4096,
				})
				if err != nil || failed != nil || shown.Show == nil ||
					!strings.Contains(shown.Show.Text, w.name) ||
					!strings.Contains(shown.Show.Text, w.excerpt) {
					t.Errorf("fingerprint-bound typedef show %s failed: %v %+v", w.name, err, failed)
					continue
				}
				if w.excludes != "" && strings.Contains(shown.Show.Text, w.excludes) {
					t.Errorf("typedef show %s swallowed adjacent %s", w.name, w.excludes)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "dart",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Errorf("typedef original MCP digest missing: %v %+v", err, failed)
			}
		})
	}
}
