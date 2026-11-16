package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemVerilogDirectEnumTypedefNavigation(t *testing.T) {
	check := func(t *testing.T, source string, want map[string]int, forbidden []string) {
		t.Helper()
		result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 256))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.Truncated {
			t.Fatalf("unexpected truncation: %+v", result.Analysis)
		}
		seen := make(map[string]bool)
		for _, sym := range result.Analysis.Symbols {
			for _, name := range forbidden {
				if sym.QualifiedName == name {
					t.Fatalf("false enum owner: %+v", sym)
				}
			}
			if line, ok := want[sym.QualifiedName]; ok {
				if seen[sym.QualifiedName] || sym.Kind != SymbolKindType || sym.NativeKind != "typedef-enum" || sym.NameRange.Start.Line != line || sym.ID == "" {
					t.Errorf("incorrect enum declaration: %+v", sym)
				}
				seen[sym.QualifiedName] = true
			}
		}
		for name := range want {
			if !seen[name] {
				t.Errorf("missing %s, found %v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
	}
	const fixture = "package pkg;\n  typedef enum { READY, BUSY } status_e;\n  // typedef enum { PHANTOM } ghost_e;\n  class nested;\n    typedef enum { CLASS_ONLY } state_e;\n  endclass\n  typedef enum logic [1:0] { RED, BLUE } color_e;\nendpackage\n"
	for _, tc := range []struct{ name, eol string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			check(t, strings.ReplaceAll(fixture, "\n", tc.eol), map[string]int{"pkg.status_e": 2, "pkg.color_e": 7}, []string{"pkg.state_e", "pkg.ghost_e"})
		})
	}
	root := os.Getenv("SCRIPTHOLD_REAL_SYSTEMVERILOG_ROOT")
	if root == "" {
		return
	}
	for _, tc := range []struct {
		name, rel, anchor string
		want              map[string]int
		forbidden         []string
	}{
		{"opentitan", filepath.Join("sv-opentitan-selected", "files", "systemverilog", "alert_esc_agent_pkg.sv"), "package alert_esc_agent_pkg;", map[string]int{"alert_esc_agent_pkg.alert_esc_trans_type_e": 25, "alert_esc_agent_pkg.alert_handshake_e": 33, "alert_esc_agent_pkg.esc_handshake_e": 46}, nil},
		{"verilator", filepath.Join("sv-verilator-selected", "files", "systemverilog", "verilated_std.sv"), "package std;", nil, []string{"std.state"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.rel)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("source unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(data), tc.anchor) != 1 {
				t.Fatal("original anchor changed")
			}
			check(t, string(data), tc.want, tc.forbidden)
		})
	}
}
