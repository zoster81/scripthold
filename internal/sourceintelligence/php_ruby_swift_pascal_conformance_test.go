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

func TestConformanceAcrossEncodingsLegacyCRLFAndDeterminism(t *testing.T) {
	tests := []struct {
		name, language, extension, encoding, text string
		bom                                       bool
		want                                      []string
	}{
		{
			name: "php-utf16le", language: "php", extension: ".php", encoding: "utf-16le", bom: true,
			text: "<?php\r\nclass Café { public function résumé(): void {} }\r\n",
			want: []string{"Café", "Café.résumé"},
		},
		{
			name: "ruby-windows1252", language: "ruby", extension: ".rb", encoding: "windows-1252",
			text: "class Café\r\n  def résumé\r\n  end\r\nend\r\n",
			want: []string{"Café", "Café.résumé"},
		},
		{
			name: "swift-utf16be", language: "swift", extension: ".swift", encoding: "utf-16be", bom: true,
			text: "struct Café {\n    func résumé() {}\n}\n",
			want: []string{"Café", "Café.résumé"},
		},
		{
			name: "pascal-ibm850-crlf", language: "pascal", extension: ".pp", encoding: "ibm850",
			text: "program Démo;\r\ntype\r\n  TRésumé = class\r\n  public\r\n    procedure Café;\r\n  end;\r\n",
			want: []string{"Démo", "Démo.TRésumé", "Démo.TRésumé.Café"},
		},
		{
			name: "delphi-windows1252-crlf", language: "delphi", extension: ".dpr", encoding: "windows-1252",
			text: "unit Café.Unit1;\r\ninterface\r\ntype\r\n  TRésumé = class\r\n  public\r\n    procedure Étape;\r\n  end;\r\nimplementation\r\nend.\r\n",
			want: []string{"Café.Unit1", "Café.Unit1.TRésumé", "Café.Unit1.TRésumé.Étape"},
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
				t.Fatal("Phase 5 output is nondeterministic")
			}
			if !first.Analysis.CoverageComplete || first.Analysis.Truncated {
				t.Fatalf("Phase 5 conformance partial: %+v", first.Analysis)
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

func TestDynamicAndDirectiveBoundariesRemainStructural(t *testing.T) {
	php, err := (PHPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("<?php\ninclude $dynamic;\ninclude 'literal.php';\n"), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if got := dependencyValues(php.Dependencies); !reflect.DeepEqual(got, []string{"literal.php"}) {
		t.Fatalf("PHP dynamic include was overclaimed: %v", got)
	}

	ruby, err := (RubyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("class Real\nend\ndefine_method(:dynamic) { }\n"), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range ruby.Analysis.Symbols {
		if symbol.Name == "dynamic" {
			t.Fatalf("Ruby metaprogramming was overclaimed: %+v", symbol)
		}
	}

	pascalText := "{$IFDEF FEATURE}\r\ntype\r\n  TMaybe = class end;\r\n{$ENDIF}\r\n"
	pascal, err := (PascalAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(pascalText), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := symbolsByQualifiedName(pascal.Analysis.Symbols)["TMaybe"]; !ok {
		t.Fatalf("Pascal directive masking lost structural declaration: %v", sortedSymbolQualifiedNames(pascal.Analysis.Symbols))
	}
	if !pascal.Analysis.CoverageComplete {
		t.Fatalf("opaque Pascal compiler directives should not corrupt lexical coverage: %+v", pascal.Analysis.Diagnostics)
	}
}

func TestPHPRubySwiftPascalGeneratedSourcesRespectSymbolLimits(t *testing.T) {
	cases := []struct {
		language string
		text     string
	}{
		{language: "php", text: generatedPHPSource(1200)},
		{language: "ruby", text: generatedRubySource(1200)},
		{language: "swift", text: generatedSwiftSource(1200)},
		{language: "pascal", text: generatedPascalSource(1200)},
		{language: "delphi", text: generatedDelphiSource(1200)},
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

func generatedPHPSource(count int) string {
	var builder strings.Builder
	builder.WriteString("<?php\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "class C%04d {}\n", index)
	}
	return builder.String()
}

func generatedRubySource(count int) string {
	var builder strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "class C%04d\nend\n", index)
	}
	return builder.String()
}

func generatedSwiftSource(count int) string {
	var builder strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "struct S%04d {}\n", index)
	}
	return builder.String()
}

func generatedPascalSource(count int) string {
	var builder strings.Builder
	builder.WriteString("type\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "  T%04d = class end;\n", index)
	}
	return builder.String()
}

func generatedDelphiSource(count int) string {
	var builder strings.Builder
	builder.WriteString("type\r\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "  T%04d = record end;\r\n", index)
	}
	return builder.String()
}
