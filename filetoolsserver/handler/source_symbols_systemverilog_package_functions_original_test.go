package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemVerilogPackageFunctionsTwoOriginalMCPNavigation(t *testing.T) {
	for _, tc := range []struct {
		env, marker, qualified, name, excerpt, excluded string
		line                                            int
	}{
		{"SCRIPTHOLD_REAL_SV_VORTEX", "package VX_tlb_pkg;", "VX_tlb_pkg.tlb_perm_ok", "tlb_perm_ok", "function automatic logic tlb_perm_ok", "tlb_vpn_mask", 61},
		{"SCRIPTHOLD_REAL_SV_CORAL", "package top_racl_pkg;", "top_racl_pkg.tlul_extract_racl_role_bits", "tlul_extract_racl_role_bits", "function automatic racl_role_t tlul_extract_racl_role_bits", "tlul_extract_ctn_uid_bits", 67},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independently authored original source not configured")
			}
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() < 1000 || stat.Size() > 65536 {
				t.Fatalf("invalid original source: %v %v", stat, err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(original), tc.marker) != 1 {
				t.Fatal("original package marker drift")
			}
			lines := strings.Split(string(original), "\n")
			if tc.line > len(lines) || !strings.Contains(lines[tc.line-1], tc.excerpt) {
				t.Fatalf("original authored function line drift: %s", tc.name)
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "systemverilog", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || out.Truncated || len(out.OutlineFiles) != 1 {
				t.Fatalf("source outline error=%v failed=%v files=%d", err, failed, len(out.OutlineFiles))
			}
			file := out.OutlineFiles[0]
			if file.SourceFingerprint == "" {
				t.Fatal("missing source fingerprint")
			}
			var selected *SourceOutlineNode
			for _, node := range flattenDartExtensionTypeOriginalOutline(file.Symbols) {
				if node.QualifiedName == tc.qualified && node.Line == tc.line && string(node.Kind) == "function" {
					selected = node
					break
				}
			}
			if selected == nil {
				t.Fatalf("authored package function missing: %s", tc.qualified)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "systemverilog", Encoding: "utf-8",
				SymbolID: selected.SymbolID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil {
				t.Fatalf("fingerprint-bound show failed: %v %v", err, failed)
			}
			if !strings.Contains(shown.Show.Text, tc.excerpt) || !strings.Contains(shown.Show.Text, "endfunction") ||
				strings.Contains(shown.Show.Text, tc.excluded) {
				t.Fatalf("selected function show is not isolated: %q", shown.Show.Text)
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "systemverilog", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint != file.SourceFingerprint {
				t.Fatalf("source digest/fingerprint mismatch: err=%v fail=%v", err, failed)
			}
		})
	}
}
