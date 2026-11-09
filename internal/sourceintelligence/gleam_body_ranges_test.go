package sourceintelligence

import (
	"context"
	"testing"
)

func TestGleamFunctionRangesPreserveEntireBody(t *testing.T) {
	source := `pub fn parse_method(method: String) -> Bool {
  let literal = "a { brace } inside a string"
  case method {
    "" -> False
    _ -> True
  }
}

pub fn long_signature(
  initial: Int,
  extra: Int,
) -> Int {
  initial + extra
}

pub fn after() -> Int { 42 }
`
	doc := sourceDocumentForScanner(source)
	doc.Path = "gleam-function-ranges.gleam"
	result, err := (GleamAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("valid Gleam source marked incomplete: %+v", result.Analysis)
	}
	found := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, want := range []struct {
		name                                string
		start, end, signatureEnd, bodyStart int
	}{
		{"parse_method", 1, 7, 1, 1},
		{"long_signature", 9, 14, 12, 12},
		{"after", 16, 16, 16, 16},
	} {
		symbol, ok := found[want.name]
		if !ok || symbol.Kind != SymbolKindFunction || symbol.DeclarationRange.Start.Line != want.start ||
			symbol.DeclarationRange.End.Line != want.end || symbol.SignatureRange == nil ||
			symbol.SignatureRange.End.Line != want.signatureEnd || symbol.BodyRange == nil ||
			symbol.BodyRange.Start.Line != want.bodyStart {
			t.Errorf("%s incomplete declaration, signature, or body: exists=%v symbol=%+v", want.name, ok, symbol)
		}
	}
}

func TestGleamExternalDeclarationDoesNotBorrowNextBody(t *testing.T) {
	source := `@external(erlang, "logger", "warning")
fn log_warning(a: String) -> Nil

pub fn after() -> Int {
  42
}
`
	doc := sourceDocumentForScanner(source)
	doc.Path = "gleam-external.gleam"
	result, err := (GleamAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("valid external declaration marked incomplete: %+v", result.Analysis)
	}
	found := symbolsByQualifiedName(result.Analysis.Symbols)
	external, ok := found["log_warning"]
	if !ok || external.BodyRange != nil || external.DeclarationRange.End.Line != 2 {
		t.Errorf("external declaration incorrectly owns source: exists=%v symbol=%+v", ok, external)
	}
	next, ok := found["after"]
	if !ok || next.BodyRange == nil || next.DeclarationRange.End.Line != 6 {
		t.Errorf("function after external lost its body: exists=%v symbol=%+v", ok, next)
	}
}
