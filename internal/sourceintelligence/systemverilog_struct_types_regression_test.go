package sourceintelligence

import (
	"context"
	"testing"
)

// Recognized struct members belong to their type; they must not be reported
// as independent signals of the enclosing interface or module.
func TestSystemVerilogStructTypesInPackageInterfaceAndModule(t *testing.T) {
	for _, eol := range []struct{ name, text string }{
		{"lf", "\n"},
		{"crlf", "\r\n"},
	} {
		t.Run(eol.name, func(t *testing.T) {
			lines := []string{
				"package config_pkg;",
				"    // typedef struct packed {logic imaginary;} bogus_t;",
				"    typedef struct packed {logic [7:0] status;} cfg_t;",
				"endpackage",
				"interface pkt_if(input logic clk);",
				"    typedef struct packed {",
				"        logic [31:0] extra;",
				"        logic valid;",
				"    } avst_s;",
				"    logic ready;",
				"endinterface",
				"module top;",
				"    typedef struct packed {logic [15:0] word;} payload_t;",
				"    logic active;",
				"endmodule",
			}
			source := ""
			for _, line := range lines {
				source += line + eol.text
			}
			result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated {
				t.Fatalf("unexpected truncated analysis: %+v", result.Analysis)
			}

			wanted := map[string]int{
				"config_pkg.cfg_t": 3,
				"pkt_if.avst_s":    9,
				"top.payload_t":    13,
			}
			forbidden := map[string]bool{
				"config_pkg.bogus_t": true,
				"config_pkg.status":  true,
				"pkt_if.extra":       true,
				"pkt_if.valid":       true,
				"top.word":           true,
			}
			found := make(map[string]bool)
			for _, symbol := range result.Analysis.Symbols {
				if forbidden[symbol.QualifiedName] {
					t.Errorf("struct member/comment misclassified: %+v", symbol)
				}
				if line, ok := wanted[symbol.QualifiedName]; ok {
					if found[symbol.QualifiedName] {
						t.Errorf("duplicate type %s", symbol.QualifiedName)
					}
					if symbol.Kind != SymbolKindType || symbol.NameRange.Start.Line != line {
						t.Errorf("type location/kind mismatch: %+v", symbol)
					}
					found[symbol.QualifiedName] = true
				}
			}
			for name := range wanted {
				if !found[name] {
					t.Errorf("missing packed struct type %s", name)
				}
			}
		})
	}
}
