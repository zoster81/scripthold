package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Groovy class fields without semicolons must not be swallowed by the next method.
// A D alias declaration names its alias, not a call in its target expression.
func TestDTypeAliasDoesNotInventTypeofFunction(t *testing.T) {
	const code = "module demo;\nclass Service(T) {\n  alias TSelf = typeof(this);\n  alias TBinding = Container!(T);\n  T value;\n  void run() {}\n}\n"
	for _, tc := range []struct{ name, text string }{{"lf", code}, {"crlf", strings.ReplaceAll(code, "\n", "\r\n")}} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := (DAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !out.Analysis.CoverageComplete || out.Analysis.Truncated {
				t.Fatalf("incomplete source: %+v", out.Analysis)
			}
			names := symbolsByQualifiedName(out.Analysis.Symbols)
			for _, want := range []struct {
				name string
				kind SymbolKind
				line int
			}{
				{"demo", SymbolKindModule, 1}, {"demo.Service", SymbolKindClass, 2},
				{"demo.Service.TSelf", SymbolKindAlias, 3}, {"demo.Service.TBinding", SymbolKindAlias, 4},
				{"demo.Service.value", SymbolKindField, 5}, {"demo.Service.run", SymbolKindMethod, 6},
			} {
				sym, ok := names[want.name]
				if !ok || sym.Kind != want.kind || sym.NameRange.Start.Line != want.line {
					t.Errorf("symbol %s present=%v got=%+v; want kind=%s line=%d; all=%v", want.name, ok, sym, want.kind, want.line, sortedSymbolQualifiedNames(out.Analysis.Symbols))
				}
			}
			if _, falseSymbol := names["demo.Service.typeof"]; falseSymbol {
				t.Error("type query indexed as a method")
			}
			if _, falseSymbol := names["demo.Service.Container"]; falseSymbol {
				t.Error("template target indexed as a method")
			}
		})
	}
}

func TestGroovyNewlineTerminatedFieldsBeforeMethod(t *testing.T) {
	const original = "package demo\nclass MethodSignature {\n  String name\n  Class[] args\n  static MethodSignature method(String name, Class... args = []) { return null }\n  MethodSignature(String name, Class[] args) {}\n}\n"
	for _, tc := range []struct{ name, source string }{
		{"lf", original}, {"crlf", strings.ReplaceAll(original, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (GroovyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 100))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("partial Groovy: %+v", result.Analysis)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, want := range []struct {
				name string
				kind SymbolKind
				line int
			}{
				{"demo", SymbolKindPackage, 1},
				{"demo.MethodSignature", SymbolKindClass, 2},
				{"demo.MethodSignature.name", SymbolKindField, 3},
				{"demo.MethodSignature.args", SymbolKindField, 4},
				{"demo.MethodSignature.method", SymbolKindMethod, 5},
				{"demo.MethodSignature.MethodSignature", SymbolKindConstructor, 6},
			} {
				symbol, ok := got[want.name]
				if !ok || symbol.Kind != want.kind || symbol.NameRange.Start.Line != want.line {
					t.Errorf("symbol %s: present=%v got=%+v want kind=%s line=%d; all=%v", want.name, ok, symbol, want.kind, want.line, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
		})
	}
}

// Getter declarations use their own names; body calls are not methods.
func TestDartGetterDeclarationsDoNotIndexBodyCalls(t *testing.T) {
	const original = "extension ColorExtension on Color {\n" +
		"  Color get opacity80 {\n    return withValues(alpha: 0.8);\n  }\n" +
		"  int get alpha8bit => (value32bit) >> 24;\n" +
		"  String get hex {\n    final red = toARGB32();\n    return '\\u0024{red.toString()}'.toUpperCase();\n  }\n" +
		"  int doubleValue(int x) { return x * 2; }\n}\n" +
		"int get globalCount => 42;\nclass Other {\n  bool get ready => true;\n}\n"
	for _, tc := range []struct{ name, source string }{
		{"lf", original}, {"crlf", strings.ReplaceAll(original, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (DartAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 80))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("Dart getter analysis partial: %+v", result.Analysis)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, want := range []struct {
				name string
				kind SymbolKind
				line int
			}{
				{"ColorExtension", SymbolKindType, 1},
				{"ColorExtension.opacity80", SymbolKindProperty, 2},
				{"ColorExtension.alpha8bit", SymbolKindProperty, 5},
				{"ColorExtension.hex", SymbolKindProperty, 6},
				{"ColorExtension.doubleValue", SymbolKindMethod, 10},
				{"globalCount", SymbolKindProperty, 12},
				{"Other", SymbolKindClass, 13},
				{"Other.ready", SymbolKindProperty, 14},
			} {
				sym, ok := got[want.name]
				if !ok || sym.Kind != want.kind || sym.NameRange.Start.Line != want.line {
					t.Errorf("%s: exists=%v got=%+v want kind=%s line=%d", want.name, ok, sym, want.kind, want.line)
				}
			}
			for _, forbidden := range []string{"ColorExtension.withValues", "ColorExtension.value32bit",
				"ColorExtension.toARGB32", "ColorExtension.toUpperCase", "Other.true"} {
				if sym, ok := got[forbidden]; ok {
					t.Errorf("false body declaration %s: %+v", forbidden, sym)
				}
			}
		})
	}
}

func TestDartRiverpodAnnotationKeepsClassAndMembers(t *testing.T) {
	const original = `@Riverpod(keepAlive: true)
class AnalyticsController extends GeneratedAnalyticsController {
  Future<bool> build() async { return true; }
  Future<void> enableAnalytics() async {}
  Future<void> disableAnalytics() async {}
}
class Sibling {}
`
	for _, tc := range []struct{ name, source string }{
		{"LF", original},
		{"CRLF", strings.ReplaceAll(original, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (DartAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("Dart annotation analysis incomplete: %+v", result.Analysis)
			}
			found := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range map[string]SymbolKind{
				"AnalyticsController":                  SymbolKindClass,
				"AnalyticsController.build":            SymbolKindMethod,
				"AnalyticsController.enableAnalytics":  SymbolKindMethod,
				"AnalyticsController.disableAnalytics": SymbolKindMethod,
				"Sibling":                              SymbolKindClass,
			} {
				if sym, ok := found[name]; !ok || sym.Kind != kind {
					t.Errorf("%s: exists=%v symbol=%+v; names=%v", name, ok, sym, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			if sym, ok := found["Riverpod"]; ok {
				t.Errorf("annotation reported as callable: %+v", sym)
			}
		})
	}
}

func TestDartOriginalHiddifyRiverpodController(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_DART_HIDDIFY_ANALYTICS_CONTROLLER")
	if path == "" {
		t.Skip("set SCRIPTHOLD_REAL_DART_HIDDIFY_ANALYTICS_CONTROLLER to the qualified Hiddify original")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 1<<20 {
		t.Fatal("Hiddify fixture exceeds 1 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := sourceDocumentForScanner(string(data))
	doc.Path = path
	result, err := (DartAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("Hiddify analyzer incomplete: %+v", result.Analysis)
	}
	want := map[string]struct {
		kind SymbolKind
		line int
	}{
		"AnalyticsController":                  {SymbolKindClass, 20},
		"AnalyticsController.build":            {SymbolKindMethod, 22},
		"AnalyticsController.enableAnalytics":  {SymbolKindMethod, 28},
		"AnalyticsController.disableAnalytics": {SymbolKindMethod, 63},
	}
	for _, sym := range result.Analysis.Symbols {
		if sym.Name == "Riverpod" {
			t.Fatalf("original Riverpod annotation became a symbol: %+v", sym)
		}
		if expected, ok := want[sym.QualifiedName]; ok {
			if sym.Kind != expected.kind || sym.NameRange.Start.Line != expected.line {
				t.Errorf("%s: got %+v want %+v", sym.QualifiedName, sym, expected)
			}
			delete(want, sym.QualifiedName)
		}
	}
	if len(want) > 0 {
		t.Fatalf("missing Hiddify symbols %v; got %v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestDartInitializerCallsAreNotFunctionDeclarations(t *testing.T) {
	const source = `final numberFormatter = NumberFormat.compact();
final currencyFormatter = NumberFormat.compactCurrency(
  locale: 'en-US',
  digits: 2,
);
final bool Function() predicate = () => true;
void main() {}
class Formatter {
  final value = NumberFormat.compact();
  void refresh() {}
}
final decoy = "final Fake = Formatter.fake();";
`
	for _, tc := range []struct{ name, source string }{
		{"LF", source},
		{"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (DartAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("Dart initializer incomplete: %+v", result.Analysis)
			}
			found := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range map[string]SymbolKind{
				"numberFormatter":   SymbolKindConstant,
				"currencyFormatter": SymbolKindConstant,
				"predicate":         SymbolKindConstant,
				"main":              SymbolKindFunction,
				"Formatter":         SymbolKindClass,
				"Formatter.value":   SymbolKindField,
				"Formatter.refresh": SymbolKindMethod,
			} {
				if sym, ok := found[name]; !ok || sym.Kind != kind {
					t.Errorf("%s: exists=%v got %+v, want %s; symbols=%v", name, ok, sym, kind, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			for _, name := range []string{"compact", "compactCurrency", "Fake", "Formatter.fake"} {
				if sym, exists := found[name]; exists {
					t.Errorf("fake initializer symbol %s: %+v", name, sym)
				}
			}
		})
	}
}

func TestDartOriginalSpotubeGlobalFormatters(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_DART_SPOTUBE_FORMATTERS")
	if path == "" {
		t.Skip("set SCRIPTHOLD_REAL_DART_SPOTUBE_FORMATTERS to the qualified original Spotube formatters.dart")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 1<<20 {
		t.Fatal("Spotube fixture exceeds 1 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := sourceDocumentForScanner(string(data))
	doc.Path = path
	result, err := (DartAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("Spotube original analyzer incomplete: %+v", result.Analysis)
	}
	want := map[string]int{"compactNumberFormatter": 3, "usdFormatter": 4}
	for _, sym := range result.Analysis.Symbols {
		if line, ok := want[sym.QualifiedName]; ok {
			if sym.NameRange.Start.Line != line || sym.Kind != SymbolKindConstant {
				t.Errorf("%s: expected constant at line %d; got %+v", sym.QualifiedName, line, sym)
			}
			delete(want, sym.QualifiedName)
		}
		if sym.Name == "compact" || sym.Name == "compactCurrency" {
			t.Errorf("formatter initializer call became a function declaration: %+v", sym)
		}
	}
	if len(want) > 0 {
		t.Fatalf("missing original Spotube globals: %v, found=%v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

// Apex accessor visibility modifies get/set, not the property's declaration name.
func TestApexPropertyWithPrivateSetterKeepsRealName(t *testing.T) {
	const original = "global class RemoteSiteHelperController {\n" +
		"  global static Exception LastMetadataAPIConnectionException { get; private set; }\n" +
		"  public String Host { get; set; }\n" +
		"  public void refresh() {}\n}\n"
	for _, tc := range []struct{ name, source string }{
		{"lf", original}, {"crlf", strings.ReplaceAll(original, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (ApexAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("Apex property analysis incomplete: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, want := range []struct {
				name string
				kind SymbolKind
				line int
			}{
				{"RemoteSiteHelperController", SymbolKindClass, 1},
				{"RemoteSiteHelperController.LastMetadataAPIConnectionException", SymbolKindProperty, 2},
				{"RemoteSiteHelperController.Host", SymbolKindProperty, 3},
				{"RemoteSiteHelperController.refresh", SymbolKindMethod, 4},
			} {
				s, ok := byName[want.name]
				if !ok || s.Kind != want.kind || s.NameRange.Start.Line != want.line {
					t.Errorf("symbol %s missing/misclassified: exists=%v got=%+v kind=%s line=%d", want.name, ok, s, want.kind, want.line)
				}
			}
			for _, falseName := range []string{"RemoteSiteHelperController.get", "RemoteSiteHelperController.set", "RemoteSiteHelperController.private"} {
				if s, ok := byName[falseName]; ok {
					t.Errorf("accessor token incorrectly indexed: %+v", s)
				}
			}
		})
	}
}

func TestApexAnnotatedTopLevelTypes(t *testing.T) {
	const source = `@NamespaceAccessible
@SuppressWarnings('PMD.ClassNamingConventions')
public interface fflib_Answer {
  Object answer(fflib_InvocationOnMock invocation);
}
@SuppressWarnings('PMD.CognitiveComplexity, PMD.ExcessivePublicCount')
public without sharing class LoggerCache {
  public static Object getOrganizationCache() { return null; }
}
class Sibling {}
`
	for _, tc := range []struct{ name, text string }{
		{"LF", source},
		{"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (ApexAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("incomplete Apex analysis: %+v", result.Analysis)
			}
			found := symbolsByQualifiedName(result.Analysis.Symbols)
			for qualified, kind := range map[string]SymbolKind{
				"fflib_Answer":                     SymbolKindInterface,
				"fflib_Answer.answer":              SymbolKindMethod,
				"LoggerCache":                      SymbolKindClass,
				"LoggerCache.getOrganizationCache": SymbolKindMethod,
				"Sibling":                          SymbolKindClass,
			} {
				if got, ok := found[qualified]; !ok || got.Kind != kind {
					t.Errorf("%s: got %+v exists=%v; want %s; symbols=%v", qualified, got, ok, kind, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			for _, falseName := range []string{"NamespaceAccessible", "SuppressWarnings", "LoggerCache.SuppressWarnings"} {
				if symbol, ok := found[falseName]; ok {
					t.Errorf("Apex annotation became a declaration: %+v", symbol)
				}
			}
		})
	}
}

func TestApexOriginalAnnotatedTopLevelTypes(t *testing.T) {
	for _, tc := range []struct {
		env, name string
		kind      SymbolKind
		line      int
		member    string
	}{
		{"SCRIPTHOLD_REAL_APEX_FFLIB_ANSWER", "fflib_Answer", SymbolKindInterface, 12, "fflib_Answer.answer"},
		{"SCRIPTHOLD_REAL_APEX_NEBULA_LOGGER_CACHE", "LoggerCache", SymbolKindClass, 11, "LoggerCache.getOrganizationCache"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("set " + tc.env + " to the pinned Apex original source")
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() > 1<<20 {
				t.Fatal("original Apex fixture exceeds 1 MiB")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			document := sourceDocumentForScanner(string(raw))
			document.Path = path
			result, err := (ApexAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 2048))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("original Apex incomplete: %+v", result.Analysis)
			}
			found := symbolsByQualifiedName(result.Analysis.Symbols)
			typeSym, ok := found[tc.name]
			if !ok || typeSym.Kind != tc.kind || typeSym.NameRange.Start.Line != tc.line {
				t.Errorf("%s type: %+v present=%v; expected %s at line %d; symbols=%v", tc.name, typeSym, ok, tc.kind, tc.line, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			if member, ok := found[tc.member]; !ok || member.Kind != SymbolKindMethod {
				t.Errorf("%s method missing or misclassified: %+v present=%v", tc.member, member, ok)
			}
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Name == "SuppressWarnings" || symbol.Name == "NamespaceAccessible" {
					t.Errorf("annotation indexed as declaration: %+v", symbol)
				}
			}
		})
	}
}

func TestBraceSharedStatementConsumption(t *testing.T) {
	tests := []struct {
		name     string
		analyzer SourceAnalyzer
		source   string
		want     map[string]SymbolKind
		absent   []string
	}{
		{
			name:     "D module and import are not variables",
			analyzer: DAnalyzer{},
			source:   "module demo.core;\nimport std.stdio;\nstruct Widget { int count; }\nint active;\n",
			want:     map[string]SymbolKind{"demo.core": SymbolKindModule, "demo.core.Widget": SymbolKindStruct, "demo.core.active": SymbolKindVariable},
			absent:   []string{"demo.core.core", "demo.core.stdio"},
		},
		{
			name:     "D unittest assertions are calls, not function declarations",
			analyzer: DAnalyzer{},
			source:   "module demo;\nint before() { return 1; }\nunittest {\n  assert(before() == 1);\n  assert(true);\n}\nvoid after() {}\n",
			want:     map[string]SymbolKind{"demo": SymbolKindModule, "demo.before": SymbolKindFunction, "demo.after": SymbolKindFunction},
			absent:   []string{"demo.assert", "demo.true", "demo.unittest"},
		},
		{
			name:     "D static assert is a compile-time check, not a function declaration",
			analyzer: DAnalyzer{},
			source:   "module demo;\nstatic assert(1 == 1, \"valid\");\nint usable() { return 1; }\n",
			want:     map[string]SymbolKind{"demo": SymbolKindModule, "demo.usable": SymbolKindFunction},
			absent:   []string{"demo.assert", "demo.static"},
		},
		{
			name:     "Solidity pragma does not become variable",
			analyzer: SolidityAnalyzer{},
			source:   "pragma solidity ^0.8.0;\nimport \"VaultBase.sol\";\ncontract Vault { uint amount; function pay() public {} }\n",
			want:     map[string]SymbolKind{"Vault": SymbolKindClass, "Vault.amount": SymbolKindField, "Vault.pay": SymbolKindMethod},
			absent:   []string{"solidity", "VaultBase", "VaultBase.sol"},
		},
		{
			name:     "Dart annotation preserves class and its next sibling",
			analyzer: DartAnalyzer{},
			source:   "@pragma(\"vm:entry-point\")\nfinal class Visible { int count = 1; }\nclass Sibling {}\n",
			want:     map[string]SymbolKind{"Visible": SymbolKindClass, "Visible.count": SymbolKindField, "Sibling": SymbolKindClass},
			absent:   []string{"pragma"},
		},
		{
			name:     "Apex annotation preserves member and its sibling",
			analyzer: ApexAnalyzer{},
			source:   "public class Ops {\n  @future(callout=true)\n  public static void dispatch(String url) {}\n  public static void next() {}\n}\n",
			want:     map[string]SymbolKind{"Ops": SymbolKindClass, "Ops.dispatch": SymbolKindMethod, "Ops.next": SymbolKindMethod},
			absent:   []string{"Ops.future"},
		},
		{
			name:     "Apex qualified annotation with nested arguments",
			analyzer: ApexAnalyzer{},
			source:   "public class Ops {\n  @TestVisible(mode=Factory.make(1))\n  public void verify() {}\n}\n",
			want:     map[string]SymbolKind{"Ops": SymbolKindClass, "Ops.verify": SymbolKindMethod},
			absent:   []string{"Ops.TestVisible", "Ops.make"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("unexpected incomplete analysis: %+v", result.Analysis)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for qualifiedName, kind := range tc.want {
				symbol, ok := got[qualifiedName]
				if !ok || symbol.Kind != kind {
					t.Errorf("symbol %q: present=%v, kind=%q; want %q", qualifiedName, ok, symbol.Kind, kind)
				}
			}
			for _, qualifiedName := range tc.absent {
				if symbol, exists := got[qualifiedName]; exists {
					t.Errorf("unexpected symbol %q: kind=%q", qualifiedName, symbol.Kind)
				}
			}
		})
	}
}
