package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original selected HDL modules establish the supported module and net/reg
// navigation subset without treating assignments or unclosed headers as names.
func TestSystemVerilogOriginalModuleSignalSubset(t *testing.T) {
	type want struct {
		name   string
		kind   SymbolKind
		line   int
		parent string
	}
	check := func(t *testing.T, text string, expected []want, complete bool) {
		t.Helper()
		result, err := (SystemVerilogAnalyzer{}).Analyze(
			context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 128))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.CoverageComplete != complete || result.Analysis.Truncated {
			t.Fatalf("unexpected SystemVerilog coverage: %+v", result.Analysis)
		}
		if len(result.Analysis.Symbols) != len(expected) {
			t.Fatalf("symbols=%v, expected %v", sortedSymbolQualifiedNames(result.Analysis.Symbols), expected)
		}
		seen := make([]bool, len(expected))
		for _, symbol := range result.Analysis.Symbols {
			if symbol.ID == "" || symbol.NameRange.Start.Line < 1 {
				t.Fatalf("unsafe symbol identity: %+v", symbol)
			}
			found := false
			for i, entry := range expected {
				if !seen[i] && symbol.Name == entry.name && symbol.Kind == entry.kind &&
					symbol.NameRange.Start.Line == entry.line &&
					symbol.ParentQualifiedName == entry.parent {
					seen[i], found = true, true
					break
				}
			}
			if !found {
				t.Fatalf("unexpected declaration or owner: %+v", symbol)
			}
		}
		for i, found := range seen {
			if !found {
				t.Errorf("missing selected declaration: %+v", expected[i])
			}
		}
		if !complete && len(result.Analysis.Diagnostics) == 0 {
			t.Fatal("unclosed module must report an incomplete diagnostic")
		}
	}
	cases := []struct {
		name, fixture string
		wants         []want
		complete      bool
	}{
		{"closed", "module top(input clk, rst, output reg [3:0] cnt);\nreg flag;\nassign flag = cnt[0];\nendmodule\n",
			[]want{{"top", SymbolKindModule, 1, ""}, {"cnt", SymbolKindSignal, 1, "top"},
				{"flag", SymbolKindSignal, 2, "top"}}, true},
		{"non-ansi", "module toplevel(clock, reset);\ninput clock;\ninput reset;\nreg flop1;\nreg flop2;\nendmodule\n",
			[]want{{"toplevel", SymbolKindModule, 1, ""}, {"flop1", SymbolKindSignal, 4, "toplevel"},
				{"flop2", SymbolKindSignal, 5, "toplevel"}}, true},
		{"unclosed", "module fifo(input clk_50, input clk_2, output empty);\n",
			nil, false},
	}
	for _, tc := range cases {
		for _, eol := range []struct{ name, separator string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
			t.Run(tc.name+"/"+eol.name, func(t *testing.T) {
				check(t, strings.ReplaceAll(tc.fixture, "\n", eol.separator), tc.wants, tc.complete)
			})
		}
	}
	root := os.Getenv("SCRIPTHOLD_REAL_SYSTEMVERILOG_ROOT")
	if root == "" {
		return
	}
	for _, tc := range []struct {
		name, relative, anchor string
		wants                  []want
		complete               bool
	}{
		{"yosys", filepath.Join("systemverilog-yosys-selected", "files", "systemverilog", "example.sv"),
			"module top (", []want{{"top", SymbolKindModule, 1, ""}, {"cnt", SymbolKindSignal, 3, "top"}}, true},
		{"pygments", filepath.Join("systemverilog-pygments-selected", "files", "systemverilog", "flipflop.sv"),
			"module toplevel(clock,reset);", []want{{"toplevel", SymbolKindModule, 1, ""},
				{"flop1", SymbolKindSignal, 5, "toplevel"}, {"flop2", SymbolKindSignal, 6, "toplevel"}}, true},
		{"linguist-unclosed", filepath.Join("systemverilog-linguist-selected", "files", "systemverilog", "fifo.sv"),
			"module fifo (", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.relative)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original source is not a bounded regular file: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			if strings.Count(source, tc.anchor) != 1 {
				t.Fatal("original source anchor changed")
			}
			check(t, source, tc.wants, tc.complete)
		})
	}
}
