package sourceintelligence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestR27Phase16ApprovedCatalogLeavesOnlyAuxiliaryMetadataInactive(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var inactive []string
	for _, row := range registry.CapabilityRows() {
		if !row.Capabilities.SourceAnalysis {
			inactive = append(inactive, row.ID)
		}
	}
	if got, want := strings.Join(inactive, ","), "dockerfile,make"; got != want {
		t.Fatalf("inactive registry rows=%s want=%s; every approved R27 catalog row must be active", got, want)
	}
	for _, id := range []string{"dockerfile", "make"} {
		descriptor, _ := registry.Lookup(id)
		if descriptor.Capabilities.SourceAnalysis || descriptor.Analyzer != "" {
			t.Fatalf("auxiliary metadata row %s was accidentally counted as R27 production support: %+v", id, descriptor)
		}
	}
}

func TestR27Phase16DetectionRoutesScalaAndFlowWithoutContaminatingJavaScript(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path, text, want string
		state                  DetectionState
	}{
		{"scala-extension", "Main.scala", "package demo\ntrait Worker:\n  def run(x: Int): Int\n", "scala", DetectionProbable},
		{"scala3-class-typed-method", "Service.scala", "package demo\nclass ScalaBox:\n  def run(value: Int): Int = value\n", "scala", DetectionProbable},
		{"flow-compound-suffix", "types.js.flow", "export type User = { name: string };\n", "flow", DetectionExact},
		{"flow-pragma-over-js-extension", "app.js", "/* @flow */\nexport type ID = string;\nexport function run(x: ID): ID { return x; }\n", "flow", DetectionProbable},
		{"ordinary-javascript", "app.js", "export class JavaScriptOnly { run() {} }\n", "javascript", DetectionProbable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: tc.path, Text: tc.text})
			if err != nil {
				t.Fatal(err)
			}
			if result.State != tc.state || result.Language != tc.want {
				t.Fatalf("detection=%+v want %s %s", result, tc.state, tc.want)
			}
			if tc.want == "javascript" && hasDetectionCandidate(result, "flow") {
				t.Fatalf("ordinary JavaScript was contaminated by Flow evidence: %+v", result)
			}
		})
	}
}

func FuzzR27Phase16ScalaAndFlowNoPanic(f *testing.F) {
	seeds := []struct {
		text     string
		selector uint8
	}{
		{"package demo\nclass Service:\n  def run(value: Int): Int = value\n", 0},
		{"/* @flow */\nexport opaque type ID = string;\nexport class Service { run(value: ID): ID { return value; } }\n", 1},
		{"// comment\nval text = \"class Fake {}\"\ntrait Real:\n  def run(x: Int): Int\n", 0},
		{"/* @flow */\nconst text = \"type Fake = string\";\nexport type Real = string;\n", 1},
	}
	for _, seed := range seeds {
		f.Add(seed.text, seed.selector)
	}
	f.Fuzz(func(t *testing.T, text string, selector uint8) {
		analyzers := []SourceAnalyzer{ScalaAnalyzer{}, FlowAnalyzer{}}
		analyzer := analyzers[int(selector)%len(analyzers)]
		result, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 128))
		if err != nil {
			kind := operation.KindOf(err)
			if kind != operation.KindInvalidInput && kind != operation.KindLimit && kind != operation.KindUnsupported {
				t.Fatalf("unexpected %s fuzz error: %v kind=%v", analyzer.Language(), err, kind)
			}
			return
		}
		if len(result.Analysis.Symbols) > 128 {
			t.Fatalf("%s fuzz result exceeded symbol bound: %d", analyzer.Language(), len(result.Analysis.Symbols))
		}
		for _, symbol := range result.Analysis.Symbols {
			if symbol.Name == "" || symbol.QualifiedName == "" || symbol.Language != analyzer.Language() || symbol.DeclarationRange.Start.Line <= 0 || symbol.DeclarationRange.Start.Column <= 0 || symbol.NameRange.Start.Line <= 0 || symbol.NameRange.Start.Column <= 0 {
				t.Fatalf("%s fuzz emitted invalid symbol: %+v", analyzer.Language(), symbol)
			}
		}
	})
}

func TestR27Phase16ScalaAndFlowExposeDistinctStructuralProviders(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		language string
		text     string
		want     map[string]SymbolKind
		deps     []string
	}{
		{
			language: "scala",
			text: `package demo.core
import scala.collection.mutable
trait Worker:
  def run(value: Int): Int
class Service extends Worker:
  val name: String = "demo"
  def run(value: Int): Int =
    value
object Helpers {
  def twice(value: Int): Int = value * 2
}
`,
			want: map[string]SymbolKind{
				"demo.core": SymbolKindPackage, "demo.core.Worker": SymbolKindTrait,
				"demo.core.Worker.run": SymbolKindMethod, "demo.core.Service": SymbolKindClass,
				"demo.core.Service.name": SymbolKindField, "demo.core.Service.run": SymbolKindMethod,
				"demo.core.Helpers": SymbolKindModule, "demo.core.Helpers.twice": SymbolKindMethod,
			},
			deps: []string{"scala.collection.mutable"},
		},
		{
			language: "flow",
			text: `/* @flow */
import type { User } from "./types";
export type ID = string;
export opaque type Token = string;
export interface Worker { run(value: ID): ID; }
export class Service extends Base { run(value: ID): ID { return value; } }
export function build(value: ID): ID { return value; }
export const helper = (value: ID): ID => value;
export const opaque = 1;
`,
			want: map[string]SymbolKind{
				"ID": SymbolKindAlias, "Token": SymbolKindAlias, "Worker": SymbolKindInterface,
				"Worker.run": SymbolKindMethod, "Service": SymbolKindClass, "Service.run": SymbolKindMethod,
				"build": SymbolKindFunction, "helper": SymbolKindFunction,
			},
			deps: []string{"./types"},
		},
	}
	seenIDs := map[AnalyzerID]string{}
	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			descriptor, ok := registry.Resolve(tc.language)
			if !ok {
				t.Fatalf("missing %s registry row", tc.language)
			}
			analyzer, available := AnalyzerFor(descriptor)
			if !available || analyzer.Language() != tc.language {
				t.Fatalf("%s analyzer unavailable: descriptor=%+v analyzer=%#v", tc.language, descriptor, analyzer)
			}
			if previous, duplicate := seenIDs[analyzer.ID()]; duplicate {
				t.Fatalf("%s and %s share analyzer identity %s", previous, tc.language, analyzer.ID())
			}
			seenIDs[analyzer.ID()] = tc.language
			result, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), phase3AnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("%s analysis partial: %+v", tc.language, result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range tc.want {
				if symbol, exists := byName[name]; !exists || symbol.Kind != kind || symbol.Language != tc.language {
					t.Fatalf("%s symbol %s=%+v exists=%v all=%v", tc.language, name, symbol, exists, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			if tc.language == "flow" {
				if symbol, exists := byName["opaque"]; !exists || symbol.Language != "flow" {
					t.Fatalf("Flow ordinary identifier named opaque was lost by Flow-only syntax masking: symbol=%+v exists=%v all=%v", symbol, exists, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			if got := dependencyValues(result.Dependencies); !sameStringSet(got, tc.deps) {
				t.Fatalf("%s dependencies=%v want=%v", tc.language, got, tc.deps)
			}
			wantRelation := StructuralRelation{}
			switch tc.language {
			case "scala":
				wantRelation = StructuralRelation{Kind: "inherits", Source: "demo.core.Service", Target: "Worker"}
			case "flow":
				wantRelation = StructuralRelation{Kind: "extends", Source: "Service", Target: "Base"}
			}
			foundRelation := false
			for _, relation := range result.Relations {
				if relation.Kind == wantRelation.Kind && relation.Source == wantRelation.Source && relation.Target == wantRelation.Target && relation.Evidence == SymbolEvidenceStructural {
					foundRelation = true
					break
				}
			}
			if !foundRelation {
				t.Fatalf("%s missing structural inheritance relation %+v; relations=%+v", tc.language, wantRelation, result.Relations)
			}
		})
	}
}

func TestR27Phase16ScalaAndFlowOpaqueMalformedLimitsCancellationAndEncodings(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		language string
		ext      string
		encoding string
		text     string
		want     string
	}{
		{"scala", ".scala", "utf-16le", "// class Fake\r\nval text = \"def Hidden() = 1\"\r\nclass Café:\r\n  def réel(x: Int): Int = x\r\n", "Café.réel"},
		{"flow", ".js.flow", "utf-32be", "// type Fake = string\nconst text = \"class Hidden {}\";\nexport type Café = string;\nexport function réel(x: Café): Café { return x; }\n", "réel"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			descriptor, _ := registry.Resolve(tc.language)
			analyzer, available := AnalyzerFor(descriptor)
			if !available {
				t.Fatalf("%s analyzer unavailable", tc.language)
			}
			path := filepath.Join(t.TempDir(), "fixture"+tc.ext)
			if err := os.WriteFile(path, encodeSourceFixture(t, tc.encoding, tc.text, true), 0o600); err != nil {
				t.Fatal(err)
			}
			document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{RequestedEncoding: tc.encoding, MaxFileBytes: 1 << 20, MaxDecodedCharacters: 1 << 20})
			if err != nil {
				t.Fatal(err)
			}
			result, err := analyzer.Analyze(context.Background(), document, phase3AnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
			if !containsSortedString(names, tc.want) || containsSortedString(names, "Fake") || containsSortedString(names, "Hidden") {
				t.Fatalf("%s encoded/opaque symbols=%v", tc.language, names)
			}
			for _, symbol := range result.Analysis.Symbols {
				if symbol.DeclarationRange.Start.Line <= 0 || symbol.NameRange.Start.Column <= 0 {
					t.Fatalf("%s invalid decoded range: %+v", tc.language, symbol)
				}
			}

			malformed, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner("class Good {\n  val text = \"unterminated\n"), phase3AnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if malformed.Analysis.CoverageComplete || len(malformed.Analysis.Diagnostics) == 0 {
				t.Fatalf("%s malformed input did not lower coverage: %+v", tc.language, malformed.Analysis)
			}

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := analyzer.Analyze(ctx, sourceDocumentForScanner("class A {}\n"), phase3AnalyzeOptions(false, 16)); operation.KindOf(err) != operation.KindCancelled {
				t.Fatalf("%s cancellation err=%v kind=%v", tc.language, err, operation.KindOf(err))
			}

			var generated strings.Builder
			for i := 0; i < 1200; i++ {
				if tc.language == "scala" {
					fmt.Fprintf(&generated, "def f%04d(x: Int): Int = x\n", i)
				} else {
					fmt.Fprintf(&generated, "export function f%04d(x: number): number { return x; }\n", i)
				}
			}
			limited, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(generated.String()), phase3AnalyzeOptions(false, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(limited.Analysis.Symbols) != 128 || !limited.Analysis.Truncated || limited.Analysis.CoverageComplete {
				t.Fatalf("%s limit result symbols=%d truncated=%v complete=%v", tc.language, len(limited.Analysis.Symbols), limited.Analysis.Truncated, limited.Analysis.CoverageComplete)
			}
		})
	}
}
