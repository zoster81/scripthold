package sourceintelligence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConformanceAcrossEncodingsUnicodeAndDeterminism(t *testing.T) {
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
			options := testAnalyzeOptions(true, 256)
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

func TestDynamicAndMacroBoundariesRemainStructural(t *testing.T) {
	jsText := `const regex = /class RegexFake { method() {} }/g;
const template = ` + "`" + `class TemplateFake {}` + "`" + `;
const jsx = () => <section data-text="class JSXFake {}">ok</section>;
const dynamic = require(moduleName);
`
	js, err := (JavaScriptAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(jsText), testAnalyzeOptions(true, 128))
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
	rust, err := (RustAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(rustText), testAnalyzeOptions(true, 128))
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

func TestECMAScriptRustGeneratedSourcesRespectSymbolLimits(t *testing.T) {
	cases := []struct {
		language string
		text     string
	}{
		{language: "javascript", text: generatedJavaScriptSource(1200)},
		{language: "typescript", text: generatedTypeScriptSource(1200)},
		{language: "rust", text: generatedRustSource(1200)},
	}
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range cases {
		t.Run(testCase.language, func(t *testing.T) {
			descriptor, _ := registry.Resolve(testCase.language)
			analyzer, _ := AnalyzerFor(descriptor)
			result, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(testCase.text), testAnalyzeOptions(false, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != 128 || !result.Analysis.Truncated || result.Analysis.CoverageComplete {
				t.Fatalf("%s bounded result: symbols=%d truncated=%v complete=%v", testCase.language, len(result.Analysis.Symbols), result.Analysis.Truncated, result.Analysis.CoverageComplete)
			}
		})
	}
}

func generatedJavaScriptSource(count int) string {
	var builder strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "export function f%04d(value) { return value; }\n", index)
	}
	return builder.String()
}

func generatedTypeScriptSource(count int) string {
	var builder strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "export interface I%04d { value: number; }\n", index)
	}
	return builder.String()
}

func generatedRustSource(count int) string {
	var builder strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "pub struct S%04d { pub value: i32 }\n", index)
	}
	return builder.String()
}
