package handler

import (
	"os"
	"testing"
)

// These scenarios check task-localized structural navigation on independent
// original VHDL projects, not VHDL synthesis, simulation, or port binding.
func TestSourceSymbolsOriginalVHDLAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original VHDL corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "vhdl-hamsternz-fpga_webserver", language: "vhdl",
			// Adjust the generated UDP broadcast packet interval and destination;
			// inspect the separate receive-only LED sink and the UDP forwarding.
			search: "udp_test_source", primary: "hdl/udp/udp_test_source.vhd",
			negativeFile: "hdl/icmp/icmp_handler.vhd", negativeQuery: "udp_test_source",
			paths: []string{
				"hdl/udp/udp_test_source.vhd",
				"hdl/udp/udp_test_sink.vhd",
				"hdl/udp/udp_handler.vhd",
				"hdl/main_design.vhd",
				"hdl/icmp/icmp_handler.vhd",
			},
			oracles: []extraAuditEvidence{
				{"hdl/udp/udp_test_source.vhd", "entity udp_test_source is"},
				{"hdl/udp/udp_test_source.vhd", "countdown <= to_unsigned(124_999_999,28);"},
				{"hdl/udp/udp_test_source.vhd", "udp_tx_dst_port <= std_logic_vector(to_unsigned(9029,16));"},
				{"hdl/udp/udp_test_sink.vhd", "entity udp_test_sink is"},
				{"hdl/udp/udp_test_sink.vhd", "udp_rx_dst_port = std_logic_vector(to_unsigned(4660, 16))"},
				{"hdl/udp/udp_handler.vhd", "i_udp_tx_packet : udp_tx_packet generic map ("},
				{"hdl/main_design.vhd", "i_udp_handler: udp_handler "},
				{"hdl/main_design.vhd", "udp_tx_dst_port      => udp_tx_dst_port"},
				{"testbenches/tb_main_design.vhd", "i_main_design: main_design port map ("},
				{"hdl/icmp/icmp_handler.vhd", "entity icmp_handler is"},
			},
			targets: []extraAuditSymbol{
				{"hdl/udp/udp_test_source.vhd", "udp_test_source", 37},
				{"hdl/udp/udp_test_source.vhd", "Behavioral", 50},
				{"hdl/udp/udp_test_source.vhd", "countdown", 53},
				{"hdl/udp/udp_test_sink.vhd", "udp_test_sink", 38},
				{"hdl/udp/udp_handler.vhd", "udp_handler", 38},
				{"hdl/main_design.vhd", "main_design", 39},
				{"hdl/icmp/icmp_handler.vhd", "icmp_handler", 38},
			},
		},
		{
			project: "vhdl-ikorb-gcvideo", language: "vhdl",
			// Change the VSync-derived heartbeat interval and follow the LED
			// outputs through two board top levels, not the scanline generator.
			search: "LED_Heartbeat", primary: "HDL/gcvideo_dvi/src/led_heartbeat.vhd",
			negativeFile:  "HDL/gcvideo_dvi/src/scanline_generator.vhd",
			negativeQuery: "LED_Heartbeat",
			paths: []string{
				"HDL/gcvideo_dvi/src/led_heartbeat.vhd",
				"HDL/gcvideo_dvi/src/toplevel_gcdual.vhd",
				"HDL/gcvideo_dvi/src/toplevel_wiidual.vhd",
				"HDL/gcvideo_dvi/src/component_defs.vhd",
				"HDL/gcvideo_dvi/src/scanline_generator.vhd",
			},
			oracles: []extraAuditEvidence{
				{"HDL/gcvideo_dvi/src/led_heartbeat.vhd", "entity LED_Heartbeat is"},
				{"HDL/gcvideo_dvi/src/led_heartbeat.vhd", "type vsync_counter_t is range 0 to 119;"},
				{"HDL/gcvideo_dvi/src/led_heartbeat.vhd", "vsync_counter <= vsync_counter_t'high;"},
				{"HDL/gcvideo_dvi/src/led_heartbeat.vhd", "HeartbeatVsync2 <= hb_vsync;"},
				{"HDL/gcvideo_dvi/src/toplevel_gcdual.vhd", "Inst_Heartbeat: LED_Heartbeat port map ("},
				{"HDL/gcvideo_dvi/src/toplevel_gcdual.vhd", "LED <= heartbeat_vsync;"},
				{"HDL/gcvideo_dvi/src/toplevel_wiidual.vhd", "Inst_Heartbeat: LED_Heartbeat port map ("},
				{"HDL/gcvideo_dvi/src/toplevel_wiidual.vhd", "LED <= heartbeat_vsync;"},
				{"HDL/gcvideo_dvi/src/component_defs.vhd", "component LED_Heartbeat is"},
				{"HDL/gcvideo_dvi/src/scanline_generator.vhd", "entity scanline_generator is"},
			},
			targets: []extraAuditSymbol{
				{"HDL/gcvideo_dvi/src/led_heartbeat.vhd", "LED_Heartbeat", 41},
				{"HDL/gcvideo_dvi/src/led_heartbeat.vhd", "Behavioral", 51},
				{"HDL/gcvideo_dvi/src/led_heartbeat.vhd", "vsync_counter_t", 52},
				{"HDL/gcvideo_dvi/src/led_heartbeat.vhd", "vsync_counter", 55},
				{"HDL/gcvideo_dvi/src/led_heartbeat.vhd", "hb_vsync", 59},
				{"HDL/gcvideo_dvi/src/toplevel_gcdual.vhd", "toplevel_gcdual", 41},
				{"HDL/gcvideo_dvi/src/toplevel_wiidual.vhd", "toplevel_wiidual", 41},
				{"HDL/gcvideo_dvi/src/scanline_generator.vhd", "scanline_generator", 39},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
