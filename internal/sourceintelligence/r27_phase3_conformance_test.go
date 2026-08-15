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

func TestR27Phase3ConformanceAcrossEncodingsAndUnicode(t *testing.T) {
	tests := []struct {
		name, language, extension, encoding, text string
		bom                                       bool
		want, forbidden                           []string
	}{
		{name: "c-utf16le", language: "c", extension: ".c", encoding: "utf-16le", bom: true,
			text: "struct Café { int résumé; };\r\nconst char *s = \"struct Faux {};\";\r\n", want: []string{"Café", "Café.résumé"}, forbidden: []string{"Faux"}},
		{name: "cpp-utf32le", language: "cpp", extension: ".cpp", encoding: "utf-32le", bom: true,
			text: "namespace Δemo { class Café { public: int résumé; }; }\n", want: []string{"Δemo", "Δemo.Café", "Δemo.Café.résumé"}},
		{name: "java-utf16be", language: "java", extension: ".java", encoding: "utf-16be", bom: true,
			text: "package café; public class Résumé { int value; String s = \"class Faux {}\"; }\r\n", want: []string{"café", "café.Résumé", "café.Résumé.value"}, forbidden: []string{"café.Résumé.Faux"}},
		{name: "kotlin-windows1252", language: "kotlin", extension: ".kt", encoding: "windows-1252",
			text: "package café\r\nclass Résumé(val value: Int)\r\n", want: []string{"café", "café.Résumé", "café.Résumé.Résumé", "café.Résumé.value"}},
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
			document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{RequestedEncoding: testCase.encoding, MaxFileBytes: 4 * 1024 * 1024, MaxDecodedCharacters: 1_000_000})
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
				t.Fatal("Phase 3 output is nondeterministic")
			}
			if !first.Analysis.CoverageComplete {
				t.Fatalf("Phase 3 conformance partial: %+v", first.Analysis.Diagnostics)
			}
			actual := sortedSymbolQualifiedNames(first.Analysis.Symbols)
			for _, want := range testCase.want {
				if !containsSortedString(actual, want) {
					t.Fatalf("missing %s; symbols=%v", want, actual)
				}
			}
			for _, forbidden := range testCase.forbidden {
				if containsSortedString(actual, forbidden) {
					t.Fatalf("false positive %s; symbols=%v", forbidden, actual)
				}
			}
		})
	}
}

func TestR27Phase3OpaqueAdvancedStringFamiliesAndPreprocessorTruthfulness(t *testing.T) {
	cppText := "class Real {};\nconst char* raw = R\"DELIM(\" class RawFake { void Nope(); } \"\nsecond line)DELIM\";\n"
	cpp, err := (CPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(cppText), phase3AnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range cpp.Analysis.Symbols {
		if symbol.Name == "RawFake" || symbol.Name == "Nope" {
			t.Fatalf("C++ raw string leaked symbol: %+v", symbol)
		}
	}
	if !cpp.Analysis.CoverageComplete {
		t.Fatalf("valid C++ raw string reported partial: %+v", cpp.Analysis.Diagnostics)
	}

	javaText := "class Real { String text = \"\"\"\nclass TextBlockFake { void nope() {} }\n\"\"\"; void work() {} }\n"
	java, err := (JavaAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(javaText), phase3AnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range java.Analysis.Symbols {
		if symbol.Name == "TextBlockFake" || symbol.Name == "nope" {
			t.Fatalf("Java text block leaked symbol: %+v", symbol)
		}
	}

	kotlinText := "class Real { val text = \"\"\"\nclass TripleFake { fun nope() {} }\n\"\"\"; fun work() {} }\n"
	kotlin, err := (KotlinAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(kotlinText), phase3AnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range kotlin.Analysis.Symbols {
		if symbol.Name == "TripleFake" || symbol.Name == "nope" {
			t.Fatalf("Kotlin raw string leaked symbol: %+v", symbol)
		}
	}

	conditionalText := "#ifdef FEATURE\nstruct Maybe { int value; };\n#endif\nstruct Always { int value; };\n"
	conditional, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(conditionalText), phase3AnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if conditional.Analysis.CoverageComplete || len(conditional.Analysis.Diagnostics) == 0 {
		t.Fatalf("conditional preprocessing was overclaimed as complete: %+v", conditional.Analysis)
	}
	if _, ok := symbolsByQualifiedName(conditional.Analysis.Symbols)["Always"]; !ok {
		t.Fatalf("conditional parsing lost unaffected declaration: %v", sortedSymbolQualifiedNames(conditional.Analysis.Symbols))
	}
}

func TestR27Phase3LargeGeneratedSourcesRespectSymbolLimits(t *testing.T) {
	cases := []struct{ language, text string }{
		{language: "c", text: generatedPhase3CSource(1200)},
		{language: "cpp", text: generatedPhase3CPPSource(1200)},
		{language: "java", text: generatedPhase3JavaSource(1200)},
		{language: "kotlin", text: generatedPhase3KotlinSource(1200)},
	}
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range cases {
		t.Run(testCase.language, func(t *testing.T) {
			descriptor, _ := registry.Resolve(testCase.language)
			analyzer, _ := AnalyzerFor(descriptor)
			document := sourceDocumentForScanner(testCase.text)
			result, err := analyzer.Analyze(context.Background(), document, phase3AnalyzeOptions(false, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != 128 || !result.Analysis.Truncated || result.Analysis.CoverageComplete {
				t.Fatalf("%s bounded result: symbols=%d truncated=%v complete=%v", testCase.language, len(result.Analysis.Symbols), result.Analysis.Truncated, result.Analysis.CoverageComplete)
			}
		})
	}
}

func generatedPhase3CSource(count int) string {
	var builder strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "struct C%04d { int value; };\n", index)
	}
	return builder.String()
}
func generatedPhase3CPPSource(count int) string {
	var builder strings.Builder
	builder.WriteString("namespace generated {\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "class C%04d { public: int value; };\n", index)
	}
	builder.WriteString("}\n")
	return builder.String()
}
func generatedPhase3JavaSource(count int) string {
	var builder strings.Builder
	builder.WriteString("package generated;\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "class C%04d { int value; }\n", index)
	}
	return builder.String()
}
func generatedPhase3KotlinSource(count int) string {
	var builder strings.Builder
	builder.WriteString("package generated\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "class C%04d(val value: Int)\n", index)
	}
	return builder.String()
}
