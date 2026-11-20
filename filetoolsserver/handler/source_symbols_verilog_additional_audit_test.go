package handler

import (
	"os"
	"testing"
)

// Verify task-scoped navigation against independently read original Verilog
// sources. Neither original RTL design nor its simulations are executed.
func TestSourceSymbolsOriginalVerilogAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Verilog corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "verilog-corundum-corundum", language: "verilog",
			// Find TDMA period defaults, the scheduler integration, and its test.
			search: "SCHEDULE_PERIOD_NS", primary: "fpga/common/rtl/tdma_scheduler.v",
			negativeFile: "fpga/common/rtl/rx_checksum.v", negativeQuery: "SCHEDULE_PERIOD_NS",
			paths: []string{
				"fpga/common/rtl/tdma_scheduler.v",
				"fpga/common/rtl/mqnic_tx_scheduler_block_rr_tdma.v",
				"fpga/common/rtl/tx_scheduler_ctrl_tdma.v",
				"fpga/common/rtl/rx_checksum.v",
			},
			oracles: []extraAuditEvidence{
				{"fpga/common/rtl/tdma_scheduler.v", "module tdma_scheduler #"},
				{"fpga/common/rtl/tdma_scheduler.v", "parameter SCHEDULE_PERIOD_NS = 30'd1000000"},
				{"fpga/common/rtl/tdma_scheduler.v", "parameter TIMESLOT_PERIOD_NS = 30'd100000"},
				{"fpga/common/rtl/tdma_scheduler.v", "parameter ACTIVE_PERIOD_NS = 30'd100000"},
				{"fpga/common/rtl/tdma_scheduler.v", "reg [30:0] schedule_period_ns_reg = SCHEDULE_PERIOD_NS;"},
				{"fpga/common/rtl/mqnic_tx_scheduler_block_rr_tdma.v", "tdma_scheduler #("},
				{"fpga/common/rtl/mqnic_tx_scheduler_block_rr_tdma.v", ".SCHEDULE_PERIOD_NS(30'd1000000),"},
				{"fpga/common/rtl/tx_scheduler_ctrl_tdma.v", "module tx_scheduler_ctrl_tdma #"},
				{"fpga/common/tb/tdma_scheduler/test_tdma_scheduler.py", "parameters['SCHEDULE_PERIOD_NS'] = 1000000"},
				{"fpga/common/rtl/rx_checksum.v", "module rx_checksum #"},
			},
			targets: []extraAuditSymbol{
				{"fpga/common/rtl/tdma_scheduler.v", "tdma_scheduler", 15},
				{"fpga/common/rtl/tdma_scheduler.v", "SCHEDULE_PERIOD_NS", 26},
				{"fpga/common/rtl/tdma_scheduler.v", "TIMESLOT_PERIOD_NS", 30},
				{"fpga/common/rtl/rx_checksum.v", "rx_checksum", 15},
			},
		},
		{
			project: "verilog-darklife-darkriscv", language: "verilog",
			// Locate the UART baud divisor, its IO integration, and source
			// defaults without confusing UART configuration with SPI.
			search: "UART_TIMER", primary: "rtl/darkuart.v",
			negativeFile: "rtl/darkspi.v", negativeQuery: "UART_TIMER",
			paths: []string{
				"rtl/darkuart.v",
				"rtl/darkio.v",
				"rtl/darksocv.v",
				"rtl/darkspi.v",
			},
			oracles: []extraAuditEvidence{
				{"rtl/darkuart.v", "module darkuart"},
				{"rtl/darkuart.v", "reg [15:0]  UART_TIMER = " + "\x60" + "__BAUD__;"},
				{"rtl/darkuart.v", "UART_XBAUD <= UART_XSTATE==" + "\x60" + "UART_STATE_IDLE ? UART_TIMER :"},
				{"rtl/darkio.v", "module darkio"},
				{"rtl/darkio.v", "    darkuart"},
				{"rtl/darkio.v", "    uart0"},
				{"rtl/darksocv.v", "module darksocv"},
				{"rtl/darkspi.v", "module darkspi #(parameter integer DIV_COEF = 0) ("},
				{"rtl/config.vh", "\x60define  __BAUD__ ((\x60BOARD_CK/\x60__UARTSPEED__))"},
			},
			targets: []extraAuditSymbol{
				{"rtl/darkuart.v", "darkuart", 74},
				{"rtl/darkuart.v", "UART_TIMER", 100},
				{"rtl/darkio.v", "darkio", 34},
				{"rtl/darksocv.v", "darksocv", 34},
				{"rtl/darkspi.v", "darkspi", 72},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
