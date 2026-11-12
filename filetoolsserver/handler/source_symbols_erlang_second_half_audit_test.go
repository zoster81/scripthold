package handler

import (
	"os"
	"testing"
)

// Provider 65: Cowboy and Erlang/OTP are independent original-project origins,
// different from the initial Ranch and recon Erlang cases. All selected files
// were read in full before constructing these maintenance navigation oracles.
// No third-party Erlang source or tests are executed.
func TestSourceSymbolsErlangSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original Erlang projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "erlang65-cowboy-audit-selected", language: "erlang",
			search: "compile_rules", primary: "src/cowboy_router.erl",
			negativeFile: "src/cowboy_handler.erl", negativeQuery: "compile_rules",
			paths: []string{"src/cowboy_router.erl", "src/cowboy_handler.erl"},
			oracles: []extraAuditEvidence{
				{"src/cowboy_router.erl", "compile_rules(HostMatch, $., [], [], <<>>)."},
				{"src/cowboy_router.erl", "PathRules = compile_paths(Paths, []),"},
				{"src/cowboy_router.erl", "compile_rules(<< $:, Rest/bits >>, S, Segments, Rules, <<>>) ->"},
				{"src/cowboy_router.erl", "compile_brackets_split(Rest, <<>>, 0),"},
				{"src/cowboy_router.erl", "case match(Dispatch, Host, Path) of"},
				{"src/cowboy_router.erl", "case cowboy_constraints:validate(Value0, Constraints) of"},
				{"src/cowboy_router.erl", "compile_test_() ->"},
				{"src/cowboy_router.erl", "match_constraints_test() ->"},
				{"src/cowboy_handler.erl", "-callback init(Req, any())"},
				{"src/cowboy_handler.erl", "try Handler:init(Req, HandlerOpts) of"},
				{"src/cowboy_handler.erl", "Mod:upgrade(Req2, Env, Handler, State, Opts)"},
				{"src/cowboy_handler.erl", "erlang:function_exported(Handler, terminate, 3)"},
			},
			targets: []extraAuditSymbol{
				{"src/cowboy_router.erl", "compile", 51},
				{"src/cowboy_router.erl", "compile_host", 70},
				{"src/cowboy_router.erl", "compile_paths", 75},
				{"src/cowboy_router.erl", "compile_rules", 96},
				{"src/cowboy_router.erl", "compile_binding", 129},
				{"src/cowboy_router.erl", "compile_brackets", 139},
				{"src/cowboy_router.erl", "execute", 163},
				{"src/cowboy_router.erl", "match", 222},
				{"src/cowboy_router.erl", "match_path", 253},
				{"src/cowboy_router.erl", "check_constraints", 280},
				{"src/cowboy_router.erl", "compile_test_", 394},
				{"src/cowboy_router.erl", "match_constraints_test", 555},
				{"src/cowboy_handler.erl", "execute", 36},
				{"src/cowboy_handler.erl", "terminate", 51},
			},
		},
		{
			project: "erlang65-otp-audit-selected", language: "erlang",
			search: "mime_decode", primary: "lib/stdlib/src/base64.erl",
			negativeFile: "lib/stdlib/test/base64_SUITE.erl", negativeQuery: "get_padding",
			paths: []string{"lib/stdlib/src/base64.erl", "lib/stdlib/test/base64_SUITE.erl"},
			oracles: []extraAuditEvidence{
				{"lib/stdlib/src/base64.erl", "-type decode_options() :: #{padding => boolean(), mode => base64_mode()}."},
				{"lib/stdlib/src/base64.erl", "decode(Bin, Options) when is_binary(Bin) ->"},
				{"lib/stdlib/src/base64.erl", "mime_decode_binary(get_decoding_offset(Options), get_padding(Options), Bin, <<>>);"},
				{"lib/stdlib/src/base64.erl", "get_padding(#{}) -> true."},
				{"lib/stdlib/src/base64.erl", "missing_padding_error() ->"},
				{"lib/stdlib/test/base64_SUITE.erl", "base64_decode_modes(Config) when is_list(Config) ->"},
				{"lib/stdlib/test/base64_SUITE.erl", "?assertError(missing_padding, base64:decode("},
				{"lib/stdlib/test/base64_SUITE.erl", "MimeDecode(Base64, #{mode => urlsafe})"},
				{"lib/stdlib/test/base64_SUITE.erl", "interleaved_ws_roundtrip(Bin, List, Base64List)"},
			},
			targets: []extraAuditSymbol{
				{"lib/stdlib/src/base64.erl", "decode", 293},
				{"lib/stdlib/src/base64.erl", "mime_decode", 331},
				{"lib/stdlib/src/base64.erl", "mime_decode_binary", 496},
				{"lib/stdlib/src/base64.erl", "missing_padding_error", 808},
				{"lib/stdlib/src/base64.erl", "format_error", 812},
				{"lib/stdlib/src/base64.erl", "get_padding", 823},
				{"lib/stdlib/src/base64.erl", "get_decoding_offset", 826},
				{"lib/stdlib/test/base64_SUITE.erl", "base64_decode", 120},
				{"lib/stdlib/test/base64_SUITE.erl", "base64_decode_modes", 210},
				{"lib/stdlib/test/base64_SUITE.erl", "illegal", 248},
				{"lib/stdlib/test/base64_SUITE.erl", "mime_decode_modes", 307},
				{"lib/stdlib/test/base64_SUITE.erl", "interleaved_ws_roundtrip", 480},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
