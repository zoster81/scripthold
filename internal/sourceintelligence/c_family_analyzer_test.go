package sourceintelligence

import (
	"context"
	"fmt"
	"reflect"
	"strings"
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

func TestCPPAnalyzerGenericBaseListPreservesTemplateCommas(t *testing.T) {
	text := `template <typename T>
class Child : public Generic<T, Pair<T, T>>, private Tail<T> {};
`
	result, err := (CPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("C++ generic base analysis unexpectedly partial: %+v", result.Analysis)
	}
	if len(result.Relations) != 2 ||
		!hasStructuralRelation(result.Relations, "inherits", "Child", "Generic<T,Pair<T,T>>") ||
		!hasStructuralRelation(result.Relations, "inherits", "Child", "Tail<T>") {
		t.Fatalf("C++ generic inheritance relations = %+v", result.Relations)
	}
}

func TestCFamilyConditionalPreprocessorCoverageSemantics(t *testing.T) {
	t.Run("balanced header guard", func(t *testing.T) {
		text := `#ifndef PROJECT_WIDGET_H
#define PROJECT_WIDGET_H
#include "dependency.hpp"
struct Guarded { int value; };
#endif
`
		result, err := (CPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || !hasAnalysisDiagnostic(result.Analysis.Diagnostics, "cpp-conditional-preprocessor") {
			t.Fatalf("balanced header guard should preserve structural coverage with a macro-state diagnostic: %+v", result.Analysis)
		}
		if hasAnalysisDiagnostic(result.Analysis.Diagnostics, "cpp-malformed-conditional-preprocessor") {
			t.Fatalf("balanced header guard reported malformed preprocessing: %+v", result.Analysis.Diagnostics)
		}
		if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)["Guarded"]; !ok {
			t.Fatalf("guarded declaration missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
		if got := dependencyValues(result.Dependencies); !reflect.DeepEqual(got, []string{"dependency.hpp"}) {
			t.Fatalf("header-guard dependency lost: %v", got)
		}
	})

	t.Run("balanced feature branches expose structural union", func(t *testing.T) {
		text := `#ifdef FEATURE
#include "feature.hpp"
struct Enabled { int value; };
#else
#include "fallback.hpp"
struct Disabled { int value; };
#endif
#include "common.hpp"
struct Always { int value; };
`
		result, err := (CPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || !hasAnalysisDiagnostic(result.Analysis.Diagnostics, "cpp-conditional-preprocessor") {
			t.Fatalf("balanced feature conditional should preserve structural source coverage: %+v", result.Analysis)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		for _, want := range []string{"Enabled", "Disabled", "Always"} {
			if _, ok := byName[want]; !ok {
				t.Fatalf("conditional structural union missing %q: %v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
		if got := dependencyValues(result.Dependencies); !reflect.DeepEqual(got, []string{"feature.hpp", "fallback.hpp", "common.hpp"}) {
			t.Fatalf("conditional dependencies=%v", got)
		}
	})

	t.Run("balanced branches can share a structural closer", func(t *testing.T) {
		text := "#ifdef FEATURE\n" +
			"int conditional(int value) {\n" +
			"#else\n" +
			"int conditional(long value) {\n" +
			"#endif\n" +
			"  return (int)value;\n" +
			"}\n" +
			"int after(void) { return 1; }\n"
		result, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || !hasAnalysisDiagnostic(result.Analysis.Diagnostics, "c-conditional-preprocessor") {
			t.Fatalf("balanced C branches sharing a closer reported partial: %+v", result.Analysis)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		for _, want := range []string{"conditional", "after"} {
			if _, ok := byName[want]; !ok {
				t.Fatalf("balanced C conditional branch missing %s: %v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
	})

	for _, tc := range []struct {
		name string
		text string
	}{
		{name: "unterminated", text: "#if FEATURE\nstruct Broken { int value; };\n"},
		{name: "stray endif", text: "#endif\nstruct After { int value; };\n"},
		{name: "duplicate else", text: "#if FEATURE\n#else\n#else\n#endif\n"},
		{name: "elif after else", text: "#if FEATURE\n#else\n#elif OTHER\n#endif\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (CPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || !hasAnalysisDiagnostic(result.Analysis.Diagnostics, "cpp-malformed-conditional-preprocessor") {
				t.Fatalf("malformed conditional preprocessing did not fail closed: %+v", result.Analysis)
			}
		})
	}
}

func TestCAnalyzerRepeatedSimpleMacroStateKeepsConditionalScopeBalanced(t *testing.T) {
	text := "#ifdef __cplusplus\n" +
		"extern \"C\" {\n" +
		"#else\n" +
		"#define inline __inline\n" +
		"#endif\n" +
		"extern void worker(void);\n" +
		"#ifdef __cplusplus\n" +
		"}\n" +
		"#endif\n" +
		"int after(void) { return 1; }\n"
	result, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("repeated simple macro state produced an impossible structural variant: %+v", result.Analysis)
	}
	if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)["after"]; !ok {
		t.Fatalf("declaration after repeated macro guards missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestCAnalyzerRepeatedSimpleMacroStateAcrossElifKeepsScopeBalanced(t *testing.T) {
	text := "#ifdef __cplusplus\n" +
		"extern \"C\" {\n" +
		"#elif _MSC_VER\n" +
		"#ifndef inline\n" +
		"#define inline __inline\n" +
		"#endif\n" +
		"#endif\n" +
		"extern void worker(void);\n" +
		"#ifdef _WIN32\n" +
		"#include \"windows.h\"\n" +
		"#else\n" +
		"#include \"posix.h\"\n" +
		"#endif\n" +
		"#ifdef __cplusplus\n" +
		"}\n" +
		"#endif\n" +
		"int after(void) { return 1; }\n"
	result, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("#elif/include sequence lost the stable __cplusplus state: %+v", result.Analysis)
	}
}

func TestCAnalyzerRepeatedExactIfExpressionKeepsScopeBalanced(t *testing.T) {
	text := "int run(int enabled) {\n" +
		"#if VERSION_AT_LEAST(6,9)\n" +
		"  if (enabled) {\n" +
		"#else\n" +
		"  enabled = 0;\n" +
		"#endif\n" +
		"  enabled++;\n" +
		"#if VERSION_AT_LEAST(6,9)\n" +
		"  }\n" +
		"#endif\n" +
		"  return enabled;\n" +
		"}\n"
	result, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("identical #if expressions produced an impossible structural variant: %+v", result.Analysis)
	}
}

func TestCFamilyConditionalPlannerDoesNotExpandNestedOneShotGuards(t *testing.T) {
	var text strings.Builder
	for index := 0; index < 40; index++ {
		fmt.Fprintf(&text, "#ifdef FEATURE_%d\n", index)
	}
	text.WriteString("int guarded_value;\n")
	for index := 39; index >= 0; index-- {
		text.WriteString("#endif\n")
	}
	result, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text.String()), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || hasAnalysisDiagnostic(result.Analysis.Diagnostics, "c-conditional-variant-limit") {
		t.Fatalf("one-shot nested guards exhausted structural variants: %+v", result.Analysis)
	}
}

func TestCFamilyConditionalPlannerBreaksCorrelationAcrossMacroMutation(t *testing.T) {
	text := "#ifdef FEATURE\nint first;\n#endif\n" +
		"#undef FEATURE\n" +
		"#ifdef FEATURE\nint second;\n#endif\n"
	document := sourceDocumentForScanner(text)
	profile := CScannerProfile()
	profile.DisableDelimiterTracking = true
	scan, err := ScanSource(context.Background(), document, profile, ScannerLimits{MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: 64})
	if err != nil {
		t.Fatal(err)
	}
	plan := cFamilyConditionalPlan(scan.Tokens)
	if len(plan.groups) != 2 || plan.groups[0].conditionKey == "" || plan.groups[1].conditionKey == "" {
		t.Fatalf("unexpected simple conditional plan: %+v", plan.groups)
	}
	if plan.groups[0].conditionKey == plan.groups[1].conditionKey {
		t.Fatalf("macro mutation did not invalidate repeated-condition correlation: %+v", plan.groups)
	}
}

func TestCPPAnalyzerMultilineMacroDirectiveStaysOpaque(t *testing.T) {
	text := "#define JSON_CAST(T, expr) (__extension__ ({ \\\r\n" +
		"    ((T) (expr)); \\\r\n" +
		"}))\r\n" +
		"struct After { int value; };\r\n"
	result, err := (CPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("continued C++ macro directive leaked into structural analysis: %+v", result.Analysis)
	}
	if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)["After"]; !ok {
		t.Fatalf("declaration after continued C++ macro missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestCAnalyzerDirectiveMultilineBlockCommentStaysOpaque(t *testing.T) {
	text := "#define FLAG (1U << 0) /* comment starts on a directive\n" +
		"                         * doesn't become a character literal. */\n" +
		"int after(void) { return FLAG; }\n"
	result, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("multiline directive comment lowered C coverage: %+v", result.Analysis)
	}
	if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)["after"]; !ok {
		t.Fatalf("declaration after multiline directive comment missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestCAnalyzerDirectiveBlockCommentPreservesLineSplice(t *testing.T) {
	text := "#define WRAP() do { \\\n" +
		"  /* comment continues \\\n" +
		"   * here */ \\\n" +
		"} while (0)\n" +
		"int after(void) { return 1; }\n"
	result, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("directive block comment consumed macro line splice: %+v", result.Analysis)
	}
	if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)["after"]; !ok {
		t.Fatalf("declaration after directive block comment missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestCAnalyzerFunctionMacroStructuralOpenersMatchExplicitClosers(t *testing.T) {
	text := "#define CHECK(j) \\\n" +
		"    { int score = (j); \\\n" +
		"      if (score < 0) { \\\n" +
		"        score = -score; \\\n" +
		"\n" +
		"int run(void) {\n" +
		"    CHECK(-1) CHECK(-2) }} }}\n" +
		"    return 0;\n" +
		"}\n" +
		"int after(void) { return 1; }\n"
	result, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("proven macro structural openers did not match explicit closers: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, want := range []string{"run", "after"} {
		if _, ok := byName[want]; !ok {
			t.Fatalf("macro structural projection lost %s: %v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestCAnalyzerFunctionMacroStructuralOpenersFailClosedOnInsufficientClosers(t *testing.T) {
	text := "#define CHECK(j) \\\n" +
		"    { int score = (j); \\\n" +
		"      if (score < 0) { \\\n" +
		"        score = -score; \\\n" +
		"\n" +
		"int run(void) {\n" +
		"    CHECK(-1) }\n" +
		"    return 0;\n" +
		"}\n"
	result, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete {
		t.Fatalf("insufficient explicit macro closers were hidden: %+v", result.Analysis)
	}
}

func TestCAnalyzerObjectMacroStructuralOpenerMatchesExplicitCloser(t *testing.T) {
	text := "#define ASM_RAW __asm__ volatile(\n" +
		"#define ASM_BEGIN ASM_RAW\n" +
		"int run(void) {\n" +
		"    ASM_BEGIN\n" +
		"    \"nop\"\n" +
		"    : : :\n" +
		"    );\n" +
		"    return 0;\n" +
		"}\n" +
		"int after(void) { return 1; }\n"
	result, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("composed object macro structural opener did not match explicit closer: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, want := range []string{"run", "after"} {
		if _, ok := byName[want]; !ok {
			t.Fatalf("object macro structural projection lost %s: %v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestCAnalyzerObjectMacroStructuralCloserCancelsVirtualOpener(t *testing.T) {
	text := "#define ASM_RAW __asm__ volatile(\n" +
		"#define ASM_BEGIN ASM_RAW\n" +
		"#define ASM_END );\n" +
		"int run(void) {\n" +
		"    ASM_BEGIN\n" +
		"    \"nop\"\n" +
		"    : : :\n" +
		"    ASM_END\n" +
		"    return 0;\n" +
		"}\n" +
		"int after(void) { return 1; }\n"
	result, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("object macro closer failed to cancel virtual opener: %+v", result.Analysis)
	}
	if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)["after"]; !ok {
		t.Fatalf("object macro closer leaked virtual state into later source: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestCAnalyzerStructuralMacroCloserCanCloseProvenRealOpener(t *testing.T) {
	text := "#define OPEN_FUNC(name) int name(void) { while (1) {\n" +
		"#define END_FUNC() } }\n" +
		"OPEN_FUNC(run)\n" +
		"    }\n" +
		"    if (1) {\n" +
		"        return 1;\n" +
		"    END_FUNC()\n" +
		"int after(void) { return 2; }\n"
	result, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("proven macro closer did not balance a real opener: %+v", result.Analysis)
	}
}

func TestCAnalyzerConditionalDirectiveMultilineBlockCommentStaysOpaque(t *testing.T) {
	text := "#ifdef FEATURE\n" +
		"int selected(void) { return 1; }\n" +
		"#else /* !FEATURE && (!OTHER ||\n" +
		"         !THIRD) */\n" +
		"int fallback(void) { return 0; }\n" +
		"#endif\n" +
		"int after(void) { return 2; }\n"
	result, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("multiline conditional-directive comment lowered C coverage: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, want := range []string{"selected", "fallback", "after"} {
		if _, ok := byName[want]; !ok {
			t.Fatalf("conditional directive comment lost %s: %v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
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

func TestCStructUsesAndInitializersDoNotBecomeTypeDefinitions(t *testing.T) {
	text := `struct Item {
    int value;
};
static const struct Item items[] = {{1}};
static struct Item *make_item(void) {
    return &items[0];
}
`
	result, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid C struct uses reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	if symbol, ok := byName["Item"]; !ok || symbol.Kind != SymbolKindStruct {
		t.Fatalf("Item = %+v exists=%v; symbols=%+v", symbol, ok, result.Analysis.Symbols)
	}
	if symbol, ok := byName["make_item"]; !ok || symbol.Kind != SymbolKindFunction {
		t.Fatalf("make_item = %+v exists=%v; symbols=%+v", symbol, ok, result.Analysis.Symbols)
	}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.QualifiedName == "Item" && symbol.Kind == SymbolKindStruct && symbol.DeclarationRange.Start.Line != 1 {
			t.Fatalf("C struct use overclaimed as a second type definition: %+v", symbol)
		}
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
