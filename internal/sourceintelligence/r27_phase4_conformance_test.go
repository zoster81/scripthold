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

func TestR27Phase4ConformanceAcrossEncodingsUnicodeAndDeterminism(t *testing.T) {
	tests := []struct {
		name, language, extension, encoding, text string
		bom                                       bool
		want                                      []string
	}{
		{
			name: "javascript-utf16le", language: "javascript", extension: ".js", encoding: "utf-16le", bom: true,
			text: "export const café = () => 1;\r\nclass Résumé { méthode() {} }\r\n",
			want: []string{"café", "Résumé", "Résumé.méthode"},
		},
		{
			name: "typescript-utf32le", language: "typescript", extension: ".ts", encoding: "utf-32le", bom: true,
			text: "export interface Résumé<T> { café: T; }\nexport const identité = <T,>(value: T): T => value;\n",
			want: []string{"Résumé", "Résumé.café", "identité"},
		},
		{
			name: "rust-utf16be", language: "rust", extension: ".rs", encoding: "utf-16be", bom: true,
			text: "pub struct Café { pub résumé: i32 }\npub fn identité(value: i32) -> i32 { value }\n",
			want: []string{"Café", "Café.résumé", "identité"},
		},
	}
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "fixture"+testCase.extension)
			raw := encodeSourceFixture(t, testCase.encoding, testCase.text, testCase.bom)
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
				RequestedEncoding: testCase.encoding, MaxFileBytes: 4 * 1024 * 1024, MaxDecodedCharacters: 1_000_000,
			})
			if err != nil {
				t.Fatal(err)
			}
			descriptor, _ := registry.Resolve(testCase.language)
			analyzer, ok := AnalyzerFor(descriptor)
			if !ok {
				t.Fatalf("missing analyzer %s", testCase.language)
			}
			options := phase3AnalyzeOptions(true, 256)
			first, err := analyzer.Analyze(context.Background(), document, options)
			if err != nil {
				t.Fatal(err)
			}
			second, err := analyzer.Analyze(context.Background(), document, options)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatal("Phase 4 output is nondeterministic")
			}
			if !first.Analysis.CoverageComplete || first.Analysis.Truncated {
				t.Fatalf("Phase 4 conformance partial: %+v", first.Analysis)
			}
			actual := sortedSymbolQualifiedNames(first.Analysis.Symbols)
			for _, want := range testCase.want {
				if !containsSortedString(actual, want) {
					t.Fatalf("missing %s; symbols=%v", want, actual)
				}
			}
		})
	}
}

func TestR27Phase4DynamicAndMacroBoundariesRemainStructural(t *testing.T) {
	jsText := `const regex = /class RegexFake { method() {} }/g;
const template = ` + "`" + `class TemplateFake {}` + "`" + `;
const jsx = () => <section data-text="class JSXFake {}">ok</section>;
const dynamic = require(moduleName);
`
	js, err := (JavaScriptAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(jsText), phase3AnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !js.Analysis.CoverageComplete {
		t.Fatalf("valid JavaScript lexical boundaries reported partial: %+v", js.Analysis.Diagnostics)
	}
	for _, symbol := range js.Analysis.Symbols {
		switch symbol.Name {
		case "RegexFake", "TemplateFake", "JSXFake", "method":
			t.Fatalf("JavaScript opaque region leaked declaration: %+v", symbol)
		}
	}
	if got := dependencyValues(js.Dependencies); len(got) != 0 {
		t.Fatalf("non-literal require was overclaimed as structural dependency: %v", got)
	}

	rustText := `macro_rules! make_type {
    ($name:ident) => { struct MacroFake; fn hidden() {} };
}
make_type!(Generated);
#[cfg(feature = "x")]
pub struct Conditional { pub value: i32 }
`
	rust, err := (RustAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(rustText), phase3AnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !rust.Analysis.CoverageComplete {
		t.Fatalf("Rust macro/cfg boundaries unexpectedly partial: %+v", rust.Analysis.Diagnostics)
	}
	for _, symbol := range rust.Analysis.Symbols {
		if symbol.Name == "MacroFake" || symbol.Name == "hidden" || symbol.Name == "Generated" {
			t.Fatalf("Rust macro expansion was overclaimed: %+v", symbol)
		}
	}
	if _, ok := symbolsByQualifiedName(rust.Analysis.Symbols)["Conditional"]; !ok {
		t.Fatalf("cfg-guarded structural declaration was lost: %v", sortedSymbolQualifiedNames(rust.Analysis.Symbols))
	}
}

func TestR27Phase4LargeGeneratedSourcesRespectSymbolLimits(t *testing.T) {
	cases := []struct {
		language string
		text     string
	}{
		{language: "javascript", text: generatedPhase4JavaScriptSource(1200)},
		{language: "typescript", text: generatedPhase4TypeScriptSource(1200)},
		{language: "rust", text: generatedPhase4RustSource(1200)},
	}
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range cases {
		t.Run(testCase.language, func(t *testing.T) {
			descriptor, _ := registry.Resolve(testCase.language)
			analyzer, _ := AnalyzerFor(descriptor)
			result, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(testCase.text), phase3AnalyzeOptions(false, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != 128 || !result.Analysis.Truncated || result.Analysis.CoverageComplete {
				t.Fatalf("%s bounded result: symbols=%d truncated=%v complete=%v", testCase.language, len(result.Analysis.Symbols), result.Analysis.Truncated, result.Analysis.CoverageComplete)
			}
		})
	}
}

func generatedPhase4JavaScriptSource(count int) string {
	var builder strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "export function f%04d(value) { return value; }\n", index)
	}
	return builder.String()
}

func generatedPhase4TypeScriptSource(count int) string {
	var builder strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "export interface I%04d { value: number; }\n", index)
	}
	return builder.String()
}

func generatedPhase4RustSource(count int) string {
	var builder strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "pub struct S%04d { pub value: i32 }\n", index)
	}
	return builder.String()
}

func FuzzR27Phase4AnalyzersNoPanic(f *testing.F) {
	seeds := []struct {
		text     string
		selector uint8
	}{
		{"export const value = () => 1;\nclass Box { run() {} }\n", 0},
		{"interface Box<T> { value: T; }\nconst map = <T,>(v:T):T => v;\n", 1},
		{"pub struct Box<T> { pub value: T }\nimpl<T> Box<T> { pub fn get(&self) -> &T { &self.value } }\n", 2},
		{"const re = /class Fake {}/g;\n", 0},
		{"macro_rules! x { () => { struct Fake; } }\n", 2},
	}
	for _, seed := range seeds {
		f.Add(seed.text, seed.selector)
	}
	f.Fuzz(func(t *testing.T, text string, selector uint8) {
		analyzers := []SourceAnalyzer{JavaScriptAnalyzer{}, TypeScriptAnalyzer{}, RustAnalyzer{}}
		analyzer := analyzers[int(selector)%len(analyzers)]
		result, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 128))
		if err != nil {
			if kind := operation.KindOf(err); kind != operation.KindInvalidInput && kind != operation.KindLimit {
				t.Fatalf("unexpected %s fuzz error: %v kind=%v", analyzer.Language(), err, kind)
			}
			return
		}
		if len(result.Analysis.Symbols) > 128 {
			t.Fatalf("%s fuzz result exceeded symbol bound: %d", analyzer.Language(), len(result.Analysis.Symbols))
		}
		for _, symbol := range result.Analysis.Symbols {
			if symbol.Name == "" || symbol.QualifiedName == "" || symbol.DeclarationRange.Start.Line <= 0 || symbol.DeclarationRange.Start.Column <= 0 {
				t.Fatalf("%s fuzz emitted invalid symbol: %+v", analyzer.Language(), symbol)
			}
		}
	})
}
