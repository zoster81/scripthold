package sourceintelligence

import (
	"context"
	"os"
	"reflect"
	"strings"
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

func TestSwiftPackageScopedTypesAndMethods(t *testing.T) {
	const original = `class Application {}
extension Application {
    package struct Live {
        package func performTest() {}
    }
    package struct InMemory {
        package init() {}
        package func run() {}
    }
}
let decoy = "package struct Fake {}"`
	for _, tc := range []struct{ name, source string }{
		{"LF", original},
		{"CRLF", strings.ReplaceAll(original, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (SwiftAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 100))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("package declarations incomplete: %+v", result.Analysis)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range map[string]SymbolKind{
				"Application":                   SymbolKindClass,
				"Application.Live":              SymbolKindStruct,
				"Application.Live.performTest":  SymbolKindMethod,
				"Application.InMemory":          SymbolKindStruct,
				"Application.InMemory.InMemory": SymbolKindConstructor,
				"Application.InMemory.run":      SymbolKindMethod,
			} {
				if symbol, ok := got[name]; !ok || symbol.Kind != kind {
					t.Fatalf("%s: got %+v, exists=%v; symbols=%v", name, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			if _, falsePositive := got["Fake"]; falsePositive {
				t.Fatalf("Swift string produced false type: %+v", got["Fake"])
			}
		})
	}
}

func TestSwiftOriginalVaporPackageNestedTypes(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_SWIFT_VAPOR_TESTING_APPLICATION")
	if path == "" {
		t.Skip("set SCRIPTHOLD_REAL_SWIFT_VAPOR_TESTING_APPLICATION to original Vapor TestingApplication.swift")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 1<<20 {
		t.Fatal("original Vapor fixture exceeds 1 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	document := sourceDocumentForScanner(string(data))
	document.Path = path
	result, err := (SwiftAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("original Vapor package analysis incomplete: %+v", result.Analysis)
	}
	want := map[string]struct {
		kind SymbolKind
		line int
	}{
		"Application.Live":                 {SymbolKindStruct, 22},
		"Application.InMemory":             {SymbolKindStruct, 75},
		"Application.Live.performTest":     {SymbolKindMethod, 33},
		"Application.InMemory.performTest": {SymbolKindMethod, 82},
	}
	for _, symbol := range result.Analysis.Symbols {
		if expected, ok := want[symbol.QualifiedName]; ok {
			if symbol.Kind != expected.kind || symbol.NameRange.Start.Line != expected.line {
				t.Fatalf("Vapor %s: got %+v, expected %+v", symbol.QualifiedName, symbol, expected)
			}
			delete(want, symbol.QualifiedName)
		}
	}
	if len(want) != 0 {
		t.Fatalf("Vapor package declarations missing %v; symbols=%v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
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
