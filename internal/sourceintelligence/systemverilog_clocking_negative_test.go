package sourceintelligence

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestSystemVerilogClockingLexicalSafety(t *testing.T) {
	const valid = "interface bus_if(input clk);\n" +
		"  logic valid;\n" +
		"  // clocking ignored_comment @(posedge clk);\n" +
		"  string note = \"clocking ignored_string @(posedge clk);\";\n" +
		"  `define IGNORED clocking ignored_macro @(posedge clk);\n" +
		"  `define WRAPPED \\\n" +
		"    clocking ignored_continuation @(posedge clk);\n" +
		"  class C;\n" +
		"    clocking ignored_class @(posedge clk);\n" +
		"  endclass\n" +
		"  task automatic act(input int x);\n" +
		"    clocking ignored_task @(posedge clk);\n" +
		"  endtask\n" +
		"  clocking drive_cb @(posedge clk);\n" +
		"    output valid;\n" +
		"  endclocking : drive_cb\n" +
		"  clocking monitor_cb @(negedge clk);\n" +
		"    input valid;\n" +
		"  endclocking\n" +
		"  modport agent(clocking drive_cb);\n" +
		"endinterface\n"
	for _, tc := range []struct{ name, eol string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(strings.ReplaceAll(valid, "\n", tc.eol))
			result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			found := make(map[string]int)
			modport := false
			for _, sym := range result.Analysis.Symbols {
				if sym.NativeKind == "clocking-block" {
					found[sym.QualifiedName] = sym.NameRange.Start.Line
				}
				if sym.QualifiedName == "bus_if.agent" && sym.NativeKind == "modport" {
					modport = true
				}
			}
			if len(found) != 2 || found["bus_if.drive_cb"] != 14 ||
				found["bus_if.monitor_cb"] != 17 || !modport {
				t.Fatalf("clocking leaked nested, directive, comment or string: found=%v modport=%v", found, modport)
			}
		})
	}

	for _, tc := range []struct{ name, source string }{
		{"no-event", "clocking broken;\nendclocking\n"},
		{"no-parens", "clocking broken @posedge clk;\nendclocking\n"},
		{"missing-header-close", "clocking broken @(posedge clk;\nendclocking\n"},
		{"missing-header-semicolon", "clocking broken @(posedge clk)\nendclocking\n"},
		{"missing-endclocking", "clocking broken @(posedge clk);\ninput valid;\n"},
		{"wrong-end-name", "clocking broken @(posedge clk);\nendclocking : other\n"},
		{"nested-clocking", "clocking broken @(posedge clk);\nclocking other @(posedge clk);\nendclocking\nendclocking\n"},
		{"macro-body", "clocking broken @(posedge clk);\n`FOO\ninput valid;\nendclocking\n"},
		{"overlong-header", "clocking broken @(" + strings.Repeat(" ", 4200) + "posedge clk);\nendclocking\n"},
		{"overlong-body", "clocking broken @(posedge clk);\n" + strings.Repeat(" ", 66000) + "endclocking\n"},
	} {
		for _, eol := range []struct{ name, sep string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"-"+eol.name, func(t *testing.T) {
				doc := sourceDocumentForScanner(strings.ReplaceAll(
					"interface bus_if(input clk);\nlogic valid;\n"+tc.source+"endinterface\n",
					"\n", eol.sep))
				result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
				if err != nil {
					t.Fatal(err)
				}
				neighbor, warning := false, false
				for _, sym := range result.Analysis.Symbols {
					if sym.NativeKind == "clocking-block" {
						t.Errorf("invented malformed clocking block: %s", sym.QualifiedName)
					}
					if sym.QualifiedName == "bus_if.valid" {
						neighbor = true
					}
				}
				for _, diagnostic := range result.Analysis.Diagnostics {
					if diagnostic.Code == "systemverilog-unsupported-clocking" && diagnostic.Range != nil {
						warning = true
					}
				}
				if !neighbor || !warning || result.Analysis.CoverageComplete {
					t.Errorf("unsafe clocking not diagnosed: neighbor=%v warning=%v coverage=%v",
						neighbor, warning, result.Analysis.CoverageComplete)
				}
			})
		}
	}
}

func TestSystemVerilogClockingDuplicateAndCandidateLimit(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         int
	}{
		{"duplicate", "interface bus_if(input clk);\n" +
			"clocking cb @(posedge clk);\nendclocking\n" +
			"clocking cb @(negedge clk);\nendclocking\nendinterface\n", 1},
		{"limit", func() string {
			var out strings.Builder
			out.WriteString("interface bus_if(input clk);\n")
			for i := 0; i < 129; i++ {
				out.WriteString("clocking cb")
				out.WriteString(strconv.Itoa(i))
				out.WriteString(" @(posedge clk);\nendclocking\n")
			}
			out.WriteString("endinterface\n")
			return out.String()
		}(), 128},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(tc.source)
			result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			found, warning := 0, false
			for _, sym := range result.Analysis.Symbols {
				if sym.NativeKind == "clocking-block" {
					found++
				}
				if sym.QualifiedName == "bus_if.cb128" {
					t.Fatal("over-limit block indexed")
				}
			}
			for _, diag := range result.Analysis.Diagnostics {
				if diag.Code == "systemverilog-unsupported-clocking" && diag.Range != nil {
					warning = true
				}
			}
			if found != tc.want || !warning || result.Analysis.CoverageComplete {
				t.Fatalf("candidate/duplicate bounds: count=%d want=%d warning=%v coverage=%v",
					found, tc.want, warning, result.Analysis.CoverageComplete)
			}
		})
	}
}
