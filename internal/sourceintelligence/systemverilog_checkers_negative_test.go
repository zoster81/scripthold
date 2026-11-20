package sourceintelligence

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestSystemVerilogTopLevelCheckersLexicalAndScope(t *testing.T) {
	const tick = string(byte(96))
	valid := "module neighbor(input bit clk);\n" +
		"  logic ready;\n  checker ignored_module;\n  endchecker\nendmodule\n" +
		"interface ignored_if(input bit clk);\n  checker ignored_interface;\n  endchecker\nendinterface\n" +
		"package ignored_pkg;\n  checker ignored_package;\n  endchecker\nendpackage\n" +
		"// checker ignored_comment;\n" +
		"string note = \"checker ignored_string;\";\n" +
		tick + "define IGNORE checker ignored_macro;\n" +
		tick + "define WRAPPED \\\n" +
		" checker ignored_continuation;\n" +
		"class C;\n  checker ignored_class;\nendchecker\nendclass\n" +
		"checker first(input bit clk, logic [2:0] data);\n" +
		"  property ok;\n    @(posedge clk) data != '0;\n  endproperty\n" +
		"endchecker : first\n" +
		"checker second (\n  input bit clk,\n  input bit ready\n);\n" +
		"  assert property (@(posedge clk) ready);\nendchecker\n"
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
				if sym.NativeKind == "checker" {
					found[sym.QualifiedName] = true
				}
				if sym.QualifiedName == "neighbor.ready" {
					neighbor = true
				}
			}
			if !neighbor || len(found) != 2 || !found["first"] || !found["second"] {
				t.Fatalf("checker ownership leaked nested/comment/directives: %v neighbor=%v", found, neighbor)
			}
		})
	}
	for _, tc := range []struct{ name, source string }{
		{"no-name", "checker;\nendchecker\n"},
		{"no-semicolon", "checker broken\nendchecker\n"},
		{"invalid-header", "checker broken extra;\nendchecker\n"},
		{"unbalanced-args", "checker broken(input bit clk;\nendchecker\n"},
		{"macro-args", "checker broken(input bit " + tick + "FOO);\nendchecker\n"},
		{"missing-end", "checker broken(input bit clk);\nassert property (clk);\n"},
		{"mismatch-label", "checker broken;\nendchecker : other\n"},
		{"nested", "checker broken;\nchecker nested;\nendchecker\nendchecker\n"},
		{"macro-body", "checker broken;\n" + tick + "FOO\nendchecker\n"},
		{"large-header", "checker broken(" + strings.Repeat(" ", 4200) + ");\nendchecker\n"},
		{"large-body", "checker broken;\n" + strings.Repeat(" ", 66000) + "endchecker\n"},
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
					if sym.NativeKind == "checker" {
						t.Errorf("invented malformed checker %s", sym.QualifiedName)
					}
					if sym.QualifiedName == "neighbor.ready" {
						neighbor = true
					}
				}
				for _, d := range res.Analysis.Diagnostics {
					if d.Code == "systemverilog-unsupported-checker" && d.Range != nil {
						diag = true
					}
				}
				if !neighbor || !diag || res.Analysis.CoverageComplete {
					t.Errorf("unsupported checker lacks source diagnostic: neighbor=%v diag=%v complete=%v", neighbor, diag, res.Analysis.CoverageComplete)
				}
			})
		}
	}
}

func TestSystemVerilogTopLevelCheckerDuplicateAndLimit(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         int
	}{
		{"duplicate", "checker a;\nendchecker\nchecker a;\nendchecker\n", 1},
		{"limit", func() string {
			var b strings.Builder
			for i := 0; i < 129; i++ {
				b.WriteString("checker c")
				b.WriteString(strconv.Itoa(i))
				b.WriteString(";\nendchecker\n")
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
				if sym.NativeKind == "checker" {
					count++
				}
				if sym.QualifiedName == "c128" {
					t.Fatal("over-limit checker indexed")
				}
			}
			for _, d := range res.Analysis.Diagnostics {
				if d.Code == "systemverilog-unsupported-checker" && d.Range != nil {
					diag = true
				}
			}
			if count != tc.want || !diag || res.Analysis.CoverageComplete {
				t.Errorf("checker limits/duplicate mismatch: count=%d want=%d diag=%v complete=%v", count, tc.want, diag, res.Analysis.CoverageComplete)
			}
		})
	}
}
