package handler

import (
	"os"
	"testing"
)

// Original R packages are source-only evidence; neither package is executed.
func TestSourceSymbolsOriginalRInitialQualification(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original R corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "r-janitor", language: "r",
			search: "make_clean_names", primary: "R/make_clean_names.R",
			negativeFile: "R/remove_empties.R", negativeQuery: "make_clean_names",
			paths: []string{
				"R/make_clean_names.R", "R/clean_names.R",
				"tests/testthat/test-clean-names.R", "R/remove_empties.R",
			},
			oracles: []extraAuditEvidence{
				{"R/make_clean_names.R", "make_clean_names <- function(string,"},
				{"R/make_clean_names.R", "allow_dupes = FALSE,"},
				{"R/make_clean_names.R", "while (any(duplicated(cased_names))) {"},
				{"R/make_clean_names.R", "cased_names[dupe_count > 1] <-"},
				{"R/make_clean_names.R", "old_make_clean_names <- function(string) {"},
				{"R/clean_names.R", "clean_names <- function(dat, ...) {"},
				{"R/clean_names.R", "names(dat) <- make_clean_names(names(dat), ...)"},
				{"R/clean_names.R", "sf_cleaned <- make_clean_names(sf_names[cols_to_rename], ...)"},
				{"tests/testthat/test-clean-names.R", "test_that(\"do not create duplicates (fix #251)\", {"},
				{"tests/testthat/test-clean-names.R", "c(\"a\", \"a_2\", \"a_2_2\")"},
				{"tests/testthat/test-clean-names.R", "test_that(\"allow for duplicates (fix #495)\", {"},
				{"tests/testthat/test-clean-names.R", "make_clean_names(c(\"a\", \"a\", \"a_2\"), allow_dupes = TRUE)"},
				{"R/remove_empties.R", "remove_empty <- function(dat, which = c(\"rows\", \"cols\")"},
			},
			targets: []extraAuditSymbol{
				{"R/make_clean_names.R", "make_clean_names", 78},
				{"R/make_clean_names.R", "old_make_clean_names", 251},
				{"R/make_clean_names.R", "available_transliterators", 284},
				{"R/clean_names.R", "clean_names.default", 76},
				{"R/clean_names.R", "clean_names.sf", 100},
				{"R/remove_empties.R", "remove_empty", 36},
			},
		},
		{
			project: "r-purrr", language: "r",
			search: "map2_", primary: "R/map2.R",
			negativeFile: "R/map.R", negativeQuery: "map2_",
			paths: []string{
				"R/map2.R", "R/map.R", "R/map-mapper.R",
				"tests/testthat/test-map2.R",
			},
			oracles: []extraAuditEvidence{
				{"R/map2.R", "map2 <- function(.x, .y, .f, ..., .progress = FALSE) {"},
				{"R/map2.R", "map2_ <- function("},
				{"R/map2.R", "n <- vec_size_common(.x = .x, .y = .y, .call = .purrr_error_call)"},
				{"R/map2.R", "args <- vec_recycle_common("},
				{"R/map2.R", "names <- vec_names(.x)"},
				{"R/map2.R", "call_with_cleanup(map2_impl, environment(), .type, .progress, n, names, i)"},
				{"R/map2.R", ".f <- as_mapper(.f, ...)"},
				{"R/map-mapper.R", "as_mapper <- function(.f, ...) {"},
				{"R/map.R", "map_ <- function("},
				{"R/map.R", "vec_assert(.x, arg = \".x\", call = .purrr_error_call)"},
				{"tests/testthat/test-map2.R", "test_that(\"recycles inputs\", {"},
				{"tests/testthat/test-map2.R", "expect_equal(map2(1:2, 1, `+`), list(2, 3))"},
				{"tests/testthat/test-map2.R", "map2(1:2, 1:3, `+`)"},
				{"tests/testthat/test-map2.R", "test_that(\"only takes names from x\", {"},
				{"tests/testthat/test-map2.R", "expect_named(map2(x4, 1:2, `+`), c(\"a\", \"a\"))"},
			},
			targets: []extraAuditSymbol{
				{"R/map2.R", "map2", 38},
				{"R/map2.R", "map2_", 62},
				{"R/map2.R", "map2_vec", 114},
				{"R/map.R", "map_", 192},
				{"R/map-mapper.R", "as_mapper", 40},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
