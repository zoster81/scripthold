package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// HDL configuration declarations are source-navigation targets, not evidence
// of elaborated parameter values or module-instance binding.
func TestHDLParameterDeclarationsForDefaultValueNavigation(t *testing.T) {
	cases := []struct {
		name     string
		analyzer SourceAnalyzer
		source   string
		expected map[string]int
	}{
		{
			name:     "verilog",
			analyzer: VerilogAnalyzer{},
			source: "module axi_fifo #(\n" +
				"    parameter WRITE_FIFO_DEPTH = 32,\n" +
				"    parameter READ_FIFO_DEPTH = 32\n" +
				") (input wire clk);\n" +
				"    localparam FIFO_ADDR_WIDTH = $clog2(WRITE_FIFO_DEPTH);\n" +
				"    // parameter FAKE_COMMENT = 0;\n" +
				"    initial $display(\"parameter FAKE_STRING = 0;\");\n" +
				"    axi_fifo_wr #(.FIFO_DEPTH(WRITE_FIFO_DEPTH)) child();\n" +
				"endmodule\n",
			expected: map[string]int{
				"axi_fifo.WRITE_FIFO_DEPTH": 2,
				"axi_fifo.READ_FIFO_DEPTH":  3,
				"axi_fifo.FIFO_ADDR_WIDTH":  5,
			},
		},
		{
			name:     "systemverilog",
			analyzer: SystemVerilogAnalyzer{},
			source: "module cc_fifo #(\n" +
				"    parameter bit FallThrough = 1'b0,\n" +
				"    parameter int unsigned Depth = 8,\n" +
				"    parameter type data_t = logic [31:0],\n" +
				"    localparam int unsigned UsageWidth = 4\n" +
				") (input logic clk);\n" +
				"    localparam int unsigned FifoDepth = (Depth > 0) ? Depth : 1;\n" +
				"    // localparam int unsigned FAKE_COMMENT = 1;\n" +
				"endmodule\n",
			expected: map[string]int{
				"cc_fifo.FallThrough": 2,
				"cc_fifo.Depth":       3,
				"cc_fifo.data_t":      4,
				"cc_fifo.UsageWidth":  5,
				"cc_fifo.FifoDepth":   7,
			},
		},
		{
			name:     "systemverilog-inline-header-comma",
			analyzer: SystemVerilogAnalyzer{},
			source: "module uart_tx #(parameter DBIT = 8, SB_TICK = 16)(input logic clk);\n" +
				"    // parameter FAKE_COMMENT = 0;\n" +
				"    initial $display(\"parameter FAKE_STRING = 0;\");\n" +
				"    uart_rx #(.SB_TICK(16)) receiver();\n" +
				"endmodule\n",
			expected: map[string]int{
				"uart_tx.DBIT":    1,
				"uart_tx.SB_TICK": 1,
			},
		},
		{
			name:     "verilog-multiline-header-continuation",
			analyzer: VerilogAnalyzer{},
			source: "module fifo #(\n" +
				"    parameter integer DEPTH = choose(8, 16),\n" +
				"    ADDR_BITS = $clog2(DEPTH),\n" +
				"    parameter WIDTH = 8\n" +
				") (input wire clk);\n" +
				"    child #(.DEPTH(DEPTH), .ADDR_BITS(ADDR_BITS)) u_child();\n" +
				"endmodule\n",
			expected: map[string]int{
				"fifo.DEPTH":     2,
				"fifo.ADDR_BITS": 3,
				"fifo.WIDTH":     4,
			},
		},
		{
			name:     "systemverilog-interface-type-continuation",
			analyzer: SystemVerilogAnalyzer{},
			source: "interface api_if #(\n" +
				"    parameter type payload_t = logic [7:0], mirror_t = payload_t,\n" +
				"    localparam int unsigned DEPTH = 4\n" +
				")(input logic clk);\n" +
				"endinterface\n",
			expected: map[string]int{
				"api_if.payload_t": 2,
				"api_if.mirror_t":  2,
				"api_if.DEPTH":     3,
			},
		},
	}
	for _, tc := range cases {
		for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"-"+eol.name, func(t *testing.T) {
				doc := sourceDocumentForScanner(strings.ReplaceAll(tc.source, "\n", eol.value))
				result, err := tc.analyzer.Analyze(context.Background(), doc, testAnalyzeOptions(true, 128))
				if err != nil {
					t.Fatal(err)
				}
				if result.Analysis.Truncated {
					t.Fatalf("unexpected truncation: %+v", result.Analysis)
				}
				found := make(map[string]bool)
				for _, s := range result.Analysis.Symbols {
					if s.Name == "FAKE_COMMENT" || s.Name == "FAKE_STRING" {
						t.Fatalf("comment/string invented as parameter: %+v", s)
					}
					line, wanted := tc.expected[s.QualifiedName]
					if !wanted {
						continue
					}
					wantKind := SymbolKindConstant
					if s.QualifiedName == "cc_fifo.data_t" || s.QualifiedName == "api_if.payload_t" || s.QualifiedName == "api_if.mirror_t" {
						wantKind = SymbolKindType
					}
					if s.Kind != wantKind || s.DeclarationRange.Start.Line != line {
						t.Fatalf("parameter %s wrong kind/location: %+v", s.QualifiedName, s)
					}
					if found[s.QualifiedName] {
						t.Fatalf("duplicate HDL parameter declaration %s", s.QualifiedName)
					}
					found[s.QualifiedName] = true
				}
				for name := range tc.expected {
					if !found[name] {
						t.Errorf("missing original-style HDL parameter %s", name)
					}
				}
			})
		}
	}
}
