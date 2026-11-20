package handler

import (
	"os"
	"testing"
)

// Read-only checks on two additional independent original Common Lisp projects.
func TestSourceSymbolsOriginalCommonLispAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Common Lisp corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "common-lisp-april", language: "common-lisp",
			search: "indexer-section", primary: "aplesque/forms.lisp",
			negativeFile: "varray/base.lisp", negativeQuery: "indexer-section",
			paths: []string{"aplesque/forms.lisp", "aplesque/aplesque.lisp", "aplesque/package.lisp", "package.lisp", "varray/base.lisp"},
			oracles: []extraAuditEvidence{
				{"aplesque/forms.lisp", "(defun indexer-section (inverse dims dimensions output-shorter)"},
				{"aplesque/forms.lisp", "Return indices of an array sectioned"},
				{"aplesque/forms.lisp", "(:import-from :aplesque #:indexer-split #:indexer-section"},
				{"aplesque/aplesque.lisp", "(defun get-dimensional-factors (dimensions &optional as-vector)"},
				{"aplesque/aplesque.lisp", "(indexer (indexer-section inverse (dims input) dimensions"},
				{"package.lisp", "(:shadowing-import-from #:aplesque.forms #:indexer-section #:indexer-expand)"},
				{"varray/index.lisp", "(defun indexer-section (dims span pad output-shorter iwidth itype &optional is-flat)"},
				{"varray/base.lisp", "(defclass varray ()"},
			},
			targets: []extraAuditSymbol{
				{"aplesque/forms.lisp", "indexer-split", 8},
				{"aplesque/forms.lisp", "indexer-section", 20},
				{"aplesque/aplesque.lisp", "get-dimensional-factors", 89},
				{"aplesque/aplesque.lisp", "rmi-from-subscript-vector", 105},
				{"aplesque/package.lisp", "aplesque", 4},
				{"varray/base.lisp", "varray", 35},
			},
		},
		{
			project: "common-lisp-cl-gserver", language: "common-lisp",
			search: "merge-config", primary: "src/config.lisp",
			negativeFile: "src/actor-api.lisp", negativeQuery: "merge-config",
			paths: []string{"src/config.lisp", "tests/config-test.lisp", "src/actor-api.lisp"},
			oracles: []extraAuditEvidence{
				{"src/config.lisp", "(defun config-from (config-string)"},
				{"src/config.lisp", "(defun retrieve-value (section key)"},
				{"src/config.lisp", "(defun merge-config (config fallback-config)"},
				{"src/config.lisp", "(defun %merge-config (key config fallback)"},
				{"tests/config-test.lisp", "(:use :cl :fiveam :sento.config)"},
				{"tests/config-test.lisp", "(def-suite config-tests"},
				{"tests/config-test.lisp", "(in-suite config-tests)"},
				{"tests/config-test.lisp", "(test merge-config--deep"},
				{"tests/config-test.lisp", "(test merge-config--no-fallback"},
				{"tests/config-test.lisp", "(test merge-config--config+fallback--flat"},
				{"src/actor-api.lisp", "(defclass actor (actor-cell)"},
			},
			targets: []extraAuditSymbol{
				{"src/config.lisp", "config-from", 12},
				{"src/config.lisp", "retrieve-value", 41},
				{"src/config.lisp", "merge-config", 50},
				{"src/config.lisp", "%merge-config", 60},
				{"tests/config-test.lisp", "config-tests", 8},
				{"tests/config-test.lisp", "merge-config--no-fallback", 71},
				{"tests/config-test.lisp", "merge-config--config+fallback--flat", 93},
				{"tests/config-test.lisp", "merge-config--deep", 107},
				{"src/actor-api.lisp", "actor", 29},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
