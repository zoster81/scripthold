package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestRealWorldObjectiveCPPInlineInterfaceEndPreservesConditionalDirectives(t *testing.T) {
	text := "#if !TARGET_OS_MACCATALYST\n" +
		"@interface _LayoutController : UINavigationController @end\n" +
		"#endif\n" +
		"@interface ContentView : NSObject\n" +
		"- (void)run;\n" +
		"@end\n"
	result, err := (ObjectiveCPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Objective-C++ inline @end reported partial: %+v", result.Analysis)
	}
	for _, want := range []string{"_LayoutController", "ContentView", "ContentView.run"} {
		if !containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), want) {
			t.Fatalf("Objective-C++ inline @end missing %s: %v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestRealWorldObjectiveCPPConditionalsInsideInterfaceRemainBalanced(t *testing.T) {
	text := "#if FEATURE_ENABLED\n" +
		"@interface Bridge : NSObject<EnabledProtocol>\n" +
		"#else\n" +
		"@interface Bridge : NSObject<LegacyProtocol>\n" +
		"#endif\n" +
		"- (void)run;\n" +
		"@end\n"
	result, err := (ObjectiveCPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Objective-C++ conditional interface reported partial: %+v", result.Analysis)
	}
}

func TestRealWorldObjectiveCPPConditionalObjectiveCBranchesCanShareCloser(t *testing.T) {
	text := "@interface Config : NSObject\n" +
		"- (void)validate {\n" +
		"#ifdef FEATURE\n" +
		"  if (enabled) {\n" +
		"#else\n" +
		"  if (fallback) {\n" +
		"#endif\n" +
		"    value = @\"ok\";\n" +
		"  }\n" +
		"}\n" +
		"@end\n"
	result, err := (ObjectiveCPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Objective-C++ conditional Objective-C branches sharing a closer reported partial: %+v", result.Analysis)
	}
	if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)["Config.validate"]; !ok {
		t.Fatalf("Objective-C method around conditional shared closer missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRealWorldObjectiveCPPProtocolForwardDeclarationDoesNotMaskCPPCloser(t *testing.T) {
	text := "#ifdef __cplusplus\n" +
		"extern \"C\" {\n" +
		"#endif\n" +
		"@protocol First;\n" +
		"@protocol Second;\n" +
		"#ifdef __cplusplus\n" +
		"}\n" +
		"#endif\n" +
		"int after(void);\n"
	result, err := (ObjectiveCPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Objective-C++ forward @protocol declaration masked C++ closer: %+v", result.Analysis)
	}
	if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)["after"]; !ok {
		t.Fatalf("C++ declaration after Objective-C forwards missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRealWorldObjectiveCPPRawNSStringBodyStaysOpaque(t *testing.T) {
	text := "static NSString *Source = @R\"(\n" +
		"kernel void fake(device uint *dst [[buffer(0)]]) {\n" +
		"  dst[0] = 1;\n" +
		"}\n" +
		")\";\n" +
		"@interface Bridge : NSObject\n" +
		"- (void)run;\n" +
		"@end\n"
	result, err := (ObjectiveCPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Objective-C++ raw NSString body reported partial: %+v", result.Analysis)
	}
	if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)["Bridge.run"]; !ok {
		t.Fatalf("declaration after Objective-C++ raw NSString missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Name == "fake" {
			t.Fatalf("raw NSString body leaked shader declaration: %+v", symbol)
		}
	}
}

func TestRealWorldObjectiveCPPCompositeMergePreservesSourceOrder(t *testing.T) {
	text := "int Early = 1;\n" +
		"@interface Bridge : NSObject\n" +
		"- (void)run;\n" +
		"@end\n" +
		"int Late = 2;\n"
	result, err := (ObjectiveCPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Early", "Bridge", "Bridge.run", "Late"}
	if len(result.Analysis.Symbols) != len(want) {
		t.Fatalf("symbols=%v want=%v", sortedSymbolQualifiedNames(result.Analysis.Symbols), want)
	}
	for index, name := range want {
		if result.Analysis.Symbols[index].QualifiedName != name {
			t.Fatalf("symbol order at %d = %q want %q; symbols=%v", index, result.Analysis.Symbols[index].QualifiedName, name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestMQLAdapterResolvesDialectPreprocessorBranches(t *testing.T) {
	text := "class Store {\npublic:\n" +
		"#ifndef __MQL4__\n" +
		"Store(int flags = 5) {\n" +
		"#else\n" +
		"Store(int flags = 4) {\n" +
		"#endif\n" +
		"value = flags;\n" +
		"}\n" +
		"void After() {}\n" +
		"int value;\n" +
		"};\n"
	for _, tc := range []struct {
		name          string
		analyzer      SourceAnalyzer
		wantSignature string
	}{
		{name: "mql4", analyzer: MQL4Analyzer{}, wantSignature: "flags = 4"},
		{name: "mql5", analyzer: MQL5Analyzer{}, wantSignature: "flags = 5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Fatalf("known MQL dialect branch should be structurally complete: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			constructor, ok := byName["Store.Store"]
			if !ok || constructor.Kind != SymbolKindConstructor || !strings.Contains(constructor.Signature, tc.wantSignature) {
				t.Fatalf("dialect constructor = %+v exists=%v want signature containing %q; symbols=%v", constructor, ok, tc.wantSignature, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			if after, ok := byName["Store.After"]; !ok || after.Kind != SymbolKindMethod {
				t.Fatalf("method after dialect conditional = %+v exists=%v; symbols=%v", after, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}

	for _, tc := range []struct {
		name          string
		macro         string
		wantSignature string
	}{
		{name: "generic mql macro is defined", macro: "__MQL__", wantSignature: "flags = 6"},
		{name: "cplusplus macro is not defined", macro: "__cplusplus", wantSignature: "flags = 7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "class Store {\npublic:\n" +
				"#ifdef " + tc.macro + "\n" +
				"Store(int flags = 6) {\n" +
				"#else\n" +
				"Store(int flags = 7) {\n" +
				"#endif\n" +
				"value = flags;\n" +
				"}\n};\n"
			for _, analyzer := range []SourceAnalyzer{MQL4Analyzer{}, MQL5Analyzer{}} {
				result, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				if !result.Analysis.CoverageComplete {
					t.Fatalf("known %s branch should be structurally complete for %s: %+v", tc.macro, analyzer.Language(), result.Analysis)
				}
				constructor, ok := symbolsByQualifiedName(result.Analysis.Symbols)["Store.Store"]
				if !ok || !strings.Contains(constructor.Signature, tc.wantSignature) {
					t.Fatalf("%s constructor = %+v exists=%v want signature containing %q", analyzer.Language(), constructor, ok, tc.wantSignature)
				}
			}
		})
	}

	t.Run("unknown macro remains fail closed across shared delimiter", func(t *testing.T) {
		text := "class Store {\npublic:\n" +
			"#ifdef FEATURE\n" +
			"Store(int flags = 1) {\n" +
			"#else\n" +
			"Store(int flags = 2) {\n" +
			"#endif\n" +
			"value = flags;\n" +
			"}\n};\n"
		result, err := (MQL5Analyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.CoverageComplete || !hasAnalysisDiagnostic(result.Analysis.Diagnostics, "mql5-conditional-preprocessor") {
			t.Fatalf("unknown macro crossing a shared delimiter must remain fail closed: %+v", result.Analysis)
		}
	})
}

func TestMQLAdapterPreservesUnderlyingPreprocessorCoverageSemantics(t *testing.T) {
	t.Run("balanced header guard remains complete", func(t *testing.T) {
		text := "#ifndef SAMPLE_MQH\n#define SAMPLE_MQH\nclass Guarded { public: void Run() {} };\n#endif\n"
		result, err := (MQL5Analyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || !hasAnalysisDiagnostic(result.Analysis.Diagnostics, "mql5-conditional-preprocessor") {
			t.Fatalf("MQL5 balanced header guard should preserve structural coverage: %+v", result.Analysis)
		}
		conditionalDiagnostics := 0
		for _, diagnostic := range result.Analysis.Diagnostics {
			if diagnostic.Code == "mql5-conditional-preprocessor" {
				conditionalDiagnostics++
			}
			if strings.HasPrefix(diagnostic.Code, "cpp-") {
				t.Fatalf("C++ diagnostic leaked through MQL adapter: %+v", result.Analysis.Diagnostics)
			}
		}
		if conditionalDiagnostics != 1 {
			t.Fatalf("MQL5 conditional diagnostics=%d want 1: %+v", conditionalDiagnostics, result.Analysis.Diagnostics)
		}
	})

	t.Run("balanced feature conditional remains complete", func(t *testing.T) {
		text := "#ifdef DEBUG\nvoid Trace() {}\n#endif\nvoid Always() {}\n"
		result, err := (MQL5Analyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || !hasAnalysisDiagnostic(result.Analysis.Diagnostics, "mql5-conditional-preprocessor") {
			t.Fatalf("MQL5 balanced feature conditional should preserve structural coverage: %+v", result.Analysis)
		}
	})

	t.Run("malformed conditional remains incomplete", func(t *testing.T) {
		text := "#if ENABLE_FEATURE\nclass Conditional { };\n"
		result, err := (MQL4Analyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.CoverageComplete || !hasAnalysisDiagnostic(result.Analysis.Diagnostics, "mql4-malformed-conditional-preprocessor") {
			t.Fatalf("MQL4 malformed conditional should fail closed: %+v", result.Analysis)
		}
	})
}

func TestMQLInputVariableKindsRemainDistinct(t *testing.T) {
	text := "input int Period = 14;\n" +
		"sinput double Risk = 1.0;\n" +
		"extern bool Enabled = true;\n"
	result, err := (MQL4Analyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for name, want := range map[string]struct {
		nativeKind string
		modifier   string
	}{
		"Period":  {nativeKind: "input-variable", modifier: "input"},
		"Risk":    {nativeKind: "static-input-variable", modifier: "sinput"},
		"Enabled": {nativeKind: "extern-variable", modifier: "extern"},
	} {
		symbol, ok := byName[name]
		if !ok {
			t.Fatalf("missing MQL input variable %q; symbols=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
		if symbol.NativeKind != want.nativeKind || !containsString(symbol.Modifiers, want.modifier) {
			t.Fatalf("%s = nativeKind %q modifiers=%v, want %q + %q", name, symbol.NativeKind, symbol.Modifiers, want.nativeKind, want.modifier)
		}
	}
}

func TestMQL5InputGroupDeclarationsAreNotSymbols(t *testing.T) {
	text := "input group \"Risk\"\n" +
		"input double Lots = 0.10;\n" +
		"input group \"Execution\";\n" +
		"input int Slippage = 5;\n" +
		"input int group = 7;\n"
	result, err := (MQL5Analyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"Lots", "Slippage", "group"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("missing MQL5 input %q; symbols=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	for name, wantLine := range map[string]int{"Lots": 2, "Slippage": 4, "group": 5} {
		symbol := byName[name]
		if symbol.NativeKind != "input-variable" || symbol.DeclarationRange.Start.Line != wantLine {
			t.Fatalf("%s input projection = nativeKind %q line %d, want input-variable line %d", name, symbol.NativeKind, symbol.DeclarationRange.Start.Line, wantLine)
		}
	}
	groupCount := 0
	for _, symbol := range result.Analysis.Symbols {
		if symbol.QualifiedName == "group" {
			groupCount++
		}
	}
	if groupCount != 1 {
		t.Fatalf("input group declarations leaked as symbols: count=%d symbols=%v", groupCount, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestMQLTemplateInterfaceRetainsNativeKindAndInheritance(t *testing.T) {
	text := "template<typename TKey, typename TValue>\n" +
		"interface IMap : public ICollection<CKeyValuePair<TKey,TValue>*>\n" +
		"{\n" +
		"   TValue Get(TKey key);\n" +
		"};\n"
	result, err := (MQL5Analyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	if got, ok := byName["IMap"]; !ok || got.Kind != SymbolKindInterface || got.NativeKind != "interface" {
		t.Fatalf("templated MQL5 interface = %+v exists=%v; symbols=%v", got, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if got, ok := byName["IMap.Get"]; !ok || got.Kind != SymbolKindMethod {
		t.Fatalf("templated MQL5 interface method = %+v exists=%v; symbols=%v", got, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if !hasStructuralRelation(result.Relations, "inherits", "IMap", "ICollection<CKeyValuePair<TKey,TValue>*>") {
		t.Fatalf("templated MQL5 interface inheritance missing: %+v", result.Relations)
	}
}

func TestMQLInterfacesRetainNativeHierarchy(t *testing.T) {
	text := "interface Worker\n" +
		"{\n" +
		"   void Run();\n" +
		"   double Value(int index);\n" +
		"};\n" +
		"class Strategy : public Worker\n" +
		"{\n" +
		"public:\n" +
		"   void Run() {}\n" +
		"};\n"

	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{name: "mql4", analyzer: MQL4Analyzer{}},
		{name: "mql5", analyzer: MQL5Analyzer{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("%s interface analysis unexpectedly partial: %+v", tc.name, result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for qualified, kind := range map[string]SymbolKind{
				"Worker": SymbolKindInterface, "Worker.Run": SymbolKindMethod, "Worker.Value": SymbolKindMethod,
				"Strategy": SymbolKindClass, "Strategy.Run": SymbolKindMethod,
			} {
				if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind {
					t.Fatalf("%s %s = %+v exists=%v; symbols=%v", tc.name, qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			if _, ok := byName["Run"]; ok {
				t.Fatalf("%s interface method leaked top-level: %v", tc.name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			if _, ok := byName["Value"]; ok {
				t.Fatalf("%s interface method leaked top-level: %v", tc.name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			if !hasStructuralRelation(result.Relations, "inherits", "Strategy", "Worker") {
				t.Fatalf("%s interface inheritance relation missing: %+v", tc.name, result.Relations)
			}
		})
	}
}

func TestTradingAndSpecialtyAnalyzersExposeDistinctNativeStructure(t *testing.T) {
	tests := []struct {
		language string
		analyzer SourceAnalyzer
		text     string
		want     map[string]SymbolKind
		deps     []string
	}{
		{
			language: "mql4", analyzer: MQL4Analyzer{},
			text: "#include <stdlib.mqh>\n#import \"user32.dll\"\ninput int Period = 14;\nclass LegacyEA { public: void Run() {} };\nint OnInit() { return 0; }\n",
			want: map[string]SymbolKind{"Period": SymbolKindVariable, "LegacyEA": SymbolKindClass, "LegacyEA.Run": SymbolKindMethod, "OnInit": SymbolKindFunction},
			deps: []string{"stdlib.mqh", "user32.dll"},
		},
		{
			language: "mql5", analyzer: MQL5Analyzer{},
			text: "#include <Trade/Trade.mqh>\ninput double Lots = 0.10;\nenum Mode { Fast, Safe };\nclass Strategy : public BaseStrategy { public: void Run() {} };\nvoid OnTick() {}\n",
			want: map[string]SymbolKind{"Lots": SymbolKindVariable, "Mode": SymbolKindEnum, "Strategy": SymbolKindClass, "Strategy.Run": SymbolKindMethod, "OnTick": SymbolKindFunction},
			deps: []string{"Trade/Trade.mqh"},
		},
		{
			language: "objective-c", analyzer: ObjectiveCAnalyzer{},
			text: "#import <Foundation/Foundation.h>\n@protocol Worker\n- (void)run;\n@end\n@interface Service : NSObject <Worker>\n@property(nonatomic, copy) NSString *title;\n- (instancetype)initWithTitle:(NSString *)title;\n- (void)run;\n@end\n@implementation Service\n- (void)run {}\n@end\n",
			want: map[string]SymbolKind{"Worker": SymbolKindInterface, "Worker.run": SymbolKindMethod, "Service": SymbolKindClass, "Service.title": SymbolKindProperty, "Service.initWithTitle:": SymbolKindConstructor, "Service.run": SymbolKindMethod},
			deps: []string{"Foundation/Foundation.h"},
		},
		{
			language: "objective-cpp", analyzer: ObjectiveCPPAnalyzer{},
			text: "#import <Foundation/Foundation.h>\n@interface Bridge : NSObject\n- (void)run;\n@end\nclass CppHelper { public: void Execute() {} };\n",
			want: map[string]SymbolKind{"Bridge": SymbolKindClass, "Bridge.run": SymbolKindMethod, "CppHelper": SymbolKindClass, "CppHelper.Execute": SymbolKindMethod},
			deps: []string{"Foundation/Foundation.h"},
		},
		{
			language: "dart", analyzer: DartAnalyzer{},
			text: "import 'dart:async';\nmixin Loggable { void log() {} }\nclass Service extends Base with Loggable implements Worker { final int value; Service(this.value); void run() {} }\nvoid top() {}\n",
			want: map[string]SymbolKind{"Loggable": SymbolKindTrait, "Loggable.log": SymbolKindMethod, "Service": SymbolKindClass, "Service.value": SymbolKindField, "Service.Service": SymbolKindConstructor, "Service.run": SymbolKindMethod, "top": SymbolKindFunction},
			deps: []string{"dart:async"},
		},
		{
			language: "d", analyzer: DAnalyzer{},
			text: "module demo.service;\nimport std.stdio;\ninterface Worker { void run(); }\nclass Service : Worker { int value; this() {} void run() {} }\nstruct Point { int x; }\nvoid top() {}\n",
			want: map[string]SymbolKind{"demo.service": SymbolKindModule, "demo.service.Worker": SymbolKindInterface, "demo.service.Worker.run": SymbolKindMethod, "demo.service.Service": SymbolKindClass, "demo.service.Service.value": SymbolKindField, "demo.service.Service.run": SymbolKindMethod, "demo.service.Point": SymbolKindStruct, "demo.service.top": SymbolKindFunction},
			deps: []string{"std.stdio"},
		},
		{
			language: "zig", analyzer: ZigAnalyzer{},
			text: "const std = @import(\"std\");\npub const Point = struct { x: i32, pub fn init() Point { return .{ .x = 0 }; } };\npub fn top() void {}\nconst Answer = 42;\n",
			want: map[string]SymbolKind{"Point": SymbolKindStruct, "Point.init": SymbolKindMethod, "top": SymbolKindFunction, "Answer": SymbolKindConstant},
			deps: []string{"std"},
		},
		{
			language: "nim", analyzer: NimAnalyzer{},
			text: "import strutils\ntype\n  Service* = ref object of RootObj\n    value*: int\n  Mode* = enum\n    Fast, Safe\nproc run*(self: Service) = discard\nfunc top*(x: int): int = x\nconst Answer* = 42\n",
			want: map[string]SymbolKind{"Service": SymbolKindClass, "Service.value": SymbolKindField, "Mode": SymbolKindEnum, "run": SymbolKindFunction, "top": SymbolKindFunction, "Answer": SymbolKindConstant},
			deps: []string{"strutils"},
		},
		{
			language: "solidity", analyzer: SolidityAnalyzer{},
			text: "pragma solidity ^0.8.20;\nimport \"./Base.sol\";\ninterface IWorker { function run() external; }\ncontract Service is Base, IWorker { event Updated(uint value); uint public value; constructor() {} function run() external override {} modifier onlyOwner() { _; } }\nlibrary Math { function add(uint a, uint b) internal pure returns (uint) { return a + b; } }\n",
			want: map[string]SymbolKind{"IWorker": SymbolKindInterface, "IWorker.run": SymbolKindMethod, "Service": SymbolKindClass, "Service.Updated": SymbolKindEvent, "Service.value": SymbolKindField, "Service.Service": SymbolKindConstructor, "Service.run": SymbolKindMethod, "Service.onlyOwner": SymbolKindMethod, "Math": SymbolKindModule, "Math.add": SymbolKindMethod},
			deps: []string{"./Base.sol"},
		},
		{
			language: "apex", analyzer: ApexAnalyzer{},
			text: "public interface Worker { void run(); }\npublic with sharing class Service extends Base implements Worker { public Integer value; public Service() {} public void run() {} }\npublic enum Mode { Fast, Safe }\ntrigger AccountTrigger on Account (before insert) { }\n",
			want: map[string]SymbolKind{"Worker": SymbolKindInterface, "Worker.run": SymbolKindMethod, "Service": SymbolKindClass, "Service.value": SymbolKindField, "Service.Service": SymbolKindConstructor, "Service.run": SymbolKindMethod, "Mode": SymbolKindEnum, "AccountTrigger": SymbolKindFunction},
		},
		{
			language: "al", analyzer: ALAnalyzer{},
			text: "namespace Contoso.App;\nusing Microsoft.Sales.Customer;\ncodeunit 50100 MyCodeunit { procedure Run() begin end; local procedure Helper() begin end; }\npageextension 50101 MyPage extends \"Customer Card\" { }\n",
			want: map[string]SymbolKind{"Contoso.App": SymbolKindNamespace, "Contoso.App.MyCodeunit": SymbolKindModule, "Contoso.App.MyCodeunit.Run": SymbolKindMethod, "Contoso.App.MyCodeunit.Helper": SymbolKindMethod, "Contoso.App.MyPage": SymbolKindType},
			deps: []string{"Microsoft.Sales.Customer"},
		},
		{
			language: "arduino", analyzer: ArduinoAnalyzer{},
			text: "#include <Arduino.h>\nclass Device { public: void Run() {} };\nvoid setup() {}\nvoid loop() {}\n",
			want: map[string]SymbolKind{"Device": SymbolKindClass, "Device.Run": SymbolKindMethod, "setup": SymbolKindFunction, "loop": SymbolKindFunction},
			deps: []string{"Arduino.h"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("%s analysis partial: %+v", tc.language, result.Analysis)
			}
			if tc.analyzer.Language() != tc.language {
				t.Fatalf("language=%q want %q", tc.analyzer.Language(), tc.language)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range tc.want {
				if symbol, ok := byName[name]; !ok || symbol.Kind != kind {
					t.Fatalf("%s missing %s kind=%s; symbol=%+v exists=%v all=%v", tc.language, name, kind, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			if tc.deps != nil {
				if got := dependencyValues(result.Dependencies); !sameStringSet(got, tc.deps) {
					t.Fatalf("%s dependencies=%v want=%v", tc.language, got, tc.deps)
				}
			}
		})
	}
}

func TestDetectorKeepsObjectiveCMExtensionAmbiguousWithoutContentEvidence(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ambiguous, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "model.m", Text: "function y = model(x)\ny = x + 1;\nend\n"})
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous.State != DetectionAmbiguous || ambiguous.Language != "" {
		t.Fatalf("plain .m detection=%+v, want ambiguous Objective-C/MATLAB/Octave", ambiguous)
	}
	objc, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "Service.m", Text: "#import <Foundation/Foundation.h>\n@interface Service : NSObject\n@end\n"})
	if err != nil {
		t.Fatal(err)
	}
	if objc.State != DetectionProbable || objc.Language != "objective-c" {
		t.Fatalf("Objective-C .m detection=%+v", objc)
	}
}
