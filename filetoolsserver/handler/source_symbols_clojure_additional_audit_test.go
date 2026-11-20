package handler

import (
	"os"
	"testing"
)

// Original independent Clojure projects provide source-verified maintenance paths.
// The projects are read, never executed.
func TestSourceSymbolsOriginalClojureAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Clojure corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project:       "clojure-clj-kondo",
			language:      "clojure",
			search:        "ignored?",
			primary:       "src/clj_kondo/impl/findings.clj",
			negativeFile:  "src/clj_kondo/impl/docstring.clj",
			negativeQuery: "ignored?",
			paths: []string{
				"src/clj_kondo/impl/findings.clj",
				"src/clj_kondo/impl/docstring.clj",
				"test/clj_kondo/ignore_test.clj",
			},
			oracles: []extraAuditEvidence{
				{"src/clj_kondo/impl/findings.clj", "(defn legacy-always-true-type"},
				{"src/clj_kondo/impl/findings.clj", "(defn ignore-match? [ignore tp legacy-tp]"},
				{"src/clj_kondo/impl/findings.clj", "(defn ignored?"},
				{"src/clj_kondo/impl/findings.clj", "(defn reg-finding!"},
				{"src/clj_kondo/impl/docstring.clj", "(findings/reg-finding!"},
				{"test/clj_kondo/ignore_test.clj", "(deftest ignore-test"},
				{"test/clj_kondo/ignore_test.clj", "(deftest fine-grained-ignores-test"},
				{"test/clj_kondo/ignore_test.clj", ":clj-kondo/ignore [:invalid-arity]"},
			},
			targets: []extraAuditSymbol{
				{"src/clj_kondo/impl/findings.clj", "legacy-always-true-type", 9},
				{"src/clj_kondo/impl/findings.clj", "ignore-match?", 19},
				{"src/clj_kondo/impl/findings.clj", "ignored?", 24},
				{"src/clj_kondo/impl/findings.clj", "reg-finding!", 74},
				{"src/clj_kondo/impl/docstring.clj", "lint-docstring!", 13},
				{"test/clj_kondo/ignore_test.clj", "ignore-test", 6},
				{"test/clj_kondo/ignore_test.clj", "fine-grained-ignores-test", 38},
			},
		},
		{
			project:       "clojure-core-async",
			language:      "clojure",
			search:        "dropping-buffer",
			primary:       "src/main/clojure/clojure/core/async/impl/buffers.clj",
			negativeFile:  "src/main/clojure/clojure/core/async/impl/timers.clj",
			negativeQuery: "dropping-buffer",
			paths: []string{
				"src/main/clojure/clojure/core/async/impl/buffers.clj",
				"src/main/clojure/clojure/core/async/impl/protocols.clj",
				"src/test/clojure/clojure/core/async/buffers_test.clj",
				"src/main/clojure/clojure/core/async/impl/timers.clj",
			},
			oracles: []extraAuditEvidence{
				{"src/main/clojure/clojure/core/async/impl/buffers.clj", "(deftype DroppingBuffer"},
				{"src/main/clojure/clojure/core/async/impl/buffers.clj", "(when-not (>= (.size buf) n)"},
				{"src/main/clojure/clojure/core/async/impl/buffers.clj", "(deftype SlidingBuffer"},
				{"src/main/clojure/clojure/core/async/impl/buffers.clj", "(impl/remove! this)"},
				{"src/main/clojure/clojure/core/async/impl/protocols.clj", "(defprotocol Buffer"},
				{"src/test/clojure/clojure/core/async/buffers_test.clj", "(deftest dropping-buffer-tests"},
				{"src/test/clojure/clojure/core/async/buffers_test.clj", "(deftest sliding-buffer-tests"},
				{"src/test/clojure/clojure/core/async/buffers_test.clj", "(is (= :2 (remove! fb)))"},
				{"src/main/clojure/clojure/core/async/impl/timers.clj", "(defn timeout"},
			},
			targets: []extraAuditSymbol{
				{"src/main/clojure/clojure/core/async/impl/buffers.clj", "DroppingBuffer", 38},
				{"src/main/clojure/clojure/core/async/impl/buffers.clj", "dropping-buffer", 57},
				{"src/main/clojure/clojure/core/async/impl/buffers.clj", "SlidingBuffer", 60},
				{"src/main/clojure/clojure/core/async/impl/buffers.clj", "sliding-buffer", 80},
				{"src/main/clojure/clojure/core/async/impl/protocols.clj", "Buffer", 31},
				{"src/test/clojure/clojure/core/async/buffers_test.clj", "dropping-buffer-tests", 39},
				{"src/test/clojure/clojure/core/async/buffers_test.clj", "sliding-buffer-tests", 62},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
