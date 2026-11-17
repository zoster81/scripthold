package sourceintelligence

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestSystemVerilogSequenceLexicalSafety(t *testing.T) {
	const tick = string(byte(96))
	constPrefix := "module bus(input logic clk);\n" +
		"logic valid;\n" +
		"// sequence ignore_comment;\n" +
		"string text = \"sequence ignore_string;\";\n"
	valid := constPrefix +
		tick + "define IGNORE sequence ignore_macro;\n" +
		tick + "define WRAPPED \\\n" +
		" sequence ignore_continuation;\n" +
		"class C;\n sequence ignore_class;\n endclass\n" +
		"task automatic run(input int x);\n sequence ignore_task;\n endtask\n" +
		"function void f();\n sequence ignore_function;\n endfunction\n" +
		"sequence first;\n @(posedge clk) valid |=> valid;\n endsequence : first\n" +
		"sequence second;\n @(negedge clk) !valid;\n endsequence\n" +
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
				if sym.NativeKind == "sequence" {
					found[sym.QualifiedName] = sym.NameRange.Start.Line
				}
				if sym.QualifiedName == "bus.valid" {
					neighbor = true
				}
			}
			if !neighbor || len(found) != 2 || found["bus.first"] != 17 || found["bus.second"] != 20 {
				t.Fatalf("sequence lexical leakage or missing direct properties: %v neighbor=%v", found, neighbor)
			}
		})
	}
	for _, tc := range []struct{ name, source string }{
		{"no-name", "sequence;\nendsequence\n"},
		{"no-semicolon", "sequence broken\nendsequence\n"},
		{"parameterized-header", "sequence broken (int x);\nendsequence\n"},
		{"missing-end", "sequence broken;\nvalid |=> valid;\n"},
		{"mismatch-label", "sequence broken;\nvalid;\nendsequence : other\n"},
		{"macro-body", "sequence broken;\n" + tick + "FOO\nvalid;\nendsequence\n"},
		{"nested", "sequence broken;\nsequence nested;\nendsequence\nendsequence\n"},
		{"large-header", "sequence broken" + strings.Repeat(" ", 4200) + ";\nendsequence\n"},
		{"large-body", "sequence broken;\n" + strings.Repeat(" ", 66000) + "endsequence\n"},
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
					if sym.NativeKind == "sequence" {
						t.Errorf("invented unsupported sequence: %s", sym.QualifiedName)
					}
					if sym.QualifiedName == "bus.valid" {
						neighbor = true
					}
				}
				for _, d := range res.Analysis.Diagnostics {
					if d.Code == "systemverilog-unsupported-sequence" && d.Range != nil {
						diag = true
					}
				}
				if !neighbor || !diag || res.Analysis.CoverageComplete {
					t.Errorf("unsupported sequence lacks diagnostic: neighbor=%v diag=%v complete=%v",
						neighbor, diag, res.Analysis.CoverageComplete)
				}
			})
		}
	}
}

func TestSystemVerilogSequenceDuplicateAndLimit(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         int
	}{
		{"duplicate", "module bus;\nsequence first;\n1;\nendsequence\nsequence first;\n0;\nendsequence\nendmodule\n", 1},
		{"limit", func() string {
			var b strings.Builder
			b.WriteString("module bus;\n")
			for i := 0; i < 129; i++ {
				b.WriteString("sequence p")
				b.WriteString(strconv.Itoa(i))
				b.WriteString(";\n1;\nendsequence\n")
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
				if sym.NativeKind == "sequence" {
					count++
				}
				if sym.QualifiedName == "bus.p128" {
					t.Fatal("over-limit candidate indexed")
				}
			}
			for _, d := range res.Analysis.Diagnostics {
				if d.Code == "systemverilog-unsupported-sequence" && d.Range != nil {
					diag = true
				}
			}
			if count != tc.want || !diag || res.Analysis.CoverageComplete {
				t.Errorf("sequence guard failed: count=%d want=%d diag=%v coverage=%v", count, tc.want, diag, res.Analysis.CoverageComplete)
			}
		})
	}
}
