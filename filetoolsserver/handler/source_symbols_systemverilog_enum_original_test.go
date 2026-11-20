package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceSymbolsOriginalSystemVerilogEnumDrilldown(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_SYSTEMVERILOG_ROOT")
	if root == "" {
		t.Skip("independent SystemVerilog sources not configured")
	}
	path := filepath.Join(root, "sv-opentitan-selected", "files", "systemverilog", "alert_esc_agent_pkg.sv")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		t.Fatalf("original source unavailable: %v %v", info, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Count(string(data), "package alert_esc_agent_pkg;") != 1 {
		t.Fatalf("source anchor unavailable: %v", err)
	}
	h := NewHandler([]string{root})
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "systemverilog", Encoding: "utf-8",
		Query: "alert_handshake_e", Match: "exact", MaxFiles: 1, MaxSymbols: 16,
	})
	if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
		t.Fatalf("MCP find: err=%v fail=%+v found=%+v", err, failed, found)
	}
	var id string
	for _, sym := range found.Symbols {
		if sym.Name == "alert_handshake_e" && string(sym.Kind) == "type" && sym.NativeKind == "typedef-enum" && sym.NameRange.Start.Line == 33 && sym.ParentQualifiedName == "alert_esc_agent_pkg" {
			id = sym.ID
		}
	}
	if id == "" {
		t.Fatalf("original enum missing or incorrect: %+v", found.Symbols)
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint,
		Language: "systemverilog", Encoding: "utf-8", MaxBytes: 4096,
	})
	if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "AlertAckReceived") {
		t.Fatalf("MCP show: err=%v fail=%+v show=%+v", err, failed, shown.Show)
	}
}
