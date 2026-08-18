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
