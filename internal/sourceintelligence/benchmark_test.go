package sourceintelligence

import (
	"context"
	"testing"
)

func BenchmarkR25CanaryAnalyzers(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	cases := []struct {
		name     string
		language string
		text     string
	}{
		{name: "go", language: "go", text: generatedGoSource(400)},
		{name: "csharp", language: "csharp", text: generatedCSharpSource(400)},
		{name: "vbnet", language: "vbnet", text: generatedVBNetSource(400)},
		{name: "python", language: "python", text: generatedPythonSource(400)},
		{name: "classic-asp", language: "classic-asp", text: "<%@ Language=\"VBScript\" %>\n<% Sub Work()\nEnd Sub %>\n<html>host</html>\n"},
	}
	options := AnalyzeOptions{MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256}}
	for _, testCase := range cases {
		b.Run(testCase.name, func(b *testing.B) {
			descriptor, _ := registry.Resolve(testCase.language)
			analyzer, _ := AnalyzerFor(descriptor)
			document := sourceDocumentForScanner(testCase.text)
			document.Path = "benchmark." + testCase.language
			b.ReportAllocs()
			b.SetBytes(int64(len(testCase.text)))
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				result, err := analyzer.Analyze(context.Background(), document, options)
				if err != nil {
					b.Fatal(err)
				}
				if len(result.Analysis.Symbols) == 0 {
					b.Fatal("benchmark analyzer returned no symbols")
				}
			}
		})
	}
}

func BenchmarkR25LargeGeneratedGo(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	descriptor, _ := registry.Resolve("go")
	analyzer, _ := AnalyzerFor(descriptor)
	text := generatedGoSource(5_000)
	document := sourceDocumentForScanner(text)
	document.Path = "large.go"
	options := AnalyzeOptions{MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256}}
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		result, err := analyzer.Analyze(context.Background(), document, options)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Analysis.Symbols) != 5_001 {
			b.Fatalf("unexpected symbol count %d", len(result.Analysis.Symbols))
		}
	}
}
