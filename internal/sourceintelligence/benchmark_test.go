package sourceintelligence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func BenchmarkProjectSearchTopK(b *testing.B) {
	const total = 20_000
	matches := make([]ProjectSearchMatch, 0, total)
	for i := 0; i < total; i++ {
		index := (i * 7919) % total
		matches = append(matches, ProjectSearchMatch{
			Path: "source-" + strconv.Itoa(index%173),
			Range: Range{
				Start: Position{Line: index/173 + 1, Column: 1},
				End:   Position{Line: index/173 + 1, Column: 4},
			},
			SymbolID: strconv.Itoa(index),
		})
	}
	for _, maximum := range []int{8, 128} {
		b.Run("full-sort/"+strconv.Itoa(maximum), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result := finalizeProjectSearch(append([]ProjectSearchMatch(nil), matches...), maximum)
				if !result.Truncated || len(result.Matches) != maximum {
					b.Fatal("unexpected full sort result")
				}
			}
		})
		b.Run("top-k/"+strconv.Itoa(maximum), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				collector := newProjectSearchTopK(maximum)
				for _, match := range matches {
					collector.append(match)
				}
				result := collector.result()
				if !result.Truncated || len(result.Matches) != maximum {
					b.Fatal("unexpected bounded result")
				}
			}
		})
	}
}

func BenchmarkProjectContextRepeatedImports(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	makeFacts := func(path, text string) ProjectFileFacts {
		document := sourceDocumentForScanner(text)
		document.Path = path
		sum := sha256.Sum256([]byte(text))
		document.SourceFingerprint = hex.EncodeToString(sum[:])
		result, err := (TypeScriptAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(false, 512))
		if err != nil || !result.Analysis.CoverageComplete {
			b.Fatalf("analyze %s: %v; diagnostics=%+v", path, err, result.Analysis.Diagnostics)
		}
		return ProjectFileFacts{Path: path, Language: "typescript", SourceFingerprint: document.SourceFingerprint, Analysis: result}
	}
	var shared, client strings.Builder
	for i := 0; i < 128; i++ {
		shared.WriteString("export class Item")
		shared.WriteString(strconv.Itoa(i))
		shared.WriteString(" {}\n")
	}
	for i := 0; i < 48; i++ {
		client.WriteString("import { Item")
		client.WriteString(strconv.Itoa(i))
		client.WriteString(" } from \"./shared\";\n")
	}
	client.WriteString("export class Client {}\n")
	sharedFacts := makeFacts(filepath.Join("project", "bench", "shared.ts"), shared.String())
	clientFacts := makeFacts(filepath.Join("project", "bench", "client.ts"), client.String())
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{sharedFacts, clientFacts}, projectResolverLimitsForTest())
	if err != nil {
		b.Fatal(err)
	}
	target := queryPathSelector(clientFacts)
	options := ProjectContextOptions{BudgetBytes: 1 << 20, MaxItems: 256, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plan, err := model.PlanContext(context.Background(), []ProjectSelector{target}, options)
		if err != nil {
			b.Fatal(err)
		}
		if len(plan.Candidates) < 100 {
			b.Fatalf("expected broad dependency context; got %d symbols", len(plan.Candidates))
		}
	}
}

func BenchmarkCanaryAnalyzers(b *testing.B) {
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

func BenchmarkLargeGeneratedGo(b *testing.B) {
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

func BenchmarkLargeGeneratedGoComments(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	descriptor, _ := registry.Resolve("go")
	analyzer, _ := AnalyzerFor(descriptor)
	text := strings.ReplaceAll(
		generatedGoSource(5_000),
		"func ",
		"// generated declaration documentation with enough text to exercise comment parsing\n// second documentation line\nfunc ",
	)
	document := sourceDocumentForScanner(text)
	document.Path = "large-comments.go"
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

func BenchmarkLargeGeneratedGoSignatures(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	descriptor, _ := registry.Resolve("go")
	analyzer, _ := AnalyzerFor(descriptor)
	text := generatedGoSource(5_000)
	document := sourceDocumentForScanner(text)
	document.Path = "large-signatures.go"
	options := AnalyzeOptions{
		IncludeSignatures: true,
		MaxNesting:        256,
		Limits: SymbolBuilderLimits{
			MaxSymbols:        10_000,
			MaxSignatureBytes: 8192,
			MaxDiagnostics:    256,
		},
	}
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
		if result.Analysis.Symbols[len(result.Analysis.Symbols)-1].Signature == "" {
			b.Fatal("signature retention is disabled")
		}
	}
}
