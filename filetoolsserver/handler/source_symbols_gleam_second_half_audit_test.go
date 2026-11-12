package handler

import (
	"os"
	"testing"
)

// Provider 68: stdlib and json are independent original Gleam project origins,
// not the previously qualified gleam-lang/http or gleam-lang/otp fixtures.
// All six selected original files were read in full and their Git blobs verified
// before these source-derived navigation checks were authored.
func TestSourceSymbolsGleamSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original Gleam project snapshots not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "gleam68-stdlib-audit-selected", language: "gleam",
			search: "all", primary: "src/gleam/option.gleam",
			negativeFile: "src/gleam/result.gleam", negativeQuery: "all_loop",
			paths: []string{
				"src/gleam/option.gleam",
				"test/gleam/option_test.gleam",
				"src/gleam/result.gleam",
			},
			oracles: []extraAuditEvidence{
				{"src/gleam/option.gleam", "pub type Option(a) {"},
				{"src/gleam/option.gleam", "Some(a)"},
				{"src/gleam/option.gleam", "pub fn all(list: List(Option(a))) -> Option(List(a)) {"},
				{"src/gleam/option.gleam", "[None, ..] -> None"},
				{"src/gleam/option.gleam", "[Some(first), ..rest] -> all_loop(rest, [first, ..acc])"},
				{"src/gleam/option.gleam", "pub fn lazy_unwrap(option: Option(a), or default: fn() -> a) -> a {"},
				{"src/gleam/option.gleam", "Some(x) -> x"},
				{"src/gleam/option.gleam", "None -> default()"},
				{"src/gleam/option.gleam", "pub fn then(option: Option(a), apply fun: fn(a) -> Option(b)) -> Option(b) {"},
				{"src/gleam/option.gleam", "pub fn values(options: List(Option(a))) -> List(a) {"},
				{"test/gleam/option_test.gleam", "pub fn all_with_none_test() {"},
				{"test/gleam/option_test.gleam", "assert option.all([Some(1), None, Some(3)]) == None"},
				{"test/gleam/option_test.gleam", "pub fn then_none_test() {"},
				{"src/gleam/result.gleam", "pub fn all(results: List(Result(a, e))) -> Result(List(a), e) {"},
				{"src/gleam/result.gleam", "list.try_map(results, fn(result) { result })"},
			},
			targets: []extraAuditSymbol{
				{"src/gleam/option.gleam", "Option", 19},
				{"src/gleam/option.gleam", "all", 38},
				{"src/gleam/option.gleam", "all_loop", 42},
				{"src/gleam/option.gleam", "reverse", 53},
				{"src/gleam/option.gleam", "to_result", 108},
				{"src/gleam/option.gleam", "from_result", 127},
				{"src/gleam/option.gleam", "lazy_unwrap", 165},
				{"src/gleam/option.gleam", "then", 246},
				{"src/gleam/option.gleam", "values", 316},
				{"src/gleam/option.gleam", "values_loop", 320},
				{"test/gleam/option_test.gleam", "all_with_none_test", 11},
				{"test/gleam/option_test.gleam", "then_none_test", 95},
				{"src/gleam/result.gleam", "all", 287},
				{"src/gleam/result.gleam", "partition_loop", 307},
			},
		},
		{
			project: "gleam68-json-audit-selected", language: "gleam",
			search: "parse", primary: "src/gleam/json.gleam",
			negativeFile: "test/gleam_json_js_ffi_test.gleam", negativeQuery: "nullable",
			paths: []string{
				"src/gleam/json.gleam",
				"test/gleam_json_test.gleam",
				"test/gleam_json_js_ffi_test.gleam",
			},
			oracles: []extraAuditEvidence{
				{"src/gleam/json.gleam", "pub type DecodeError {"},
				{"src/gleam/json.gleam", "pub fn parse("},
				{"src/gleam/json.gleam", "@target(erlang)"},
				{"src/gleam/json.gleam", "@target(javascript)"},
				{"src/gleam/json.gleam", "use dynamic_value <- result.try(decode_string(json))"},
				{"src/gleam/json.gleam", "parse_bits(bits, decoder)"},
				{"src/gleam/json.gleam", "pub fn parse_bits("},
				{"src/gleam/json.gleam", "pub fn nullable(from input: Option(a), of inner_type: fn(a) -> Json) -> Json {"},
				{"src/gleam/json.gleam", "None -> null()"},
				{"src/gleam/json.gleam", "pub fn array(from entries: List(a), of inner_type: fn(a) -> Json) -> Json {"},
				{"test/gleam_json_test.gleam", "pub fn parse_unexpected_sequence_test() {"},
				{"test/gleam_json_test.gleam", "pub fn encode_nullable_none_test() {"},
				{"test/gleam_json_test.gleam", "fn should_encode(data: Json, expected: String) {"},
				{"test/gleam_json_js_ffi_test.gleam", "pub fn chromium_unexpected_byte_test() {"},
				{"test/gleam_json_js_ffi_test.gleam", "fn get_json_decode_error(a: StandardError, b: String) -> DecodeError"},
			},
			targets: []extraAuditSymbol{
				{"src/gleam/json.gleam", "DecodeError", 12},
				{"src/gleam/json.gleam", "parse", 39},
				{"src/gleam/json.gleam", "do_parse", 47},
				{"src/gleam/json.gleam", "do_parse", 56},
				{"src/gleam/json.gleam", "decode_string", 66},
				{"src/gleam/json.gleam", "parse_bits", 88},
				{"src/gleam/json.gleam", "decode_to_dynamic", 98},
				{"src/gleam/json.gleam", "to_string_tree", 140},
				{"src/gleam/json.gleam", "nullable", 241},
				{"src/gleam/json.gleam", "object", 260},
				{"src/gleam/json.gleam", "array", 277},
				{"src/gleam/json.gleam", "dict", 310},
				{"test/gleam_json_test.gleam", "parse_unexpected_sequence_test", 86},
				{"test/gleam_json_test.gleam", "encode_nullable_none_test", 158},
				{"test/gleam_json_test.gleam", "should_encode", 178},
				{"test/gleam_json_js_ffi_test.gleam", "chromium_unexpected_byte_test", 38},
				{"test/gleam_json_js_ffi_test.gleam", "get_json_decode_error", 76},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
