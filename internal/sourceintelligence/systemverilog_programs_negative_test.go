package sourceintelligence

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestSystemVerilogTopLevelProgramsLexicalAndScope(t *testing.T) {
	const tick = string(byte(96))
	valid := "module neighbor(input bit clk);\n" +
		"  logic ready;\n  program ignored_module;\n  endprogram\nendmodule\n" +
		"interface ignored_if(input bit clk);\n  program ignored_interface;\n  endprogram\nendinterface\n" +
		"package ignored_pkg;\n  program ignored_package;\n  endprogram\nendpackage\n" +
		"// program ignored_comment;\n" +
		"string note = \"program ignored_string;\";\n" +
		tick + "define IGNORE program ignored_macro;\n" +
		tick + "define WRAPPED \\\n" +
		" program ignored_continuation;\n" +
		"class C;\n  program ignored_class;\nendprogram\nendclass\n" +
		"program first(input bit clk, logic [2:0] data);\n" +
		"  property ok;\n    @(posedge clk) data != '0;\n  endproperty\n" +
		"endprogram : first\n" +
		"program second (\n  input bit clk,\n  input bit ready\n);\n" +
		"  assert property (@(posedge clk) ready);\nendprogram\n"
	for _, eol := range []struct{ name, sep string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(eol.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(strings.ReplaceAll(valid, "\n", eol.sep))
			res, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			neighbor := false
			for _, sym := range res.Analysis.Symbols {
				if sym.NativeKind == "program" {
					found[sym.QualifiedName] = true
				}
				if sym.QualifiedName == "neighbor.ready" {
					neighbor = true
				}
			}
			if !neighbor || len(found) != 2 || !found["first"] || !found["second"] {
				t.Fatalf("program ownership leaked nested/comment/directives: %v neighbor=%v", found, neighbor)
			}
		})
	}
	for _, tc := range []struct{ name, source string }{
		{"no-name", "program;\nendprogram\n"},
		{"no-semicolon", "program broken\nendprogram\n"},
		{"invalid-header", "program broken extra;\nendprogram\n"},
		{"unbalanced-args", "program broken(input bit clk;\nendprogram\n"},
		{"macro-args", "program broken(input bit " + tick + "FOO);\nendprogram\n"},
		{"malformed-parameter", "program broken #(parameter N=1;\nendprogram\n"},
		{"parameter-tail", "program broken #(parameter N=1) unexpected;\nendprogram\n"},
		{"missing-end", "program broken(input bit clk);\nassert property (clk);\n"},
		{"mismatch-label", "program broken;\nendprogram : other\n"},
		{"nested", "program broken;\nprogram nested;\nendprogram\nendprogram\n"},
		{"macro-body", "program broken;\n" + tick + "FOO\nendprogram\n"},
		{"large-header", "program broken(" + strings.Repeat(" ", 4200) + ");\nendprogram\n"},
		{"large-body", "program broken;\n" + strings.Repeat(" ", 66000) + "endprogram\n"},
	} {
		for _, eol := range []struct{ name, sep string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"-"+eol.name, func(t *testing.T) {
				doc := sourceDocumentForScanner(strings.ReplaceAll("module neighbor;\nlogic ready;\nendmodule\n"+tc.source, "\n", eol.sep))
				res, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
				if err != nil {
					t.Fatal(err)
				}
				neighbor, diag := false, false
				for _, sym := range res.Analysis.Symbols {
					if sym.NativeKind == "program" {
						t.Errorf("invented malformed program %s", sym.QualifiedName)
					}
					if sym.QualifiedName == "neighbor.ready" {
						neighbor = true
					}
				}
				for _, d := range res.Analysis.Diagnostics {
					if d.Code == "systemverilog-unsupported-program" && d.Range != nil {
						diag = true
					}
				}
				if !neighbor || !diag || res.Analysis.CoverageComplete {
					t.Errorf("unsupported program lacks source diagnostic: neighbor=%v diag=%v complete=%v", neighbor, diag, res.Analysis.CoverageComplete)
				}
			})
		}
	}
}

func TestSystemVerilogTopLevelProgramDuplicateAndLimit(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         int
	}{
		{"duplicate", "program a;\nendprogram\nprogram a;\nendprogram\n", 1},
		{"limit", func() string {
			var b strings.Builder
			for i := 0; i < 129; i++ {
				b.WriteString("program c")
				b.WriteString(strconv.Itoa(i))
				b.WriteString(";\nendprogram\n")
			}
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
				if sym.NativeKind == "program" {
					count++
				}
				if sym.QualifiedName == "c128" {
					t.Fatal("over-limit program indexed")
				}
			}
			for _, d := range res.Analysis.Diagnostics {
				if d.Code == "systemverilog-unsupported-program" && d.Range != nil {
					diag = true
				}
			}
			if count != tc.want || !diag || res.Analysis.CoverageComplete {
				t.Errorf("program limits/duplicate mismatch: count=%d want=%d diag=%v complete=%v", count, tc.want, diag, res.Analysis.CoverageComplete)
			}
		})
	}
}
