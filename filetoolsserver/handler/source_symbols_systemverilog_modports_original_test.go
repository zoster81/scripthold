package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemVerilogModportsTwoOriginalMCPNavigation(t *testing.T) {
	for _, tc := range []struct {
		env, marker, qualified, excerpt, excluded string
		line                                      int
	}{
		{"SCRIPTHOLD_REAL_SV_MODPORT_VORTEX", "interface VX_txbar_bus_if",
			"VX_txbar_bus_if.master", "modport master (", "modport slave", 22},
		{"SCRIPTHOLD_REAL_SV_MODPORT_SWABIAN", "interface axis_interface #(",
			"axis_interface.master", "modport master(input clk", "modport slave", 39},
	} {
		t.Run(tc.qualified, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independently authored original not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 500 || fi.Size() > 65536 {
				t.Fatalf("invalid original source %v %v", fi, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			src := string(raw)
			if strings.Count(src, tc.marker) != 1 {
				t.Fatal("original interface drift")
			}
			lines := strings.Split(src, "\n")
			if len(lines) < tc.line || !strings.Contains(lines[tc.line-1], tc.excerpt) {
				t.Fatal("original authored modport line drift")
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failed, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "systemverilog", Encoding: "utf-8",
				MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || outline.Truncated || len(outline.OutlineFiles) != 1 {
				t.Fatalf("MCP outline err=%v failed=%v file-count=%d", err, failed, len(outline.OutlineFiles))
			}
			file := outline.OutlineFiles[0]
			if file.SourceFingerprint == "" {
				t.Fatal("missing source fingerprint")
			}
			var selected *SourceOutlineNode
			for _, node := range flattenDartExtensionTypeOriginalOutline(file.Symbols) {
				if node.QualifiedName == tc.qualified && node.Line == tc.line && string(node.Kind) == "type" {
					selected = node
					break
				}
			}
			if selected == nil {
				t.Fatalf("MCP original modport absent: %s", tc.qualified)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "systemverilog", Encoding: "utf-8",
				SymbolID: selected.SymbolID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || failed != nil || shown.Show == nil {
				t.Fatalf("MCP fingerprint-bound show err=%v failed=%v", err, failed)
			}
			if !strings.Contains(shown.Show.Text, tc.excerpt) ||
				!strings.Contains(shown.Show.Text, ");") ||
				strings.Contains(shown.Show.Text, tc.excluded) {
				t.Fatalf("MCP show leaked sibling or lost original: %q", shown.Show.Text)
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "systemverilog", Encoding: "utf-8",
				MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint != file.SourceFingerprint {
				t.Fatalf("MCP digest/fingerprint mismatch: %v %v", err, failed)
			}
		})
	}
}
