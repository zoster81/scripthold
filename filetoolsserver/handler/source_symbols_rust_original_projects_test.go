package handler

import (
	"os"
	"testing"
)

// Source-first maintenance scenarios from two independent original Rust
// projects. Their code and test suites are inspected, never executed.
func TestSourceSymbolsOriginalRustInitialQualification(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Rust corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "project-rust-burntsushi-ripgrep", language: "rust",
			search: "case_insensitive", primary: "crates/ignore/src/overrides.rs",
			negativeFile: "crates/ignore/src/types.rs", negativeQuery: "case_insensitive",
			paths: []string{
				"crates/ignore/src/overrides.rs",
				"crates/ignore/src/gitignore.rs",
				"crates/ignore/src/types.rs",
			},
			oracles: []extraAuditEvidence{
				{"crates/ignore/src/overrides.rs", "pub struct OverrideBuilder {"},
				{"crates/ignore/src/overrides.rs", "pub fn add(&mut self, glob: &str) -> Result<&mut OverrideBuilder, Error> {"},
				{"crates/ignore/src/overrides.rs", "self.builder.add_line(None, glob)?;"},
				{"crates/ignore/src/overrides.rs", "pub fn case_insensitive("},
				{"crates/ignore/src/overrides.rs", "self.builder.case_insensitive(yes)?;"},
				{"crates/ignore/src/overrides.rs", "fn simple() {"},
				{"crates/ignore/src/overrides.rs", "fn precedence() {"},
				{"crates/ignore/src/gitignore.rs", "pub fn add_line("},
				{"crates/ignore/src/gitignore.rs", "self.case_insensitive = yes;"},
				{"crates/ignore/src/types.rs", "pub struct TypesBuilder"},
			},
			targets: []extraAuditSymbol{
				{"crates/ignore/src/overrides.rs", "OverrideBuilder", 115},
				{"crates/ignore/src/overrides.rs", "add", 142},
				{"crates/ignore/src/overrides.rs", "case_insensitive", 153},
				{"crates/ignore/src/overrides.rs", "simple", 209},
				{"crates/ignore/src/overrides.rs", "precedence", 229},
				{"crates/ignore/src/gitignore.rs", "add_line", 460},
				{"crates/ignore/src/gitignore.rs", "case_insensitive", 549},
			},
		},
		{
			project: "project-rust-sharkdp-fd", language: "rust",
			search: "parse_opt", primary: "src/filter/size.rs",
			negativeFile: "src/filter/time.rs", negativeQuery: "parse_opt",
			paths: []string{
				"src/filter/size.rs",
				"src/filter/time.rs",
				"src/filter/mod.rs",
			},
			oracles: []extraAuditEvidence{
				{"src/filter/size.rs", "pub enum SizeFilter {"},
				{"src/filter/size.rs", "pub fn from_string(s: &str) -> anyhow::Result<Self> {"},
				{"src/filter/size.rs", "fn parse_opt(s: &str) -> Option<Self> {"},
				{"src/filter/size.rs", "SIZE_CAPTURES.get_or_init"},
				{"src/filter/size.rs", "\"+\" => Some(SizeFilter::Min(size)),"},
				{"src/filter/size.rs", "pub fn is_within(&self, size: u64) -> bool {"},
				{"src/filter/size.rs", "kibi_plus:                (\"+1ki\",    SizeFilter::Min(1024)),"},
				{"src/filter/time.rs", "pub enum TimeFilter {"},
				{"src/filter/time.rs", "pub fn before(s: &str) -> Option<TimeFilter> {"},
				{"src/filter/mod.rs", "pub use self::size::SizeFilter;"},
				{"src/cli.rs", "value_parser = SizeFilter::from_string"},
			},
			targets: []extraAuditSymbol{
				{"src/filter/size.rs", "SizeFilter", 9},
				{"src/filter/size.rs", "from_string", 28},
				{"src/filter/size.rs", "parse_opt", 33},
				{"src/filter/size.rs", "is_within", 68},
				{"src/filter/time.rs", "TimeFilter", 7},
				{"src/filter/time.rs", "before", 49},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
