package sourceintelligence

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestSystemVerilogModportLexicalSafety(t *testing.T) {
	const original = "interface bus_if();\n" +
		"  logic valid;\n" +
		"  // modport ignored_comment(input valid);\n" +
		"  string text = \"modport ignored_string(input valid);\";\n" +
		"  `define ignored_macro modport ignored_macro(input valid);\n" +
		"  class C;\n" +
		"    modport ignored_class(input valid);\n" +
		"  endclass\n" +
		"  task automatic act(input int x);\n" +
		"    modport ignored_task(input valid);\n" +
		"  endtask\n" +
		"  modport master (\n" +
		"    output valid\n" +
		"  );\n" +
		"  modport slave(input valid);\n" +
		"endinterface\n"
	for _, eol := range []struct{ name, sep string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(eol.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(strings.ReplaceAll(original, "\n", eol.sep))
			result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]int{}
			for _, sym := range result.Analysis.Symbols {
				if sym.NativeKind == "modport" {
					got[sym.QualifiedName] = sym.NameRange.Start.Line
				}
			}
			if len(got) != 2 || got["bus_if.master"] != 12 || got["bus_if.slave"] != 15 {
				t.Fatalf("comment/macro/nested leakage or missing direct modport: %v", got)
			}
		})
	}
	for _, tc := range []struct{ name, declaration string }{
		{"no-parentheses", "modport broken;\n"},
		{"missing-close", "modport broken(input valid;\n"},
		{"missing-semicolon", "modport broken(input valid)\n"},
		{"unsupported-multi-declaration", "modport broken(input valid), extra(input valid);\n"},
		{"empty-name", "modport (input valid);\n"},
		{"directive-inside", "modport broken(\n`FOO\ninput valid);\n"},
		{"overlong-header", "modport broken(" + strings.Repeat(" ", 4200) + "input valid);\n"},
		{"oversized-one-line", "modport broken(" + strings.Repeat(" ", 66000) + "input valid);\n"},
	} {
		for _, eol := range []struct{ name, sep string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"-"+eol.name, func(t *testing.T) {
				source := "interface bus_if();\nlogic valid;\n" + tc.declaration + "endinterface\n"
				doc := sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol.sep))
				result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
				if err != nil {
					t.Fatal(err)
				}
				neighbor, diagnosed := false, false
				for _, sym := range result.Analysis.Symbols {
					if sym.NativeKind == "modport" {
						t.Errorf("invented malformed modport: %s", sym.QualifiedName)
					}
					if sym.QualifiedName == "bus_if.valid" {
						neighbor = true
					}
				}
				for _, diagnostic := range result.Analysis.Diagnostics {
					if diagnostic.Code == "systemverilog-unsupported-modport" && diagnostic.Range != nil {
						diagnosed = true
					}
				}
				if !neighbor || !diagnosed || result.Analysis.CoverageComplete {
					t.Errorf("malformed source not diagnosed: neighbor=%v diagnostic=%v complete=%v", neighbor, diagnosed, result.Analysis.CoverageComplete)
				}
			})
		}
	}
}

func TestSystemVerilogModportDuplicateAndCandidateLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		want   int
	}{
		{"duplicate", "interface bus_if();\nmodport first(input valid);\nmodport first(output valid);\nendinterface\n", 1},
		{"limit", func() string {
			var b strings.Builder
			b.WriteString("interface bus_if();\n")
			for i := 0; i < 129; i++ {
				b.WriteString("modport port")
				b.WriteString(strconv.Itoa(i))
				b.WriteString("(input valid);\n")
			}
			b.WriteString("endinterface\n")
			return b.String()
		}(), 128},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(tc.source)
			result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			count, diagnosed := 0, false
			for _, sym := range result.Analysis.Symbols {
				if sym.NativeKind == "modport" {
					count++
				}
				if sym.QualifiedName == "bus_if.port128" {
					t.Error("over-limit modport retained")
				}
			}
			for _, d := range result.Analysis.Diagnostics {
				if d.Code == "systemverilog-unsupported-modport" && d.Range != nil {
					diagnosed = true
				}
			}
			if count != tc.want || !diagnosed || result.Analysis.CoverageComplete {
				t.Errorf("candidate/duplicate guard failed: symbols=%d want=%d diagnosed=%v complete=%v",
					count, tc.want, diagnosed, result.Analysis.CoverageComplete)
			}
		})
	}
}
