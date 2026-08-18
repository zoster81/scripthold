package sourceintelligence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestConformanceCorpusAcrossEncodings(t *testing.T) {
	tests := []struct {
		name      string
		language  string
		encoding  string
		bom       bool
		text      string
		want      []string
		forbidden []string
	}{
		{
			name: "go-utf16le", language: "go", encoding: "utf-16le", bom: true,
			text: "package café\r\n\r\ntype Box struct{}\r\nfunc Résumé() {}\r\n// func Fake() {}\r\n",
			want: []string{"café", "café.Box", "café.Résumé"}, forbidden: []string{"café.Fake"},
		},
		{
			name: "csharp-windows1252", language: "csharp", encoding: "windows-1252",
			text: "namespace Demo { public class Café { public void Résumé() {} string s = \"class Fake {}\"; } }\r\n",
			want: []string{"Demo", "Demo.Café", "Demo.Café.Résumé"}, forbidden: []string{"Demo.Café.Fake"},
		},
		{
			name: "vbnet-utf16be", language: "vbnet", encoding: "utf-16be", bom: true,
			text: "Namespace Demo\r\nPublic Class Café\r\nPublic Sub Résumé()\r\nEnd Sub\r\n' Public Sub Fake()\r\nEnd Class\r\nEnd Namespace\r\n",
			want: []string{"Demo", "Demo.Café", "Demo.Café.Résumé"}, forbidden: []string{"Demo.Café.Fake"},
		},
		{
			name: "python-utf32le", language: "python", encoding: "utf-32le", bom: true,
			text: "class Café:\n    def résumé(self):\n        return 1\n\ntext = \"def fake(): pass\"\n",
			want: []string{"Café", "Café.résumé"}, forbidden: []string{"fake"},
		},
		{
			name: "asp-windows1252", language: "classic-asp", encoding: "windows-1252",
			text: "<%@ Language=\"VBScript\" %>\r\n<% Sub Café()\r\nEnd Sub %>\r\n<html>class Fake</html>\r\n",
			want: []string{"Café"}, forbidden: []string{"Fake"},
		},
	}
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			extension := map[string]string{"go": ".go", "csharp": ".cs", "vbnet": ".vb", "python": ".py", "classic-asp": ".asp"}[testCase.language]
			path := filepath.Join(root, "fixture"+extension)
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
			descriptor, ok := registry.Resolve(testCase.language)
			if !ok {
				t.Fatalf("missing registry descriptor %s", testCase.language)
			}
			analyzer, ok := AnalyzerFor(descriptor)
			if !ok {
				t.Fatalf("missing analyzer %s", testCase.language)
			}
			options := AnalyzeOptions{IncludeSignatures: true, MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: 256, MaxSignatureBytes: 8192, MaxDiagnostics: 32}}
			first, err := analyzer.Analyze(context.Background(), document, options)
			if err != nil {
				t.Fatal(err)
			}
			second, err := analyzer.Analyze(context.Background(), document, options)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatal("conformance output is nondeterministic")
			}
			if !first.Analysis.CoverageComplete {
				t.Fatalf("conformance analysis partial: %+v", first.Analysis.Diagnostics)
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

func TestCanaryLargeGeneratedSourcesRespectSymbolLimits(t *testing.T) {
	cases := []struct {
		language string
		text     string
	}{
		{language: "go", text: generatedGoSource(1500)},
		{language: "csharp", text: generatedCSharpSource(1500)},
		{language: "vbnet", text: generatedVBNetSource(1500)},
		{language: "python", text: generatedPythonSource(1500)},
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
			document.Path = "generated." + testCase.language
			result, err := analyzer.Analyze(context.Background(), document, AnalyzeOptions{MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: 128, MaxSignatureBytes: 4096, MaxDiagnostics: 16}})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) > 128 || !result.Analysis.Truncated || result.Analysis.CoverageComplete {
				t.Fatalf("bounded generated result = symbols=%d truncated=%v complete=%v", len(result.Analysis.Symbols), result.Analysis.Truncated, result.Analysis.CoverageComplete)
			}
		})
	}
}

func TestAnalyzerRegistryCoverageIsMechanicallyConsistent(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range defaultLanguageDescriptors() {
		descriptor, ok := registry.Resolve(expected.ID)
		if !ok {
			t.Fatalf("registry is missing %s", expected.ID)
		}
		analyzer, available := AnalyzerFor(descriptor)
		if descriptor.Capabilities.SourceAnalysis != available {
			t.Fatalf("registry/analyzer mismatch for %s: capability=%v available=%v", descriptor.ID, descriptor.Capabilities.SourceAnalysis, available)
		}
		if available && (analyzer.ID() != descriptor.Analyzer || analyzer.Language() == "") {
			t.Fatalf("analyzer identity mismatch for %s: %q/%q", descriptor.ID, analyzer.ID(), analyzer.Language())
		}
	}
}

func containsSortedString(values []string, target string) bool {
	index := sort.SearchStrings(values, target)
	return index < len(values) && values[index] == target
}

func generatedGoSource(count int) string {
	var builder strings.Builder
	builder.WriteString("package generated\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "func F%04d() {}\n", index)
	}
	return builder.String()
}

func generatedCSharpSource(count int) string {
	var builder strings.Builder
	builder.WriteString("class Generated {\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "public void F%04d() {}\n", index)
	}
	builder.WriteString("}\n")
	return builder.String()
}

func generatedVBNetSource(count int) string {
	var builder strings.Builder
	builder.WriteString("Class Generated\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "Sub F%04d(): End Sub\n", index)
	}
	builder.WriteString("End Class\n")
	return builder.String()
}

func generatedPythonSource(count int) string {
	var builder strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder, "def f%04d():\n    return %d\n", index, index)
	}
	return builder.String()
}
