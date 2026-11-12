package handler

import (
	"os"
	"testing"
)

// Provider 64: two additional independent original Rust project origins,
// distinct from the original ripgrep/fd cases. Sources and inline tests were
// fully read before the source navigation oracle was recorded; nothing runs.
func TestSourceSymbolsRustSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original Rust projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "rust64-rust-analyzer-audit-selected", language: "rust",
			search:       "flip_binexpr",
			primary:      "crates/ide-assists/src/handlers/flip_binexpr.rs",
			negativeFile: "crates/ide-assists/src/handlers/flip_comma.rs", negativeQuery: "flip_binexpr",
			paths: []string{
				"crates/ide-assists/src/handlers/flip_binexpr.rs",
				"crates/ide-assists/src/handlers/flip_comma.rs",
			},
			oracles: []extraAuditEvidence{
				{"crates/ide-assists/src/handlers/flip_binexpr.rs", "pub(crate) fn flip_binexpr(acc: &mut Assists"},
				{"crates/ide-assists/src/handlers/flip_binexpr.rs", "let action: FlipAction = expr.op_kind()?.into();"},
				{"crates/ide-assists/src/handlers/flip_binexpr.rs", "FlipAction::DontFlip"},
				{"crates/ide-assists/src/handlers/flip_binexpr.rs", "editor.replace(lhs.syntax(), rhs.syntax());"},
				{"crates/ide-assists/src/handlers/flip_binexpr.rs", "impl From<ast::BinaryOp> for FlipAction {"},
				{"crates/ide-assists/src/handlers/flip_binexpr.rs", "pub(crate) fn flip_range_expr(acc: &mut Assists"},
				{"crates/ide-assists/src/handlers/flip_binexpr.rs", "fn flip_binexpr_not_applicable_for_assignment() {"},
				{"crates/ide-assists/src/handlers/flip_binexpr.rs", "fn flip_binexpr_works_for_lteq() {"},
				{"crates/ide-assists/src/handlers/flip_comma.rs", "pub(crate) fn flip_comma(acc: &mut Assists"},
				{"crates/ide-assists/src/handlers/flip_comma.rs", "if next.kind().is_punct() {"},
				{"crates/ide-assists/src/handlers/flip_comma.rs", "fn flip_comma_not_applicable_for_macro_input() {"},
			},
			targets: []extraAuditSymbol{
				{"crates/ide-assists/src/handlers/flip_binexpr.rs", "flip_binexpr", 24},
				{"crates/ide-assists/src/handlers/flip_binexpr.rs", "FlipAction", 63},
				{"crates/ide-assists/src/handlers/flip_binexpr.rs", "from", 73},
				{"crates/ide-assists/src/handlers/flip_binexpr.rs", "flip_range_expr", 117},
				{"crates/ide-assists/src/handlers/flip_binexpr.rs", "flip_binexpr_not_applicable_for_assignment", 170},
				{"crates/ide-assists/src/handlers/flip_comma.rs", "flip_comma", 24},
				{"crates/ide-assists/src/handlers/flip_comma.rs", "flip_tree", 60},
			},
		},
		{
			project: "rust64-mini-redis-audit-selected", language: "rust",
			search: "parse_frames", primary: "src/cmd/set.rs",
			negativeFile: "src/cmd/get.rs", negativeQuery: "Set",
			paths: []string{"src/cmd/set.rs", "src/cmd/get.rs"},
			oracles: []extraAuditEvidence{
				{"src/cmd/set.rs", "pub struct Set {"},
				{"src/cmd/set.rs", "impl Set {"},
				{"src/cmd/set.rs", "pub(crate) fn parse_frames(parse: &mut Parse) -> crate::Result<Set> {"},
				{"src/cmd/set.rs", "Ok(s) if s.to_uppercase() == \"EX\" => {"},
				{"src/cmd/set.rs", "Ok(s) if s.to_uppercase() == \"PX\" => {"},
				{"src/cmd/set.rs", "db.set(self.key, self.value, self.expire);"},
				{"src/cmd/set.rs", "frame.push_bulk(Bytes::from(\"px\".as_bytes()));"},
				{"src/cmd/get.rs", "pub struct Get {"},
				{"src/cmd/get.rs", "pub(crate) fn parse_frames(parse: &mut Parse) -> crate::Result<Get> {"},
				{"src/cmd/get.rs", "let response = if let Some(value) = db.get(&self.key) {"},
				{"src/cmd/get.rs", "Frame::Null"},
				{"src/cmd/get.rs", "frame.push_bulk(Bytes::from(\"get\".as_bytes()));"},
			},
			targets: []extraAuditSymbol{
				{"src/cmd/set.rs", "Set", 21},
				{"src/cmd/set.rs", "new", 37},
				{"src/cmd/set.rs", "parse_frames", 80},
				{"src/cmd/set.rs", "apply", 128},
				{"src/cmd/set.rs", "into_frame", 144},
				{"src/cmd/get.rs", "Get", 12},
				{"src/cmd/get.rs", "parse_frames", 50},
				{"src/cmd/get.rs", "apply", 64},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
