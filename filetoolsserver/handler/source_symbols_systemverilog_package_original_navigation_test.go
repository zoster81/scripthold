package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original package defaults are navigated through the development MCP handler,
// not the separately installed server or a guessed parameter value.
func TestSourceSymbolsOriginalSystemVerilogPackageParameterDrilldown(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_SYSTEMVERILOG_ROOT")
	if root == "" {
		t.Skip("independent SystemVerilog original sources not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		name, relative, anchor, query, owner, excerpt string
		line                                          int
	}{
		{
			name: "ctags-width", relative: filepath.Join("final-ctags-systemverilog-selected",
				"files", "systemverilog", "input.sv"),
			anchor: "package my_pack;", query: "MY_WIDTH", owner: "my_pack",
			excerpt: "localparam int MY_WIDTH", line: 3,
		},
		{
			name: "ctags-depth", relative: filepath.Join("final-ctags-systemverilog-selected",
				"files", "systemverilog", "input.sv"),
			anchor: "package my_pack;", query: "MY_DEPTH", owner: "my_pack",
			excerpt: "localparam int MY_DEPTH", line: 4,
		},
		{
			name: "opentitan-delay", relative: filepath.Join("sv-opentitan-selected",
				"files", "systemverilog", "alert_esc_agent_pkg.sv"),
			anchor: "package alert_esc_agent_pkg;", query: "ALERT_B2B_DELAY",
			owner: "alert_esc_agent_pkg", excerpt: "parameter uint ALERT_B2B_DELAY", line: 16,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.relative)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original source unavailable or unbounded: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil || strings.Count(string(data), tc.anchor) != 1 {
				t.Fatalf("original declaration context changed: %v", err)
			}
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "systemverilog",
				Encoding: "utf-8", Query: tc.query, Match: "exact",
				MaxFiles: 1, MaxSymbols: 16,
			})
			if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("MCP find failed: err=%v toolErr=%+v result=%+v", err, failed, found)
			}
			var symbolID string
			for _, symbol := range found.Symbols {
				if symbol.Name == tc.query && string(symbol.Kind) == "constant" &&
					symbol.NameRange.Start.Line == tc.line &&
					symbol.ParentQualifiedName == tc.owner {
					symbolID = symbol.ID
				}
			}
			if symbolID == "" {
				t.Fatalf("missing original package constant %s:%d (owner %s), got %+v",
					tc.query, tc.line, tc.owner, found.Symbols)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, SymbolID: symbolID,
				SourceFingerprint: found.Files[0].SourceFingerprint,
				Language:          "systemverilog", Encoding: "utf-8", MaxBytes: 4096,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("fingerprint-bound MCP show failed: err=%v toolErr=%+v show=%+v",
					err, failed, shown.Show)
			}
		})
	}
}
