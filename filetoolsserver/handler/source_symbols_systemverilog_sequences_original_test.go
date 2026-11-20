package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemVerilogSequencesTwoOriginalMCPNavigation(t *testing.T) {
	for _, tc := range []struct {
		env, owner, marker string
		names              map[string]int
	}{
		{
			env: "SCRIPTHOLD_REAL_SV_SEQUENCE_GENTLEMAN", owner: "sequence_triggered",
			marker: "module sequence_triggered(input bit clk);",
			names:  map[string]int{"accepted_request": 12},
		},
		{
			env: "SCRIPTHOLD_REAL_SV_SEQUENCE_OPENFORMAL", owner: "sequence_operators_tb",
			marker: "module sequence_operators_tb();",
			names:  map[string]int{"grant_1_5_S": 57, "grant_2_6_S": 61, "grant_1_6_S": 79, "grant_1_4_5_S": 112, "request_6_fell_S": 165},
		},
	} {
		t.Run(tc.owner, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independent original source not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 500 || fi.Size() > 65536 {
				t.Fatalf("invalid original source: %v %v", fi, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			src := string(raw)
			if strings.Count(src, tc.marker) != 1 {
				t.Fatal("original module drift")
			}
			lines := strings.Split(src, "\n")
			for name, line := range tc.names {
				if len(lines) < line || !strings.Contains(lines[line-1], "sequence "+name+";") {
					t.Fatalf("original authored sequence drift %s:%d", name, line)
				}
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failed, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "systemverilog",
				Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || outline.Truncated || len(outline.OutlineFiles) != 1 {
				t.Fatalf("MCP original outline failure: %v %v", err, failed)
			}
			file := outline.OutlineFiles[0]
			if file.SourceFingerprint == "" {
				t.Fatal("missing source fingerprint")
			}
			selected := map[string]*SourceOutlineNode{}
			for _, node := range flattenDartExtensionTypeOriginalOutline(file.Symbols) {
				for name, line := range tc.names {
					if node.QualifiedName == tc.owner+"."+name && node.Line == line && string(node.Kind) == "entity" {
						selected[name] = node
					}
				}
			}
			if len(selected) != len(tc.names) {
				t.Fatalf("missing MCP sequence navigation: got %d want %d", len(selected), len(tc.names))
			}
			for name, node := range selected {
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "systemverilog", Encoding: "utf-8",
					SymbolID: node.SymbolID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil {
					t.Fatalf("fingerprint bound show %s failed: %v %v", name, err, failed)
				}
				show := shown.Show.Text
				if !strings.Contains(show, "sequence "+name+";") || strings.Count(show, "endsequence") != 1 {
					t.Fatalf("sequence excerpt malformed or crossed boundary %s: %q", name, show)
				}
				for sibling := range tc.names {
					if sibling != name && strings.Contains(show, "sequence "+sibling+";") {
						t.Fatalf("show leaked sibling %s into %s", sibling, name)
					}
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "systemverilog",
				Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint != file.SourceFingerprint {
				t.Fatalf("MCP digest/fingerprint mismatch: %v %v", err, failed)
			}
		})
	}
}
