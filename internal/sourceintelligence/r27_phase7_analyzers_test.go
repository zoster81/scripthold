package sourceintelligence

import (
	"context"
	"testing"
)

func TestR27RealWorldObjectiveCPPCompositeMergePreservesSourceOrder(t *testing.T) {
	text := "int Early = 1;\n" +
		"@interface Bridge : NSObject\n" +
		"- (void)run;\n" +
		"@end\n" +
		"int Late = 2;\n"
	result, err := (ObjectiveCPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 64))
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

func TestR27Phase7TradingAndSpecialtyAnalyzersExposeDistinctNativeStructure(t *testing.T) {
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
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), phase3AnalyzeOptions(true, 512))
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

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]int, len(got))
	for _, value := range got {
		seen[value]++
	}
	for _, value := range want {
		if seen[value] == 0 {
			return false
		}
		seen[value]--
	}
	return true
}

func TestR27Phase7DetectorKeepsObjectiveCMExtensionAmbiguousWithoutContentEvidence(t *testing.T) {
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
