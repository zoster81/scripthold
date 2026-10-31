package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

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
