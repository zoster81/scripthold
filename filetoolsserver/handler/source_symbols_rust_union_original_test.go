package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRustUnionTwoOriginalMCPNavigation(t *testing.T) {
	type want struct {
		name     string
		line     int
		kind     string
		excerpt  string
		excluded string
	}
	for _, tc := range []struct {
		name, env, marker string
		wants             []want
	}{
		{
			name: "embassy", env: "SCRIPTHOLD_REAL_RUST_UNION_EMBASSY", marker: "pub union KbOptionsParams {",
			wants: []want{
				{"KbOptionsParams", 71, "type", "pub union KbOptionsParams", "pub struct KbOptions"},
				{"KbOptionsParams.authenticate", 74, "field", "pub authenticate: KbAuthenticate", "pub load_sb:"},
				{"KbOptionsParams.load_sb", 75, "field", "pub load_sb: KbLoadSb", "pub struct KbOptions"},
			},
		},
		{
			name: "windows-rs", env: "SCRIPTHOLD_REAL_RUST_UNION_WINDOWS", marker: "pub union IMAGE_COR20_HEADER_0 {",
			wants: []want{
				{"IMAGE_COR20_HEADER_0", 24, "type", "pub union IMAGE_COR20_HEADER_0", "impl Default for IMAGE_COR20_HEADER_0"},
				{"IMAGE_COR20_HEADER_0.EntryPointToken", 27, "field", "pub EntryPointToken: DWORD", "pub EntryPointRVA:"},
				{"IMAGE_COR20_HEADER_0.EntryPointRVA", 28, "field", "pub EntryPointRVA: DWORD", "impl Default for IMAGE_COR20_HEADER_0"},
				{"IMAGE_SECTION_HEADER_0", 190, "type", "pub union IMAGE_SECTION_HEADER_0", "impl Default for IMAGE_SECTION_HEADER_0"},
				{"IMAGE_SECTION_HEADER_0.PhysicalAddress", 193, "field", "pub PhysicalAddress: DWORD", "pub VirtualSize:"},
				{"IMAGE_SECTION_HEADER_0.VirtualSize", 194, "field", "pub VirtualSize: DWORD", "impl Default for IMAGE_SECTION_HEADER_0"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent complete original Rust union source not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 100 || fi.Size() > 1<<16 {
				t.Fatalf("invalid source %v %v", fi, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tc.marker) {
				t.Fatal("original source marker drift")
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "rust", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || out.Truncated || len(out.OutlineFiles) != 1 || out.OutlineFiles[0].SourceFingerprint == "" {
				t.Fatalf("original Rust union outline failed: %v %+v %+v", err, failed, out)
			}
			file := out.OutlineFiles[0]
			nodes := flattenDartExtensionTypeOriginalOutline(file.Symbols)
			for _, w := range tc.wants {
				var found *SourceOutlineNode
				for _, n := range nodes {
					if n.QualifiedName == w.name && n.Line == w.line && string(n.Kind) == w.kind {
						found = n
						break
					}
				}
				if found == nil {
					t.Errorf("original %s %s line %d missing", w.kind, w.name, w.line)
					continue
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "rust", Encoding: "utf-8", SymbolID: found.SymbolID,
					SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
				})
				if err != nil || failed != nil || shown.Show == nil {
					t.Errorf("original Rust union show %s failed %v %+v", w.name, err, failed)
					continue
				}
				if !strings.Contains(shown.Show.Text, w.excerpt) || strings.Contains(shown.Show.Text, w.excluded) {
					t.Errorf("original Rust union show %s inaccurate excerpt: %q", w.name, shown.Show.Text)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "rust", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || digest.Truncated || len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Errorf("original Rust union digest failed: %v %+v", err, failed)
			}
		})
	}
}
