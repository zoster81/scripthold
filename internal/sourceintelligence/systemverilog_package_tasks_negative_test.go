package sourceintelligence

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestSystemVerilogPackageTasksLexicalSafety(t *testing.T) {
	const original = "package p;\n localparam int Neighbor=1;\n" +
		" // task automatic fake_comment(input int v);\n" +
		" class C;\n task automatic hidden_class(input int v);\n endtask\n endclass\n" +
		" task automatic first_task (\n input int v\n );\n" +
		" v = v + 1;\n endtask : first_task\n" +
		" function automatic logic next_function(input logic v);\n" +
		" next_function = v;\n endfunction\n" +
		" task automatic second_task(input int v);\n v = 3;\n endtask\nendpackage\n"
	for _, eol := range []struct{ name, separator string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run("valid-"+eol.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(strings.ReplaceAll(original, "\n", eol.separator))
			result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]int{}
			for _, s := range result.Analysis.Symbols {
				if s.NativeKind == "package-task" || s.NativeKind == "package-function" {
					got[s.QualifiedName] = s.NameRange.Start.Line
				}
			}
			if len(got) != 3 || got["p.first_task"] != 8 || got["p.next_function"] != 13 ||
				got["p.second_task"] != 16 {
				t.Fatalf("nested/comment task or neighboring function misidentified: %v", got)
			}
		})
	}
	for _, tc := range []struct{ name, body string }{
		{"no-args", "task automatic broken;\n endtask\n"},
		{"missing-close-paren", "task automatic broken(input int x;\n endtask\n"},
		{"missing-endtask", "task automatic broken(input int x);\n x=1;\n"},
		{"wrong-closer", "task automatic broken(input int x);\n endfunction\n"},
		{"header-over-limit", "task automatic broken(" + strings.Repeat(" ", 4200) + "input int x);\n endtask\n"},
		{"body-over-limit", "task automatic broken(input int x);\n" + strings.Repeat(" ", 66000) + "endtask\n"},
	} {
		for _, eol := range []struct{ name, separator string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"-"+eol.name, func(t *testing.T) {
				doc := sourceDocumentForScanner(strings.ReplaceAll("package p;\nlocalparam int Neighbor=1;\n"+tc.body+"endpackage\n", "\n", eol.separator))
				result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
				if err != nil {
					t.Fatal(err)
				}
				neighbor, warning := false, false
				for _, sym := range result.Analysis.Symbols {
					if sym.NativeKind == "package-task" {
						t.Errorf("invented malformed task: %s", sym.QualifiedName)
					}
					if sym.QualifiedName == "p.Neighbor" {
						neighbor = true
					}
				}
				for _, d := range result.Analysis.Diagnostics {
					if (d.Code == "systemverilog-unsupported-package-task" || d.Code == "systemverilog-unclosed-package-task") && d.Range != nil {
						warning = true
					}
				}
				if !neighbor || !warning || result.Analysis.CoverageComplete {
					t.Errorf("must fail closed, preserve neighbor, diagnose: neighbor=%v warning=%v coverage=%v", neighbor, warning, result.Analysis.CoverageComplete)
				}
			})
		}
	}
}

func TestSystemVerilogPackageTaskCandidateLimit(t *testing.T) {
	var source strings.Builder
	source.WriteString("package p;\nlocalparam int Neighbor = 1;\n")
	for i := 0; i < 129; i++ {
		source.WriteString("task automatic t")
		source.WriteString(strconv.Itoa(i))
		source.WriteString("(input int x);\nendtask\n")
	}
	source.WriteString("endpackage\n")
	doc := sourceDocumentForScanner(source.String())
	result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	count, neighbor, warn := 0, false, false
	for _, s := range result.Analysis.Symbols {
		if s.NativeKind == "package-task" {
			count++
		}
		if s.QualifiedName == "p.Neighbor" {
			neighbor = true
		}
		if s.QualifiedName == "p.t128" {
			t.Error("candidate past 128 retained")
		}
	}
	for _, d := range result.Analysis.Diagnostics {
		if d.Code == "systemverilog-unsupported-package-task" && d.Range != nil {
			warn = true
		}
	}
	if count != 128 || !neighbor || !warn || result.Analysis.CoverageComplete {
		t.Errorf("candidate bound violated: retained=%d neighbor=%v warning=%v coverage=%v", count, neighbor, warn, result.Analysis.CoverageComplete)
	}
}
