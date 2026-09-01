package sourceintelligence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

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
