package sourceintelligence

import (
	"context"
	"reflect"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

var _ SourceAnalyzer = SwiftAnalyzer{}

func TestSwiftAnalyzerImportsTypesExtensionsMembersAndConformance(t *testing.T) {
	text := `import Foundation
import struct CoreGraphics.CGPoint

protocol Store: AnyObject {
    associatedtype Value
    var count: Int { get }
    func get(id: Int) -> Value
}

struct Item: Codable {
    let id: Int
    var name: String
    func describe() -> String { name }
}

final class Service: BaseService, Store {
    typealias Value = String
    @Published private var title: String = ""
    let count: Int
    init(count: Int) { self.count = count }
    func get(id: Int) -> String { title }
}

extension Service: CustomStringConvertible {
    var description: String { title }
    func helper() {}
}

enum State: String { case ready, done }
func identity<T>(_ value: T) -> T { value }
let fake = "class StringFake { func nope() {} }"
`
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.swift"
	result, err := (SwiftAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("Swift analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		"Store":               SymbolKindInterface,
		"Store.Value":         SymbolKindAlias,
		"Store.count":         SymbolKindProperty,
		"Store.get":           SymbolKindMethod,
		"Item":                SymbolKindStruct,
		"Item.id":             SymbolKindProperty,
		"Item.name":           SymbolKindProperty,
		"Item.describe":       SymbolKindMethod,
		"Service":             SymbolKindClass,
		"Service.Value":       SymbolKindAlias,
		"Service.title":       SymbolKindProperty,
		"Service.count":       SymbolKindProperty,
		"Service.Service":     SymbolKindConstructor,
		"Service.get":         SymbolKindMethod,
		"Service.helper":      SymbolKindMethod,
		"Service.description": SymbolKindProperty,
		"State":               SymbolKindEnum,
		"identity":            SymbolKindFunction,
	} {
		if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Name == "StringFake" || symbol.Name == "nope" || symbol.Name == "Published" {
			t.Fatalf("Swift attribute/string false positive: %+v", symbol)
		}
	}
	if got := dependencyValues(result.Dependencies); !reflect.DeepEqual(got, []string{"Foundation", "CoreGraphics.CGPoint"}) {
		t.Fatalf("Swift imports = %v", got)
	}
	if !hasStructuralRelation(result.Relations, "inherits", "Service", "BaseService") ||
		!hasStructuralRelation(result.Relations, "conforms", "Service", "Store") ||
		!hasStructuralRelation(result.Relations, "conforms", "Item", "Codable") ||
		!hasStructuralRelation(result.Relations, "conforms", "Service", "CustomStringConvertible") {
		t.Fatalf("Swift relations = %+v", result.Relations)
	}
}

func TestSwiftAnalyzerMalformedLimitsAndCancellation(t *testing.T) {
	partial, err := (SwiftAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("struct Good {}\nclass Broken {\n"), testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if partial.Analysis.CoverageComplete || len(partial.Analysis.Diagnostics) == 0 {
		t.Fatalf("malformed Swift did not report partial coverage: %+v", partial.Analysis)
	}
	if _, ok := symbolsByQualifiedName(partial.Analysis.Symbols)["Good"]; !ok {
		t.Fatalf("malformed Swift lost Good: %v", sortedSymbolQualifiedNames(partial.Analysis.Symbols))
	}

	limited, err := (SwiftAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("struct A {}\nstruct B {}\nstruct C {}\n"), testAnalyzeOptions(false, 2))
	if err != nil {
		t.Fatal(err)
	}
	if !limited.Analysis.Truncated || limited.Analysis.CoverageComplete || len(limited.Analysis.Symbols) != 2 {
		t.Fatalf("Swift bounded result = %+v", limited.Analysis)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (SwiftAnalyzer{}).Analyze(ctx, sourceDocumentForScanner("struct A {}"), testAnalyzeOptions(false, 16))
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("Swift cancellation error=%v kind=%v", err, operation.KindOf(err))
	}
}
