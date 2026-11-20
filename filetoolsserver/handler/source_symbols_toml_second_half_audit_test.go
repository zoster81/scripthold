package handler

import (
	"os"
	"testing"
)

// Provider 60: independent Git-original TOML configuration projects, zoxide
// and ripgrep, distinct from the first-stage gookit/config and configr cases.
// Rust source files are independently inspected as cross-file oracles only;
// neither Cargo nor any original project code is executed.
func TestSourceSymbolsTOMLSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("independent original TOML projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "toml60-zoxide-audit-selected", language: "toml",
			search: "nix-dev", primary: "Cargo.toml",
			negativeFile: "Cross.toml", negativeQuery: "nix-dev",
			paths: []string{
				"Cargo.toml",
				".cargo/config.toml",
				"Cross.toml",
			},
			oracles: []extraAuditEvidence{
				{"Cargo.toml", "[package]"},
				{"Cargo.toml", "name = \"zoxide\""},
				{"Cargo.toml", "[features]"},
				{"Cargo.toml", "default = []"},
				{"Cargo.toml", "nix-dev = []"},
				{".cargo/config.toml", "[alias]"},
				{".cargo/config.toml", "rustflags = [\"-C\", \"target-feature=+crt-static\"]"},
				{"Cross.toml", "[build.env]"},
				{"src/shell.rs", "#[cfg(feature = \"nix-dev\")]"},
				{"tests/completions.rs", "#![cfg(feature = \"nix-dev\")]"},
			},
			targets: []extraAuditSymbol{
				{"Cargo.toml", "package", 1},
				{"Cargo.toml", "features", 51},
				{"Cargo.toml", "default", 52},
				{"Cargo.toml", "nix-dev", 53},
				{".cargo/config.toml", "alias", 1},
			},
		},
		{
			project: "toml60-ripgrep-audit-selected", language: "toml",
			search: "pcre2", primary: "Cargo.toml",
			negativeFile: "crates/printer/Cargo.toml", negativeQuery: "pcre2",
			paths: []string{
				"Cargo.toml",
				"crates/grep/Cargo.toml",
				"crates/pcre2/Cargo.toml",
				"crates/printer/Cargo.toml",
			},
			oracles: []extraAuditEvidence{
				{"Cargo.toml", "name = \"ripgrep\""},
				{"Cargo.toml", "[workspace]"},
				{"Cargo.toml", "\"crates/pcre2\""},
				{"Cargo.toml", "[features]"},
				{"Cargo.toml", "pcre2 = [\"grep/pcre2\"]"},
				{"crates/grep/Cargo.toml", "name = \"grep\""},
				{"crates/grep/Cargo.toml", "grep-pcre2 = { version = \"0.1.9\", path = \"../pcre2\", optional = true }"},
				{"crates/grep/Cargo.toml", "pcre2 = [\"grep-pcre2\"]"},
				{"crates/pcre2/Cargo.toml", "name = \"grep-pcre2\""},
				{"crates/pcre2/Cargo.toml", "pcre2 = \"0.2.6\""},
				{"crates/printer/Cargo.toml", "name = \"grep-printer\""},
				{"crates/printer/Cargo.toml", "default = [\"serde\"]"},
			},
			targets: []extraAuditSymbol{
				{"Cargo.toml", "features", 77},
				{"Cargo.toml", "pcre2", 78},
				{"crates/grep/Cargo.toml", "pcre2", 30},
				{"crates/pcre2/Cargo.toml", "pcre2", 20},
				{"crates/printer/Cargo.toml", "default", 19},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
