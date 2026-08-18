package sourceintelligence

import (
	"context"
	"reflect"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

var _ SourceAnalyzer = CAnalyzer{}
var _ SourceAnalyzer = CPPAnalyzer{}

func TestCAnalyzerDeclarationsDefinitionsIncludesAndFalsePositiveResistance(t *testing.T) {
	text := `#include <stdio.h>
#include "local.h"
#define DECLARE_FAKE(name) void name()

struct Point { int x; int y; };
union Value { int i; double d; };
enum State { Ready, Done };
static int helper(int value);
static int helper(int value) { return value; }
int add(int left, int right) { return left + right; }
const char *text = "struct Fake { int Nope(); };";
// int Commented(void);
`
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.c"
	result, err := (CAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("C analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		"Point": SymbolKindStruct, "Point.x": SymbolKindField, "Point.y": SymbolKindField,
		"Value": SymbolKindType, "Value.i": SymbolKindField, "Value.d": SymbolKindField,
		"State": SymbolKindEnum, "add": SymbolKindFunction,
	} {
		if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	var helperKinds, helperIDs []string
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Name == "helper" {
			helperKinds = append(helperKinds, symbol.NativeKind)
			helperIDs = append(helperIDs, symbol.ID)
		}
		if symbol.Name == "Fake" || symbol.Name == "Nope" || symbol.Name == "Commented" || symbol.Name == "DECLARE_FAKE" {
			t.Fatalf("C false positive: %+v", symbol)
		}
	}
	if !reflect.DeepEqual(helperKinds, []string{"function-declaration", "function-definition"}) || len(helperIDs) != 2 || helperIDs[0] == helperIDs[1] {
		t.Fatalf("C declaration/definition identity = kinds=%v ids=%v", helperKinds, helperIDs)
	}
	if got := dependencyValues(result.Dependencies); !reflect.DeepEqual(got, []string{"stdio.h", "local.h"}) {
		t.Fatalf("C includes = %v", got)
	}
}

func TestCPPAnalyzerNamespacesTemplatesOverloadsAndRawStrings(t *testing.T) {
	text := `#include <vector>
#include "box.hpp"
#define CLASS_FAKE class MacroFake {}
namespace Demo {
template <typename T>
class Box : public Base, private Interface<T> {
public:
    Box(T value);
    ~Box();
    T get() const;
    T& operator[](int index);
    T value;
};
struct Pair { int left; int right; };
enum class State { Ready, Done };
using IntBox = Box<int>;
int work(int value);
double work(double value);
const char* raw = R"tag(class RawFake { void Nope(); })tag";
}
`
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.cpp"
	result, err := (CPPAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("C++ analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		"Demo": SymbolKindNamespace, "Demo.Box": SymbolKindClass,
		"Demo.Box.get": SymbolKindMethod, "Demo.Box.operator[]": SymbolKindOperator, "Demo.Box.value": SymbolKindField,
		"Demo.Pair": SymbolKindStruct, "Demo.State": SymbolKindEnum, "Demo.IntBox": SymbolKindAlias,
	} {
		if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	constructors := 0
	destructors := 0
	var constructorIDs, destructorIDs, overloadIDs []string
	for _, symbol := range result.Analysis.Symbols {
		if symbol.QualifiedName == "Demo.Box.Box" && symbol.Kind == SymbolKindConstructor {
			constructors++
			constructorIDs = append(constructorIDs, symbol.ID)
		}
		if symbol.QualifiedName == "Demo.Box.Box" && symbol.Kind == SymbolKindDestructor {
			destructors++
			destructorIDs = append(destructorIDs, symbol.ID)
		}
		if symbol.QualifiedName == "Demo.work" {
			overloadIDs = append(overloadIDs, symbol.ID)
		}
		if symbol.Name == "MacroFake" || symbol.Name == "RawFake" || symbol.Name == "Nope" {
			t.Fatalf("C++ false positive: %+v", symbol)
		}
	}
	if constructors != 1 || destructors != 1 || constructorIDs[0] == destructorIDs[0] || len(overloadIDs) != 2 || overloadIDs[0] == overloadIDs[1] {
		t.Fatalf("C++ ctor/dtor/overload identity missing: constructors=%d destructors=%d constructorIDs=%v destructorIDs=%v overloadIDs=%v", constructors, destructors, constructorIDs, destructorIDs, overloadIDs)
	}
	if got := dependencyValues(result.Dependencies); !reflect.DeepEqual(got, []string{"vector", "box.hpp"}) {
		t.Fatalf("C++ includes = %v", got)
	}
	if !hasStructuralRelation(result.Relations, "inherits", "Demo.Box", "Base") || !hasStructuralRelation(result.Relations, "inherits", "Demo.Box", "Interface<T>") {
		t.Fatalf("C++ inheritance relations = %+v", result.Relations)
	}
}

func TestCFamilyAnalyzerMalformedLimitsAndCancellation(t *testing.T) {
	malformed := sourceDocumentForScanner("struct Good { int x; };\nint broken( {\n")
	malformed.Path = "broken.c"
	result, err := (CAnalyzer{}).Analyze(context.Background(), malformed, testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
		t.Fatalf("malformed C did not report partial coverage: %+v", result.Analysis)
	}
	if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)["Good"]; !ok {
		t.Fatalf("malformed recovery lost Good: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}

	limitedResult, err := (CPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("struct A {}; struct B {}; struct C {};\n"), testAnalyzeOptions(false, 2))
	if err != nil {
		t.Fatal(err)
	}
	if !limitedResult.Analysis.Truncated || limitedResult.Analysis.CoverageComplete || len(limitedResult.Analysis.Symbols) != 2 {
		t.Fatalf("C++ bounded result = %+v", limitedResult.Analysis)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (CPPAnalyzer{}).Analyze(ctx, sourceDocumentForScanner("class A {};"), testAnalyzeOptions(false, 16))
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("C++ cancellation error=%v kind=%v", err, operation.KindOf(err))
	}
}

func testAnalyzeOptions(signatures bool, maxSymbols int) AnalyzeOptions {
	return AnalyzeOptions{IncludeSignatures: signatures, MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: maxSymbols, MaxSignatureBytes: 8192, MaxDiagnostics: 64}}
}

func dependencyValues(values []StructuralDependency) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.Value
	}
	return result
}

func hasStructuralRelation(values []StructuralRelation, kind, source, target string) bool {
	for _, value := range values {
		if value.Kind == kind && value.Source == source && value.Target == target && value.Evidence == SymbolEvidenceStructural {
			return true
		}
	}
	return false
}

func TestCPPAnalyzerOutOfClassDefinitionsPreserveOwnership(t *testing.T) {
	text := `namespace Demo {
template <class T>
class Box {
public:
    Box(T value);
    ~Box();
    T get() const;
};
template <class T>
Box<T>::Box(T value) {}
template <class T>
Box<T>::~Box() {}
template <class T>
T Box<T>::get() const { return T{}; }
}
`
	document := sourceDocumentForScanner(text)
	document.Path = "qualified.cpp"
	result, err := (CPPAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("qualified C++ analysis partial: %+v", result.Analysis.Diagnostics)
	}
	counts := map[string]int{}
	ids := map[string]map[string]struct{}{}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.QualifiedName != "Demo.Box.Box" && symbol.QualifiedName != "Demo.Box.get" {
			continue
		}
		key := string(symbol.Kind) + ":" + symbol.NativeKind
		counts[key]++
		if ids[symbol.QualifiedName] == nil {
			ids[symbol.QualifiedName] = map[string]struct{}{}
		}
		ids[symbol.QualifiedName][symbol.ID] = struct{}{}
	}
	for _, key := range []string{
		"constructor:constructor-declaration", "constructor:constructor-definition",
		"destructor:destructor-declaration", "destructor:destructor-definition",
		"method:method-declaration", "method:method-definition",
	} {
		if counts[key] != 1 {
			t.Fatalf("qualified C++ %s count=%d; symbols=%+v", key, counts[key], result.Analysis.Symbols)
		}
	}
	if len(ids["Demo.Box.Box"]) != 4 || len(ids["Demo.Box.get"]) != 2 {
		t.Fatalf("qualified C++ IDs are not distinct: %+v", ids)
	}
}

func TestCAnalyzerFunctionPointersAreVariablesNotFunctions(t *testing.T) {
	text := `int (*callback)(int);
struct Hooks {
    void (*on_event)(int);
};
`
	result, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	if symbol, ok := byName["callback"]; !ok || symbol.Kind != SymbolKindVariable {
		t.Fatalf("callback = %+v exists=%v; symbols=%+v", symbol, ok, result.Analysis.Symbols)
	}
	if symbol, ok := byName["Hooks.on_event"]; !ok || symbol.Kind != SymbolKindField {
		t.Fatalf("Hooks.on_event = %+v exists=%v; symbols=%+v", symbol, ok, result.Analysis.Symbols)
	}
	for _, symbol := range result.Analysis.Symbols {
		if (symbol.Name == "callback" || symbol.Name == "on_event") && (symbol.Kind == SymbolKindFunction || symbol.Kind == SymbolKindMethod) {
			t.Fatalf("function pointer overclaimed as callable declaration: %+v", symbol)
		}
	}
}
