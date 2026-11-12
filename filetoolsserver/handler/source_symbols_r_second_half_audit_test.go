package handler

import (
	"os"
	"testing"
)

// Provider 62: these are separate original R project origins from the first
// qualification's janitor/purrr cases. Sources are pinned to public upstream
// commits (cli ceadbfd81961437524e0eeedd9d388242c25399c,
// dplyr 819c99f7284b3a413b391c60bea49ae7bfb8bafc).
// Their source and tests are inspected, never executed.
func TestSourceSymbolsRSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original R projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "r62-cli-audit-selected", language: "r",
			search: "combine_ansi_styles", primary: "R/ansi.R",
			negativeFile: "tests/testthat/test-ansi.R", negativeQuery: "create_ansi_style_fun",
			paths: []string{"R/ansi.R", "tests/testthat/test-ansi.R"},
			oracles: []extraAuditEvidence{
				{"R/ansi.R", "create_ansi_style_tag <- function(name, open, close, palette = NULL) {"},
				{"R/ansi.R", "create_ansi_style_fun <- function(styles) {"},
				{"R/ansi.R", "for (st in rev(mystyles)) {"},
				{"R/ansi.R", "gsub(st$close, st$open, txt, fixed = TRUE)"},
				{"R/ansi.R", "create_ansi_style <- function(name, open = NULL, close = NULL) {"},
				{"R/ansi.R", "ansi_style_from_rgb <- function(rgb, bg, num_colors, grey) {"},
				{"R/ansi.R", "return(ansi256(rgb, bg, grey))"},
				{"R/ansi.R", "ansi256 <- function(rgb, bg = FALSE, grey = FALSE) {"},
				{"R/ansi.R", "combine_ansi_styles <- function(...) {"},
				{"R/ansi.R", "create_ansi_style_fun(styles)"},
				{"tests/testthat/test-ansi.R", "test_that(\"Applying multiple styles at once works\", {"},
				{"tests/testthat/test-ansi.R", "st <- combine_ansi_styles(col_red, bg_green, \"underline\")"},
				{"tests/testthat/test-ansi.R", "test_that(\"Nested styles are supported\", {"},
				{"tests/testthat/test-ansi.R", "test_that(\"Nested styles of the same type are supported\", {"},
				{"tests/testthat/test-ansi.R", "test_that(\"zero length vectors\", {"},
			},
			targets: []extraAuditSymbol{
				{"R/ansi.R", "create_ansi_style_fun", 129},
				{"R/ansi.R", "create_ansi_style", 158},
				{"R/ansi.R", "make_ansi_style", 229},
				{"R/ansi.R", "ansi_style_from_rgb", 326},
				{"R/ansi.R", "ansi256", 358},
				{"R/ansi.R", "combine_ansi_styles", 446},
			},
		},
		{
			project: "r62-dplyr-audit-selected", language: "r",
			search: "rename_with.data.frame", primary: "R/rename.R",
			negativeFile: "tests/testthat/test-rename.R", negativeQuery: "rename_with.data.frame",
			paths: []string{"R/rename.R", "tests/testthat/test-rename.R"},
			oracles: []extraAuditEvidence{
				{"R/rename.R", "rename <- function(.data, ...) {"},
				{"R/rename.R", "UseMethod(\"rename\")"},
				{"R/rename.R", "rename.data.frame <- function(.data, ...) {"},
				{"R/rename.R", "loc <- tidyselect::eval_rename(expr(c(...)), .data)"},
				{"R/rename.R", "names[loc] <- names(loc)"},
				{"R/rename.R", "rename_with <- function(.data, .fn, .cols = everything(), ...) {"},
				{"R/rename.R", "UseMethod(\"rename_with\")"},
				{"R/rename.R", "rename_with.data.frame <- function(.data, .fn, .cols = everything(), ...) {"},
				{"R/rename.R", "cols <- tidyselect::eval_select(enquo(.cols), .data, allow_rename = FALSE)"},
				{"R/rename.R", "if (!is_character(new)) {"},
				{"R/rename.R", "if (length(new) != length(sel)) {"},
				{"R/rename.R", "names <- vec_as_names(names, repair = \"check_unique\")"},
				{"tests/testthat/test-rename.R", "test_that(\"rename preserves grouping\", {"},
				{"tests/testthat/test-rename.R", "expect_equal(group_vars(out), \"h\")"},
				{"tests/testthat/test-rename.R", "test_that(\"can select columns\", {"},
				{"tests/testthat/test-rename.R", "test_that(\"can't create duplicated names\", {"},
			},
			targets: []extraAuditSymbol{
				{"R/rename.R", "rename", 63},
				{"R/rename.R", "rename.data.frame", 68},
				{"R/rename.R", "rename_with", 83},
				{"R/rename.R", "rename_with.data.frame", 88},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
