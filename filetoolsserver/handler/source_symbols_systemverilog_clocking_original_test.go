package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemVerilogClockingTwoOriginalMCPNavigation(t *testing.T) {
	for _, tc := range []struct {
		env, owner, marker string
		lines              map[string]int
	}{
		{
			env: "SCRIPTHOLD_REAL_SV_CLOCKING_TVIP", owner: "tvip_axi_if",
			marker: "interface tvip_axi_if (",
			lines:  map[string]int{"master_cb": 50, "slave_cb": 88, "monitor_cb": 126},
		},
		{
			env: "SCRIPTHOLD_REAL_SV_CLOCKING_CESNET", owner: "mfb_if",
			marker: "interface mfb_if #(",
			lines:  map[string]int{"driver_rx_cb": 42, "driver_tx_cb": 47, "monitor_cb": 54},
		},
	} {
		t.Run(tc.owner, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independent original source not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 1000 || fi.Size() > 65536 {
				t.Fatalf("invalid original source %v %v", fi, err)
			}
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			src := string(bytes)
			if strings.Count(src, tc.marker) != 1 {
				t.Fatal("original interface drift")
			}
			authored := strings.Split(src, "\n")
			for name, line := range tc.lines {
				if len(authored) < line || !strings.Contains(authored[line-1], "clocking "+name) {
					t.Fatalf("authored clocking line drift: %s:%d", name, line)
				}
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failed, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "systemverilog",
				Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || outline.Truncated || len(outline.OutlineFiles) != 1 {
				t.Fatalf("MCP original outline failure: err=%v fail=%v count=%d", err, failed, len(outline.OutlineFiles))
			}
			file := outline.OutlineFiles[0]
			if file.SourceFingerprint == "" {
				t.Fatal("missing source fingerprint")
			}
			found := map[string]*SourceOutlineNode{}
			for _, node := range flattenDartExtensionTypeOriginalOutline(file.Symbols) {
				for name, line := range tc.lines {
					if node.QualifiedName == tc.owner+"."+name &&
						node.Line == line && string(node.Kind) == "entity" {
						found[name] = node
					}
				}
			}
			if len(found) != len(tc.lines) {
				t.Fatalf("MCP clocking blocks missing: got %d want %d", len(found), len(tc.lines))
			}
			for name, node := range found {
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "systemverilog", Encoding: "utf-8",
					SymbolID: node.SymbolID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil {
					t.Fatalf("fingerprint-bound clocking show %s failed: %v %v", name, err, failed)
				}
				show := shown.Show.Text
				if !strings.Contains(show, "clocking "+name) || !strings.Contains(show, "endclocking") {
					t.Fatalf("clocking show lost source: %s %q", name, show)
				}
				if strings.Count(show, "endclocking") != 1 {
					t.Fatalf("clocking show crossed sibling boundary: %s %q", name, show)
				}
				for sibling := range tc.lines {
					if sibling != name && strings.Contains(show, "clocking "+sibling+" ") {
						t.Fatalf("clocking show leaked sibling %s into %s", sibling, name)
					}
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "systemverilog",
				Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint != file.SourceFingerprint {
				t.Fatalf("MCP digest/fingerprint differs: %v %v", err, failed)
			}
		})
	}
}
