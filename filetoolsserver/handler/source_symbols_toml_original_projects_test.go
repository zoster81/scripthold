package handler

import (
	"os"
	"testing"
)

// Independent original TOML configurations and their Go/R consumers are
// read-only evidence; no original application or embedded command is run.
func TestSourceSymbolsOriginalTOMLInitialQualification(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original TOML corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "toml-gookit-config", language: "toml",
			search: "name", primary: "testdata/toml_base.toml",
			negativeFile: "testdata/toml_other.toml", negativeQuery: "connection_max",
			paths: []string{"testdata/toml_base.toml", "testdata/toml_other.toml"},
			oracles: []extraAuditEvidence{
				{"testdata/toml_base.toml", "title = \"TOML Example\""},
				{"testdata/toml_base.toml", "name = \"app\""},
				{"testdata/toml_base.toml", "[map1]"},
				{"testdata/toml_base.toml", "name = \"inhere\""},
				{"testdata/toml_base.toml", "[owner]"},
				{"testdata/toml_base.toml", "[database]"},
				{"testdata/toml_base.toml", "connection_max = 5000"},
				{"testdata/toml_base.toml", "[servers.alpha]"},
				{"testdata/toml_base.toml", "[servers.beta]"},
				{"testdata/toml_other.toml", "name = \"app2\""},
				{"testdata/toml_other.toml", "Cats = [ \"Cauchy\", \"Plato\" ]"},
				{"toml/toml_test.go", "config.LoadFiles(\"../testdata/toml_base.toml\")"},
				{"toml/toml_test.go", "config.LoadFiles(\"../testdata/toml_other.toml\")"},
				{"toml/toml_test.go", "name := config.String(\"name\")"},
				{"toml/toml_test.go", "val0 = config.String(\"map1.name\")"},
				{"toml/toml_test.go", "// - val: app2"},
				{"_examples/toml.go", "config.LoadFiles(\"testdata/toml_base.toml\")"},
				{"_examples/toml.go", "config.LoadFiles(\"testdata/toml_other.toml\")"},
				{"_examples/toml.go", "config.String(\"map1.name\")"},
			},
			targets: []extraAuditSymbol{
				{"testdata/toml_base.toml", "name", 4},
				{"testdata/toml_base.toml", "map1", 14},
				{"testdata/toml_base.toml", "name", 15},
				{"testdata/toml_base.toml", "name", 19},
				{"testdata/toml_base.toml", "database", 24},
				{"testdata/toml_base.toml", "connection_max", 27},
				{"testdata/toml_base.toml", "alpha", 33},
				{"testdata/toml_base.toml", "beta", 37},
				{"testdata/toml_other.toml", "name", 1},
			},
		},
		{
			project: "toml-miachol-configr", language: "toml",
			search: "global_vars", primary: "inst/extdata/config.global.toml",
			negativeFile: "inst/extdata/config.simple.toml", negativeQuery: "global_vars",
			paths: []string{
				"inst/extdata/config.global.toml",
				"inst/extdata/config.toml",
				"inst/extdata/config.simple.toml",
			},
			oracles: []extraAuditEvidence{
				{"inst/extdata/config.global.toml", "global_vars = [\"gvar_1\", \"gvar_2\", \"gvar_3\", \"gvar_5\"]"},
				{"inst/extdata/config.global.toml", "gvar_1 = \"G1\""},
				{"inst/extdata/config.global.toml", "gvar_5 = \"G5\""},
				{"inst/extdata/config.global.toml", "[subsection]"},
				{"inst/extdata/config.global.toml", "value_1 = \"{{gvar_1}}/value_1\""},
				{"inst/extdata/config.global.toml", "value_2 = \"{{gvar_2}}/value_2\""},
				{"inst/extdata/config.toml", "[default]"},
				{"inst/extdata/config.toml", "debug = \"{{debug}} {{debug2}}\""},
				{"inst/extdata/config.toml", "[glue_parse]"},
				{"inst/extdata/config.simple.toml", "[input]"},
				{"inst/extdata/config.simple.toml", "bam = ["},
				{"R/parse.R", "parse.extra.global.vars <- function(config, global.vars.field = \"global_vars\") {"},
				{"R/parse.R", "config <- parse.extra(config, extra.list = extra.list, global.vars.field = NULL)"},
				{"R/read.R", "glue.flag = \"!!glue\", global.vars.field = \"global_vars\", file.type = NULL, ...) {"},
				{"tests/testthat/test_k_parse.R", "config.global <- system.file(\"extdata\", \"config.global.toml\", package = \"configr\")"},
				{"tests/testthat/test_k_parse.R", "test_that(\"parse.extra global var\", {"},
				{"tests/testthat/test_k_parse.R", "equals(\"G1/value_1\")"},
				{"tests/testthat/test_k_parse.R", "global.vars.field = NULL"},
				{"tests/testthat/test_k_parse.R", "equals(\"{{gvar_1}}/value_1\")"},
			},
			targets: []extraAuditSymbol{
				{"inst/extdata/config.global.toml", "global_vars", 3},
				{"inst/extdata/config.global.toml", "gvar_1", 4},
				{"inst/extdata/config.global.toml", "subsection", 9},
				{"inst/extdata/config.global.toml", "value_1", 10},
				{"inst/extdata/config.toml", "default", 5},
				{"inst/extdata/config.toml", "comments", 8},
				{"inst/extdata/config.simple.toml", "input", 1},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
