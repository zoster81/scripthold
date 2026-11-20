package sourceintelligence

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestSystemVerilogPropertyLexicalSafety(t *testing.T) {
	const tick = string(byte(96))
	constPrefix := "module bus(input logic clk);\n" +
		"logic valid;\n" +
		"// property ignore_comment;\n" +
		"string text = \"property ignore_string;\";\n"
	valid := constPrefix +
		tick + "define IGNORE property ignore_macro;\n" +
		tick + "define WRAPPED \\\n" +
		" property ignore_continuation;\n" +
		"class C;\n property ignore_class;\n endclass\n" +
		"task automatic run(input int x);\n property ignore_task;\n endtask\n" +
		"function void f();\n property ignore_function;\n endfunction\n" +
		"property first;\n @(posedge clk) valid |=> valid;\n endproperty : first\n" +
		"property second;\n @(negedge clk) !valid;\n endproperty\n" +
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
				if sym.NativeKind == "property" {
					found[sym.QualifiedName] = sym.NameRange.Start.Line
				}
				if sym.QualifiedName == "bus.valid" {
					neighbor = true
				}
			}
			if !neighbor || len(found) != 2 || found["bus.first"] != 17 || found["bus.second"] != 20 {
				t.Fatalf("property lexical leakage or missing direct properties: %v neighbor=%v", found, neighbor)
			}
		})
	}
	for _, tc := range []struct{ name, source string }{
		{"no-name", "property;\nendproperty\n"},
		{"no-semicolon", "property broken\nendproperty\n"},
		{"parameterized-header", "property broken (int x);\nendproperty\n"},
		{"missing-end", "property broken;\nvalid |=> valid;\n"},
		{"mismatch-label", "property broken;\nvalid;\nendproperty : other\n"},
		{"macro-body", "property broken;\n" + tick + "FOO\nvalid;\nendproperty\n"},
		{"nested", "property broken;\nproperty nested;\nendproperty\nendproperty\n"},
		{"large-header", "property broken" + strings.Repeat(" ", 4200) + ";\nendproperty\n"},
		{"large-body", "property broken;\n" + strings.Repeat(" ", 66000) + "endproperty\n"},
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
					if sym.NativeKind == "property" {
						t.Errorf("invented unsupported property: %s", sym.QualifiedName)
					}
					if sym.QualifiedName == "bus.valid" {
						neighbor = true
					}
				}
				for _, d := range res.Analysis.Diagnostics {
					if d.Code == "systemverilog-unsupported-property" && d.Range != nil {
						diag = true
					}
				}
				if !neighbor || !diag || res.Analysis.CoverageComplete {
					t.Errorf("unsupported property lacks diagnostic: neighbor=%v diag=%v complete=%v",
						neighbor, diag, res.Analysis.CoverageComplete)
				}
			})
		}
	}
}

func TestSystemVerilogPropertyDuplicateAndLimit(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         int
	}{
		{"duplicate", "module bus;\nproperty first;\n1;\nendproperty\nproperty first;\n0;\nendproperty\nendmodule\n", 1},
		{"limit", func() string {
			var b strings.Builder
			b.WriteString("module bus;\n")
			for i := 0; i < 129; i++ {
				b.WriteString("property p")
				b.WriteString(strconv.Itoa(i))
				b.WriteString(";\n1;\nendproperty\n")
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
				if sym.NativeKind == "property" {
					count++
				}
				if sym.QualifiedName == "bus.p128" {
					t.Fatal("over-limit candidate indexed")
				}
			}
			for _, d := range res.Analysis.Diagnostics {
				if d.Code == "systemverilog-unsupported-property" && d.Range != nil {
					diag = true
				}
			}
			if count != tc.want || !diag || res.Analysis.CoverageComplete {
				t.Errorf("property guard failed: count=%d want=%d diag=%v coverage=%v", count, tc.want, diag, res.Analysis.CoverageComplete)
			}
		})
	}
}
