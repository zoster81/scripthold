package sourceintelligence

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestSystemVerilogPackageFunctionsBoundedSourceNavigation(t *testing.T) {
	const valid = "package p;\n localparam int Neighbor = 1;\n" +
		" // function automatic logic fake_comment();\n" +
		" class Nested;\n function automatic logic fake_class();\n endfunction\n endclass\n" +
		" task automatic NestedTask();\n function automatic logic fake_task();\n endfunction\n endtask\n" +
		" function automatic logic [3:0] direct_good (\n" +
		" input logic [3:0] value\n );\n" +
		" direct_good = value;\n endfunction\n" +
		" function automatic logic second_good(input logic enabled);\n" +
		" second_good = enabled;\n endfunction\nendpackage\n"
	for _, eol := range []struct{ name, separator string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run("valid-"+eol.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(strings.ReplaceAll(valid, "\n", eol.separator))
			result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]int{}
			for _, s := range result.Analysis.Symbols {
				if s.Kind == SymbolKindFunction {
					got[s.QualifiedName] = s.NameRange.Start.Line
				}
			}
			if got["p.direct_good"] != 12 || got["p.second_good"] != 17 || len(got) != 2 {
				t.Errorf("functions must have exact authored owner/line, without nested inventions: %+v", got)
			}
		})
	}
	for _, tc := range []struct{ name, fragment string }{
		{"missing-parameter-list", "function automatic logic Broken;\n endfunction\n"},
		{"unclosed", "function automatic logic Broken(input logic v);\n Broken=v;\n"},
		{"header-over-limit", "function automatic logic Broken(" + strings.Repeat(" ", 4200) + "input logic v);\n endfunction\n"},
		{"body-over-limit", "function automatic logic Broken(input logic v);\n" + strings.Repeat(" ", 66000) + "\n endfunction\n"},
		{"body-over-limit-single-line", "function automatic logic Broken(input logic v);\n" + strings.Repeat(" ", 66000) + "endfunction\n"},
		{"missing-close-paren", "function automatic logic Broken(input logic v;\n endfunction\n"},
	} {
		for _, eol := range []struct{ name, separator string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"-"+eol.name, func(t *testing.T) {
				source := strings.ReplaceAll("package p;\n localparam int Neighbor = 1;\n"+tc.fragment+"endpackage\n", "\n", eol.separator)
				doc := sourceDocumentForScanner(source)
				result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 256))
				if err != nil {
					t.Fatal(err)
				}
				hasNeighbor, warning := false, false
				for _, s := range result.Analysis.Symbols {
					if s.Kind == SymbolKindFunction {
						t.Errorf("unsupported function invented: %+v", s)
					}
					if s.QualifiedName == "p.Neighbor" {
						hasNeighbor = true
					}
				}
				for _, d := range result.Analysis.Diagnostics {
					if (d.Code == "systemverilog-unsupported-package-function" ||
						d.Code == "systemverilog-unclosed-package-function") && d.Range != nil {
						warning = true
					}
				}
				if !hasNeighbor || !warning || result.Analysis.CoverageComplete {
					t.Errorf("unsupported declaration must preserve neighbor and mark incomplete: neighbor=%v warning=%v complete=%v", hasNeighbor, warning, result.Analysis.CoverageComplete)
				}
			})
		}
	}
}

func TestSystemVerilogPackageFunctionCandidateLimit(t *testing.T) {
	var source strings.Builder
	source.WriteString("package p;\nlocalparam int Neighbor = 1;\n")
	for i := 0; i < 129; i++ {
		source.WriteString("function automatic logic f")
		source.WriteString(strconv.Itoa(i))
		source.WriteString("();\nendfunction\n")
	}
	source.WriteString("endpackage\n")
	doc := sourceDocumentForScanner(source.String())
	result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	count, neighbor, warning := 0, false, false
	for _, sym := range result.Analysis.Symbols {
		if sym.NativeKind == "package-function" {
			count++
		}
		if sym.QualifiedName == "p.Neighbor" {
			neighbor = true
		}
		if sym.QualifiedName == "p.f128" {
			t.Error("129th candidate was indexed beyond the limit")
		}
	}
	for _, d := range result.Analysis.Diagnostics {
		if d.Code == "systemverilog-unsupported-package-function" && d.Range != nil {
			warning = true
		}
	}
	if count != 128 || !neighbor || !warning || result.Analysis.CoverageComplete {
		t.Errorf("candidate limit not fail-closed: functions=%d neighbor=%v warning=%v complete=%v", count, neighbor, warning, result.Analysis.CoverageComplete)
	}
}
