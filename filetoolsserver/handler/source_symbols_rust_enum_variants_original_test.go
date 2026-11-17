package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRustEnumVariantsTwoOriginalMCPNavigation(t *testing.T) {
	type want struct {
		qualified         string
		line              int
		excerpt, excluded string
	}
	for _, tc := range []struct {
		name, env, marker string
		variants          []want
	}{
		{"tokio", "SCRIPTHOLD_REAL_RUST_ENUM_TOKIO", "pub enum Either<L, R> {",
			[]want{{"Either.Left", 66, "Left(L)", "Right(R)"},
				{"Either.Right", 67, "Right(R)", "macro_rules! delegate_call"}}},
		{"serde-json", "SCRIPTHOLD_REAL_RUST_ENUM_SERDE", "pub enum ErrorKind {",
			[]want{{"ErrorKind.Other", 9, "Other", "pub struct Error"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete original Rust enum source not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 100 || fi.Size() > 20000 {
				t.Fatalf("original source invalid %v %v", fi, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(data), "\n")
			if !strings.Contains(string(data), tc.marker) {
				t.Fatal("original source marker drift")
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "rust", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || out.Truncated || len(out.OutlineFiles) != 1 || out.OutlineFiles[0].SourceFingerprint == "" {
				t.Fatalf("original enum outline failed: %v %+v %+v", err, failed, out)
			}
			file := out.OutlineFiles[0]
			nodes := flattenDartExtensionTypeOriginalOutline(file.Symbols)
			for _, w := range tc.variants {
				if w.line < 1 || w.line > len(lines) || !strings.Contains(lines[w.line-1], w.excerpt) {
					t.Fatalf("original author-line oracle drift: %+v", w)
				}
				var found *SourceOutlineNode
				for _, n := range nodes {
					if n.QualifiedName == w.qualified && n.Line == w.line && string(n.Kind) == "constant" {
						found = n
						break
					}
				}
				if found == nil {
					t.Errorf("original Rust enum variant %s missing from MCP outline", w.qualified)
					continue
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "rust", Encoding: "utf-8",
					SymbolID: found.SymbolID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
				})
				if err != nil || failed != nil || shown.Show == nil {
					t.Errorf("original enum show %s failed %v %+v", w.qualified, err, failed)
					continue
				}
				if !strings.Contains(shown.Show.Text, w.excerpt) || strings.Contains(shown.Show.Text, w.excluded) {
					t.Errorf("original isolated enum variant %s inaccurate excerpt: %q", w.qualified, shown.Show.Text)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "rust", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || digest.Truncated || len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Errorf("original Rust enum digest failed: %v %+v", err, failed)
			}
		})
	}
}
