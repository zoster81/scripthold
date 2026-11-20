package sourceintelligence

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestSystemVerilogCovergroupLexicalSafety(t *testing.T) {
	const tick = string(byte(96))
	constPrefix := "module bus(input logic clk);\n" +
		"logic valid;\n" +
		"// covergroup ignore_comment;\n" +
		"string text = \"covergroup ignore_string;\";\n"
	valid := constPrefix +
		tick + "define IGNORE covergroup ignore_macro;\n" +
		tick + "define WRAPPED \\\n" +
		" covergroup ignore_continuation;\n" +
		"class C;\n covergroup ignore_class;\n endclass\n" +
		"task automatic run(input int x);\n covergroup ignore_task;\n endtask\n" +
		"function void f();\n covergroup ignore_function;\n endfunction\n" +
		"covergroup first @(posedge clk);\n coverpoint valid;\n endgroup : first\n" +
		"covergroup second;\n coverpoint valid;\n endgroup\n" +
		"endmodule\n"
	for _, eol := range []struct{ name, sep string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(eol.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(strings.ReplaceAll(valid, "\n", eol.sep))
			res, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			found, neighbor := map[string]int{}, false
			for _, sym := range res.Analysis.Symbols {
				if sym.NativeKind == "covergroup" {
					found[sym.QualifiedName] = sym.NameRange.Start.Line
				}
				if sym.QualifiedName == "bus.valid" {
					neighbor = true
				}
			}
			if !neighbor || len(found) != 2 || found["bus.first"] != 17 || found["bus.second"] != 20 {
				t.Fatalf("covergroup lexical leakage or missing direct covergroups: %v neighbor=%v", found, neighbor)
			}
		})
	}
	for _, tc := range []struct{ name, source string }{
		{"no-name", "covergroup;\nendgroup\n"},
		{"no-semicolon", "covergroup broken\nendgroup\n"},
		{"parameterized-header", "covergroup broken (int x);\nendgroup\n"},
		{"unparenthesized-event", "covergroup broken @posedge clk;\nendgroup\n"},
		{"unbalanced-event", "covergroup broken @(posedge clk;\nendgroup\n"},
		{"invalid-event-tail", "covergroup broken @(posedge clk) something;\nendgroup\n"},
		{"missing-end", "covergroup broken;\ncoverpoint valid;\n"},
		{"mismatch-label", "covergroup broken;\nvalid;\nendgroup : other\n"},
		{"macro-body", "covergroup broken;\n" + tick + "FOO\nvalid;\nendgroup\n"},
		{"nested", "covergroup broken;\ncovergroup nested;\nendgroup\nendgroup\n"},
		{"large-header", "covergroup broken" + strings.Repeat(" ", 4200) + ";\nendgroup\n"},
		{"large-body", "covergroup broken;\n" + strings.Repeat(" ", 66000) + "endgroup\n"},
	} {
		for _, eol := range []struct{ name, sep string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"-"+eol.name, func(t *testing.T) {
				src := "module bus(input logic clk);\nlogic valid;\n" + tc.source + "endmodule\n"
				doc := sourceDocumentForScanner(strings.ReplaceAll(src, "\n", eol.sep))
				res, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
				if err != nil {
					t.Fatal(err)
				}
				neighbor, diag := false, false
				for _, sym := range res.Analysis.Symbols {
					if sym.NativeKind == "covergroup" {
						t.Errorf("invented unsupported covergroup: %s", sym.QualifiedName)
					}
					if sym.QualifiedName == "bus.valid" {
						neighbor = true
					}
				}
				for _, d := range res.Analysis.Diagnostics {
					if d.Code == "systemverilog-unsupported-covergroup" && d.Range != nil {
						diag = true
					}
				}
				if !neighbor || !diag || res.Analysis.CoverageComplete {
					t.Errorf("unsupported covergroup lacks diagnostic: neighbor=%v diag=%v complete=%v",
						neighbor, diag, res.Analysis.CoverageComplete)
				}
			})
		}
	}
}

func TestSystemVerilogCovergroupDuplicateAndLimit(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         int
	}{
		{"duplicate", "module bus;\ncovergroup first;\n1;\nendgroup\ncovergroup first;\n0;\nendgroup\nendmodule\n", 1},
		{"limit", func() string {
			var b strings.Builder
			b.WriteString("module bus;\n")
			for i := 0; i < 129; i++ {
				b.WriteString("covergroup p")
				b.WriteString(strconv.Itoa(i))
				b.WriteString(";\n1;\nendgroup\n")
			}
			b.WriteString("endmodule\n")
			return b.String()
		}(), 128},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(tc.source)
			res, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			count, diag := 0, false
			for _, sym := range res.Analysis.Symbols {
				if sym.NativeKind == "covergroup" {
					count++
				}
				if sym.QualifiedName == "bus.p128" {
					t.Fatal("over-limit candidate indexed")
				}
			}
			for _, d := range res.Analysis.Diagnostics {
				if d.Code == "systemverilog-unsupported-covergroup" && d.Range != nil {
					diag = true
				}
			}
			if count != tc.want || !diag || res.Analysis.CoverageComplete {
				t.Errorf("covergroup guard failed: count=%d want=%d diag=%v coverage=%v", count, tc.want, diag, res.Analysis.CoverageComplete)
			}
		})
	}
}
