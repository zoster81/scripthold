package sourceintelligence

import (
	"context"
	"testing"
)

func TestClojureNamespaceCollectsAllStaticRequireVectors(t *testing.T) {
	text := "(ns demo.core\n  (:require [clojure.string :as str]\n            [clojure.set :as set]\n            [demo.util]))\n(defn run [x] x)\n"
	result, err := (ClojureAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("demo.clj", text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if got := dependencyValues(result.Dependencies); !sameStringSet(got, []string{"clojure.string", "clojure.set", "demo.util"}) {
		t.Fatalf("Clojure namespace dependencies=%v", got)
	}
}

func TestStaticDependencyFormsRejectDynamicTargets(t *testing.T) {
	tests := []struct {
		name     string
		analyzer SourceAnalyzer
		text     string
	}{
		{"r-dynamic-library", RAnalyzer{}, "pkg <- \"stats\"\nlibrary(pkg, character.only = TRUE)\nrun <- function(x) x\n"},
		{"common-lisp-dynamic-require", CommonLispAnalyzer{}, "(let ((name :alexandria)) (require name))\n(defun run (x) x)\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("dynamic.fixture", tc.text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Dependencies) != 0 {
				t.Fatalf("dynamic dependency was overclaimed: %+v", result.Dependencies)
			}
		})
	}
}
