package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Package-level parameters are direct navigable declarations, not inferred
// values or signal declarations. Original sources from two independent projects
// pin expected owners and lines.
func TestSystemVerilogOriginalPackageParameters(t *testing.T) {
	check := func(t *testing.T, source string, want map[string]int) {
		t.Helper()
		result, err := (SystemVerilogAnalyzer{}).Analyze(
			context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 256))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.Truncated {
			t.Fatalf("unexpected SystemVerilog truncation: %+v", result.Analysis)
		}
		got := make(map[string]bool)
		for _, sym := range result.Analysis.Symbols {
			if sym.Name == "Fake" || sym.Name == "Spoof" ||
				strings.HasPrefix(sym.Name, "ThisNameDoesNotMatter") ||
				strings.HasPrefix(sym.Name, "assert_static_in_package_") {
				t.Fatalf("comment or quoted content became a declaration: %+v", sym)
			}
			if line, ok := want[sym.QualifiedName]; ok {
				if sym.Kind != SymbolKindConstant || sym.NameRange.Start.Line != line ||
					sym.ID == "" || !strings.Contains(sym.QualifiedName, ".") {
					t.Errorf("invalid package parameter identity: %+v", sym)
				}
				if got[sym.QualifiedName] {
					t.Fatalf("duplicate package parameter: %+v", sym)
				}
				got[sym.QualifiedName] = true
			}
			if sym.Kind == SymbolKindSignal && (strings.Contains(sym.QualifiedName, "MY_") ||
				sym.Name == "WIDTH" || sym.Name == "config_t" || sym.Name == "logic_returning") {
				t.Errorf("parameter or typedef fabricated a signal: %+v", sym)
			}
		}
		for qname := range want {
			if !got[qname] {
				t.Errorf("missing original package parameter %q: found=%v", qname,
					sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
	}
	const portable = "package config_pkg;\n" +
		"  localparam int WIDTH = 8;\n" +
		"  parameter logic [3:0] DEPTH = 4;\n" +
		"  typedef logic [WIDTH-1:0] config_t;\n" +
		"  // localparam int Fake = 42;\n" +
		"  string note = \"parameter int Spoof = 5;\";\n" +
		"  function automatic logic logic_returning();\n" +
		"    return 1'b1;\n" +
		"  endfunction\n" +
		"endpackage\n"
	for _, eol := range []struct{ name, delimiter string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(eol.name, func(t *testing.T) {
			check(t, strings.ReplaceAll(portable, "\n", eol.delimiter),
				map[string]int{"config_pkg.WIDTH": 2, "config_pkg.DEPTH": 3})
		})
	}
	root := os.Getenv("SCRIPTHOLD_REAL_SYSTEMVERILOG_ROOT")
	if root == "" {
		return
	}
	for _, tc := range []struct {
		name, relative, anchor string
		want                   map[string]int
	}{
		{"ctags", filepath.Join("final-ctags-systemverilog-selected", "files", "systemverilog", "input.sv"),
			"package my_pack;", map[string]int{"my_pack.MY_WIDTH": 3, "my_pack.MY_DEPTH": 4}},
		{"opentitan", filepath.Join("sv-opentitan-selected", "files", "systemverilog", "alert_esc_agent_pkg.sv"),
			"package alert_esc_agent_pkg;", map[string]int{"alert_esc_agent_pkg.ALERT_B2B_DELAY": 16}},
		{"surelog", filepath.Join("sv-surelog-selected", "files", "systemverilog", "AllPackageSignal.sv"),
			"package pkg_a;", map[string]int{"pkg_b.ParameterIntEqual4": 17, "pkg_a.ParameterIntInPkgA": 28}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.relative)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original source unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(data), tc.anchor) != 1 {
				t.Fatal("original package source anchor changed")
			}
			check(t, string(data), tc.want)
		})
	}
}
