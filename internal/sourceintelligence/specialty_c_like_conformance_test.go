package sourceintelligence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestSpecialtyCLikeConformanceAcrossEncodingsAndDeterminism(t *testing.T) {
	tests := []struct {
		name, language, extension, encoding, text string
		bom                                       bool
		want                                      []string
	}{
		{name: "mql4-windows1252-crlf", language: "mql4", extension: ".mq4", encoding: "windows-1252", text: "// café\r\ninput int Period = 14;\r\nint OnInit() { return 0; }\r\n", want: []string{"Period", "OnInit"}},
		{name: "mql5-utf16le", language: "mql5", extension: ".mq5", encoding: "utf-16le", bom: true, text: "input double Lots = 0.1;\r\nvoid OnTick() {}\r\n", want: []string{"Lots", "OnTick"}},
		{name: "objective-c-utf16le", language: "objective-c", extension: ".m", encoding: "utf-16le", bom: true, text: "// résumé\n@interface Service : NSObject\n- (void)run;\n@end\n", want: []string{"Service", "Service.run"}},
		{name: "objective-cpp-utf16be", language: "objective-cpp", extension: ".mm", encoding: "utf-16be", bom: true, text: "@interface Bridge : NSObject\n- (void)run;\n@end\nclass Helper { public: void Execute() {} };\n", want: []string{"Bridge", "Bridge.run", "Helper", "Helper.Execute"}},
		{name: "dart-utf32le", language: "dart", extension: ".dart", encoding: "utf-32le", bom: true, text: "// café\nclass Service { void run() {} }\nvoid top() {}\n", want: []string{"Service", "Service.run", "top"}},
		{name: "d-windows1252", language: "d", extension: ".d", encoding: "windows-1252", text: "module demo;\n// café\nclass Service { void run() {} }\n", want: []string{"demo", "demo.Service", "demo.Service.run"}},
		{name: "zig-utf16le", language: "zig", extension: ".zig", encoding: "utf-16le", bom: true, text: "// résumé\npub const Point = struct { x: i32, pub fn init() Point { return .{ .x = 0 }; } };\n", want: []string{"Point", "Point.x", "Point.init"}},
		{name: "nim-windows1252-crlf", language: "nim", extension: ".nim", encoding: "windows-1252", text: "# café\r\nproc run*(x: int) = discard\r\nconst Answer* = 42\r\n", want: []string{"run", "Answer"}},
		{name: "solidity-utf16be", language: "solidity", extension: ".sol", encoding: "utf-16be", bom: true, text: "// résumé\ncontract Service { function run() external {} }\n", want: []string{"Service", "Service.run"}},
		{name: "apex-utf16le", language: "apex", extension: ".cls", encoding: "utf-16le", bom: true, text: "// café\npublic class Service { public void run() {} }\n", want: []string{"Service", "Service.run"}},
		{name: "al-windows1252-crlf", language: "al", extension: ".al", encoding: "windows-1252", text: "// café\r\nnamespace Contoso.App;\r\ncodeunit 50100 Worker { procedure Run() begin end; }\r\n", want: []string{"Contoso.App", "Contoso.App.Worker", "Contoso.App.Worker.Run"}},
		{name: "arduino-utf16le", language: "arduino", extension: ".ino", encoding: "utf-16le", bom: true, text: "// résumé\nvoid setup() {}\nvoid loop() {}\n", want: []string{"setup", "loop"}},
	}
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture"+tc.extension)
			if err := os.WriteFile(path, encodeSourceFixture(t, tc.encoding, tc.text, tc.bom), 0o600); err != nil {
				t.Fatal(err)
			}
			document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{RequestedEncoding: tc.encoding, MaxFileBytes: 4 * 1024 * 1024, MaxDecodedCharacters: 1_000_000})
			if err != nil {
				t.Fatal(err)
			}
			descriptor, _ := registry.Resolve(tc.language)
			analyzer, ok := AnalyzerFor(descriptor)
			if !ok {
				t.Fatalf("missing analyzer %s", tc.language)
			}
			first, err := analyzer.Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			second, err := analyzer.Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("%s output is nondeterministic", tc.language)
			}
			if !first.Analysis.CoverageComplete || first.Analysis.Truncated {
				t.Fatalf("%s conformance partial: %+v", tc.language, first.Analysis)
			}
			names := sortedSymbolQualifiedNames(first.Analysis.Symbols)
			for _, want := range tc.want {
				if !containsSortedString(names, want) {
					t.Fatalf("%s missing %s; symbols=%v", tc.language, want, names)
				}
			}
		})
	}
}

func TestALQuotedObjectNamesRemainStructural(t *testing.T) {
	text := "namespace Contoso.App;\n" +
		"pageextension 50104 \"Customer Card Ext\" extends \"Customer Card\"\n" +
		"{\n" +
		"    trigger OnOpenPage()\n" +
		"    begin\n" +
		"    end;\n" +
		"}\n"
	result, err := (ALAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("quoted AL object analysis incomplete: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"Contoso.App.Customer Card Ext", "Contoso.App.Customer Card Ext.OnOpenPage"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("quoted AL object missing %q; symbols=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	if len(result.Relations) != 1 || result.Relations[0].Kind != "extends" || result.Relations[0].Source != "Contoso.App.Customer Card Ext" || result.Relations[0].Target != "Customer Card" {
		t.Fatalf("quoted AL extension relation=%+v", result.Relations)
	}
}

func TestALConditionalBranchesPreserveStructureAndDeclarations(t *testing.T) {
	t.Run("shared structural closings", func(t *testing.T) {
		text := "namespace Contoso.App;\n" +
			"pageextension 50104 CustomerCardExt extends \"Customer Card\"\n" +
			"{\n" +
			"    layout\n" +
			"    {\n" +
			"#if FEATURE\n" +
			"        addlast(General)\n" +
			"        {\n" +
			"            group(FeatureGroup)\n" +
			"            {\n" +
			"#else\n" +
			"        addlast(General)\n" +
			"        {\n" +
			"            group(FallbackGroup)\n" +
			"            {\n" +
			"#endif\n" +
			"                field(SharedField; Rec.SharedField)\n" +
			"                {\n" +
			"                    ApplicationArea = All;\n" +
			"                }\n" +
			"            }\n" +
			"        }\n" +
			"    }\n" +
			"    trigger OnOpenPage()\n" +
			"    begin\n" +
			"    end;\n" +
			"}\n"
		options := testAnalyzeOptions(true, 128)
		options.MaxNesting = 5
		result, err := (ALAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), options)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
			t.Fatalf("conditional AL structure incomplete: %+v", result.Analysis)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		for _, name := range []string{"Contoso.App.CustomerCardExt", "Contoso.App.CustomerCardExt.OnOpenPage"} {
			if _, ok := byName[name]; !ok {
				t.Fatalf("conditional AL structure missing %q; symbols=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
	})

	t.Run("all source branches remain visible", func(t *testing.T) {
		text := "namespace Contoso.App;\n" +
			"#if FEATURE\n" +
			"codeunit 50110 FeatureUnit { procedure FeatureOnly() begin end; }\n" +
			"#elif LEGACY\n" +
			"codeunit 50111 LegacyUnit { procedure LegacyOnly() begin end; }\n" +
			"#else\n" +
			"codeunit 50112 FallbackUnit { procedure FallbackOnly() begin end; }\n" +
			"#endif\n"
		result, err := (ALAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 128))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
			t.Fatalf("conditional AL branch union incomplete: %+v", result.Analysis)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		for _, name := range []string{
			"Contoso.App.FeatureUnit", "Contoso.App.FeatureUnit.FeatureOnly",
			"Contoso.App.LegacyUnit", "Contoso.App.LegacyUnit.LegacyOnly",
			"Contoso.App.FallbackUnit", "Contoso.App.FallbackUnit.FallbackOnly",
		} {
			if _, ok := byName[name]; !ok {
				t.Fatalf("conditional AL branch union missing %q; symbols=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
	})

	t.Run("nested branches remain visible", func(t *testing.T) {
		text := "namespace Contoso.App;\n" +
			"#if OUTER\n" +
			"codeunit 50120 OuterUnit { procedure OuterOnly() begin end; }\n" +
			"#if INNER\n" +
			"codeunit 50121 InnerUnit { procedure InnerOnly() begin end; }\n" +
			"#else\n" +
			"codeunit 50122 InnerFallbackUnit { procedure InnerFallbackOnly() begin end; }\n" +
			"#endif\n" +
			"#else\n" +
			"codeunit 50123 OuterFallbackUnit { procedure OuterFallbackOnly() begin end; }\n" +
			"#endif\n"
		result, err := (ALAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 128))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
			t.Fatalf("nested conditional AL analysis incomplete: %+v", result.Analysis)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		for _, name := range []string{
			"Contoso.App.OuterUnit", "Contoso.App.OuterUnit.OuterOnly",
			"Contoso.App.InnerUnit", "Contoso.App.InnerUnit.InnerOnly",
			"Contoso.App.InnerFallbackUnit", "Contoso.App.InnerFallbackUnit.InnerFallbackOnly",
			"Contoso.App.OuterFallbackUnit", "Contoso.App.OuterFallbackUnit.OuterFallbackOnly",
		} {
			if _, ok := byName[name]; !ok {
				t.Fatalf("nested conditional AL analysis missing %q; symbols=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
	})

	t.Run("malformed directives fail closed", func(t *testing.T) {
		text := "namespace Contoso.App;\n#if FEATURE\ncodeunit 50130 BrokenConditional { }\n"
		result, err := (ALAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
			t.Fatalf("unterminated AL conditional was overclaimed: %+v", result.Analysis)
		}
		found := false
		for _, diagnostic := range result.Analysis.Diagnostics {
			if diagnostic.Code == "al-conditional-directive" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("unterminated AL conditional missing explicit diagnostic: %+v", result.Analysis.Diagnostics)
		}
	})
}

func TestALConditionalGroupsShareGlobalSymbolState(t *testing.T) {
	text := "namespace Contoso.App;\n" +
		"report 50150 DemoReport\n" +
		"{\n" +
		"    dataset\n" +
		"    {\n" +
		"#if not CLEAN28\n" +
		"        dataitem(LegacyContainer; Integer)\n" +
		"        {\n" +
		"#endif\n" +
		"            dataitem(SharedItem; Integer)\n" +
		"            {\n" +
		"            }\n" +
		"#if not CLEAN28\n" +
		"        }\n" +
		"#else\n" +
		"        dataitem(CleanOnly; Integer)\n" +
		"        {\n" +
		"        }\n" +
		"#endif\n" +
		"    }\n" +
		"}\n"
	result, err := (ALAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("repeated AL conditional symbol produced impossible structural variant: %+v", result.Analysis)
	}
}

func TestALFieldNamesFollowTheDeclaredNameSlot(t *testing.T) {
	text := "namespace Contoso.App;\n" +
		"page 50100 \"Demo Page\"\n" +
		"{\n" +
		"    layout\n" +
		"    {\n" +
		"        area(content)\n" +
		"        {\n" +
		"            field(Space; ' ') { ApplicationArea = All; }\n" +
		"            field(\"Customer Name\"; Rec.Name) { ApplicationArea = All; }\n" +
		"        }\n" +
		"    }\n" +
		"}\n" +
		"table 50101 \"Demo Table\"\n" +
		"{\n" +
		"    fields\n" +
		"    {\n" +
		"        field(1; \"Customer Name\"; Text[100]) { }\n" +
		"    }\n" +
		"}\n"
	result, err := (ALAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid AL field declarations were incomplete: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{
		"Contoso.App.Demo Page.Space",
		"Contoso.App.Demo Page.Customer Name",
		"Contoso.App.Demo Table.Customer Name",
	} {
		symbol, ok := byName[name]
		if !ok || symbol.Kind != SymbolKindField {
			t.Fatalf("AL field missing %q; symbols=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	for _, wrong := range []string{"Contoso.App.Demo Page.Rec", "Contoso.App.Demo Page.Name"} {
		if _, ok := byName[wrong]; ok {
			t.Fatalf("AL field source expression leaked as declaration %q", wrong)
		}
	}
}

func TestALConditionalPathPreservesErrorKinds(t *testing.T) {
	text := "#if FEATURE\ncodeunit 50140 ConditionalUnit { }\n#endif\n"

	invalid := testAnalyzeOptions(true, 64)
	invalid.Limits.MaxSymbols = 0
	if _, err := (ALAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), invalid); operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("conditional AL invalid options kind=%v err=%v", operation.KindOf(err), err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (ALAnalyzer{}).Analyze(ctx, sourceDocumentForScanner(text), testAnalyzeOptions(true, 64)); operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("conditional AL cancellation kind=%v err=%v", operation.KindOf(err), err)
	}
}

func TestALConditionalExpressionParserUsesDocumentedOperators(t *testing.T) {
	expression, err := alConditionalDirectiveExpression("#if not CLEAN28 and (FEATURE or FALLBACK) // comment", "#if")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		assignment map[string]bool
		want       bool
	}{
		{name: "feature", assignment: map[string]bool{"clean28": false, "feature": true, "fallback": false}, want: true},
		{name: "fallback", assignment: map[string]bool{"clean28": false, "feature": false, "fallback": true}, want: true},
		{name: "neither", assignment: map[string]bool{"clean28": false, "feature": false, "fallback": false}, want: false},
		{name: "clean", assignment: map[string]bool{"clean28": true, "feature": true, "fallback": true}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := alConditionalEvaluate(expression, tc.assignment) == alConditionalTrue
			if got != tc.want {
				t.Fatalf("conditional expression = %v want %v", got, tc.want)
			}
		})
	}
	if _, err := alConditionalDirectiveExpression("#if FEATURE xor OTHER", "#if"); err == nil {
		t.Fatal("unsupported AL conditional operator was accepted")
	}
}

func TestALConditionalVariantPlanningIsBoundedAndFailClosed(t *testing.T) {
	condition := func(index int) *alConditionalExpr {
		return &alConditionalExpr{kind: alConditionalExprSymbol, symbol: fmt.Sprintf("feature_%d", index)}
	}
	independent := make([]alConditionalGroup, 100)
	for index := range independent {
		independent[index] = alConditionalGroup{
			openOffset:   index,
			parentGroup:  -1,
			parentBranch: -1,
			branches: []alConditionalBranch{
				{start: index * 2, end: index*2 + 1, condition: condition(index)},
				{start: index*2 + 1, end: index*2 + 2, elseBranch: true},
			},
		}
	}
	variants, issue := alConditionalVariantSelections(independent)
	if issue != nil {
		t.Fatalf("independent conditional groups unexpectedly exceeded variant budget: %+v", issue)
	}
	if len(variants) != 2 {
		t.Fatalf("independent conditional groups produced %d variants, want 2", len(variants))
	}

	nested := make([]alConditionalGroup, alConditionalVariantLimit)
	for index := range nested {
		parent := index - 1
		nested[index] = alConditionalGroup{
			openOffset:   index,
			parentGroup:  parent,
			parentBranch: 0,
			branches: []alConditionalBranch{
				{start: index * 2, end: index*2 + 1, condition: condition(index)},
				{start: index*2 + 1, end: index*2 + 2, elseBranch: true},
			},
		}
	}
	variants, issue = alConditionalVariantSelections(nested)
	if issue == nil || !strings.Contains(issue.message, "more than 32 structural variants") {
		t.Fatalf("pathological nested conditionals did not fail closed: variants=%d issue=%+v", len(variants), issue)
	}
}

func TestALConditionalMergeIsIncrementalBoundedAndConflictAware(t *testing.T) {
	limits := SymbolBuilderLimits{MaxSymbols: 2, MaxSignatureBytes: 128, MaxDiagnostics: 8}
	merge := newALConditionalMerge(limits)
	symbol := func(id string, offset int) NormalizedSymbol {
		return NormalizedSymbol{ID: id, Name: id, QualifiedName: id, declarationOffsets: OffsetRange{Start: offset, End: offset + 1}, nameOffsets: OffsetRange{Start: offset, End: offset + 1}}
	}
	dependency := func(value string, line int) StructuralDependency {
		return StructuralDependency{Kind: StructuralDependencyImport, Value: value, Range: Range{Start: Position{Line: line, Column: 1}, End: Position{Line: line, Column: 2}}, Evidence: SymbolEvidenceStructural}
	}
	relation := func(source string, line int) StructuralRelation {
		return StructuralRelation{Kind: "extends", Source: source, Target: "Base", Range: Range{Start: Position{Line: line, Column: 1}, End: Position{Line: line, Column: 2}}, Evidence: SymbolEvidenceStructural}
	}
	merge.add(AnalyzerResult{
		Analysis:     AnalysisResult{CoverageComplete: true, Symbols: []NormalizedSymbol{symbol("b", 10), symbol("a", 0)}},
		Dependencies: []StructuralDependency{dependency("one", 1), dependency("two", 2)},
		Relations:    []StructuralRelation{relation("One", 1), relation("Two", 2)},
	})
	merge.add(AnalyzerResult{
		Analysis:     AnalysisResult{CoverageComplete: true, Symbols: []NormalizedSymbol{symbol("c", 20)}},
		Dependencies: []StructuralDependency{dependency("three", 3)},
		Relations:    []StructuralRelation{relation("Three", 3)},
	})
	result := merge.finish()
	if len(result.Analysis.Symbols) != 2 || len(result.Dependencies) != 2 || len(result.Relations) != 2 {
		t.Fatalf("bounded AL merge sizes symbols=%d dependencies=%d relations=%d", len(result.Analysis.Symbols), len(result.Dependencies), len(result.Relations))
	}
	if !result.Analysis.Truncated || result.Analysis.CoverageComplete {
		t.Fatalf("bounded AL merge did not fail closed: %+v", result.Analysis)
	}
	codes := map[string]bool{}
	for _, diagnostic := range result.Analysis.Diagnostics {
		codes[diagnostic.Code] = true
	}
	for _, code := range []string{"symbol-limit", "dependency-limit", "relation-limit"} {
		if !codes[code] {
			t.Fatalf("bounded AL merge missing %s diagnostic: %+v", code, result.Analysis.Diagnostics)
		}
	}
	if result.Analysis.Symbols[0].ID != "a" || result.Analysis.Symbols[1].ID != "b" {
		t.Fatalf("bounded AL merge symbol order=%v", []string{result.Analysis.Symbols[0].ID, result.Analysis.Symbols[1].ID})
	}

	conflict := newALConditionalMerge(SymbolBuilderLimits{MaxSymbols: 4, MaxSignatureBytes: 128, MaxDiagnostics: 4})
	first := symbol("same", 0)
	second := first
	second.Name = "different"
	conflict.add(AnalyzerResult{Analysis: AnalysisResult{CoverageComplete: true, Symbols: []NormalizedSymbol{first}}})
	conflict.add(AnalyzerResult{Analysis: AnalysisResult{CoverageComplete: true, Symbols: []NormalizedSymbol{second}}})
	conflictResult := conflict.finish()
	if conflictResult.Analysis.CoverageComplete {
		t.Fatalf("conflicting AL variant symbol was overclaimed: %+v", conflictResult.Analysis)
	}
	foundConflict := false
	for _, diagnostic := range conflictResult.Analysis.Diagnostics {
		if diagnostic.Code == "al-conditional-symbol-conflict" {
			foundConflict = true
			break
		}
	}
	if !foundConflict {
		t.Fatalf("conflicting AL variant symbol missing diagnostic: %+v", conflictResult.Analysis.Diagnostics)
	}
}

func TestDetectionKeepsSharedHeadersAmbiguousAndRoutesDistinctFormats(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	rust, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "lib.rs", Text: "pub struct RustBox { pub value: i32 }\npub fn rustwork() {}\n"})
	if err != nil {
		t.Fatal(err)
	}
	if rust.State != DetectionProbable || rust.Language != "rust" {
		t.Fatalf("Rust source contaminated by Phase 7 markers: %+v", rust)
	}

	mqlHeader, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "shared.mqh", Text: "input int Period = 14;\n"})
	if err != nil {
		t.Fatal(err)
	}
	if mqlHeader.State != DetectionAmbiguous || mqlHeader.Language != "" {
		t.Fatalf("shared .mqh detection=%+v, want MQL4/MQL5 ambiguity", mqlHeader)
	}
	for _, tc := range []struct{ path, text, want string }{
		{"expert.mq4", "input int Period = 14;\nint OnInit() { return 0; }\n", "mql4"},
		{"expert.mq5", "input double Lots = 0.1;\nvoid OnTick() {}\n", "mql5"},
		{"Bridge.mm", "#import <Foundation/Foundation.h>\n@interface Bridge : NSObject\n@end\n", "objective-cpp"},
		{"main.dart", "import 'dart:async';\nclass Service {}\n", "dart"},
		{"main.d", "module demo;\nimport std.stdio;\n", "d"},
		{"main.zig", "const std = @import(\"std\");\npub fn main() void {}\n", "zig"},
		{"main.nim", "proc run*() = discard\n", "nim"},
		{"Token.sol", "pragma solidity ^0.8.20;\ncontract Token {}\n", "solidity"},
		{"Service.cls", "public with sharing class Service {}\n", "apex"},
		{"App.al", "codeunit 50100 Worker { }\n", "al"},
		{"Sketch.ino", "#include <Arduino.h>\nvoid setup() {}\nvoid loop() {}\n", "arduino"},
	} {
		result, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: tc.path, Text: tc.text})
		if err != nil {
			t.Fatal(err)
		}
		if result.State != DetectionProbable || result.Language != tc.want {
			t.Fatalf("%s detection=%+v want probable %s", tc.path, result, tc.want)
		}
	}
}

func TestOpaqueAndMalformedBoundaries(t *testing.T) {
	opaque := []struct {
		name     string
		analyzer SourceAnalyzer
		text     string
		fake     string
		real     string
	}{
		{"mql5", MQL5Analyzer{}, "// void Fake() {}\nstring s = \"void StringFake() {}\";\nvoid Real() {}\n", "Fake", "Real"},
		{"objective-c", ObjectiveCAnalyzer{}, "// @interface Fake\nNSString *s = @\"@interface StringFake\";\n@interface Real\n@end\n", "Fake", "Real"},
		{"d", DAnalyzer{}, "/+ class Fake {} +/\nstring s = `class StringFake {}`;\nclass Real {}\n", "Fake", "Real"},
		{"solidity", SolidityAnalyzer{}, "/* contract Fake {} */\nstring constant S = \"contract StringFake {}\";\ncontract Real {}\n", "Fake", "Real"},
		{"nim", NimAnalyzer{}, "#[ proc Fake() = discard ]#\nlet s = \"proc StringFake() = discard\"\nproc Real() = discard\n", "Fake", "Real"},
	}
	for _, tc := range opaque {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
			if containsSortedString(names, tc.fake) || !containsSortedString(names, tc.real) {
				t.Fatalf("%s opaque boundary symbols=%v", tc.name, names)
			}
		})
	}

	malformed := []struct {
		name     string
		analyzer SourceAnalyzer
		text     string
	}{
		{"mql4", MQL4Analyzer{}, "void Good() {}\n/* unterminated"},
		{"mql5", MQL5Analyzer{}, "void Good() {}\n\"unterminated"},
		{"objective-c", ObjectiveCAnalyzer{}, "@interface Good\n@end\n@\"unterminated"},
		{"objective-cpp", ObjectiveCPPAnalyzer{}, "@interface Good\n@end\n\"unterminated"},
		{"dart", DartAnalyzer{}, "void good() {}\n\"unterminated"},
		{"d", DAnalyzer{}, "void good() {}\n/+ unterminated"},
		{"zig", ZigAnalyzer{}, "pub fn good() void {}\n\"unterminated"},
		{"nim", NimAnalyzer{}, "proc good() = discard\n#[ unterminated"},
		{"solidity", SolidityAnalyzer{}, "contract Good {}\n\"unterminated"},
		{"apex", ApexAnalyzer{}, "public class Good {}\n\"unterminated"},
		{"al", ALAnalyzer{}, "codeunit 50100 Good {}\n'unterminated"},
		{"arduino", ArduinoAnalyzer{}, "void setup() {}\n/* unterminated"},
	}
	for _, tc := range malformed {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
				t.Fatalf("%s malformed source did not lower coverage: %+v", tc.name, result.Analysis)
			}
		})
	}

	for _, analyzer := range specialtyCLikeAnalyzers() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := analyzer.Analyze(ctx, sourceDocumentForScanner("class A {}\n"), testAnalyzeOptions(false, 16))
		if operation.KindOf(err) != operation.KindCancelled {
			t.Fatalf("%s cancellation err=%v kind=%v", analyzer.Language(), err, operation.KindOf(err))
		}
	}
}

func TestSpecialtyCLikeGeneratedSourcesRespectSymbolLimits(t *testing.T) {
	cases := []struct {
		language string
		analyzer SourceAnalyzer
		text     string
	}{
		{"mql4", MQL4Analyzer{}, generatedCFunctions(1200)},
		{"mql5", MQL5Analyzer{}, generatedCFunctions(1200)},
		{"objective-c", ObjectiveCAnalyzer{}, generatedObjectiveC(1200)},
		{"objective-cpp", ObjectiveCPPAnalyzer{}, generatedObjectiveC(1200)},
		{"dart", DartAnalyzer{}, generatedCFunctions(1200)},
		{"d", DAnalyzer{}, generatedCFunctions(1200)},
		{"zig", ZigAnalyzer{}, generatedZig(1200)},
		{"nim", NimAnalyzer{}, generatedNim(1200)},
		{"solidity", SolidityAnalyzer{}, generatedSolidity(1200)},
		{"apex", ApexAnalyzer{}, generatedApex(1200)},
		{"al", ALAnalyzer{}, generatedAL(1200)},
		{"arduino", ArduinoAnalyzer{}, generatedCFunctions(1200)},
	}
	for _, tc := range cases {
		t.Run(tc.language, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(false, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != 128 || !result.Analysis.Truncated || result.Analysis.CoverageComplete {
				t.Fatalf("%s bounded result: symbols=%d truncated=%v complete=%v diagnostics=%+v", tc.language, len(result.Analysis.Symbols), result.Analysis.Truncated, result.Analysis.CoverageComplete, result.Analysis.Diagnostics)
			}
		})
	}
}

func generatedCFunctions(count int) string {
	var builder strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&builder, "void f%04d() {}\n", i)
	}
	return builder.String()
}

func generatedObjectiveC(count int) string {
	var builder strings.Builder
	builder.WriteString("@interface Generated : NSObject\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&builder, "- (void)m%04d;\n", i)
	}
	builder.WriteString("@end\n")
	return builder.String()
}

func generatedZig(count int) string {
	var builder strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&builder, "pub fn f%04d() void {}\n", i)
	}
	return builder.String()
}

func generatedNim(count int) string {
	var builder strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&builder, "proc f%04d() = discard\n", i)
	}
	return builder.String()
}

func generatedSolidity(count int) string {
	var builder strings.Builder
	builder.WriteString("contract Generated {\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&builder, "function f%04d() external {}\n", i)
	}
	builder.WriteString("}\n")
	return builder.String()
}

func generatedApex(count int) string {
	var builder strings.Builder
	builder.WriteString("public class Generated {\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&builder, "public void f%04d() {}\n", i)
	}
	builder.WriteString("}\n")
	return builder.String()
}

func generatedAL(count int) string {
	var builder strings.Builder
	builder.WriteString("codeunit 50100 Generated {\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&builder, "procedure P%04d() begin end;\n", i)
	}
	builder.WriteString("}\n")
	return builder.String()
}

func specialtyCLikeAnalyzers() []SourceAnalyzer {
	return []SourceAnalyzer{
		MQL4Analyzer{}, MQL5Analyzer{}, ObjectiveCAnalyzer{}, ObjectiveCPPAnalyzer{}, DartAnalyzer{}, DAnalyzer{}, ZigAnalyzer{}, NimAnalyzer{}, SolidityAnalyzer{}, ApexAnalyzer{}, ALAnalyzer{}, ArduinoAnalyzer{},
	}
}

func TestSpecialtyCLikeCapabilityCeilings(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"mql4", "mql5", "objective-c", "objective-cpp", "dart", "d", "zig", "nim", "solidity", "apex", "al", "arduino"} {
		descriptor, _ := registry.Lookup(language)
		caps := descriptor.Capabilities
		if caps.ScopeResolvedReferences || caps.ProjectResolvedReferences || caps.ProjectResolvedDefinitions || caps.Implementations || caps.Overrides || caps.SemanticRelations {
			t.Fatalf("%s overclaims project/semantic capability: %+v", language, caps)
		}
	}
}
