package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemVerilogPackageTasksTwoOriginalMCPNavigation(t *testing.T) {
	for _, tc := range []struct {
		env, marker, qualified, excerpt, closing, excluded string
		line                                               int
	}{
		{
			"SCRIPTHOLD_REAL_SV_TASK_VORTEX", "package VX_dxa_pkg;",
			"VX_dxa_pkg.trace_ex_op", "task automatic trace_ex_op", "endtask",
			"dxa_tiled_dest_byte", 171,
		},
		{
			"SCRIPTHOLD_REAL_SV_TASK_PULP", "package tb_package;",
			"tb_package.periph_write", "task automatic periph_write", "endtask : periph_write",
			"periph_read", 60,
		},
	} {
		t.Run(tc.qualified, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independent original source not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 1000 || fi.Size() > 65536 {
				t.Fatalf("original source invalid: %v %v", fi, err)
			}
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(bytes)
			if strings.Count(source, tc.marker) != 1 {
				t.Fatal("original marker drift")
			}
			lines := strings.Split(source, "\n")
			if tc.line > len(lines) || !strings.Contains(lines[tc.line-1], tc.excerpt) {
				t.Fatalf("independent authored line changed: %d", tc.line)
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "systemverilog",
				Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || out.Truncated || len(out.OutlineFiles) != 1 {
				t.Fatalf("original outline failed: %v %v files=%d", err, failed, len(out.OutlineFiles))
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
				t.Fatalf("missing directly authored task: %s", tc.qualified)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "systemverilog", Encoding: "utf-8",
				SymbolID: selected.SymbolID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil {
				t.Fatalf("fingerprint-bound task show failed: %v %v", err, failed)
			}
			if !strings.Contains(shown.Show.Text, tc.excerpt) ||
				!strings.Contains(shown.Show.Text, tc.closing) ||
				strings.Contains(shown.Show.Text, tc.excluded) {
				t.Fatalf("task show not exact and isolated: %q", shown.Show.Text)
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "systemverilog",
				Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint != file.SourceFingerprint {
				t.Fatalf("task digest/fingerprint mismatch: %v %v", err, failed)
			}
		})
	}
}
