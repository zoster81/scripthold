package handler

import (
	"os"
	"testing"
)

// Source-first navigation of independent original Erlang projects.
// Original programs and their test suites are read but never executed.
func TestSourceSymbolsOriginalErlangInitialQualification(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Erlang corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "erlang-ninenines-ranch", language: "erlang",
			search: "prepare_socket_opts", primary: "src/ranch_tcp.erl",
			negativeFile: "test/check_tcp_options.erl", negativeQuery: "prepare_socket_opts",
			paths: []string{
				"src/ranch_tcp.erl",
				"src/ranch.erl",
				"test/check_tcp_options.erl",
			},
			oracles: []extraAuditEvidence{
				{"src/ranch_tcp.erl", "-module(ranch_tcp)."},
				{"src/ranch_tcp.erl", "gen_tcp:listen(0, prepare_socket_opts(SocketOpts, Logger))."},
				{"src/ranch_tcp.erl", "prepare_socket_opts(SocketOpts0, Logger) ->"},
				{"src/ranch_tcp.erl", "ranch:set_option_default(SocketOpts0, backlog, 1024)"},
				{"src/ranch_tcp.erl", "ranch:filter_options(SocketOpts4, disallowed_listen_options(),"},
				{"src/ranch_tcp.erl", "disallowed_listen_options() ->"},
				{"src/ranch.erl", "filter_options(UserOptions, DisallowedKeys, DefaultOptions, Logger) ->"},
				{"src/ranch.erl", "set_option_default(Opts, Key, Value) ->"},
				{"test/check_tcp_options.erl", "inet:getopts(Socket, [Key || {Key, _} <- TcpOptions])"},
				{"test/acceptor_SUITE.erl", "check_tcp_options, [{pid, self()} | Opts1]"},
			},
			targets: []extraAuditSymbol{
				{"src/ranch_tcp.erl", "listen", 91},
				{"src/ranch_tcp.erl", "prepare_socket_opts", 100},
				{"src/ranch_tcp.erl", "disallowed_listen_options", 116},
				{"src/ranch.erl", "filter_options", 581},
				{"src/ranch.erl", "set_option_default", 619},
				{"test/check_tcp_options.erl", "start_link", 7},
				{"test/check_tcp_options.erl", "init", 11},
			},
		},
		{
			project: "erlang-ferd-recon", language: "erlang",
			search: "validate_tspec", primary: "src/recon_trace.erl",
			negativeFile: "src/recon_lib.erl", negativeQuery: "validate_tspec",
			paths: []string{
				"src/recon_trace.erl",
				"test/recon_trace_SUITE.erl",
				"src/recon_lib.erl",
			},
			oracles: []extraAuditEvidence{
				{"src/recon_trace.erl", "-module(recon_trace)."},
				{"src/recon_trace.erl", "validate_tspec(Mod, Fun, Args, PidsOnly) when is_function(Args) ->"},
				{"src/recon_trace.erl", "BannedMods = ['_', ?MODULE, io, lists]"},
				{"src/recon_trace.erl", "error({dangerous_combo, {Mod,Fun,Args}})"},
				{"src/recon_trace.erl", "recon_lib:term_to_pid(PidTerm)"},
				{"test/recon_trace_SUITE.erl", "dangerous_combo(_Config) ->"},
				{"test/recon_trace_SUITE.erl", "dangerous_combo_specific_pids(_Config) ->"},
				{"test/recon_trace_SUITE.erl", "recon_trace:calls([{'_', '_', '_'}], 1)"},
				{"src/recon_lib.erl", "term_to_pid(Pid) when is_pid(Pid) -> Pid;"},
			},
			targets: []extraAuditSymbol{
				{"src/recon_trace.erl", "clear", 225},
				{"src/recon_trace.erl", "calls", 235},
				{"src/recon_trace.erl", "validate_pid_specs", 471},
				{"src/recon_trace.erl", "validate_tspec", 487},
				{"test/recon_trace_SUITE.erl", "dangerous_combo", 13},
				{"test/recon_trace_SUITE.erl", "dangerous_combo_specific_pids", 31},
				{"src/recon_lib.erl", "term_to_pid", 186},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
