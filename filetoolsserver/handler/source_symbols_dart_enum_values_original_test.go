package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDartEnumValuesOriginalMCPNavigation(t *testing.T) {
	type want struct {
		qname    string
		name     string
		line     int
		neighbor string
	}
	for _, tc := range []struct {
		name, env, marker string
		wants             []want
		neighborName      string
		neighborLine      int
	}{
		{
			name: "flutter-material-card", env: "SCRIPTHOLD_REAL_DART_ENUM_FLUTTER",
			marker: "enum _CardVariant { elevated, filled, outlined }",
			wants: []want{
				{"_CardVariant.elevated", "elevated", 18, "filled"},
				{"_CardVariant.filled", "filled", 18, "outlined"},
				{"_CardVariant.outlined", "outlined", 18, "Card"},
			},
			neighborName: "Card", neighborLine: 73,
		},
		{
			name: "dart-sdk-dot-shorthand", env: "SCRIPTHOLD_REAL_DART_ENUM_SDK",
			marker: "enum Color { red, blue, green }",
			wants: []want{
				{"Color.red", "red", 5, "blue"},
				{"Color.blue", "blue", 5, "green"},
				{"Color.green", "green", 5, "void main"},
			},
			neighborName: "main", neighborLine: 7,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent complete Dart enum original not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 100 || fi.Size() > 20000 {
				t.Fatalf("invalid Dart original: %v %v", fi, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("original source marker drift")
			}
			lines := strings.Split(string(body), "\n")
			h := NewHandler([]string{filepath.Dir(path)})
			failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "dart",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || out.Truncated || len(out.OutlineFiles) != 1 ||
				out.OutlineFiles[0].SourceFingerprint == "" {
				t.Fatalf("enum original MCP outline failed: %v %+v %+v", err, failed, out)
			}
			file := out.OutlineFiles[0]
			nodes := flattenDartExtensionTypeOriginalOutline(file.Symbols)
			neighborFound := false
			for _, n := range nodes {
				if n.QualifiedName == tc.neighborName && n.Line == tc.neighborLine {
					neighborFound = true
				}
			}
			if !neighborFound {
				t.Errorf("original adjacent %s lost at line %d", tc.neighborName, tc.neighborLine)
			}
			for _, w := range tc.wants {
				if w.line < 1 || w.line > len(lines) || !strings.Contains(lines[w.line-1], w.name) {
					t.Fatalf("Dart authored enum value line drift %+v", w)
				}
				var found *SourceOutlineNode
				for _, n := range nodes {
					if n.QualifiedName == w.qname && n.Line == w.line && string(n.Kind) == "constant" {
						found = n
						break
					}
				}
				if found == nil {
					t.Errorf("Dart enum value absent/wrong %s at %d", w.qname, w.line)
					continue
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "dart", Encoding: "utf-8",
					SymbolID: found.SymbolID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
				})
				if err != nil || failed != nil || shown.Show == nil {
					t.Errorf("fingerprint-bound enum value show failed for %s: %v %+v", w.qname, err, failed)
					continue
				}
				value := strings.TrimSpace(shown.Show.Text)
				if value != w.name || strings.Contains(value, w.neighbor) {
					t.Errorf("enum show %s lost source isolation; excerpt=%q", w.qname, value)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "dart",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Errorf("enum original MCP digest failed: %v %+v", err, failed)
			}
		})
	}
}
