package handler

// Source-derived maintenance questions on two independent original projects.
func originalClojureCases() []originalClojureCase {
	return []originalClojureCase{
		{
			project: "clojure-ring", query: "params-request",
			unrelated: "ring-core/src/ring/middleware/cookies.clj",
			files: []string{
				"ring-core/src/ring/middleware/params.clj",
				"ring-core/src/ring/util/request.clj",
				"ring-core/test/ring/middleware/test/params.clj",
				"ring-core/src/ring/middleware/cookies.clj",
			},
			source: []originalClojureText{
				{"ring-core/src/ring/middleware/params.clj", "(defn params-request"},
				{"ring-core/src/ring/middleware/params.clj", "(or (:encoding options)"},
				{"ring-core/src/ring/middleware/params.clj", "(req/character-encoding request)"},
				{"ring-core/src/ring/middleware/params.clj", "UTF-8"},
				{"ring-core/src/ring/middleware/params.clj", "(assoc-form-params request encoding)"},
				{"ring-core/src/ring/middleware/params.clj", "(assoc-query-params request encoding)"},
				{"ring-core/src/ring/util/request.clj", "(defn character-encoding"},
				{"ring-core/test/ring/middleware/test/params.clj", "(deftest wrap-params-encoding"},
				{"ring-core/test/ring/middleware/test/params.clj", "(deftest wrap-params-query-params-only"},
			},
			targets: []originalClojureTarget{
				{"ring-core/src/ring/middleware/params.clj", "params-request", 39},
				{"ring-core/src/ring/middleware/params.clj", "wrap-params", 56},
				{"ring-core/src/ring/middleware/params.clj", "assoc-query-params", 16},
				{"ring-core/src/ring/middleware/params.clj", "assoc-form-params", 27},
				{"ring-core/src/ring/util/request.clj", "character-encoding", 31},
			},
		},
		{
			project: "clojure-weavejester-compojure", query: "head-response",
			unrelated: "src/compojure/response.clj",
			files:     []string{"src/compojure/core.clj", "test/compojure/core_test.clj", "src/compojure/response.clj", "src/compojure/route.clj"},
			source: []originalClojureText{
				{"src/compojure/core.clj", "(defn- method-matches?"},
				{"src/compojure/core.clj", "(or (= method :head) (= method :get))"},
				{"src/compojure/core.clj", "(defn- head-response"},
				{"src/compojure/core.clj", "(assoc response :body nil)"},
				{"src/compojure/core.clj", "(head-response request method)"},
				{"src/compojure/core.clj", "(defmacro GET"},
				{"test/compojure/core_test.clj", "(deftest route-matching"},
				{"test/compojure/core_test.clj", "HEAD requests"},
				{"test/compojure/core_test.clj", "(assoc resp :body nil)"},
			},
			targets: []originalClojureTarget{
				{"src/compojure/core.clj", "method-matches?", 20},
				{"src/compojure/core.clj", "head-response", 33},
				{"src/compojure/core.clj", "make-route", 162},
				{"src/compojure/core.clj", "GET", 209},
			},
		},
	}
}
