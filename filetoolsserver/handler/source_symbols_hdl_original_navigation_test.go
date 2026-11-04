package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each scenario and its positive/negative oracle was selected by reading and
// grepping the original project before exercising source_*.
func TestSourceSymbolsOriginalHDLTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original source corpus not configured")
	}
	type target struct {
		file, name, kind string
		line             int
		excerpt          string
	}
	type evidence struct{ file, text string }
	cases := []struct {
		name, project, language, query, unrelated string
		files                                     []string
		evidence                                  []evidence
		targets                                   []target
	}{
		{
			name: "verilog-axi-fifo-depth-forwarding", project: "verilog-alexforencich-verilog-axi",
			language: "verilog", query: "WRITE_FIFO_DEPTH", unrelated: "rtl/axi_ram.v",
			files: []string{"rtl/axi_fifo.v", "rtl/axi_fifo_wr.v", "rtl/axi_fifo_rd.v"},
			evidence: []evidence{
				{"rtl/axi_fifo.v", ".FIFO_DEPTH(WRITE_FIFO_DEPTH)"},
				{"rtl/axi_fifo.v", ".FIFO_DEPTH(READ_FIFO_DEPTH)"},
				{"tb/axi_fifo/test_axi_fifo.py", "parameters['WRITE_FIFO_DEPTH'] = 32"},
				{"tb/axi_fifo/test_axi_fifo.py", "parameters['READ_FIFO_DEPTH'] = 32"},
			},
			targets: []target{
				{"rtl/axi_fifo.v", "axi_fifo", "module", 34, "module axi_fifo"},
				{"rtl/axi_fifo.v", "WRITE_FIFO_DEPTH", "constant", 65, "WRITE_FIFO_DEPTH = 32"},
				{"rtl/axi_fifo.v", "READ_FIFO_DEPTH", "constant", 67, "READ_FIFO_DEPTH = 32"},
				{"rtl/axi_fifo_wr.v", "FIFO_DEPTH", "constant", 57, "FIFO_DEPTH = 32"},
				{"rtl/axi_fifo_rd.v", "FIFO_DEPTH", "constant", 53, "FIFO_DEPTH = 32"},
			},
		},
		{
			name: "verilog-ethernet-fcs-error-and-tests", project: "verilog-alexforencich-verilog-ethernet",
			language: "verilog", query: "error_bad_fcs", unrelated: "rtl/axis_eth_fcs_insert.v",
			files: []string{"rtl/axis_eth_fcs_check.v", "rtl/axis_eth_fcs_check_64.v", "tb/test_axis_eth_fcs_check.v", "rtl/axis_eth_fcs_insert.v"},
			evidence: []evidence{
				{"rtl/axis_eth_fcs_check.v", ".LFSR_POLY(32'h4c11db7)"},
				{"rtl/axis_eth_fcs_check_64.v", ".LFSR_POLY(32'h4c11db7)"},
				{"tb/test_axis_eth_fcs_check.py", "assert error_bad_fcs_asserted"},
			},
			targets: []target{
				{"rtl/axis_eth_fcs_check.v", "axis_eth_fcs_check", "module", 34, "module axis_eth_fcs_check"},
				{"rtl/axis_eth_fcs_check.v", "error_bad_fcs", "signal", 61, "error_bad_fcs"},
				{"rtl/axis_eth_fcs_check_64.v", "axis_eth_fcs_check_64", "module", 34, "module axis_eth_fcs_check_64"},
				{"tb/test_axis_eth_fcs_check.v", "test_axis_eth_fcs_check", "module", 32, "module test_axis_eth_fcs_check"},
			},
		},
		{
			name: "systemverilog-fifo-depth-passthrough-and-formal", project: "sv-common-cells",
			language: "systemverilog", query: "Depth", unrelated: "src/cc_counter.sv",
			files: []string{"src/cc_fifo.sv", "formal/cc_fifo_properties.sv", "src/cc_counter.sv"},
			evidence: []evidence{
				{"src/cc_fifo.sv", "if (Depth == 0) begin : gen_pass_through"},
				{"formal/cc_fifo_properties.sv", "assert property (@(posedge clk_i)"},
				{"src/cc_fifo.sv", "localparam int unsigned FifoDepth = (Depth > 0) ? Depth : 1;"},
			},
			targets: []target{
				{"src/cc_fifo.sv", "cc_fifo", "module", 16, "module cc_fifo"},
				{"src/cc_fifo.sv", "Depth", "constant", 19, "Depth       = 8"},
				{"src/cc_fifo.sv", "FifoDepth", "constant", 43, "FifoDepth = (Depth > 0)"},
				{"formal/cc_fifo_properties.sv", "Depth", "constant", 16, "Depth       = 8"},
				{"src/cc_counter.sv", "cc_counter", "module", 14, "module cc_counter"},
			},
		},
		{
			name: "systemverilog-cv32e40p-illegal-branch-decode", project: "sv-cv32e40p",
			language: "systemverilog", query: "illegal_insn_o", unrelated: "rtl/cv32e40p_compressed_decoder.sv",
			files: []string{"rtl/cv32e40p_decoder.sv", "rtl/cv32e40p_compressed_decoder.sv", "rtl/cv32e40p_controller.sv", "rtl/cv32e40p_id_stage.sv"},
			evidence: []evidence{
				{"rtl/cv32e40p_decoder.sv", "OPCODE_BRANCH: begin // Branch"},
				{"rtl/cv32e40p_decoder.sv", "default: illegal_insn_o = 1'b1;"},
				{"rtl/cv32e40p_id_stage.sv", ".illegal_insn_o(illegal_insn_dec)"},
				{"rtl/cv32e40p_compressed_decoder.sv", "OPCODE_BRANCH"},
			},
			targets: []target{
				{"rtl/cv32e40p_decoder.sv", "cv32e40p_decoder", "module", 28, "module cv32e40p_decoder"},
				{"rtl/cv32e40p_decoder.sv", "COREV_PULP", "constant", 33, "COREV_PULP        = 1"},
				{"rtl/cv32e40p_decoder.sv", "illegal_insn_o", "signal", 49, "illegal_insn_o"},
				{"rtl/cv32e40p_compressed_decoder.sv", "cv32e40p_compressed_decoder", "module", 27, "cv32e40p_compressed_decoder"},
				{"rtl/cv32e40p_controller.sv", "cv32e40p_controller", "module", 31, "cv32e40p_controller"},
				{"rtl/cv32e40p_id_stage.sv", "cv32e40p_id_stage", "module", 30, "cv32e40p_id_stage"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			h := NewHandler([]string{root})
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original source %s: %v", rel, err)
				}
				paths = append(paths, path)
			}
			for _, oracle := range tc.evidence {
				data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(oracle.file)))
				if err != nil || !strings.Contains(string(data), oracle.text) {
					t.Fatalf("independent original-source oracle changed %s %q: %v", oracle.file, oracle.text, err)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: tc.language, Encoding: "utf-8",
				Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("compact digest: err=%v failed=%+v result=%+v", err, failed, digest)
			}
			data, err := json.Marshal(digest)
			if err != nil || len(data) > 8192 {
				t.Fatalf("compact digest too large bytes=%d err=%v", len(data), err)
			}
			t.Logf("files=%d symbols=%d bytes=%d reportedComplete=%v", digest.FilesParsed, digest.SymbolCount, len(data), digest.CoverageComplete)
			for _, want := range tc.targets {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: tc.language, Encoding: "utf-8",
					Query: want.name, Match: "exact", MaxFiles: 1, MaxSymbols: 16,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("find %s: err=%v failed=%+v result=%+v", want.name, err, failed, found)
				}
				id := ""
				for _, symbol := range found.Symbols {
					if symbol.Name == want.name && string(symbol.Kind) == want.kind && symbol.DeclarationRange.Start.Line == want.line {
						id = symbol.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("missing original declaration %s:%d %s kind=%s results=%+v", want.file, want.line, want.name, want.kind, found.Symbols)
				}
				// Large module bodies require targeted source reads; never request
				// an unbounded full-module excerpt just to verify its declaration.
				if want.kind == "module" {
					continue
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint,
					Language: tc.language, Encoding: "utf-8", MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.excerpt) {
					t.Fatalf("fingerprint-bound show failed %s: err=%v failed=%+v show=%+v", want.name, err, failed, shown.Show)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Language: tc.language, Encoding: "utf-8",
				Query: tc.query, Match: "exact", Mode: "structural", MaxResults: 25,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
				t.Fatalf("source_query %s: err=%v failed=%+v result=%+v", tc.query, err, failed, search.Search)
			}
			failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))},
				Language: tc.language, Encoding: "utf-8", Query: tc.query, Match: "exact", MaxFiles: 1, MaxSymbols: 10,
			})
			if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated alternative incorrectly returns %s: err=%v failed=%+v symbols=%+v", tc.query, err, failed, negative.Symbols)
			}
		})
	}
}
