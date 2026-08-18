package sourceintelligence

import (
	"context"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

var _ SourceAnalyzer = RustAnalyzer{}

func TestRustAnalyzerModulesTraitsImplsFunctionsAndMacroOpacity(t *testing.T) {
	text := `mod inner {
    pub struct Item<T> { pub value: T }
    pub trait Store<T> {
        fn get(&self, id: u64) -> T;
    }
    impl<T> Store<T> for Item<T> {
        fn get(&self, id: u64) -> T { todo!() }
    }
    impl<T> Item<T> {
        pub fn new(value: T) -> Self { Self { value } }
        pub const LIMIT: usize = 1;
        type Output = T;
    }
}
mod external;
use crate::inner::{Item, Store};
pub enum State { Ready, Done }
pub fn top<'a, T: Clone>(value: &'a T) -> T { value.clone() }
macro_rules! fake {
    () => { struct MacroFake; fn nope() {} }
}
fake!();
const RAW: &str = r###"struct RawFake; fn hidden() {}"###;
`
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.rs"
	result, err := (RustAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("Rust analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		"inner": SymbolKindModule, "inner.Item": SymbolKindStruct, "inner.Item.value": SymbolKindField,
		"inner.Store": SymbolKindTrait, "inner.Store.get": SymbolKindMethod,
		"external": SymbolKindModule, "State": SymbolKindEnum, "top": SymbolKindFunction, "RAW": SymbolKindConstant,
	} {
		if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	implCount := 0
	methodNames := map[string]int{}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Kind == SymbolKindImplementation {
			implCount++
		}
		if symbol.Kind == SymbolKindMethod {
			methodNames[symbol.Name]++
		}
		switch symbol.Name {
		case "MacroFake", "nope", "RawFake", "hidden":
			t.Fatalf("Rust macro/raw-string false positive: %+v", symbol)
		}
	}
	if implCount != 2 || methodNames["get"] != 2 || methodNames["new"] != 1 {
		t.Fatalf("Rust impl/method coverage = impls=%d methods=%v symbols=%+v", implCount, methodNames, result.Analysis.Symbols)
	}
	if got := dependencyValues(result.Dependencies); !containsString(got, "external") || !containsString(got, "crate::inner::{Item,Store}") {
		t.Fatalf("Rust dependencies = %v", got)
	}
	if !hasStructuralRelation(result.Relations, "implements", "Item<T>", "Store<T>") {
		t.Fatalf("Rust trait implementation relation = %+v", result.Relations)
	}
	top := byName["top"]
	compactSignature := strings.ReplaceAll(top.Signature, " ", "")
	if !strings.Contains(compactSignature, "<'a,T:Clone>") || !strings.Contains(compactSignature, "&'aT") {
		t.Fatalf("Rust generic/lifetime signature = %q", top.Signature)
	}
}

func TestRustAnalyzerAttributesNestedCommentsMalformedLimitsAndCancellation(t *testing.T) {
	text := `#[derive(Debug)]
pub struct Good {
    pub value: i32,
}
/* outer
   /* struct NestedFake; */
*/
#[cfg(feature = "x")]
pub fn work() {}
`
	result, err := (RustAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("valid Rust attributes/comments reported partial: %+v", result.Analysis.Diagnostics)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	if _, ok := byName["Good"]; !ok {
		t.Fatalf("Rust attribute handling lost Good: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if _, ok := byName["work"]; !ok {
		t.Fatalf("Rust attribute handling lost work: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if _, ok := byName["NestedFake"]; ok {
		t.Fatalf("nested comment leaked NestedFake: %+v", byName["NestedFake"])
	}

	malformed := sourceDocumentForScanner("struct Good {}\nconst RAW: &str = r#\"unterminated\n")
	partial, err := (RustAnalyzer{}).Analyze(context.Background(), malformed, testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if partial.Analysis.CoverageComplete || len(partial.Analysis.Diagnostics) == 0 {
		t.Fatalf("malformed Rust did not report partial coverage: %+v", partial.Analysis)
	}

	limited, err := (RustAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("struct A {}\nstruct B {}\nstruct C {}\n"), testAnalyzeOptions(false, 2))
	if err != nil {
		t.Fatal(err)
	}
	if !limited.Analysis.Truncated || limited.Analysis.CoverageComplete || len(limited.Analysis.Symbols) != 2 {
		t.Fatalf("Rust bounded result = %+v", limited.Analysis)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (RustAnalyzer{}).Analyze(ctx, sourceDocumentForScanner("struct A {}"), testAnalyzeOptions(false, 16))
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("Rust cancellation error=%v kind=%v", err, operation.KindOf(err))
	}
}
