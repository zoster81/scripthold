package sourceintelligence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"unsafe"
)

var cppRawStringStartBenchmarkSink int

func BenchmarkScannerDelimiterValidation(b *testing.B) {
	for _, count := range []int{3, 8, 9, 32} {
		b.Run(fmt.Sprintf("rules-%d", count), func(b *testing.B) {
			rules := make([]DelimiterRule, count)
			for index := range rules {
				rules[index] = DelimiterRule{Open: fmt.Sprintf("<%d", index), Close: fmt.Sprintf("%d>", index)}
			}
			scanner := sourceScanner{profile: ScannerProfile{Name: "delimiter-validation", Delimiters: rules}}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				if err := scanner.validateScannerProfile(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

var maskDelimitedSourceRegionsBenchmarkSink string

func BenchmarkMaskDelimitedSourceRegions(b *testing.B) {
	regions := [][2]string{{"<!--", "-->"}, {"{#", "#}"}, {"{{", "}}"}}
	cases := []struct {
		name string
		text string
	}{
		{name: "negative-heavy", text: strings.Repeat("plain markup text value=42; ", 256)},
		{name: "mixed", text: strings.Repeat("plain markup text ", 64) + "<!-- hidden -->" + strings.Repeat(" visible ", 32) + "{# secret #}" + strings.Repeat(" tail ", 32) + "{{ expression }}"},
	}
	for _, testCase := range cases {
		b.Run(testCase.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(testCase.text)))
			for iteration := 0; iteration < b.N; iteration++ {
				maskDelimitedSourceRegionsBenchmarkSink = maskDelimitedSourceRegions(testCase.text, regions)
			}
		})
	}
}

func BenchmarkSharedScannerPrimitives(b *testing.B) {
	text := strings.Repeat("namespace Demo { class Item { string Text = \"value // not comment\"; void Run() { /* comment */ Call(\"x\"); } } }\n", 512)
	document := sourceDocumentForScanner(text)
	profile := CSharpScannerProfile()
	limits := ScannerLimits{MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: 256}
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		result, err := ScanSource(context.Background(), document, profile, limits)
		if err != nil {
			b.Fatal(err)
		}
		if !result.Complete || len(result.Tokens) == 0 {
			b.Fatalf("unexpected scanner result: complete=%t tokens=%d diagnostics=%+v", result.Complete, len(result.Tokens), result.Diagnostics)
		}
	}
}

func BenchmarkSharedScannerScaling(b *testing.B) {
	const line = "namespace Demo { class Item { string Text = \"value // not comment\"; void Run() { /* comment */ Call(\"x\"); } } }\n"
	for _, repeats := range []int{64, 512, 4096} {
		text := strings.Repeat(line, repeats)
		b.Run(fmt.Sprintf("bytes-%d", len(text)), func(b *testing.B) {
			document := sourceDocumentForScanner(text)
			profile := CSharpScannerProfile()
			limits := ScannerLimits{MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: 256}
			b.ReportAllocs()
			b.SetBytes(int64(len(text)))
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				result, err := ScanSource(context.Background(), document, profile, limits)
				if err != nil {
					b.Fatal(err)
				}
				if !result.Complete || len(result.Tokens) == 0 {
					b.Fatalf("unexpected scanner scaling result: complete=%t tokens=%d diagnostics=%+v", result.Complete, len(result.Tokens), result.Diagnostics)
				}
			}
		})
	}
}

func BenchmarkScannerTokenCapacityProfiles(b *testing.B) {
	const typicalLine = "namespace Demo { class Item { string Text = \"value // not comment\"; void Run() { /* comment */ Call(\"x\"); } } }\n"
	cases := []struct {
		name string
		text string
	}{
		{name: "typical-small", text: strings.Repeat(typicalLine, 64)},
		{name: "typical-medium", text: strings.Repeat(typicalLine, 512)},
		{name: "dense-small", text: strings.Repeat("a+a;", 2048)},
		{name: "dense-medium", text: strings.Repeat("a+a;", 16_384)},
		{name: "sparse-medium", text: strings.Repeat("value ", 4096) + "/*" + strings.Repeat("x", 64*1024) + "*/\n"},
		{name: "sparse-large", text: strings.Repeat("value ", 33_000) + "/*" + strings.Repeat("x", 512*1024) + "*/\n"},
	}
	for _, testCase := range cases {
		b.Run(testCase.name, func(b *testing.B) {
			document := sourceDocumentForScanner(testCase.text)
			profile := CSharpScannerProfile()
			limits := ScannerLimits{MaxTokens: scannerTokenBudget(testCase.text), MaxTokenBytes: 1024 * 1024, MaxNesting: 256}
			sample, err := ScanSource(context.Background(), document, profile, limits)
			if err != nil {
				b.Fatal(err)
			}
			if !sample.Complete || len(sample.Tokens) == 0 {
				b.Fatalf("unexpected scanner capacity sample: complete=%t tokens=%d diagnostics=%+v", sample.Complete, len(sample.Tokens), sample.Diagnostics)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(testCase.text)))
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				result, err := ScanSource(context.Background(), document, profile, limits)
				if err != nil {
					b.Fatal(err)
				}
				if !result.Complete || len(result.Tokens) != len(sample.Tokens) {
					b.Fatalf("unexpected scanner capacity result: complete=%t tokens=%d want=%d diagnostics=%+v", result.Complete, len(result.Tokens), len(sample.Tokens), result.Diagnostics)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(initialScannerTokenCapacity(len(testCase.text), limits.MaxTokens)), "initial-cap")
			b.ReportMetric(float64(len(sample.Tokens)), "tokens")
			b.ReportMetric(float64(cap(sample.Tokens)), "token-cap")
			b.ReportMetric(float64(cap(sample.Tokens)-len(sample.Tokens)), "spare-tokens")
		})
	}
}

func BenchmarkSharedScannerSparseTail(b *testing.B) {
	text := strings.Repeat("value ", 33_000) + "/*" + strings.Repeat("x", 512*1024) + "*/\n"
	document := sourceDocumentForScanner(text)
	profile := CSharpScannerProfile()
	limits := ScannerLimits{MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: 256}
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		result, err := ScanSource(context.Background(), document, profile, limits)
		if err != nil {
			b.Fatal(err)
		}
		if !result.Complete || len(result.Tokens) == 0 {
			b.Fatalf("unexpected sparse-tail scanner result: complete=%t tokens=%d diagnostics=%+v", result.Complete, len(result.Tokens), result.Diagnostics)
		}
	}
}

func BenchmarkSharedScannerCaseInsensitiveKeywords(b *testing.B) {
	const line = "MoDuLe Alpha FuNcTiOn Beta SuBrOuTiNe Gamma\n"
	for _, repeats := range []int{1, 64, 512} {
		text := strings.Repeat(line, repeats)
		b.Run(fmt.Sprintf("bytes-%d", len(text)), func(b *testing.B) {
			document := sourceDocumentForScanner(text)
			profile := FortranScannerProfile()
			limits := ScannerLimits{MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: 256}
			b.ReportAllocs()
			b.SetBytes(int64(len(text)))
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				result, err := ScanSource(context.Background(), document, profile, limits)
				if err != nil {
					b.Fatal(err)
				}
				if !result.Complete || len(result.Tokens) == 0 {
					b.Fatalf("unexpected case-insensitive scanner result: complete=%t tokens=%d diagnostics=%+v", result.Complete, len(result.Tokens), result.Diagnostics)
				}
			}
		})
	}
}

func BenchmarkSharedScannerCaseSensitiveKeywordStrategies(b *testing.B) {
	cases := []struct {
		name    string
		profile ScannerProfile
		line    string
		repeats int
	}{
		{name: "small-set-large-source", profile: ScannerProfile{Name: "small-keywords", Keywords: []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta"}}, line: "alpha beta gamma value delta epsilon zeta eta theta value\n", repeats: 128},
		{name: "large-set-small-source", profile: CPPScannerProfile(), line: "namespace demo { class Box { int run(int value) { return value; } } }\n", repeats: 1},
		{name: "large-set-large-source", profile: CPPScannerProfile(), line: "namespace demo { class Box { int run(int value) { return value; } } }\n", repeats: 128},
	}
	for _, testCase := range cases {
		text := strings.Repeat(testCase.line, testCase.repeats)
		b.Run(testCase.name, func(b *testing.B) {
			document := sourceDocumentForScanner(text)
			limits := ScannerLimits{MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: 256}
			b.ReportAllocs()
			b.SetBytes(int64(len(text)))
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				result, err := ScanSource(context.Background(), document, testCase.profile, limits)
				if err != nil {
					b.Fatal(err)
				}
				if !result.Complete || len(result.Tokens) == 0 {
					b.Fatalf("unexpected keyword-strategy result: complete=%t tokens=%d diagnostics=%+v", result.Complete, len(result.Tokens), result.Diagnostics)
				}
			}
		})
	}
}

func BenchmarkSharedScannerDeepNesting(b *testing.B) {
	const depth = 192
	const groups = 64
	var source strings.Builder
	for group := 0; group < groups; group++ {
		source.WriteString(strings.Repeat("{", depth))
		source.WriteString("value")
		source.WriteString(strings.Repeat("}", depth))
		source.WriteByte('\n')
	}
	text := source.String()
	document := sourceDocumentForScanner(text)
	profile := CSharpScannerProfile()
	limits := ScannerLimits{MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: 256}
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		result, err := ScanSource(context.Background(), document, profile, limits)
		if err != nil {
			b.Fatal(err)
		}
		if !result.Complete || result.MaxDepth != depth {
			b.Fatalf("unexpected deep scanner result: complete=%t maxDepth=%d diagnostics=%+v", result.Complete, result.MaxDepth, result.Diagnostics)
		}
	}
}

func BenchmarkSharedLogicalLines(b *testing.B) {
	text := strings.Repeat("class Demo:\n    def work(self):\n        value = \"x\"\n        return value\n", 512)
	document := sourceDocumentForScanner(text)
	scan, err := ScanSource(context.Background(), document, PythonScannerProfile(), ScannerLimits{
		MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: 256,
	})
	if err != nil {
		b.Fatal(err)
	}
	profile := LogicalLineProfile{TrackIndentation: true, SkipDirectives: true}
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		lines := BuildLogicalLines(scan.Tokens, profile)
		if len(lines) != 2048 {
			b.Fatalf("unexpected logical line count %d", len(lines))
		}
	}
}

func BenchmarkSharedSymbolBuilder(b *testing.B) {
	const symbolCount = 2_000
	var source strings.Builder
	source.Grow(symbolCount * 12)
	specs := make([]SymbolSpec, 0, symbolCount)
	for index := 0; index < symbolCount; index++ {
		name := fmt.Sprintf("item%04d", index)
		start := source.Len()
		source.WriteString(name)
		source.WriteByte('\n')
		specs = append(specs, SymbolSpec{
			Kind: SymbolKindVariable, NativeKind: "variable", Name: name,
			Declaration: OffsetRange{Start: start, End: start + len(name)},
			NameRange:   OffsetRange{Start: start, End: start + len(name)},
		})
	}
	document := sourceDocumentForScanner(source.String())
	options := SymbolBuilderOptions{
		Language: "go", Analyzer: string(AnalyzerGo), MaxEvidence: SymbolEvidenceStructural,
		Limits: SymbolBuilderLimits{MaxSymbols: symbolCount, MaxSignatureBytes: 8192, MaxDiagnostics: 64},
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(document.Text)))
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		builder := NewSymbolBuilder(document, options)
		builder.reserveSymbols(symbolCount)
		for _, spec := range specs {
			if err := builder.addDiscard(spec); err != nil {
				b.Fatal(err)
			}
		}
		result := builder.Result()
		if len(result.Symbols) != symbolCount || !result.CoverageComplete {
			b.Fatalf("unexpected builder result: symbols=%d complete=%t", len(result.Symbols), result.CoverageComplete)
		}
	}
}

func BenchmarkSharedSymbolBuilderOptionalOffsets(b *testing.B) {
	for _, symbolCount := range []int{1, 16, 2_000} {
		var source strings.Builder
		specs := make([]SymbolSpec, 0, symbolCount)
		for index := 0; index < symbolCount; index++ {
			name := fmt.Sprintf("item%04d", index)
			start := source.Len()
			source.WriteString(name)
			source.WriteString(" value\n")
			nameEnd := start + len(name)
			end := source.Len() - 1
			signature := OffsetRange{Start: start, End: nameEnd}
			body := OffsetRange{Start: nameEnd + 1, End: end}
			specs = append(specs, SymbolSpec{
				Kind: SymbolKindFunction, NativeKind: "function", Name: name,
				Declaration: OffsetRange{Start: start, End: end},
				NameRange:   OffsetRange{Start: start, End: nameEnd},
				Signature:   &signature, Body: &body,
			})
		}
		document := sourceDocumentForScanner(source.String())
		options := SymbolBuilderOptions{
			Language: "go", Analyzer: string(AnalyzerGo), MaxEvidence: SymbolEvidenceStructural,
			Limits: SymbolBuilderLimits{MaxSymbols: symbolCount, MaxSignatureBytes: 8192, MaxDiagnostics: 64},
		}
		b.Run(fmt.Sprintf("symbols-%d", symbolCount), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				builder := NewSymbolBuilder(document, options)
				builder.reserveSymbols(symbolCount)
				for _, spec := range specs {
					if err := builder.addDiscard(spec); err != nil {
						b.Fatal(err)
					}
				}
				result := builder.takeResult()
				if len(result.Symbols) != symbolCount || !result.CoverageComplete {
					b.Fatalf("unexpected optional-offset result: symbols=%d complete=%t", len(result.Symbols), result.CoverageComplete)
				}
			}
		})
	}
}

func BenchmarkSharedSymbolBuilderScaling(b *testing.B) {
	for _, symbolCount := range []int{250, 2_000, 8_000} {
		var source strings.Builder
		source.Grow(symbolCount * 12)
		specs := make([]SymbolSpec, 0, symbolCount)
		for index := 0; index < symbolCount; index++ {
			name := fmt.Sprintf("item%04d", index)
			start := source.Len()
			source.WriteString(name)
			source.WriteByte('\n')
			specs = append(specs, SymbolSpec{
				Kind: SymbolKindVariable, NativeKind: "variable", Name: name,
				Declaration: OffsetRange{Start: start, End: start + len(name)},
				NameRange:   OffsetRange{Start: start, End: start + len(name)},
			})
		}
		document := sourceDocumentForScanner(source.String())
		options := SymbolBuilderOptions{
			Language: "go", Analyzer: string(AnalyzerGo), MaxEvidence: SymbolEvidenceStructural,
			Limits: SymbolBuilderLimits{MaxSymbols: symbolCount, MaxSignatureBytes: 8192, MaxDiagnostics: 64},
		}
		b.Run(fmt.Sprintf("symbols-%d", symbolCount), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(document.Text)))
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				builder := NewSymbolBuilder(document, options)
				builder.reserveSymbols(symbolCount)
				for _, spec := range specs {
					if err := builder.addDiscard(spec); err != nil {
						b.Fatal(err)
					}
				}
				result := builder.Result()
				if len(result.Symbols) != symbolCount || !result.CoverageComplete {
					b.Fatalf("unexpected builder scaling result: symbols=%d complete=%t", len(result.Symbols), result.CoverageComplete)
				}
			}
		})
	}
}

func BenchmarkSharedSymbolBuilderDuplicateTracking(b *testing.B) {
	for _, symbolCount := range []int{1, 4, 8, 9, 64} {
		var source strings.Builder
		specs := make([]SymbolSpec, 0, symbolCount)
		for index := 0; index < symbolCount; index++ {
			name := fmt.Sprintf("item%04d", index)
			start := source.Len()
			source.WriteString(name)
			source.WriteByte('\n')
			specs = append(specs, SymbolSpec{
				Kind: SymbolKindVariable, NativeKind: "variable", Name: name,
				Declaration: OffsetRange{Start: start, End: start + len(name)},
				NameRange:   OffsetRange{Start: start, End: start + len(name)},
			})
		}
		document := sourceDocumentForScanner(source.String())
		options := SymbolBuilderOptions{
			Language: "go", Analyzer: string(AnalyzerGo), MaxEvidence: SymbolEvidenceStructural,
			Limits: SymbolBuilderLimits{MaxSymbols: symbolCount, MaxSignatureBytes: 8192, MaxDiagnostics: 64},
		}
		b.Run(fmt.Sprintf("symbols-%d", symbolCount), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				builder := NewSymbolBuilder(document, options)
				for _, spec := range specs {
					if err := builder.addDiscard(spec); err != nil {
						b.Fatal(err)
					}
				}
				result := builder.takeResult()
				if len(result.Symbols) != symbolCount || !result.CoverageComplete {
					b.Fatalf("unexpected duplicate-tracking result: symbols=%d complete=%t", len(result.Symbols), result.CoverageComplete)
				}
			}
		})
	}
}

func BenchmarkASCIILowerPreservingBytes(b *testing.B) {
	cases := []struct {
		name string
		text string
	}{
		{name: "lowercase", text: strings.Repeat("blade-template-é-", 256)},
		{name: "mixed", text: strings.Repeat("Blade-TEMPLATE-é-", 256)},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.text)))
			for iteration := 0; iteration < b.N; iteration++ {
				asciiLowerAllocationSink = asciiLowerPreservingBytes(tc.text)
			}
		})
	}
}

func BenchmarkSharedCompositeSegmentation(b *testing.B) {
	text := strings.Repeat("<div>host</div><% class Demo { void Run() {} } %>{{ value }}\n", 512)
	document := sourceDocumentForScanner(text)
	profile := CompositeProfile{
		HostKind: "host", HostLanguage: "html",
		Rules: []CompositeDelimiterRule{
			{Open: "<%", Close: "%>", Kind: "server", Language: "csharp"},
			{Open: "{{", Close: "}}", Kind: "expression", Language: "template"},
		},
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		segments, complete, err := SegmentCompositeSource(context.Background(), document, profile, 4096)
		if err != nil {
			b.Fatal(err)
		}
		if !complete || len(segments) == 0 {
			b.Fatalf("unexpected composite result: complete=%t segments=%d", complete, len(segments))
		}
	}
}

func BenchmarkSharedProjectResolver(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	facts := benchmarkJavaProjectFacts(b, 128)
	limits := ProjectResolverLimits{
		MaxFiles: len(facts), MaxSymbols: 4096, MaxDependencies: 4096, MaxReferences: 4096, MaxCandidatesPerResolution: 64,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		model, err := BuildProjectModel(context.Background(), registry, facts, limits)
		if err != nil {
			b.Fatal(err)
		}
		if len(model.References()) == 0 {
			b.Fatal("project resolver returned no references")
		}
	}
}

func BenchmarkSharedProjectResolverScaling(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	for _, children := range []int{16, 64, 256} {
		facts := benchmarkJavaProjectFacts(b, children)
		limits := ProjectResolverLimits{
			MaxFiles: len(facts), MaxSymbols: 8192, MaxDependencies: 8192, MaxReferences: 8192, MaxCandidatesPerResolution: 64,
		}
		b.Run(fmt.Sprintf("files-%d", len(facts)), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				model, err := BuildProjectModel(context.Background(), registry, facts, limits)
				if err != nil {
					b.Fatal(err)
				}
				if len(model.References()) == 0 {
					b.Fatal("project resolver scaling returned no references")
				}
			}
		})
	}
}

// BenchmarkSharedProjectResolverBuildAndSnapshotScaling separates immutable
// project-model construction from the public, deep-copied reference snapshot.
func BenchmarkSharedProjectResolverBuildAndSnapshotScaling(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	for _, children := range []int{16, 64, 256} {
		facts := benchmarkJavaProjectFacts(b, children)
		limits := ProjectResolverLimits{
			MaxFiles: len(facts), MaxSymbols: 8192, MaxDependencies: 8192, MaxReferences: 8192, MaxCandidatesPerResolution: 64,
		}
		b.Run(fmt.Sprintf("files-%d/build-only", len(facts)), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				model, err := BuildProjectModel(context.Background(), registry, facts, limits)
				if err != nil {
					b.Fatal(err)
				}
				if len(model.references) == 0 {
					b.Fatal("project model returned no references")
				}
			}
		})
		b.Run(fmt.Sprintf("files-%d/references-snapshot", len(facts)), func(b *testing.B) {
			model, err := BuildProjectModel(context.Background(), registry, facts, limits)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				snapshot := model.References()
				if len(snapshot) == 0 {
					b.Fatal("public reference snapshot is empty")
				}
			}
		})
	}
}

func BenchmarkSharedProjectIndexRefresh(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	facts := benchmarkJavaProjectFacts(b, 64)
	snapshots := make([]ProjectIndexFileSnapshot, 0, len(facts))
	factsByPath := make(map[string]ProjectFileFacts, len(facts))
	for _, fact := range facts {
		snapshots = append(snapshots, ProjectIndexFileSnapshot{Path: fact.Path, SourceFingerprint: fact.SourceFingerprint})
		factsByPath[fact.Path] = fact
	}
	resolverLimits := ProjectResolverLimits{
		MaxFiles: len(facts), MaxSymbols: 4096, MaxDependencies: 4096, MaxReferences: 4096, MaxCandidatesPerResolution: 64,
	}
	analysisFingerprint, err := ProjectIndexAnalysisFingerprint(registry, ProjectIndexAnalysisConfig{
		MaxFileBytes: 8 * 1024 * 1024, MaxDecodedCharacters: 16 * 1024 * 1024,
		MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256, MaxDetectorProbes: 4,
		MaxNesting: 256, MaxProjectEdges: 4096, IncludeSignatures: false,
	})
	if err != nil {
		b.Fatal(err)
	}
	options := ProjectIndexRefreshOptions{
		ScopeFingerprint: benchmarkDigest("shared-project-index-scope"), AnalysisFingerprint: analysisFingerprint,
		Snapshots: snapshots, ResolverLimits: resolverLimits,
		Analyze: func(_ context.Context, snapshot ProjectIndexFileSnapshot) (ProjectIndexAnalysisResult, error) {
			fact, ok := factsByPath[snapshot.Path]
			if !ok {
				return ProjectIndexAnalysisResult{}, fmt.Errorf("missing benchmark fact %s", snapshot.Path)
			}
			copy := fact
			return ProjectIndexAnalysisResult{ObservedFingerprint: snapshot.SourceFingerprint, Facts: &copy}, nil
		},
	}

	b.Run("cold", func(b *testing.B) {
		b.ReportAllocs()
		for iteration := 0; iteration < b.N; iteration++ {
			manager, err := NewProjectIndexManager(ProjectIndexManagerLimits{MaxProjects: 1, MaxGenerations: 2})
			if err != nil {
				b.Fatal(err)
			}
			selection, err := manager.Refresh(context.Background(), registry, options)
			if err != nil {
				b.Fatal(err)
			}
			if selection.Coverage.FilesParsed != len(facts) || !selection.Coverage.CoverageComplete {
				b.Fatalf("unexpected cold coverage: %+v", selection.Coverage)
			}
		}
	})

	b.Run("warm", func(b *testing.B) {
		manager, err := NewProjectIndexManager(ProjectIndexManagerLimits{MaxProjects: 1, MaxGenerations: 2})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := manager.Refresh(context.Background(), registry, options); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			selection, err := manager.Refresh(context.Background(), registry, options)
			if err != nil {
				b.Fatal(err)
			}
			if selection.Coverage.FilesParsed != len(facts) || !selection.Coverage.CoverageComplete {
				b.Fatalf("unexpected warm coverage: %+v", selection.Coverage)
			}
		}
	})
}

func BenchmarkSharedProjectIndexPartialRefresh(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	facts := benchmarkJavaProjectFacts(b, 64)
	snapshotsA := make([]ProjectIndexFileSnapshot, 0, len(facts))
	factsByPath := make(map[string]ProjectFileFacts, len(facts))
	for _, fact := range facts {
		snapshotsA = append(snapshotsA, ProjectIndexFileSnapshot{Path: fact.Path, SourceFingerprint: fact.SourceFingerprint})
		factsByPath[fact.Path] = fact
	}
	snapshotsB := append([]ProjectIndexFileSnapshot(nil), snapshotsA...)
	snapshotsB[0].SourceFingerprint = benchmarkDigest("partial-refresh-alternate-base")
	resolverLimits := ProjectResolverLimits{
		MaxFiles: len(facts), MaxSymbols: 4096, MaxDependencies: 4096, MaxReferences: 4096, MaxCandidatesPerResolution: 64,
	}
	analysisFingerprint, err := ProjectIndexAnalysisFingerprint(registry, ProjectIndexAnalysisConfig{
		MaxFileBytes: 8 * 1024 * 1024, MaxDecodedCharacters: 16 * 1024 * 1024,
		MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256, MaxDetectorProbes: 4,
		MaxNesting: 256, MaxProjectEdges: 4096, IncludeSignatures: false,
	})
	if err != nil {
		b.Fatal(err)
	}
	manager, err := NewProjectIndexManager(ProjectIndexManagerLimits{MaxProjects: 1, MaxGenerations: 2})
	if err != nil {
		b.Fatal(err)
	}
	options := ProjectIndexRefreshOptions{
		ScopeFingerprint: benchmarkDigest("shared-project-index-partial-scope"), AnalysisFingerprint: analysisFingerprint,
		Snapshots: snapshotsA, ResolverLimits: resolverLimits,
		Analyze: func(_ context.Context, snapshot ProjectIndexFileSnapshot) (ProjectIndexAnalysisResult, error) {
			fact, ok := factsByPath[snapshot.Path]
			if !ok {
				return ProjectIndexAnalysisResult{}, fmt.Errorf("missing benchmark fact %s", snapshot.Path)
			}
			fact.SourceFingerprint = snapshot.SourceFingerprint
			return ProjectIndexAnalysisResult{ObservedFingerprint: snapshot.SourceFingerprint, Facts: &fact}, nil
		},
	}
	if _, err := manager.Refresh(context.Background(), registry, options); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if iteration&1 == 0 {
			options.Snapshots = snapshotsB
		} else {
			options.Snapshots = snapshotsA
		}
		selection, err := manager.Refresh(context.Background(), registry, options)
		if err != nil {
			b.Fatal(err)
		}
		if selection.Stats.AnalyzedFiles != 1 || selection.Stats.ReusedFiles != len(facts)-1 {
			b.Fatalf("unexpected partial refresh stats: %+v", selection.Stats)
		}
	}
}

func BenchmarkSharedProjectQueryAndContext(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	facts := benchmarkJavaProjectFacts(b, 128)
	model, err := BuildProjectModel(context.Background(), registry, facts, ProjectResolverLimits{
		MaxFiles: len(facts), MaxSymbols: 4096, MaxDependencies: 4096, MaxReferences: 4096, MaxCandidatesPerResolution: 64,
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Run("search", func(b *testing.B) {
		b.ReportAllocs()
		for iteration := 0; iteration < b.N; iteration++ {
			result, err := model.StructuralSearch(context.Background(), ProjectSearchOptions{Query: "Child", Match: ProjectSearchPrefix, MaxResults: 256})
			if err != nil {
				b.Fatal(err)
			}
			if len(result.Matches) == 0 {
				b.Fatal("project structural search returned no matches")
			}
		}
	})
	b.Run("context", func(b *testing.B) {
		target := facts[len(facts)-1]
		selector := ProjectSelector{Kind: ProjectSelectorPath, Path: target.Path, SourceFingerprint: target.SourceFingerprint}
		options := ProjectContextOptions{BudgetBytes: 64 * 1024, MaxItems: 64, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly}
		b.ReportAllocs()
		for iteration := 0; iteration < b.N; iteration++ {
			plan, err := model.PlanContext(context.Background(), []ProjectSelector{selector}, options)
			if err != nil {
				b.Fatal(err)
			}
			if len(plan.Candidates) == 0 {
				b.Fatal("project context returned no candidates")
			}
		}
	})
}

// BenchmarkProjectDenseSameFileResolution separates the per-reference source
// lookup from the full model build for files with many local declarations.
func BenchmarkProjectDenseSameFileResolution(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	descriptor, ok := registry.Resolve("java")
	if !ok {
		b.Fatal("java descriptor is unavailable")
	}
	for _, children := range []int{16, 64, 256} {
		b.Run(fmt.Sprintf("classes-%d", children), func(b *testing.B) {
			var source strings.Builder
			source.WriteString("package demo;\nclass Base {}\n")
			for index := 0; index < children; index++ {
				fmt.Fprintf(&source, "class Child%03d extends Base {}\n", index)
			}
			facts := benchmarkAnalyzerFacts(b, JavaAnalyzer{}, "project/benchmark/Dense.java", source.String())
			if len(facts.Analysis.Relations) != children {
				b.Fatalf("analyzer relationships: got %d want %d", len(facts.Analysis.Relations), children)
			}
			limits := ProjectResolverLimits{MaxFiles: 1, MaxSymbols: 4096, MaxDependencies: 4096, MaxReferences: 4096, MaxCandidatesPerResolution: 64}
			b.Run("model-build", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{facts}, limits)
					if err != nil || len(model.references) != children {
						b.Fatalf("project build: references=%d err=%v", len(model.references), err)
					}
				}
			})
			model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{facts}, limits)
			if err != nil {
				b.Fatal(err)
			}
			file := model.files[projectPathKey(facts.Path)]
			relation := facts.Analysis.Relations[len(facts.Analysis.Relations)-1]
			target := projectSymbolLookupKey(descriptor, relation.Target)
			parent := qualifiedParent(projectSymbolLookupKey(descriptor, relation.Source))
			if got := model.sameFileCandidates(file, descriptor, parent, target); len(got) != 1 {
				b.Fatalf("unexpected same-file targets: %d", len(got))
			}
			b.Run("same-file-candidates", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					if got := model.sameFileCandidates(file, descriptor, parent, target); len(got) != 1 {
						b.Fatalf("unexpected same-file targets: %d", len(got))
					}
				}
			})
			if got := model.resolveRelationSource(file, descriptor, relation, nil); got.SymbolID == "" {
				b.Fatalf("last source could not be resolved: %+v", got)
			}
			b.Run("source-lookup", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					if got := model.resolveRelationSource(file, descriptor, relation, nil); got.SymbolID == "" {
						b.Fatal("source lookup missing symbol")
					}
				}
			})
		})
	}
}

// BenchmarkProjectSymbolIDContextScaling isolates repeated symbol-ID lookups
// from full context planning and accounts for model construction overhead.
func BenchmarkProjectSymbolIDContextScaling(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	for _, children := range []int{16, 64, 256} {
		b.Run(fmt.Sprintf("classes-%d", children), func(b *testing.B) {
			var source strings.Builder
			source.WriteString("package demo;\nclass Base {}\n")
			for index := 0; index < children; index++ {
				fmt.Fprintf(&source, "class Child%03d extends Base {}\n", index)
			}
			facts := benchmarkAnalyzerFacts(b, JavaAnalyzer{}, "project/benchmark/DenseID.java", source.String())
			limits := ProjectResolverLimits{MaxFiles: 1, MaxSymbols: 4096, MaxDependencies: 4096, MaxReferences: 4096, MaxCandidatesPerResolution: 64}
			b.Run("model-build", func(b *testing.B) {
				b.ReportAllocs()
				for iteration := 0; iteration < b.N; iteration++ {
					model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{facts}, limits)
					if err != nil || len(model.references) != children {
						b.Fatalf("model build references=%d err=%v", len(model.references), err)
					}
				}
			})
			model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{facts}, limits)
			if err != nil {
				b.Fatal(err)
			}
			pathKey := projectPathKey(facts.Path)
			records := model.symbolsByFile[pathKey]
			if len(records) < children+1 {
				b.Fatalf("analyzer returned %d symbols, expected >= %d", len(records), children+1)
			}
			var baseID, lastID string
			for _, record := range records {
				switch record.symbol.QualifiedName {
				case "demo.Base":
					baseID = record.symbol.ID
				case fmt.Sprintf("demo.Child%03d", children-1):
					lastID = record.symbol.ID
				}
			}
			if baseID == "" || lastID == "" {
				b.Fatalf("missing Base or late-child ID: base=%q child=%q", baseID, lastID)
			}
			last := ProjectSelector{Kind: ProjectSelectorSymbol, Path: facts.Path, SourceFingerprint: facts.SourceFingerprint, SymbolID: lastID}
			base := ProjectSelector{Kind: ProjectSelectorSymbol, Path: facts.Path, SourceFingerprint: facts.SourceFingerprint, SymbolID: baseID}
			options := ProjectContextOptions{BudgetBytes: 1024 * 1024, MaxItems: children + 8, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly}
			if seeds, err := model.contextSeedsForSelector(last); err != nil || len(seeds) != 1 {
				b.Fatalf("late symbol selector seeds=%d err=%v", len(seeds), err)
			}
			if plan, err := model.PlanContext(context.Background(), []ProjectSelector{base}, options); err != nil || len(plan.Candidates) < children {
				b.Fatalf("base context candidates=%d err=%v", len(plan.Candidates), err)
			}
			b.Run("id-lookup", func(b *testing.B) {
				b.ReportAllocs()
				for iteration := 0; iteration < b.N; iteration++ {
					if _, ok := model.contextSymbolByID(pathKey, lastID); !ok {
						b.Fatal("late symbol not found")
					}
				}
			})
			b.Run("query-symbol", func(b *testing.B) {
				b.ReportAllocs()
				for iteration := 0; iteration < b.N; iteration++ {
					if _, err := model.resolveProjectSelector(last); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("context-selector", func(b *testing.B) {
				b.ReportAllocs()
				for iteration := 0; iteration < b.N; iteration++ {
					if _, err := model.contextSeedsForSelector(last); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("context-plan-base", func(b *testing.B) {
				b.ReportAllocs()
				for iteration := 0; iteration < b.N; iteration++ {
					if _, err := model.PlanContext(context.Background(), []ProjectSelector{base}, options); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkProjectPositionSelectorModelBuild includes immutable model setup
// to account for the memory and CPU cost of any new lookup index.
func BenchmarkProjectPositionSelectorModelBuild(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	for _, children := range []int{16, 64, 256} {
		b.Run(fmt.Sprintf("files-%d", children+1), func(b *testing.B) {
			facts := benchmarkJavaProjectFacts(b, children)
			limits := ProjectResolverLimits{
				MaxFiles: len(facts), MaxSymbols: 4096, MaxDependencies: 4096,
				MaxReferences: 4096, MaxCandidatesPerResolution: 64,
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				model, err := BuildProjectModel(context.Background(), registry, facts, limits)
				if err != nil {
					b.Fatal(err)
				}
				if len(model.references) == 0 {
					b.Fatal("model build returned no references")
				}
			}
		})
	}
}

func BenchmarkProjectPositionSelectorScaling(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	for _, children := range []int{16, 64, 256} {
		b.Run(fmt.Sprintf("files-%d", children+1), func(b *testing.B) {
			facts := benchmarkJavaProjectFacts(b, children)
			model, err := BuildProjectModel(context.Background(), registry, facts, ProjectResolverLimits{
				MaxFiles: len(facts), MaxSymbols: 4096, MaxDependencies: 4096,
				MaxReferences: 4096, MaxCandidatesPerResolution: 64,
			})
			if err != nil {
				b.Fatal(err)
			}
			if len(model.references) == 0 {
				b.Fatal("position benchmark needs analyzer-proven references")
			}
			reference := model.references[len(model.references)-1]
			file, ok := model.files[projectPathKey(reference.Source.Path)]
			if !ok || reference.Range.Start.Line <= 0 || reference.Range.Start.Column <= 0 {
				b.Fatalf("invalid reference fixture: %+v", reference)
			}
			position := reference.Range.Start
			selector := ProjectSelector{
				Kind: ProjectSelectorPosition, Path: file.facts.Path,
				SourceFingerprint: file.facts.SourceFingerprint, Position: &position,
			}
			if resolved, err := model.resolveProjectSelector(selector); err != nil || resolved.reference == nil {
				b.Fatalf("position selector fixture is not a reference: %+v, err=%v", resolved, err)
			}
			if seeds, err := model.contextSeedsForSelector(selector); err != nil || len(seeds) == 0 {
				b.Fatalf("context selector fixture failed: seeds=%d err=%v", len(seeds), err)
			}
			b.Run("relations", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					selected, err := model.resolveProjectSelector(selector)
					if err != nil || selected.reference == nil {
						b.Fatalf("relation position selector failed: %+v err=%v", selected, err)
					}
				}
			})
			b.Run("context", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					seeds, err := model.contextSeedsForSelector(selector)
					if err != nil || len(seeds) == 0 {
						b.Fatalf("context position selector failed: seeds=%d err=%v", len(seeds), err)
					}
				}
			})
		})
	}
}

func BenchmarkCPPRawStringStart(b *testing.B) {
	negative := strings.Repeat("ordinary_identifier + value; ", 64)
	matches := []string{`R"tag(`, `LR"tag(`, `UR"tag(`, `uR"tag(`, `u8R"tag(`}
	b.Run("negative-heavy", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(negative)))
		for iteration := 0; iteration < b.N; iteration++ {
			total := 0
			for index := 0; index < len(negative); index++ {
				prefix, delimiter, ok := cppRawStringStart(negative, index)
				if ok {
					total += prefix + delimiter
				}
			}
			cppRawStringStartBenchmarkSink = total
		}
	})
	b.Run("all-prefixes", func(b *testing.B) {
		b.ReportAllocs()
		for iteration := 0; iteration < b.N; iteration++ {
			total := 0
			for _, text := range matches {
				prefix, delimiter, ok := cppRawStringStart(text, 0)
				if !ok {
					b.Fatal("raw string prefix was not recognized")
				}
				total += prefix + delimiter
			}
			cppRawStringStartBenchmarkSink = total
		}
	})
}

func BenchmarkRepresentativeFamilyAnalyzers(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	cases := []struct {
		name     string
		language string
		path     string
		text     string
	}{
		{name: "c-family", language: "cpp", path: "bench.cpp", text: "#include <vector>\nnamespace demo { class Box { public: int run(int value) { return value; } }; }\n"},
		{name: "lisp", language: "common-lisp", path: "bench.lisp", text: "(defpackage :demo (:use :cl))\n(in-package :demo)\n(defun run (value) value)\n"},
		{name: "scientific", language: "matlab", path: "bench.m", text: "function y = run(x)\n  y = x + 1;\nend\n"},
		{name: "shell", language: "bash", path: "bench.sh", text: "#!/usr/bin/env bash\nsource ./lib.sh\nrun() { echo \"$1\"; }\n"},
		{name: "template", language: "blade", path: "bench.blade.php", text: "@extends('layouts.app')\n@section('content')\n<div>{{ $value }}</div>\n@endsection\n"},
		{name: "data-config", language: "json", path: "bench.json", text: "{\"name\":\"demo\",\"items\":[{\"id\":1},{\"id\":2}]}\n"},
		{name: "hardware", language: "systemverilog", path: "bench.sv", text: "module demo(input logic clk); always_ff @(posedge clk) begin end endmodule\n"},
	}
	options := AnalyzeOptions{IncludeSignatures: true, MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256}}
	for _, testCase := range cases {
		b.Run(testCase.name, func(b *testing.B) {
			descriptor, ok := registry.Resolve(testCase.language)
			if !ok {
				b.Fatalf("language %s is not registered", testCase.language)
			}
			analyzer, ok := AnalyzerFor(descriptor)
			if !ok {
				b.Fatalf("language %s has no analyzer", testCase.language)
			}
			document := sourceDocumentForScanner(testCase.text)
			document.Path = testCase.path
			b.ReportAllocs()
			b.SetBytes(int64(len(testCase.text)))
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				result, err := analyzer.Analyze(context.Background(), document, options)
				if err != nil {
					b.Fatal(err)
				}
				if len(result.Analysis.Symbols) == 0 {
					b.Fatalf("%s analyzer returned no symbols: %+v", testCase.language, result.Analysis.Diagnostics)
				}
			}
		})
	}
}

// BenchmarkJVMAnalyzerDeclarationScaling measures full Java/Kotlin analysis,
// including the final owned or defensive symbol result, at fixed source sizes.
func BenchmarkJVMAnalyzerDeclarationScaling(b *testing.B) {
	for _, language := range []string{"java", "kotlin"} {
		for _, declarationCount := range []int{16, 64, 256} {
			b.Run(fmt.Sprintf("%s/%d", language, declarationCount), func(b *testing.B) {
				var source strings.Builder
				source.WriteString("package bench;\n")
				var analyzer SourceAnalyzer = JavaAnalyzer{}
				if language == "kotlin" {
					source.Reset()
					source.WriteString("package bench\n")
					analyzer = KotlinAnalyzer{}
				}
				for index := 0; index < declarationCount; index++ {
					if language == "kotlin" {
						fmt.Fprintf(&source, "class Item%03d { val value: Int = %d; fun get(): Int = value }\n", index, index)
					} else {
						fmt.Fprintf(&source, "public class Item%03d { private int value = %d; public int get() { return value; } }\n", index, index)
					}
				}
				document := sourceDocumentForScanner(source.String())
				document.Path = "benchmark.java"
				if language == "kotlin" {
					document.Path = "benchmark.kt"
				}
				options := AnalyzeOptions{
					IncludeSignatures: true, MaxNesting: 256,
					Limits: SymbolBuilderLimits{MaxSymbols: 4096, MaxSignatureBytes: 8192, MaxDiagnostics: 256},
				}
				reference, err := analyzer.Analyze(context.Background(), document, options)
				if err != nil || !reference.Analysis.CoverageComplete || len(reference.Analysis.Symbols) < declarationCount {
					b.Fatalf("seed analysis symbols=%d complete=%t err=%v", len(reference.Analysis.Symbols), reference.Analysis.CoverageComplete, err)
				}
				wantCount := len(reference.Analysis.Symbols)
				b.ReportAllocs()
				b.SetBytes(int64(len(document.Text)))
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					result, err := analyzer.Analyze(context.Background(), document, options)
					if err != nil || !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != wantCount {
						b.Fatalf("analysis symbols=%d want=%d complete=%t err=%v", len(result.Analysis.Symbols), wantCount, result.Analysis.CoverageComplete, err)
					}
				}
			})
		}
	}
}

func BenchmarkHDLSignalExtraction(b *testing.B) {
	var source strings.Builder
	source.WriteString("module demo;\n")
	for index := 0; index < 256; index++ {
		fmt.Fprintf(&source, "logic signal_%03d;\n", index)
	}
	source.WriteString("endmodule\n")
	document := sourceDocumentForScanner(source.String())
	document.Path = "bench.sv"
	analyzer := SystemVerilogAnalyzer{}
	options := AnalyzeOptions{IncludeSignatures: true, MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256}}
	b.ReportAllocs()
	b.SetBytes(int64(len(document.Text)))
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		result, err := analyzer.Analyze(context.Background(), document, options)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Analysis.Symbols) != 257 || !result.Analysis.CoverageComplete {
			b.Fatalf("unexpected SystemVerilog result: symbols=%d complete=%t", len(result.Analysis.Symbols), result.Analysis.CoverageComplete)
		}
	}
}

func BenchmarkBladeDeclarationRetention(b *testing.B) {
	for _, declarationCount := range []int{1, 2, 4, 8, 16, 32, 64, 128} {
		b.Run(fmt.Sprintf("declarations-%d", declarationCount), func(b *testing.B) {
			var source strings.Builder
			for index := 0; index < declarationCount; index++ {
				fmt.Fprintf(&source, "@section('section%03d')\n", index)
			}
			document := sourceDocumentForScanner(source.String())
			document.Path = "bench.blade.php"
			options := SymbolBuilderOptions{
				Language: "blade", Analyzer: string(AnalyzerBlade), MaxEvidence: SymbolEvidenceStructural,
				Limits: SymbolBuilderLimits{MaxSymbols: 512, MaxSignatureBytes: 8192, MaxDiagnostics: 64},
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(document.Text)))
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				builder := NewSymbolBuilder(document, options)
				if _, err := addBladeDeclarations(builder, document, document.Text); err != nil {
					b.Fatal(err)
				}
				result := builder.takeResult()
				if len(result.Symbols) != declarationCount || !result.CoverageComplete {
					b.Fatalf("unexpected Blade declaration result: symbols=%d complete=%t", len(result.Symbols), result.CoverageComplete)
				}
			}
		})
	}
}

func BenchmarkSharedDelimiterPairing(b *testing.B) {
	text := strings.Repeat("call(alpha[beta{gamma(delta)}], other);\n", 1024)
	profile := CSharpScannerProfile()
	scan, err := ScanSource(context.Background(), sourceDocumentForScanner(text), profile, ScannerLimits{
		MaxTokens: scannerTokenBudget(text), MaxTokenBytes: 1024 * 1024, MaxNesting: 256,
	})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		pairs := PairDelimiterTokens(scan.Tokens, profile.Delimiters)
		if len(pairs) == 0 {
			b.Fatal("delimiter pairing returned no pairs")
		}
	}
}

func BenchmarkSharedDelimiterPairingSparse(b *testing.B) {
	tokens := make([]Token, 32*1024)
	for index := range tokens {
		tokens[index] = Token{Kind: TokenIdentifier, Text: "value"}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		pairs := PairDelimiterTokens(tokens, nil)
		if len(pairs) != 0 {
			b.Fatalf("sparse delimiter pairing returned %d entries", len(pairs))
		}
	}
}

func BenchmarkSharedConditionalSelections(b *testing.B) {
	const groupCount = 24
	groups := make([]conditionalGroup, groupCount)
	for index := range groups {
		groups[index] = conditionalGroup{
			parentGroup: -1, parentBranch: -1,
			branches:       []conditionalBranch{{start: index * 4, end: index*4 + 1}, {start: index*4 + 2, end: index*4 + 3}},
			conditionKey:   fmt.Sprintf("BENCH_%02d", index),
			conditionState: []int8{1, -1},
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		selections, ok := conditionalSelections(groups)
		if !ok || len(selections) == 0 || len(selections) > conditionalVariantLimit {
			b.Fatalf("unexpected conditional selections: ok=%t variants=%d", ok, len(selections))
		}
	}
}

// BenchmarkRepeatedStructuralDependencyInsertion measures the per-statement
// deduplication path separately from a single batch merge.
func BenchmarkRepeatedStructuralDependencyInsertion(b *testing.B) {
	document := testSourceDocument("benchmark/dependencies.go", "dependencies")
	for _, count := range []int{16, 128, 512} {
		b.Run(fmt.Sprintf("unique_%d", count), func(b *testing.B) {
			values := make([]string, count)
			for index := range values {
				values[index] = fmt.Sprintf("module/%04d", index)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				dependencies := make([]StructuralDependency, 0, count)
				for _, value := range values {
					addStructuralDependency(document, &dependencies, StructuralDependencyImport, value, 0, 1)
				}
				if len(dependencies) != count {
					b.Fatalf("unexpected dependencies: got %d, want %d", len(dependencies), count)
				}
			}
		})
	}
}

func BenchmarkSharedDependencyMerge(b *testing.B) {
	const dependencyCount = 512
	base := make([]StructuralDependency, 0, dependencyCount)
	extra := make([]StructuralDependency, 0, dependencyCount)
	for index := 0; index < dependencyCount; index++ {
		base = append(base, StructuralDependency{Kind: StructuralDependencyImport, Value: fmt.Sprintf("module/base/%04d", index), Evidence: SymbolEvidenceStructural})
		extraIndex := index + dependencyCount/2
		extra = append(extra, StructuralDependency{Kind: StructuralDependencyImport, Value: fmt.Sprintf("module/base/%04d", extraIndex), Evidence: SymbolEvidenceStructural})
	}
	const expected = dependencyCount + dependencyCount/2
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		merged := appendUniqueDependencies(base, extra)
		if len(merged) != expected {
			b.Fatalf("unexpected dependency merge count %d, want %d", len(merged), expected)
		}
	}
}

func BenchmarkSharedLanguageDetection(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	cases := []struct {
		name     string
		input    DetectionInput
		expected string
	}{
		{name: "explicit", input: DetectionInput{Path: "source.txt", Text: "package main\nfunc main() {}\n", ExplicitLanguage: "go"}, expected: "go"},
		{name: "extension", input: DetectionInput{Path: "main.go", Text: "package main\nfunc main() {}\n"}, expected: "go"},
		{name: "shebang", input: DetectionInput{Path: "script", Text: "#!/usr/bin/env python3\ndef main():\n    return 1\n"}, expected: "python"},
	}
	for _, testCase := range cases {
		b.Run(testCase.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				result, err := DetectLanguage(context.Background(), registry, testCase.input)
				if err != nil {
					b.Fatal(err)
				}
				if result.Language != testCase.expected {
					b.Fatalf("detected %q, want %q: %+v", result.Language, testCase.expected, result)
				}
			}
		})
	}
}

// BenchmarkProjectContextOutgoingReferenceScaling measures the complete
// context plan separately for narrow and broad inheritance targets. Facts are
// analyzer-proven and model construction occurs outside timed queries.
func BenchmarkProjectContextOutgoingReferenceScaling(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	for _, children := range []int{16, 64, 256} {
		b.Run(fmt.Sprintf("files-%d", children+1), func(b *testing.B) {
			facts := benchmarkJavaProjectFacts(b, children)
			model, err := BuildProjectModel(context.Background(), registry, facts, ProjectResolverLimits{
				MaxFiles: len(facts), MaxSymbols: 4096, MaxDependencies: 4096,
				MaxReferences: 4096, MaxCandidatesPerResolution: 64,
			})
			if err != nil {
				b.Fatal(err)
			}
			if len(model.references) != children {
				b.Fatalf("expected %d references, got %d", children, len(model.references))
			}
			symbolSelector := func(facts ProjectFileFacts, qualifiedName string) ProjectSelector {
				var id string
				for _, symbol := range facts.Analysis.Analysis.Symbols {
					if symbol.QualifiedName == qualifiedName {
						id = symbol.ID
						break
					}
				}
				if id == "" {
					b.Fatalf("missing benchmark symbol %q in %q", qualifiedName, facts.Path)
				}
				return ProjectSelector{Kind: ProjectSelectorSymbol, Path: facts.Path, SourceFingerprint: facts.SourceFingerprint, SymbolID: id}
			}
			base := symbolSelector(facts[0], "demo.Base")
			late := symbolSelector(facts[len(facts)-1], fmt.Sprintf("demo.Child%03d", children-1))
			multi := make([]ProjectSelector, 0, min(8, children))
			for index := 0; index < cap(multi); index++ {
				multi = append(multi, symbolSelector(facts[len(facts)-1-index], fmt.Sprintf("demo.Child%03d", children-1-index)))
			}
			path := ProjectSelector{Kind: ProjectSelectorPath, Path: facts[len(facts)-1].Path, SourceFingerprint: facts[len(facts)-1].SourceFingerprint}
			options := ProjectContextOptions{BudgetBytes: 1 << 20, MaxItems: 512, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly}
			for _, tc := range []struct {
				name      string
				selectors []ProjectSelector
			}{
				{"single-late", []ProjectSelector{late}},
				{"single-base", []ProjectSelector{base}},
				{"multi-late", multi},
				{"path-late", []ProjectSelector{path}},
			} {
				b.Run(tc.name, func(b *testing.B) {
					plan, err := model.PlanContext(context.Background(), tc.selectors, options)
					if err != nil || len(plan.Candidates) == 0 {
						b.Fatalf("fixture plan: candidates=%d err=%v", len(plan.Candidates), err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for iteration := 0; iteration < b.N; iteration++ {
						if _, err := model.PlanContext(context.Background(), tc.selectors, options); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

func benchmarkReverseTargetCount(model *ProjectModel, targetID string) int {
	if model.referencesByTargetIndices != nil {
		return len(model.referencesByTargetIndices[targetID])
	}
	return len(model.referencesByTarget[targetID])
}

// BenchmarkProjectReverseTargetIndexScaling measures model construction and
// reverse-reference consumers on analyzer-proven inheritance relationships.
// The full project model is built outside timed queries.
func BenchmarkProjectReverseTargetIndexScaling(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	for _, children := range []int{16, 64, 256} {
		b.Run(fmt.Sprintf("references-%d", children), func(b *testing.B) {
			facts := benchmarkJavaProjectFacts(b, children)
			limits := ProjectResolverLimits{
				MaxFiles: len(facts), MaxSymbols: 4096, MaxDependencies: 4096,
				MaxReferences: 4096, MaxCandidatesPerResolution: 64,
			}
			var baseID string
			for _, symbol := range facts[0].Analysis.Analysis.Symbols {
				if symbol.QualifiedName == "demo.Base" {
					baseID = symbol.ID
					break
				}
			}
			if baseID == "" {
				b.Fatal("base declaration was not analyzed")
			}
			b.Run("model-build", func(b *testing.B) {
				b.ReportAllocs()
				for iteration := 0; iteration < b.N; iteration++ {
					model, err := BuildProjectModel(context.Background(), registry, facts, limits)
					if err != nil {
						b.Fatal(err)
					}
					if got := benchmarkReverseTargetCount(model, baseID); got != children {
						b.Fatalf("reverse model references=%d want=%d", got, children)
					}
				}
			})
			model, err := BuildProjectModel(context.Background(), registry, facts, limits)
			if err != nil {
				b.Fatal(err)
			}
			if got := benchmarkReverseTargetCount(model, baseID); got != children {
				b.Fatalf("reverse model setup references=%d want=%d", got, children)
			}
			b.Run("references-to", func(b *testing.B) {
				b.ReportAllocs()
				for iteration := 0; iteration < b.N; iteration++ {
					values := model.ReferencesTo(baseID)
					if len(values) != children {
						b.Fatalf("defensive references=%d want=%d", len(values), children)
					}
				}
			})
			selector := ProjectSelector{Kind: ProjectSelectorSymbol, Path: facts[0].Path, SourceFingerprint: facts[0].SourceFingerprint, SymbolID: baseID}
			queryLimits := ProjectQueryLimits{MaxResults: children + 8, MaxNodes: children + 8, MaxEdges: children + 8, MaxDepth: 2}
			b.Run("relations-references", func(b *testing.B) {
				b.ReportAllocs()
				for iteration := 0; iteration < b.N; iteration++ {
					result, err := model.QueryRelations(context.Background(), RelationReferences, selector, ProjectSelector{}, nil, queryLimits)
					if err != nil || len(result.Records) != children {
						b.Fatalf("reverse query records=%d err=%v", len(result.Records), err)
					}
				}
			})
		})
	}
}

// BenchmarkProjectReverseTargetFanoutScaling records the construction cost and
// retained backing-array footprint when each source reference has multiple
// equally plausible target declarations. Public lookup still clones snapshots.
func BenchmarkProjectReverseTargetFanoutScaling(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	const children = 128
	for _, definitions := range []int{1, 4, 8, 16} {
		b.Run(fmt.Sprintf("targets-%d", definitions), func(b *testing.B) {
			facts := make([]ProjectFileFacts, 0, definitions+children)
			for index := 0; index < definitions; index++ {
				path := fmt.Sprintf("project/benchmark/Base%03d.java", index)
				facts = append(facts, benchmarkAnalyzerFacts(b, JavaAnalyzer{}, path, "package demo; public class Base {}\n"))
			}
			for index := 0; index < children; index++ {
				path := fmt.Sprintf("project/benchmark/Child%03d.java", index)
				text := fmt.Sprintf("package demo; public class Child%03d extends Base {}\n", index)
				facts = append(facts, benchmarkAnalyzerFacts(b, JavaAnalyzer{}, path, text))
			}
			limits := ProjectResolverLimits{MaxFiles: len(facts), MaxSymbols: 4096, MaxDependencies: 4096, MaxReferences: 4096, MaxCandidatesPerResolution: 64}
			baseID := ""
			for _, symbol := range facts[0].Analysis.Analysis.Symbols {
				if symbol.QualifiedName == "demo.Base" {
					baseID = symbol.ID
					break
				}
			}
			if baseID == "" {
				b.Fatal("base symbol missing")
			}
			sample, err := BuildProjectModel(context.Background(), registry, facts, limits)
			if err != nil {
				b.Fatal(err)
			}
			if got := len(sample.ReferencesTo(baseID)); got != children {
				b.Fatalf("baseline reverse=%d, want %d", got, children)
			}
			capacity := 0
			for _, bucket := range sample.referencesByTarget {
				capacity += cap(bucket)
			}
			bucketBytes := capacity * int(unsafe.Sizeof(ProjectReference{}))
			indexCapacity := 0
			for _, indices := range sample.referencesByTargetIndices {
				indexCapacity += cap(indices)
			}
			indexBytes := indexCapacity * int(unsafe.Sizeof(int(0)))
			b.Run("model-build", func(b *testing.B) {
				b.ReportAllocs()
				b.ReportMetric(float64(bucketBytes), "reverse-bucket-B")
				b.ReportMetric(float64(indexBytes), "reverse-index-B")
				for iteration := 0; iteration < b.N; iteration++ {
					model, err := BuildProjectModel(context.Background(), registry, facts, limits)
					if err != nil {
						b.Fatal(err)
					}
					if got := benchmarkReverseTargetCount(model, baseID); got != children {
						b.Fatalf("model reverse=%d, want %d", got, children)
					}
				}
			})
		})
	}
}

// BenchmarkProjectReverseTargetLookupParityScaling compares indexed and
// legacy bucket traversal on the same immutable facts and query workload.
func BenchmarkProjectReverseTargetLookupParityScaling(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	for _, children := range []int{64, 256} {
		b.Run(fmt.Sprintf("references-%d", children), func(b *testing.B) {
			facts := benchmarkJavaProjectFacts(b, children)
			baseID := ""
			for _, symbol := range facts[0].Analysis.Analysis.Symbols {
				if symbol.QualifiedName == "demo.Base" {
					baseID = symbol.ID
					break
				}
			}
			model, err := BuildProjectModel(context.Background(), registry, facts, ProjectResolverLimits{
				MaxFiles: len(facts), MaxSymbols: 4096, MaxDependencies: 4096,
				MaxReferences: 4096, MaxCandidatesPerResolution: 64,
			})
			if err != nil {
				b.Fatal(err)
			}
			if model.referencesByTargetIndices == nil {
				b.Fatal("dense unique fixture must use compact target index")
			}
			legacy := *model
			legacy.referencesByTargetIndices = nil
			legacy.referencesByTarget = make(map[string][]ProjectReference)
			for _, reference := range model.references {
				for _, target := range reference.Targets {
					if target.SymbolID != "" {
						legacy.referencesByTarget[target.SymbolID] = append(legacy.referencesByTarget[target.SymbolID], reference)
					}
				}
			}
			for key := range legacy.referencesByTarget {
				sortProjectReferences(legacy.referencesByTarget[key])
			}
			selector := ProjectSelector{Kind: ProjectSelectorSymbol, Path: facts[0].Path, SourceFingerprint: facts[0].SourceFingerprint, SymbolID: baseID}
			limits := ProjectQueryLimits{MaxResults: children + 8, MaxNodes: children + 8, MaxEdges: children + 8, MaxDepth: 2}
			for _, tc := range []struct {
				name  string
				model *ProjectModel
			}{{"indexed", model}, {"legacy", &legacy}} {
				b.Run(tc.name+"-query", func(b *testing.B) {
					b.ReportAllocs()
					for iteration := 0; iteration < b.N; iteration++ {
						result, err := tc.model.QueryRelations(context.Background(), RelationReferences, selector, ProjectSelector{}, nil, limits)
						if err != nil || len(result.Records) != children {
							b.Fatalf("query records=%d err=%v", len(result.Records), err)
						}
					}
				})
				b.Run(tc.name+"-references-to", func(b *testing.B) {
					b.ReportAllocs()
					for iteration := 0; iteration < b.N; iteration++ {
						result := tc.model.ReferencesTo(baseID)
						if len(result) != children {
							b.Fatalf("references=%d, want %d", len(result), children)
						}
					}
				})
			}
		})
	}
}

// BenchmarkProjectForwardInheritanceScaling isolates whole-project source-side
// reference scanning from the surrounding public relationship query cost.
func BenchmarkProjectForwardInheritanceScaling(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	for _, children := range []int{16, 64, 256} {
		b.Run(fmt.Sprintf("references-%d", children), func(b *testing.B) {
			facts := benchmarkJavaProjectFacts(b, children)
			model, err := BuildProjectModel(context.Background(), registry, facts, ProjectResolverLimits{
				MaxFiles: len(facts), MaxSymbols: 4096, MaxDependencies: 4096,
				MaxReferences: 4096, MaxCandidatesPerResolution: 64,
			})
			if err != nil || len(model.references) != children {
				b.Fatalf("project references=%d err=%v", len(model.references), err)
			}
			symbolSelector := func(fact ProjectFileFacts, qualifiedName string) ProjectSelector {
				for _, symbol := range fact.Analysis.Analysis.Symbols {
					if symbol.QualifiedName == qualifiedName {
						return ProjectSelector{Kind: ProjectSelectorSymbol, Path: fact.Path, SourceFingerprint: fact.SourceFingerprint, SymbolID: symbol.ID}
					}
				}
				b.Fatalf("missing benchmark symbol %q in %q", qualifiedName, fact.Path)
				return ProjectSelector{}
			}
			base := symbolSelector(facts[0], "demo.Base")
			early := symbolSelector(facts[1], "demo.Child000")
			late := symbolSelector(facts[len(facts)-1], fmt.Sprintf("demo.Child%03d", children-1))
			limits := ProjectQueryLimits{MaxResults: children + 8, MaxNodes: children + 8, MaxEdges: children + 8, MaxDepth: 2}
			for _, tc := range []struct {
				name     string
				selector ProjectSelector
				want     int
			}{
				{"no-source", base, 0},
				{"early-source", early, 1},
				{"late-source", late, 1},
			} {
				b.Run(tc.name, func(b *testing.B) {
					b.ReportAllocs()
					for iteration := 0; iteration < b.N; iteration++ {
						result, err := model.QueryRelations(context.Background(), RelationInheritance, tc.selector, ProjectSelector{}, nil, limits)
						if err != nil || len(result.Records) != tc.want {
							b.Fatalf("forward query got=%d want=%d err=%v", len(result.Records), tc.want, err)
						}
					}
				})
			}
			b.Run("raw-source-scan", func(b *testing.B) {
				b.ReportAllocs()
				for iteration := 0; iteration < b.N; iteration++ {
					records := model.referenceRecordsFrom(late.SymbolID, RelationInheritance, isInheritanceStructuralKind, nil)
					if len(records) != 1 {
						b.Fatalf("forward scan got=%d want=1", len(records))
					}
				}
			})
		})
	}
}

func benchmarkJavaProjectFacts(b *testing.B, children int) []ProjectFileFacts {
	b.Helper()
	facts := make([]ProjectFileFacts, 0, children+1)
	facts = append(facts, benchmarkAnalyzerFacts(b, JavaAnalyzer{}, "project/benchmark/Base.java", "package demo; public class Base {}\n"))
	for index := 0; index < children; index++ {
		path := fmt.Sprintf("project/benchmark/Child%03d.java", index)
		text := fmt.Sprintf("package demo; public class Child%03d extends Base {}\n", index)
		facts = append(facts, benchmarkAnalyzerFacts(b, JavaAnalyzer{}, path, text))
	}
	return facts
}

func benchmarkAnalyzerFacts(b *testing.B, analyzer SourceAnalyzer, path, text string) ProjectFileFacts {
	b.Helper()
	document := sourceDocumentForScanner(text)
	document.Path = path
	document.SourceFingerprint = benchmarkDigest(text)
	result, err := analyzer.Analyze(context.Background(), document, testAnalyzeOptions(false, 512))
	if err != nil {
		b.Fatalf("analyze %s: %v", path, err)
	}
	if !result.Analysis.CoverageComplete {
		b.Fatalf("analysis for %s is partial: %+v", path, result.Analysis.Diagnostics)
	}
	return ProjectFileFacts{Path: path, Language: analyzer.Language(), SourceFingerprint: document.SourceFingerprint, Analysis: result}
}

func benchmarkDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// BenchmarkProjectStructuralSearchSparseScaling isolates matching cost from
// analyzer/model construction and separates sparse hits from broad output.
func BenchmarkProjectStructuralSearchSparseScaling(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	for _, children := range []int{16, 64, 256} {
		b.Run(fmt.Sprintf("files-%d", children), func(b *testing.B) {
			facts := benchmarkJavaProjectFacts(b, children)
			model, err := BuildProjectModel(context.Background(), registry, facts, ProjectResolverLimits{
				MaxFiles: len(facts), MaxSymbols: 4096, MaxDependencies: 4096, MaxReferences: 4096, MaxCandidatesPerResolution: 64,
			})
			if err != nil {
				b.Fatal(err)
			}
			testCases := []struct {
				name  string
				query string
				match ProjectSearchMatchMode
				kinds []string
				want  int
			}{
				{name: "exact-late", query: fmt.Sprintf("Child%03d", children-1), match: ProjectSearchExact, want: 1},
				{name: "exact-missing", query: "MissingClassNoMatch", match: ProjectSearchExact, want: 0},
				{name: "exact-late-class-only", query: fmt.Sprintf("Child%03d", children-1), match: ProjectSearchExact, kinds: []string{string(SymbolKindClass)}, want: 1},
				{name: "exact-missing-class-only", query: "MissingClassNoMatch", match: ProjectSearchExact, kinds: []string{string(SymbolKindClass)}, want: 0},
				{name: "prefix-all", query: "Child", match: ProjectSearchPrefix, want: children},
				{name: "prefix-all-class-only", query: "Child", match: ProjectSearchPrefix, kinds: []string{string(SymbolKindClass)}, want: children},
				{name: "prefix-all-method-only", query: "Child", match: ProjectSearchPrefix, kinds: []string{string(SymbolKindMethod)}, want: 0},
			}
			for _, testCase := range testCases {
				b.Run(testCase.name, func(b *testing.B) {
					options := ProjectSearchOptions{
						Query: testCase.query, Match: testCase.match, Kinds: testCase.kinds, MaxResults: children + 8,
					}
					b.ReportAllocs()
					b.ResetTimer()
					for iteration := 0; iteration < b.N; iteration++ {
						result, err := model.StructuralSearch(context.Background(), options)
						if err != nil || len(result.Matches) != testCase.want {
							b.Fatalf("structural search got=%d want=%d err=%v", len(result.Matches), testCase.want, err)
						}
					}
				})
			}
		})
	}
}
