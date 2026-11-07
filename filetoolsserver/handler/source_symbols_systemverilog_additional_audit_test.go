package handler

import (
	"os"
	"testing"
)

// Source-first oracles verify selected navigation, not third-party HDL execution.
func TestSourceSymbolsOriginalSystemVerilogAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original SystemVerilog corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "sv-verilator", language: "systemverilog",
			// Diagnose a streaming packed-interface test without confusing
			// the interface with a simple unrelated Verilog example module.
			search: "pkt_if", primary: "test_regress/t/t_stream_queue_interface.sv",
			negativeFile: "examples/make_hello_c/top.v", negativeQuery: "pkt_if",
			paths: []string{
				"test_regress/t/t_stream_queue_interface.sv",
				"examples/make_hello_c/top.v",
			},
			oracles: []extraAuditEvidence{
				{"test_regress/t/t_stream_queue_interface.sv", "module t;"},
				{"test_regress/t/t_stream_queue_interface.sv", "pkt_if pkt_if_init (clk);"},
				{"test_regress/t/t_stream_queue_interface.sv", "byte_pkt = {>>{pkt_if_init.s.extra}};"},
				{"test_regress/t/t_stream_queue_interface.sv", "interface pkt_if ("},
				{"test_regress/t/t_stream_queue_interface.sv", "typedef struct packed {"},
				{"test_regress/t/t_stream_queue_interface.sv", "} avst_s;"},
				{"test_regress/t/t_stream_queue_interface.sv", "modport src ("},
				{"test_regress/t/t_stream_queue_interface.py", "test.top_filename = \"t/t_stream_queue_interface.sv\""},
				{"test_regress/t/t_stream_queue_interface.py", "--timing -Wno-WIDTHEXPAND"},
				{"examples/make_hello_c/top.v", "module top;"},
			},
			targets: []extraAuditSymbol{
				{"test_regress/t/t_stream_queue_interface.sv", "t", 9},
				{"test_regress/t/t_stream_queue_interface.sv", "pkt_if", 40},
				{"test_regress/t/t_stream_queue_interface.sv", "avst_s", 51},
				{"examples/make_hello_c/top.v", "top", 9},
			},
		},
		{
			project: "systemverilog-24x7fpga-rtl", language: "systemverilog",
			// Find UART TX/RX frame-length defaults and the baud tick source,
			// tracing parent wiring, the APB control and its testbench.
			search: "SB_TICK", primary: "rtl_designs/apb/uart_tx.sv",
			negativeFile: "rtl_designs/apb/apb_master.sv", negativeQuery: "SB_TICK",
			paths: []string{
				"rtl_designs/apb/uart.sv",
				"rtl_designs/apb/uart_tx.sv",
				"rtl_designs/apb/uart_rx.sv",
				"rtl_designs/apb/baud_rate.sv",
				"rtl_designs/apb/apb_ctrl.sv",
				"rtl_designs/apb/tb_apb.sv",
				"rtl_designs/apb/apb_master.sv",
			},
			oracles: []extraAuditEvidence{
				{"rtl_designs/apb/uart.sv", "module uart #(parameter DBIT = 8, SB_TICK = 16)"},
				{"rtl_designs/apb/uart.sv", "baud_rate BR ("},
				{"rtl_designs/apb/uart.sv", "uart_tx #("},
				{"rtl_designs/apb/uart.sv", "uart_rx #("},
				{"rtl_designs/apb/uart_tx.sv", "module uart_tx #(parameter DBIT = 8, SB_TICK = 16)"},
				{"rtl_designs/apb/uart_tx.sv", "if(tk_reg == (SB_TICK-1))begin"},
				{"rtl_designs/apb/uart_rx.sv", "module uart_rx #(parameter DBIT = 8, SB_TICK = 16)"},
				{"rtl_designs/apb/uart_rx.sv", "if(tk_reg == (SB_TICK - 1))begin"},
				{"rtl_designs/apb/baud_rate.sv", "module baud_rate("},
				{"rtl_designs/apb/baud_rate.sv", "assign cnt_nxt = (cnt_reg == dvsr) ? 0 : cnt_reg + 1;"},
				{"rtl_designs/apb/apb_ctrl.sv", "module apb_ctrl("},
				{"rtl_designs/apb/apb_ctrl.sv", ".SB_TICK"},
				{"rtl_designs/apb/tb_apb.sv", "intf.m_br(11'd66);"},
				{"rtl_designs/apb/package.svh", "SB_TICK 16"},
				{"rtl_designs/apb/apb_master.sv", "module apb_master("},
			},
			targets: []extraAuditSymbol{
				{"rtl_designs/apb/uart_tx.sv", "uart_tx", 2},
				{"rtl_designs/apb/uart_tx.sv", "DBIT", 2},
				{"rtl_designs/apb/uart_tx.sv", "SB_TICK", 2},
				{"rtl_designs/apb/uart_rx.sv", "uart_rx", 2},
				{"rtl_designs/apb/uart_rx.sv", "SB_TICK", 2},
				{"rtl_designs/apb/uart.sv", "uart", 2},
				{"rtl_designs/apb/uart.sv", "SB_TICK", 2},
				{"rtl_designs/apb/baud_rate.sv", "baud_rate", 15},
				{"rtl_designs/apb/apb_ctrl.sv", "apb_ctrl", 1},
				{"rtl_designs/apb/tb_apb.sv", "tb_apb", 2},
				{"rtl_designs/apb/apb_master.sv", "apb_master", 1},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
