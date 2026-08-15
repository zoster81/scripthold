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

func TestR27Phase5ConformanceAcrossEncodingsLegacyCRLFAndDeterminism(t *testing.T) {
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

func TestR27Phase5DynamicAndDirectiveBoundariesRemainStructural(t *testing.T) {
	php, err := (PHPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("<?php\ninclude $dynamic;\ninclude 'literal.php';\n"), phase3AnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if got := dependencyValues(php.Dependencies); !reflect.DeepEqual(got, []string{"literal.php"}) {
		t.Fatalf("PHP dynamic include was overclaimed: %v", got)
	}

	ruby, err := (RubyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("class Real\nend\ndefine_method(:dynamic) { }\n"), phase3AnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range ruby.Analysis.Symbols {
		if symbol.Name == "dynamic" {
			t.Fatalf("Ruby metaprogramming was overclaimed: %+v", symbol)
		}
	}

	pascalText := "{$IFDEF FEATURE}\r\ntype\r\n  TMaybe = class end;\r\n{$ENDIF}\r\n"
	pascal, err := (PascalAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(pascalText), phase3AnalyzeOptions(true, 64))
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

func TestR27Phase5LargeGeneratedSourcesRespectSymbolLimits(t *testing.T) {
	cases := []struct {
		language string
		text     string
	}{
		{language: "php", text: generatedPhase5PHPSource(1200)},
		{language: "ruby", text: generatedPhase5RubySource(1200)},
		{language: "swift", text: generatedPhase5SwiftSource(1200)},
		{language: "pascal", text: generatedPhase5PascalSource(1200)},
		{language: "delphi", text: generatedPhase5DelphiSource(1200)},
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

func generatedPhase5PHPSource(count int) string {
	var builder strings.Builder
	builder.WriteString("<?php\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "class C%04d {}\n", index)
	}
	return builder.String()
}

func generatedPhase5RubySource(count int) string {
	var builder strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "class C%04d\nend\n", index)
	}
	return builder.String()
}

func generatedPhase5SwiftSource(count int) string {
	var builder strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "struct S%04d {}\n", index)
	}
	return builder.String()
}

func generatedPhase5PascalSource(count int) string {
	var builder strings.Builder
	builder.WriteString("type\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "  T%04d = class end;\n", index)
	}
	return builder.String()
}

func generatedPhase5DelphiSource(count int) string {
	var builder strings.Builder
	builder.WriteString("type\r\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "  T%04d = record end;\r\n", index)
	}
	return builder.String()
}

func FuzzR27Phase5AnalyzersNoPanic(f *testing.F) {
	seeds := []struct {
		text     string
		selector uint8
	}{
		{"<?php\nclass Box { public function get() {} }\n", 0},
		{"class Box\n  def get\n  end\nend\n", 1},
		{"struct Box { func get() {} }\n", 2},
		{"program Demo;\ntype\n  TBox = class end;\n", 3},
		{"unit Demo;\ninterface\ntype\n  TBox<T> = class end;\nimplementation\nend.\n", 4},
	}
	for _, seed := range seeds {
		f.Add(seed.text, seed.selector)
	}
	f.Fuzz(func(t *testing.T, text string, selector uint8) {
		analyzers := []SourceAnalyzer{PHPAnalyzer{}, RubyAnalyzer{}, SwiftAnalyzer{}, PascalAnalyzer{}, DelphiAnalyzer{}}
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
