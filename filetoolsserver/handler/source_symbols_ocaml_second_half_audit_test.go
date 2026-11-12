package handler

import (
	"os"
	"testing"
)

// Provider 79 uses two further independent OCaml projects, excluding the
// earlier standard-library Option and Exercism examples. All selected source
// files were read fully and verified against pinned upstream Git blobs.
func TestSourceSymbolsOCamlSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original OCaml project snapshots not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "ocaml79-yojson-audit-selected", language: "ocaml",
			search: "format_field", primary: "lib/prettyprint.ml",
			negativeFile: "lib/codec.ml", negativeQuery: "format_field",
			paths: []string{"lib/codec.ml", "lib/prettyprint.ml"},
			oracles: []extraAuditEvidence{
				{"lib/codec.ml", "let utf8_of_code buf x ="},
				{"lib/codec.ml", "let code_of_surrogate_pair i j ="},
				{"lib/codec.ml", "let utf8_of_surrogate_pair buf i j ="},
				{"lib/prettyprint.ml", "let rec format ~inside_box"},
				{"lib/prettyprint.ml", "and format_field out (name, x) ="},
				{"lib/prettyprint.ml", "let to_string ?std x ="},
				{"lib/prettyprint.ml", "let to_channel ?std oc x ="},
			},
			targets: []extraAuditSymbol{
				{"lib/codec.ml", "utf8_of_code", 1},
				{"lib/codec.ml", "code_of_surrogate_pair", 41},
				{"lib/codec.ml", "utf8_of_surrogate_pair", 46},
				{"lib/prettyprint.ml", "format", 69},
				{"lib/prettyprint.ml", "format_field", 111},
				{"lib/prettyprint.ml", "to_string", 118},
				{"lib/prettyprint.ml", "to_channel", 121},
			},
		},
		{
			project: "ocaml79-cohttp-audit-selected", language: "ocaml",
			search: "make_for_client", primary: "cohttp/src/request.ml",
			negativeFile: "cohttp/src/response.ml", negativeQuery: "make_for_client",
			paths: []string{"cohttp/src/request.ml", "cohttp/src/response.ml"},
			oracles: []extraAuditEvidence{
				{"cohttp/src/request.ml", "let make_for_client ?headers ?chunked"},
				{"cohttp/src/request.ml", "cannot set both ?chunked and ?body_length"},
				{"cohttp/src/request.ml", "let is_valid_uri path meth ="},
				{"cohttp/src/request.ml", "module Make (IO : S.IO) = struct"},
				{"cohttp/src/request.ml", "let write_header req oc ="},
				{"cohttp/src/response.ml", "let allowed_body response ="},
				{"cohttp/src/response.ml", "let parse_response_fst_line ic ="},
				{"cohttp/src/response.ml", "let write_header res oc ="},
			},
			targets: []extraAuditSymbol{
				{"cohttp/src/request.ml", "t", 19},
				{"cohttp/src/request.ml", "make", 44},
				{"cohttp/src/request.ml", "make_for_client", 95},
				{"cohttp/src/request.ml", "is_valid_uri", 111},
				{"cohttp/src/request.ml", "uri", 119},
				{"cohttp/src/request.ml", "Make", 160},
				{"cohttp/src/request.ml", "write_header", 194},
				{"cohttp/src/response.ml", "make", 40},
				{"cohttp/src/response.ml", "allowed_body", 57},
				{"cohttp/src/response.ml", "Make", 64},
				{"cohttp/src/response.ml", "parse_response_fst_line", 76},
				{"cohttp/src/response.ml", "write_header", 104},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
