package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemVerilogEnumTwoOriginalMCPNavigation(t *testing.T) {
	for _, tc := range []struct {
		env, marker, qualified, excerpt, excluded string
		line                                      int
	}{
		{"SCRIPTHOLD_REAL_SV_OPENTITAN", "package kmac_pkg;", "kmac_pkg.key_len_e.Key128", "Key128 = 3'b 000", "Key192", 63},
		{"SCRIPTHOLD_REAL_SV_AXI", "package axi_pkg;", "axi_pkg.mem_type_t.DEVICE_NONBUFFERABLE", "DEVICE_NONBUFFERABLE", "DEVICE_BUFFERABLE", 230},
	} {
		t.Run(tc.qualified, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete original SystemVerilog source not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 1000 || fi.Size() > 100000 {
				t.Fatalf("invalid original %v %v", fi, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(b)
			if !strings.Contains(source, tc.marker) {
				t.Fatal("original marker drift")
			}
			lines := strings.Split(source, "\n")
			if tc.line > len(lines) || !strings.Contains(lines[tc.line-1], tc.excerpt) {
				t.Fatal("independent authored line drift")
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "systemverilog", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || out.Truncated || len(out.OutlineFiles) != 1 {
				t.Fatalf("original outline failed: err=%v failed=%v files=%d truncated=%v", err, failed != nil, len(out.OutlineFiles), out.Truncated)
			}
			file := out.OutlineFiles[0]
			if file.SourceFingerprint == "" {
				t.Fatal("missing original fingerprint")
			}
			var selected *SourceOutlineNode
			for _, n := range flattenDartExtensionTypeOriginalOutline(file.Symbols) {
				if n.QualifiedName == tc.qualified && n.Line == tc.line && string(n.Kind) == "constant" {
					selected = n
					break
				}
			}
			if selected == nil {
				t.Fatalf("authored member missing from outline: %s", tc.qualified)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "systemverilog", Encoding: "utf-8",
				SymbolID: selected.SymbolID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || failed != nil || shown.Show == nil {
				t.Fatalf("member show failed: %v %v", err, failed)
			}
			if !strings.Contains(shown.Show.Text, tc.excerpt) || strings.Contains(shown.Show.Text, tc.excluded) {
				t.Errorf("member show not isolated from neighbor: %q", shown.Show.Text)
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "systemverilog", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || digest.Truncated || len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Errorf("original digest failed: err=%v failed=%v", err, failed)
			}
		})
	}
}
