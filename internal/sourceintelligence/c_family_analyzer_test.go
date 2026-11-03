package sourceintelligence

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

var _ SourceAnalyzer = CAnalyzer{}
var _ SourceAnalyzer = CPPAnalyzer{}

func TestCPPRawStringStartRecognizesPrefixesAndIdentifierBoundaries(t *testing.T) {
	cases := []struct {
		name           string
		text           string
		index          int
		wantPrefix     int
		wantDelimiter  int
		wantRecognized bool
	}{
		{name: "plain", text: `R"tag(`, wantPrefix: 2, wantDelimiter: 2, wantRecognized: true},
		{name: "wide", text: `LR"tag(`, wantPrefix: 3, wantDelimiter: 3, wantRecognized: true},
		{name: "utf32", text: `UR"tag(`, wantPrefix: 3, wantDelimiter: 3, wantRecognized: true},
		{name: "utf16", text: `uR"tag(`, wantPrefix: 3, wantDelimiter: 3, wantRecognized: true},
		{name: "utf8", text: `u8R"tag(`, wantPrefix: 4, wantDelimiter: 4, wantRecognized: true},
		{name: "punctuation-boundary", text: `:R"tag(`, index: 1, wantPrefix: 2, wantDelimiter: 3, wantRecognized: true},
		{name: "unicode-byte-boundary", text: "éR\"tag(", index: len("é"), wantPrefix: 2, wantDelimiter: len("é") + 2, wantRecognized: true},
		{name: "identifier-letter", text: `xR"tag(`, index: 1},
		{name: "identifier-digit", text: `0R"tag(`, index: 1},
		{name: "identifier-underscore", text: `_R"tag(`, index: 1},
		{name: "truncated", text: `u8R`, index: 0},
		{name: "near-miss", text: `uXR"tag(`, index: 0},
		{name: "end", text: `plain`, index: len("plain")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prefix, delimiter, ok := cppRawStringStart(tc.text, tc.index)
			if prefix != tc.wantPrefix || delimiter != tc.wantDelimiter || ok != tc.wantRecognized {
				t.Fatalf("cppRawStringStart(%q, %d) = (%d, %d, %v), want (%d, %d, %v)", tc.text, tc.index, prefix, delimiter, ok, tc.wantPrefix, tc.wantDelimiter, tc.wantRecognized)
			}
		})
	}
}

func TestMaskCPPRawStringsLeavesNonRawLookalikesUnchanged(t *testing.T) {
	cases := []string{
		`// R"tag(class CommentFake {})tag"`,
		`/* u8R"tag(class BlockCommentFake {})tag" */`,
		`const char* text = "R\"tag(class QuotedFake {})tag\"";`,
		`auto invalid = R"bad delim(class InvalidDelimiterFake {})bad delim";`,
	}
	for _, text := range cases {
		masked, diagnostics, err := maskCPPRawStrings(context.Background(), text)
		if err != nil {
			t.Fatal(err)
		}
		if masked != text || len(diagnostics) != 0 {
			t.Fatalf("non-raw lookalike changed: masked=%q diagnostics=%+v", masked, diagnostics)
		}
	}
}

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
const wchar_t* wide = LR"tag(class WideRawFake { void NopeWide(); })tag";
const char32_t* utf32 = UR"tag(class UTF32RawFake { void Nope32(); })tag";
const char16_t* utf16 = uR"tag(class UTF16RawFake { void Nope16(); })tag";
const char8_t* utf8 = u8R"tag(class UTF8RawFake { void Nope8(); })tag";
const char* ordinary = "R\"tag(class QuotedFake {})tag\"";
// R"tag(class CommentRawFake {})tag"
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
		switch symbol.Name {
		case "MacroFake", "RawFake", "Nope", "WideRawFake", "NopeWide", "UTF32RawFake", "Nope32", "UTF16RawFake", "Nope16", "UTF8RawFake", "Nope8", "QuotedFake", "CommentRawFake":
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

func TestCFamilyReduceMacroDelimiterEffectContracts(t *testing.T) {
	effects := map[string]cFamilyMacroStructuralEffect{
		"OPEN": {
			suffixOpeners: []string{"{", "("},
		},
		"CLOSE": {
			prefixClosers: []string{")", "}"},
		},
		"MIXED": {
			prefixClosers: []string{")"},
			suffixOpeners: []string{"["},
		},
		"CALL": {
			functionLike:  true,
			suffixOpeners: []string{"{"},
		},
	}
	tests := []struct {
		name       string
		body       string
		wantPrefix []string
		wantSuffix []string
		wantOK     bool
	}{
		{
			name:   "balanced delimiters",
			body:   "({[]})",
			wantOK: true,
		},
		{
			name:       "unmatched closers and openers are preserved",
			body:       ")} text {[",
			wantPrefix: []string{")", "}"},
			wantSuffix: []string{"{", "["},
			wantOK:     true,
		},
		{
			name:   "quotes and comments are opaque",
			body:   "\"({\" '])' /* {[( */ // )]}\n({})",
			wantOK: true,
		},
		{
			name:       "line continuation LF is ignored",
			body:       ")\\\n{",
			wantPrefix: []string{")"},
			wantSuffix: []string{"{"},
			wantOK:     true,
		},
		{
			name:       "line continuation CRLF is ignored",
			body:       "}\\\r\n(",
			wantPrefix: []string{"}"},
			wantSuffix: []string{"("},
			wantOK:     true,
		},
		{
			name:   "object macro effects compose",
			body:   "OPEN CLOSE",
			wantOK: true,
		},
		{
			name:       "object macro prefix and suffix are preserved",
			body:       "MIXED",
			wantPrefix: []string{")"},
			wantSuffix: []string{"["},
			wantOK:     true,
		},
		{
			name:   "bare function-like macro is structurally inert",
			body:   "CALL",
			wantOK: true,
		},
		{
			name:   "function-like macro invocation fails closed",
			body:   "CALL (value)",
			wantOK: false,
		},
		{
			name:   "mismatched delimiter fails closed",
			body:   "(]",
			wantOK: false,
		},
		{
			name:   "token pasting fails closed",
			body:   "value ## other",
			wantOK: false,
		},
		{
			name:   "unterminated quote fails closed",
			body:   "\"(",
			wantOK: false,
		},
		{
			name:   "unterminated block comment fails closed",
			body:   "/* (",
			wantOK: false,
		},
		{
			name:       "exact nesting cap is accepted",
			body:       strings.Repeat("(", 64),
			wantSuffix: strings.Split(strings.Repeat("( ", 64), " "),
			wantOK:     true,
		},
		{
			name:   "nesting above cap fails closed",
			body:   strings.Repeat("(", 65),
			wantOK: false,
		},
	}
	tests[len(tests)-2].wantSuffix = tests[len(tests)-2].wantSuffix[:64]

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prefix, suffix, ok := cFamilyReduceMacroDelimiterEffect(test.body, effects)
			if ok != test.wantOK || !slices.Equal(prefix, test.wantPrefix) || !slices.Equal(suffix, test.wantSuffix) {
				t.Fatalf("reduce(%q) = prefix=%v suffix=%v ok=%t, want prefix=%v suffix=%v ok=%t",
					test.body, prefix, suffix, ok, test.wantPrefix, test.wantSuffix, test.wantOK)
			}
		})
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

// A qualified assignment operator and defaulted destructor must not become variables.
func TestCPPQualifiedOperatorAndDefaultedDestructorFromOriginalShape(t *testing.T) {
	const source = `namespace BT {
TreeNode& TreeNode::operator=(TreeNode&& other) noexcept
{
  return *this;
}
TreeNode::~TreeNode() = default;
}`
	for _, tc := range []struct{ name, text string }{
		{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (CPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("qualified source partial: %+v", result.Analysis)
			}
			foundOperator, foundDefaulted := false, false
			for _, s := range result.Analysis.Symbols {
				if s.NameRange.Start.Line != 2 && s.NameRange.Start.Line != 6 {
					continue
				}
				if s.Kind == SymbolKindVariable {
					t.Errorf("false variable: %+v", s)
				}
				if s.NameRange.Start.Line == 2 && s.Kind == SymbolKindOperator && s.Name == "operator=" &&
					s.QualifiedName == "BT.TreeNode.operator=" && s.NativeKind == "operator-definition" && s.BodyRange != nil {
					foundOperator = true
				}
				if s.NameRange.Start.Line == 6 && s.Kind == SymbolKindFunction && s.QualifiedName == "BT.TreeNode.TreeNode" &&
					s.NativeKind == "qualified-destructor-defaulted-definition" && s.BodyRange == nil && s.ParentQualifiedName == "BT" {
					foundDefaulted = true
				}
			}
			if !foundOperator || !foundDefaulted {
				t.Fatalf("operator=%v defaulted=%v; symbols=%+v", foundOperator, foundDefaulted, result.Analysis.Symbols)
			}
		})
	}
}

// Out-of-class nested C++ type definitions must not masquerade as a
// definition of their qualifier; the qualifier can live in a separate header.
func TestCPPQualifiedNestedStructDoesNotInventOuterClass(t *testing.T) {
	const source = `namespace BT {
struct TreeNode::PImpl {
  PImpl(int n) {}
  int value;
};
TreeNode::~TreeNode() = default;
}`
	result, err := (CPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("partial original-shaped nested class: %+v", result.Analysis)
	}
	wantStruct, wantCtor, wantDestructor := false, false, false
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Kind == SymbolKindStruct && symbol.QualifiedName == "BT.TreeNode" {
			t.Errorf("invented outer class from nested type: %+v", symbol)
		}
		if symbol.Kind == SymbolKindStruct && symbol.Name == "PImpl" && symbol.QualifiedName == "BT.TreeNode.PImpl" &&
			symbol.ParentQualifiedName == "BT" && symbol.NameRange.Start.Line == 2 {
			wantStruct = true
		}
		if symbol.Kind == SymbolKindConstructor && symbol.QualifiedName == "BT.TreeNode.PImpl.PImpl" &&
			symbol.NameRange.Start.Line == 3 {
			wantCtor = true
		}
		if symbol.Kind == SymbolKindFunction && symbol.QualifiedName == "BT.TreeNode.TreeNode" &&
			symbol.NativeKind == "qualified-destructor-defaulted-definition" && symbol.BodyRange == nil &&
			symbol.ParentQualifiedName == "BT" && symbol.NameRange.Start.Line == 6 {
			wantDestructor = true
		}
	}
	if !wantStruct || !wantCtor || !wantDestructor {
		t.Fatalf("nested type=%v constructor=%v defaulted destructor=%v; symbols=%+v", wantStruct, wantCtor, wantDestructor, result.Analysis.Symbols)
	}
}

func TestCPPUnresolvedQualifiedDefinitionsRetainAuthoredScope(t *testing.T) {
	const source = `void printer::print(int value) {}
void other_scope::free_function() {}
namespace Local {
void printer::reset() {}
}
void Outer::Inner::complicated() {}
`
	for _, tc := range []struct {
		name, text string
	}{
		{"LF", source},
		{"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (CPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for qualified, line := range map[string]int{
				"printer.print":             1,
				"other_scope.free_function": 2,
				"Local.printer.reset":       4,
			} {
				got, ok := byName[qualified]
				if !ok || got.Kind != SymbolKindFunction || got.NameRange.Start.Line != line {
					t.Fatalf("unresolved authored scope %q at line %d: got %+v, exists=%v; symbols=%v", qualified, line, got, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
				if got.ParentQualifiedName != "" && got.ParentQualifiedName != "Local" {
					t.Fatalf("invented C++ type owner: %+v", got)
				}
			}
			if _, falseOwnership := byName["Inner.complicated"]; falseOwnership {
				t.Fatalf("nested unresolved qualification was truncated into a false scope: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}

func TestCPPOriginalClinkQualifiedDefinitions(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_CPP_CLINK_PRINTER_CPP")
	if path == "" {
		t.Skip("set SCRIPTHOLD_REAL_CPP_CLINK_PRINTER_CPP to the qualified Clink printer.cpp fixture")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 1<<20 {
		t.Fatal("original Clink fixture exceeds 1 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	document := sourceDocumentForScanner(string(data))
	document.Path = path
	result, err := (CPPAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]bool{36: false, 62: false, 71: false}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.QualifiedName != "printer.print" {
			continue
		}
		if symbol.Name != "print" || symbol.Kind != SymbolKindFunction || symbol.ParentQualifiedName != "" {
			t.Fatalf("unproven Clink class membership falsely asserted: %+v", symbol)
		}
		if _, ok := want[symbol.NameRange.Start.Line]; !ok {
			t.Fatalf("unexpected qualified Clink print definition: %+v", symbol)
		}
		want[symbol.NameRange.Start.Line] = true
	}
	for line, found := range want {
		if !found {
			t.Fatalf("original Clink printer::print definition at line %d lost; symbols=%v", line, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
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

func TestCPPExportDecoratedTypesPreserveNamesAndMembers(t *testing.T) {
	const source = `namespace Demo {
class LIBRARY_API Widget final : public Base {
public:
    Widget();
    void work();
};
struct LIBRARY_EXPORT Record { int count; };
union LIBRARY_EXPORT Variant { int value; };
class Plain { public: void regular(); };
class Valid_API final {};
class DECLARED_EXPORT final {};
class LIBRARY_API Forward;
}`
	for _, tc := range []struct {
		name string
		text string
	}{
		{name: "LF", text: source},
		{name: "CRLF", text: strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (CPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("valid decorated class declarations reported partial: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for qualified, kind := range map[string]SymbolKind{
				"Demo.Widget":          SymbolKindClass,
				"Demo.Widget.Widget":   SymbolKindConstructor,
				"Demo.Widget.work":     SymbolKindMethod,
				"Demo.Record":          SymbolKindStruct,
				"Demo.Record.count":    SymbolKindField,
				"Demo.Variant":         SymbolKindType,
				"Demo.Variant.value":   SymbolKindField,
				"Demo.Plain":           SymbolKindClass,
				"Demo.Plain.regular":   SymbolKindMethod,
				"Demo.Valid_API":       SymbolKindClass,
				"Demo.DECLARED_EXPORT": SymbolKindClass,
				"Demo.Forward":         SymbolKindClass,
			} {
				if got, ok := byName[qualified]; !ok || got.Kind != kind {
					t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, got, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Name == "LIBRARY_API" || symbol.Name == "LIBRARY_EXPORT" {
					t.Fatalf("export decoration indexed as a declaration: %+v", symbol)
				}
			}
			if !hasStructuralRelation(result.Relations, "inherits", "Demo.Widget", "Base") {
				t.Fatalf("decorated class lost inheritance: %+v", result.Relations)
			}
		})
	}
}

func TestCPPOriginalSFMLExportDecoratedTypes(t *testing.T) {
	for _, tc := range []struct {
		name, env, qualified, constructor string
		line                              int
	}{
		{"RenderWindow", "SCRIPTHOLD_REAL_CPP_SFML_RENDERWINDOW_HEADER", "sf.RenderWindow", "sf.RenderWindow.RenderWindow", 54},
		{"Sprite", "SCRIPTHOLD_REAL_CPP_SFML_SPRITE_HEADER", "sf.Sprite", "sf.Sprite.Sprite", 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skipf("set %s to an original qualified SFML header", tc.env)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() > 1<<20 {
				t.Fatal("original fixture exceeds 1 MiB")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			document := sourceDocumentForScanner(string(data))
			document.Path = path
			result, err := (CPPAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			foundClass, foundConstructor := false, false
			for _, symbol := range result.Analysis.Symbols {
				if symbol.QualifiedName == tc.qualified && symbol.Kind == SymbolKindClass {
					if symbol.NameRange.Start.Line != tc.line {
						t.Fatalf("original SFML class name range = %+v, want line %d", symbol.NameRange, tc.line)
					}
					foundClass = true
				}
				if symbol.QualifiedName == tc.constructor && symbol.Kind == SymbolKindConstructor {
					if symbol.ParentQualifiedName != tc.qualified {
						t.Fatalf("original SFML constructor lost owner: %+v", symbol)
					}
					foundConstructor = true
				}
				if symbol.Name == "SFML_GRAPHICS_API" {
					t.Fatalf("SFML export decoration became a symbol: %+v", symbol)
				}
			}
			if !foundClass || !foundConstructor {
				t.Fatalf("original SFML symbols missing: class=%v constructor=%v; symbols=%v diagnostics=%+v", foundClass, foundConstructor, sortedSymbolQualifiedNames(result.Analysis.Symbols), result.Analysis.Diagnostics)
			}
		})
	}
}

func TestCPPQualifiedDefinitionOwnersDoNotInventCrossFileTypes(t *testing.T) {
	cases := []struct {
		name, input string
		ownerKnown  bool
	}{
		{"known-class", "class Gadget { public: Gadget(); ~Gadget(); int Read(); };\nGadget::Gadget() {}\nGadget::~Gadget() {}\nint Gadget::Read() { return 0; }\n", true},
		{"separate-definition", "#include \"gadget.h\"\nGadget::Gadget() {}\nGadget::~Gadget() {}\nint Gadget::Read() { return 0; }\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (CPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.input), testAnalyzeOptions(true, 40))
			if err != nil {
				t.Fatal(err)
			}
			definitions := map[string]bool{"Gadget.Gadget@2": false, "Gadget.Gadget@3": false, "Gadget.Read@4": false}
			for _, symbol := range result.Analysis.Symbols {
				if symbol.BodyRange == nil || symbol.Kind == SymbolKindClass {
					continue
				}
				key := fmt.Sprintf("%s@%d", symbol.QualifiedName, symbol.NameRange.Start.Line)
				if _, exists := definitions[key]; !exists {
					t.Errorf("unexpected definition: %+v", symbol)
					continue
				}
				definitions[key] = true
				if tc.ownerKnown {
					if symbol.ParentQualifiedName != "Gadget" || symbol.ParentID == "" {
						t.Errorf("lost known parent: %+v", symbol)
					}
				} else if symbol.ParentQualifiedName != "" || symbol.ParentID != "" {
					t.Errorf("invented class in different file: %+v", symbol)
				}
			}
			for key, found := range definitions {
				if !found {
					t.Errorf("missing explicit qualified definition %s: %v", key, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
		})
	}
}

func TestCPP20ConceptDoesNotBecomeVariable(t *testing.T) {
	input := "namespace Util {\ntemplate<typename T>\nconcept function_pointer = is_pointer<T>;\ntemplate<typename T> using value_type = T;\nint ordinary = 1;\n}\n"
	result, err := (CPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(input), testAnalyzeOptions(true, 40))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || !hasAnalysisDiagnostic(result.Analysis.Diagnostics, "cpp-unsupported-concept") {
		t.Fatalf("unsupported concept must signal partial coverage: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	if _, found := byName["Util.function_pointer"]; found {
		t.Fatalf("concept fabricated as a variable: %+v", byName["Util.function_pointer"])
	}
	if byName["Util"].Kind != SymbolKindNamespace || byName["Util.value_type"].Kind != SymbolKindAlias || byName["Util.ordinary"].Kind != SymbolKindVariable {
		t.Fatalf("adjacent declarations lost: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestCPPQualifiedTemplateSpecializationPreservesDefinitions(t *testing.T) {
	cases := []struct {
		name, code string
		want       map[string]int
	}{
		{
			name: "explicit-specialization",
			code: "template<>\nvoid Error::impl::Handle<spdlog::level::err>(int value) {\n  if (value) { Log(); }\n}\nvoid Error::impl::Ready() {}\n",
			want: map[string]int{"Error.impl.Handle": 2, "Error.impl.Ready": 5},
		},
		{
			name: "nested-qualified-function",
			code: "std::wstring Alpha::Beta::Get(int count) { return {}; }\n",
			want: map[string]int{"Alpha.Beta.Get": 1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (CPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.code), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("unexpected incomplete scan: %+v", result.Analysis)
			}
			if len(result.Analysis.Symbols) != len(tc.want) {
				t.Fatalf("declarations %v; expected %v", sortedSymbolQualifiedNames(result.Analysis.Symbols), tc.want)
			}
			for _, symbol := range result.Analysis.Symbols {
				line, ok := tc.want[symbol.QualifiedName]
				if !ok || symbol.Kind != SymbolKindFunction || symbol.NameRange.Start.Line != line ||
					symbol.BodyRange == nil || symbol.ParentID != "" || symbol.ParentQualifiedName != "" {
					t.Errorf("false or misqualified C++ declaration: %+v", symbol)
				}
			}
		})
	}
}
