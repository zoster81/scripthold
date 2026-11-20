package handler

import (
	"os"
	"testing"
)

// Two further original C projects: source-first implementation, unit tests,
// nearby consumers and a plausible unrelated subsystem. No original code runs.
func TestSourceSymbolsOriginalCAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original C corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "c-curl", language: "c",
			// Trace how final lines without a LF are normalized while
			// bounded dynbuf input rejects overlong lines.
			search:        "Curl_get_line",
			primary:       "lib/curl_get_line.c",
			negativeFile:  "lib/api.c",
			negativeQuery: "Curl_get_line",
			paths: []string{
				"lib/curl_get_line.c",
				"lib/curl_get_line.h",
				"tests/unit/unit3200.c",
				"lib/altsvc.c",
				"lib/api.c",
			},
			oracles: []extraAuditEvidence{
				{"lib/curl_get_line.c", "CURLcode Curl_get_line(struct dynbuf *buf, FILE *input, bool *eof)"},
				{"lib/curl_get_line.c", "curlx_dyn_reset(buf);"},
				{"lib/curl_get_line.c", "if(!b && ferror(input))"},
				{"lib/curl_get_line.c", "return curlx_dyn_addn(buf, \"\\n\", 1);"},
				{"lib/curl_get_line.h", "CURLcode Curl_get_line(struct dynbuf *buf, FILE *input, bool *eof);"},
				{"tests/unit/unit3200.c", "static CURLcode test_unit3200(const char *arg)"},
				{"tests/unit/unit3200.c", "\"LINE2 NONEWLINE\","},
				{"tests/unit/unit3200.c", "strcmp(\"LINE2 NONEWLINE\\n\", line)"},
				{"tests/unit/unit3200.c", "\"Did not bail out on too long line\""},
				{"lib/altsvc.c", "result = Curl_get_line(&buf, fp, &eof);"},
			},
			targets: []extraAuditSymbol{
				{"lib/curl_get_line.c", "Curl_get_line", 35},
				{"lib/curl_get_line.h", "Curl_get_line", 29},
				{"tests/unit/unit3200.c", "test_unit3200", 27},
				{"lib/altsvc.c", "altsvc_load", 237},
			},
		},
		{
			project: "c-genymobile-scrcpy", language: "c",
			// Trace adb serial names containing spaces, noisy daemon
			// startup, CRLF input and device type through parser/tests.
			search:        "sc_adb_parse_devices",
			primary:       "app/src/adb/adb_parser.c",
			negativeFile:  "app/src/audio_player.c",
			negativeQuery: "sc_adb_parse_devices",
			paths: []string{
				"app/src/adb/adb_parser.c",
				"app/src/adb/adb_parser.h",
				"app/tests/test_adb_parser.c",
				"app/src/adb/adb_device.c",
				"app/src/audio_player.c",
			},
			oracles: []extraAuditEvidence{
				{"app/src/adb/adb_parser.c", "static bool"},
				{"app/src/adb/adb_parser.c", "locate_last_token(const char *s, size_t len, size_t *start, size_t *end)"},
				{"app/src/adb/adb_parser.c", "is_device_state(const char *s)"},
				{"app/src/adb/adb_parser.c", "sc_adb_parse_devices(char *str, struct sc_vec_adb_devices *out_vec)"},
				{"app/src/adb/adb_parser.c", "sc_str_remove_trailing_cr(line, len)"},
				{"app/src/adb/adb_parser.c", "sc_adb_parse_device(line, &device);"},
				{"app/src/adb/adb_parser.h", "sc_adb_parse_devices(char *str, struct sc_vec_adb_devices *out_vec);"},
				{"app/tests/test_adb_parser.c", "static void test_adb_devices_cr(void) {"},
				{"app/tests/test_adb_parser.c", "static void test_adb_devices_serial_with_spaces(void) {"},
				{"app/tests/test_adb_parser.c", "adb-0123456789AB-CDdefg (2)._adb-tls-connect._tcp"},
				{"app/src/adb/adb_device.c", "sc_adb_device_get_type(const char *serial) {"},
				{"app/src/adb/adb_device.c", "if (strstr(serial, \"adb-tls-connect\")) {"},
			},
			targets: []extraAuditSymbol{
				{"app/src/adb/adb_parser.c", "is_device_state", 37},
				{"app/src/adb/adb_parser.c", "sc_adb_parse_device", 55},
				{"app/src/adb/adb_parser.c", "sc_adb_parse_devices", 147},
				{"app/src/adb/adb_parser.h", "sc_adb_parse_devices", 18},
				{"app/tests/test_adb_parser.c", "test_adb_devices_cr", 34},
				{"app/tests/test_adb_parser.c", "test_adb_devices_serial_with_spaces", 166},
				{"app/src/adb/adb_device.c", "sc_adb_device_get_type", 30},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
