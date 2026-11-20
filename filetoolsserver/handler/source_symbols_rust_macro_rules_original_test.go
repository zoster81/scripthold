package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRustMacroRulesOriginalMCPNavigation(t *testing.T) {
	type want struct {
		name    string
		line    int
		exclude string
	}
	for _, tc := range []struct {
		name, env, marker string
		wants             []want
	}{
		{"tokio", "SCRIPTHOLD_REAL_RUST_MACRO_TOKIO",
			"macro_rules! assert_ready_none {",
			[]want{{"assert_ready_none", 9, "fn pin_box"}}},
		{"rustc", "SCRIPTHOLD_REAL_RUST_MACRO_RUSTC",
			"macro_rules! impls_dyn_send_neg {",
			[]want{
				{"impls_dyn_send_neg", 21, "impls_dyn_send_neg!("},
				{"already_send", 54, "already_send!("},
				{"impl_dyn_send", 73, "impl_dyn_send!("},
				{"impls_dyn_sync_neg", 101, "impls_dyn_sync_neg!("},
				{"already_sync", 135, "already_sync!("},
				{"impl_dyn_sync", 156, "impl_dyn_sync!("},
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("full independent original Rust macro source not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 100 || fi.Size() > 15000 {
				t.Fatalf("invalid original Rust source %v %v", fi, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tc.marker) {
				t.Fatal("authored upstream marker drift")
			}
			lines := strings.Split(string(data), "\n")
			h := NewHandler([]string{filepath.Dir(path)})
			failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "rust", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || out.Truncated || len(out.OutlineFiles) != 1 || out.OutlineFiles[0].SourceFingerprint == "" {
				t.Fatalf("original Rust macro outline failed %v %+v %+v", err, failed, out)
			}
			file := out.OutlineFiles[0]
			nodes := flattenDartExtensionTypeOriginalOutline(file.Symbols)
			for _, w := range tc.wants {
				if w.line < 1 || w.line > len(lines) || !strings.Contains(lines[w.line-1], "macro_rules! "+w.name) {
					t.Fatalf("independent original Rust line drift %+v", w)
				}
				var found *SourceOutlineNode
				for _, n := range nodes {
					if n.QualifiedName == w.name && n.Line == w.line && string(n.Kind) == "entity" {
						found = n
						break
					}
				}
				if found == nil {
					t.Errorf("original macro %s missing/wrong in outline", w.name)
					continue
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "rust", Encoding: "utf-8",
					SymbolID: found.SymbolID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
				})
				if err != nil || failed != nil || shown.Show == nil ||
					!strings.Contains(shown.Show.Text, "macro_rules! "+w.name) {
					t.Errorf("original fingerprint-bound macro show failed %s: %v %+v", w.name, err, failed)
					continue
				}
				if strings.Contains(shown.Show.Text, w.exclude) {
					t.Errorf("macro %s swallowed next invocation/function", w.name)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "rust",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || digest.Truncated || len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Errorf("original Rust macro digest failed: %v %+v", err, failed)
			}
		})
	}
}
