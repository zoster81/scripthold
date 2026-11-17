package sourceintelligence

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestSystemVerilogTwoOriginalPackageNavigation(t *testing.T) {
	tests := []struct {
		env, marker, owner string
		want               []string
	}{
		{"SCRIPTHOLD_REAL_SV_OPENTITAN", "package kmac_pkg;", "kmac_pkg", []string{"MsgWidth", "MaxKeyLen", "key_len_e", "key_len_e.Key128", "key_len_e.Key192"}},
		{"SCRIPTHOLD_REAL_SV_AXI", "package axi_pkg;", "axi_pkg", []string{"BurstWidth", "BURST_FIXED", "mem_type_t", "mem_type_t.DEVICE_NONBUFFERABLE", "mem_type_t.DEVICE_BUFFERABLE"}},
	}
	for _, tc := range tests {
		t.Run(tc.owner, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independent original SystemVerilog file not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 1000 || fi.Size() > 100000 {
				t.Fatalf("invalid original: %v %v", fi, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(b), tc.marker) {
				t.Fatal("original package marker drift")
			}
			doc := sourceDocumentForScanner(string(b))
			doc.Path = path
			result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range tc.want {
				target := tc.owner + "." + name
				found := false
				for _, s := range result.Analysis.Symbols {
					if s.QualifiedName == target {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("original directly authored declaration %s omitted (coverage=%v symbols=%d)", target, result.Analysis.CoverageComplete, len(result.Analysis.Symbols))
				}
			}
		})
	}
}

func TestSystemVerilogEnumMembersPortableAndUnsupported(t *testing.T) {
	const valid = "package p;\n typedef enum logic [1:0] {\n A = 2'b00,\n B,\n C = helper(1, 2)\n } Choice;\n localparam int Outside = 4;\nendpackage\n"
	for _, eol := range []struct{ name, text string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run("valid-"+eol.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(strings.ReplaceAll(valid, "\n", eol.text))
			doc.Path = "portable.sv"
			res, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			for _, wanted := range []struct {
				name string
				line int
			}{{"p.Choice.A", 3}, {"p.Choice.B", 4}, {"p.Choice.C", 5}} {
				found := false
				for _, s := range res.Analysis.Symbols {
					if s.QualifiedName == wanted.name && s.Kind == SymbolKindConstant &&
						s.NativeKind == "enum-value" && s.NameRange.Start.Line == wanted.line {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing owned enum member %+v", wanted)
				}
			}
		})
	}
	many := "typedef enum {"
	for i := 0; i < 130; i++ {
		many += " V" + strconv.Itoa(i) + ","
	}
	many += " } E;"
	for _, tc := range []struct{ name, declaration string }{
		{"duplicate", "typedef enum { A, A } E;"},
		{"empty-entry", "typedef enum { A,, B } E;"},
		{"unsupported-range", "typedef enum { A[3], B } E;"},
		{"macro-invocation", "typedef enum { " + string(rune(96)) + "GENERATED, B } E;"},
		{"missing-value", "typedef enum { A =, B } E;"},
		{"oversized-count", many},
		{"oversized-bytes", "typedef enum { A = " + strings.Repeat(" ", 17000) + "1, B } E;"},
	} {
		for _, eol := range []struct{ name, text string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"-"+eol.name, func(t *testing.T) {
				source := strings.ReplaceAll("package p;\n"+tc.declaration+"\nlocalparam int Outside = 4;\nendpackage\n", "\n", eol.text)
				doc := sourceDocumentForScanner(source)
				doc.Path = "invalid.sv"
				res, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
				if err != nil {
					t.Fatal(err)
				}
				hasNeighbor, hasDiagnostic := false, false
				for _, s := range res.Analysis.Symbols {
					if s.NativeKind == "enum-value" {
						t.Errorf("unsupported members invented: %+v", s)
					}
					if s.QualifiedName == "p.Outside" {
						hasNeighbor = true
					}
				}
				for _, d := range res.Analysis.Diagnostics {
					if d.Code == "systemverilog-unsupported-enum-members" && d.Range != nil {
						hasDiagnostic = true
					}
				}
				if !hasNeighbor || !hasDiagnostic || res.Analysis.CoverageComplete {
					t.Errorf("unsupported enum not safely incomplete: neighbor=%v diagnostic=%v coverage=%v", hasNeighbor, hasDiagnostic, res.Analysis.CoverageComplete)
				}
			})
		}
	}
}
