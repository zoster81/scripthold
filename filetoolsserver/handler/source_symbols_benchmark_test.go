package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

// TestSourceConciseParentProjectionPreservesNestedAndAmbiguousOwnership checks
// missing parents, cyclic ancestry, duplicate IDs, and selected owners.
func TestSourceConciseParentProjectionPreservesNestedAndAmbiguousOwnership(t *testing.T) {
	const path = "project/source.go"
	symbols := []sourceintelligence.NormalizedSymbol{
		{ID: "pkg", Name: "Package", Kind: sourceintelligence.SymbolKindPackage, Path: path},
		{ID: "ns", Name: "Namespace", ParentID: "pkg", Kind: sourceintelligence.SymbolKindNamespace, Path: path},
		{ID: "top", Name: "Top", Kind: sourceintelligence.SymbolKindFunction, Path: path},
		{ID: "via", Name: "ViaNamespace", ParentID: "ns", Kind: sourceintelligence.SymbolKindFunction, Path: path},
		{ID: "owner", Name: "Owner", ParentID: "ns", Kind: sourceintelligence.SymbolKindClass, Path: path},
		{ID: "method", Name: "Method", ParentID: "owner", Kind: sourceintelligence.SymbolKindMethod, Path: path},
		{ID: "orphan", Name: "Orphan", ParentID: "missing", Kind: sourceintelligence.SymbolKindFunction, Path: path},
		{ID: "cycle-a", Name: "CycleA", ParentID: "cycle-b", Kind: sourceintelligence.SymbolKindModule, Path: path},
		{ID: "cycle-b", Name: "CycleB", ParentID: "cycle-a", Kind: sourceintelligence.SymbolKindModule, Path: path},
		{ID: "cycle-func", Name: "CycleFunction", ParentID: "cycle-a", Kind: sourceintelligence.SymbolKindFunction, Path: path},
		{ID: "duplicate", Name: "EarlierNamespace", Kind: sourceintelligence.SymbolKindNamespace, Path: path},
		{ID: "duplicate", Name: "LaterClass", Kind: sourceintelligence.SymbolKindClass, Path: path},
		{ID: "duplicate-child", Name: "DuplicateChild", ParentID: "duplicate", Kind: sourceintelligence.SymbolKindFunction, Path: path},
	}
	analyses := []sourceFileAnalysis{{
		file: SourceSymbolsFile{Path: path, Status: "parsed", CoverageComplete: true},
		analysis: sourceintelligence.AnalyzerResult{
			Analysis: sourceintelligence.AnalysisResult{Symbols: symbols, CoverageComplete: true},
		},
	}}
	digest := buildSourceConciseDigest(analyses, []string{path}, false)
	if len(digest.ConciseFiles) != 1 {
		t.Fatalf("concise digest files: %+v", digest.ConciseFiles)
	}
	if got := digest.ConciseFiles[0]; !slices.Equal(got.Functions, []string{"Top", "ViaNamespace"}) ||
		!slices.Equal(got.Classes, []string{"Owner", "LaterClass"}) {
		t.Fatalf("top-level digest changed: %+v", got)
	}
	outline := buildSourceConciseOutline(analyses, []string{path}, "", nil, false, 64)
	if len(outline.ConciseFiles) != 1 {
		t.Fatalf("concise outline files: %+v", outline.ConciseFiles)
	}
	got := outline.ConciseFiles[0]
	if !slices.Equal(got.Functions, []string{"Top", "ViaNamespace"}) ||
		len(got.Owners) != 2 || got.Owners[0].Name != "Owner" ||
		len(got.Owners[0].Methods) != 1 || got.Owners[0].Methods[0].Name != "Method" {
		t.Fatalf("top-level concise outline changed: %+v", got)
	}
	selected := buildSourceConciseOutline(analyses, []string{path}, "Owner", nil, false, 64)
	if len(selected.ConciseFiles) != 1 || len(selected.ConciseFiles[0].Owners) != 1 ||
		selected.ConciseFiles[0].Owners[0].Name != "Owner" ||
		len(selected.ConciseFiles[0].Owners[0].Methods) != 1 ||
		len(selected.ConciseFiles[0].Functions) != 0 {
		t.Fatalf("filtered concise outline changed: %+v", selected)
	}
	missing := buildSourceConciseOutline(analyses, []string{path}, "MissingOwner", nil, false, 64)
	if len(missing.ConciseFiles) != 0 || !missing.CoverageComplete || missing.Ambiguous {
		t.Fatalf("missing-owner result changed: %+v", missing)
	}
}

// BenchmarkSourceSymbolsConciseDenseParents exercises the full handler and
// isolated presentation for files with many top-level functions. A requested
// owner intentionally matches nothing and must preserve complete coverage.
func BenchmarkSourceSymbolsConciseDenseParents(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "dense.go")
	var source strings.Builder
	source.WriteString("package benchmark\n")
	const count = 5000
	for i := 0; i < count; i++ {
		fmt.Fprintf(&source, "func Function%05d(value int) int { return value }\n", i)
	}
	if err := os.WriteFile(path, []byte(source.String()), 0o600); err != nil {
		b.Fatal(err)
	}
	h := NewHandler([]string{root})
	input := SourceSymbolsInput{
		Operation: "outline", Presentation: "concise", Paths: []string{path},
		Language: "go", Encoding: "utf-8", Owner: "MissingOwner",
		MaxSymbols: 64,
	}
	ctx := context.Background()
	callResult, baseline, err := h.SourceSymbols(ctx, nil, input)
	if err != nil || callResult != nil || baseline.SymbolCount != 0 ||
		!baseline.CoverageComplete || len(baseline.ConciseFiles) != 0 {
		b.Fatalf("missing owner seed: result=%+v output=%+v err=%v", callResult, baseline, err)
	}
	benchSymbols := make([]sourceintelligence.NormalizedSymbol, count)
	for i := range benchSymbols {
		benchSymbols[i] = sourceintelligence.NormalizedSymbol{
			ID:   fmt.Sprintf("function-%05d", i),
			Path: path, Name: fmt.Sprintf("Function%05d", i),
			Kind: sourceintelligence.SymbolKindFunction,
		}
	}
	analyses := []sourceFileAnalysis{{
		file: SourceSymbolsFile{Path: path, Status: "parsed", CoverageComplete: true},
		analysis: sourceintelligence.AnalyzerResult{
			Analysis: sourceintelligence.AnalysisResult{Symbols: benchSymbols, CoverageComplete: true},
		},
	}}
	b.Run("isolated-no-owner", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			out := buildSourceConciseOutline(analyses, []string{path}, "", nil, false, 64)
			if len(out.ConciseFiles) != 1 || len(out.ConciseFiles[0].Functions) != 64 ||
				!out.ConciseResultsTruncated {
				b.Fatal("concise unfiltered output changed")
			}
		}
	})
	b.Run("isolated-missing-owner", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			out := buildSourceConciseOutline(analyses, []string{path}, "MissingOwner", nil, false, 64)
			if len(out.ConciseFiles) != 0 || !out.CoverageComplete || out.SymbolCount != 0 {
				b.Fatal("concise missing-owner output changed")
			}
		}
	})
	b.Run("full-handler-missing-owner", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			toolResult, output, err := h.SourceSymbols(ctx, nil, input)
			if err != nil || toolResult != nil || output.SymbolCount != 0 ||
				!output.CoverageComplete || len(output.ConciseFiles) != 0 {
				b.Fatalf("full missing-owner output changed: %v %+v", err, toolResult)
			}
		}
	})
}

// BenchmarkSourceSymbolsDenseDetailed exercises bounded, full-response
// single-file outlines at representative large declaration counts.
func BenchmarkSourceSymbolsDenseDetailed(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "dense-detailed.go")
	var source strings.Builder
	source.WriteString("package benchmark\n")
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&source, "func Function%05d(value int) int { return value }\n", i)
	}
	if err := os.WriteFile(path, []byte(source.String()), 0o600); err != nil {
		b.Fatal(err)
	}
	h := NewHandler([]string{root})
	input := SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "go", Encoding: "utf-8", MaxSymbols: 6000,
	}
	ctx := context.Background()
	toolResult, seed, err := h.SourceSymbols(ctx, nil, input)
	if err != nil || toolResult != nil || len(seed.Symbols) != 5001 || seed.Truncated {
		b.Fatalf("full detailed seed failed: output=%d truncated=%v err=%v tool=%+v",
			len(seed.Symbols), seed.Truncated, err, toolResult)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toolResult, output, err := h.SourceSymbols(ctx, nil, input)
		if err != nil || toolResult != nil || len(output.Symbols) != len(seed.Symbols) ||
			output.Symbols[0].ID != seed.Symbols[0].ID ||
			output.Symbols[len(output.Symbols)-1].ID != seed.Symbols[len(seed.Symbols)-1].ID {
			b.Fatalf("detailed output changed: symbols=%d tool=%+v err=%v", len(output.Symbols), toolResult, err)
		}
	}
}

func BenchmarkSourceSymbolsOperations(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "sample.go")
	var source string
	source = "package sample\n"
	for index := 0; index < 200; index++ {
		source += fmt.Sprintf("func F%03d(value int) int { return value }\n", index)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		b.Fatal(err)
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	_, seed, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "go", Encoding: "utf-8"})
	if err != nil || len(seed.Symbols) == 0 {
		b.Fatalf("seed outline failed: %v %+v", err, seed)
	}
	showInput := SourceSymbolsInput{Operation: "show", Path: path, SymbolID: seed.Symbols[len(seed.Symbols)-1].ID, SourceFingerprint: seed.Files[0].SourceFingerprint, Language: "go", Encoding: "utf-8", MaxBytes: 4096}
	cases := []struct {
		name  string
		input SourceSymbolsInput
	}{
		{name: "outline", input: SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "go", Encoding: "utf-8", MaxSymbols: 1000}},
		{name: "tree", input: SourceSymbolsInput{Operation: "outline", Presentation: "tree", Paths: []string{path}, Language: "go", Encoding: "utf-8", MaxSymbols: 1000}},
		{name: "compact", input: SourceSymbolsInput{Operation: "outline", Presentation: "compact", Paths: []string{path}, Language: "go", Encoding: "utf-8", MaxSymbols: 1000}},
		{name: "digest", input: SourceSymbolsInput{Operation: "digest", Paths: []string{path}, Language: "go", Encoding: "utf-8"}},
		{name: "find", input: SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "go", Encoding: "utf-8", Query: "F199", Match: "exact", MaxSymbols: 16}},
		{name: "show", input: showInput},
	}
	for _, testCase := range cases {
		b.Run(testCase.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				toolErr, _, err := h.SourceSymbols(ctx, nil, testCase.input)
				if err != nil || toolErr != nil {
					b.Fatalf("source_symbols %s failed: err=%v toolErr=%+v", testCase.name, err, toolErr)
				}
			}
		})
	}
}

// sourceSymbolsCountingWriter measures serialized byte lengths without
// retaining an extra JSON buffer.
type sourceSymbolsCountingWriter struct {
	bytes int
}

func (counter *sourceSymbolsCountingWriter) Write(data []byte) (int, error) {
	counter.bytes += len(data)
	return len(data), nil
}

// BenchmarkSourceSymbolsPhaseAttribution separates secure source opening,
// language detection, native Go analysis, and the complete handler operation.
// The phases are measured independently and their timings are not additive.
func BenchmarkSourceSymbolsPhaseAttribution(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "phase.go")
	var source strings.Builder
	source.WriteString("package sample\n")
	for index := 0; index < 200; index++ {
		fmt.Fprintf(&source, "func F%03d(value int) int { return value }\n", index)
	}
	if err := os.WriteFile(path, []byte(source.String()), 0o600); err != nil {
		b.Fatal(err)
	}
	h := NewHandler([]string{root})
	validated := h.ValidatePath(path)
	if !validated.Ok() {
		b.Fatal("generated source path was not authorized")
	}
	limits := h.sourceLimits()
	options := sourceintelligence.OpenDocumentOptions{
		RequestedEncoding:    "utf-8",
		MaxFileBytes:         limits.MaxFileBytes,
		MaxDecodedCharacters: h.maxDecodedCharacters(),
	}
	ctx := context.Background()
	document, err := sourceintelligence.OpenSourceDocument(ctx, validated.Path, options)
	if err != nil {
		b.Fatal(err)
	}
	registry, err := sourceintelligence.DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	detectionInput := sourceintelligence.DetectionInput{
		Path: validated.Path, Text: document.Text, ExplicitLanguage: "go",
		MaxProbes: limits.MaxDetectorProbes,
	}
	analysisOptions := sourceintelligence.AnalyzeOptions{
		IncludeSignatures: false, MaxNesting: limits.MaxNesting,
		Limits: sourceintelligence.SymbolBuilderLimits{
			MaxSymbols: 1000, MaxSignatureBytes: limits.MaxSignatureBytes,
			MaxDiagnostics: limits.MaxDiagnostics,
		},
	}
	analyzer := sourceintelligence.GoAnalyzer{}
	seedAnalysis, err := analyzer.Analyze(ctx, document, analysisOptions)
	if err != nil || !seedAnalysis.Analysis.CoverageComplete || len(seedAnalysis.Analysis.Symbols) != 201 {
		b.Fatalf("Go analyzer seed: symbols=%d complete=%t err=%v", len(seedAnalysis.Analysis.Symbols), seedAnalysis.Analysis.CoverageComplete, err)
	}
	input := SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{validated.Path},
		Language: "go", Encoding: "utf-8", MaxSymbols: 1000,
	}
	callErr, seedOutput, err := h.SourceSymbols(ctx, nil, input)
	if err != nil || callErr != nil || seedOutput.SymbolCount != len(seedAnalysis.Analysis.Symbols) {
		b.Fatalf("handler seed: symbols=%d want=%d err=%v toolErr=%v", seedOutput.SymbolCount, len(seedAnalysis.Analysis.Symbols), err, callErr)
	}
	b.Run("open-and-decode", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(document.Text)))
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			opened, err := sourceintelligence.OpenSourceDocument(ctx, validated.Path, options)
			if err != nil || opened.SourceFingerprint != document.SourceFingerprint || opened.Text != document.Text {
				b.Fatalf("source snapshot changed: err=%v", err)
			}
		}
	})
	b.Run("detect-language", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(document.Text)))
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			detection, err := sourceintelligence.DetectLanguage(ctx, registry, detectionInput)
			if err != nil || detection.Language != "go" {
				b.Fatalf("language detection changed: %+v err=%v", detection, err)
			}
		}
	})
	b.Run("analyze-go", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(document.Text)))
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			result, err := analyzer.Analyze(ctx, document, analysisOptions)
			if err != nil || !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != len(seedAnalysis.Analysis.Symbols) {
				b.Fatalf("Go analysis changed: symbols=%d err=%v", len(result.Analysis.Symbols), err)
			}
		}
	})
	// Keep each authorization boundary measurable without treating the independent
	// stages as additive components of the complete handler latency.
	b.Run("validate-request-path", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			checked := h.ValidatePath(path)
			if !checked.Ok() || checked.Path != validated.Path {
				b.Fatalf("request path authorization changed: %+v", checked)
			}
		}
	})
	b.Run("visit-explicit-file", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			seen := 0
			err := h.visitIncludedFiles(ctx, []string{validated.Path}, nil, nil, false, func(found string) {
				if found != validated.Path {
					b.Fatalf("file visitation changed: %q", found)
				}
				seen++
			})
			if err != nil || seen != 1 {
				b.Fatalf("explicit source file visitation changed: seen=%d err=%v", seen, err)
			}
		}
	})
	b.Run("select-authorized-files", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(document.Text)))
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			files, truncated, toolErr := h.collectSourceSymbolFiles(ctx, input, limits.MaxFiles, limits)
			if toolErr != nil || truncated || len(files) != 1 || files[0] != validated.Path {
				b.Fatalf("file selection changed: files=%v truncated=%t err=%v", files, truncated, toolErr)
			}
		}
	})
	b.Run("analyze-file", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(document.Text)))
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			analyzed := h.analyzeSourceFile(ctx, registry, validated.Path, "go", "utf-8", false, 1000, limits)
			if analyzed.file.Status != "parsed" || len(analyzed.analysis.Analysis.Symbols) != len(seedAnalysis.Analysis.Symbols) ||
				analyzed.file.SourceFingerprint != document.SourceFingerprint {
				b.Fatalf("file analysis changed: status=%s symbols=%d", analyzed.file.Status, len(analyzed.analysis.Analysis.Symbols))
			}
		}
	})
	analyses := []sourceFileAnalysis{{
		file: SourceSymbolsFile{
			Path: validated.Path, Status: "parsed", CoverageComplete: true,
			Language: "go", Encoding: "utf-8",
			SourceFingerprint: document.SourceFingerprint,
		},
		analysis: seedAnalysis,
	}}
	b.Run("assemble-outline", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(document.Text)))
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			output := buildSourceSymbolsOutput("outline", input, 1, false, analyses, 1000)
			if output.SymbolCount != seedOutput.SymbolCount || len(output.Files) != 1 {
				b.Fatalf("assembled symbols=%d files=%d", output.SymbolCount, len(output.Files))
			}
		}
	})
	b.Run("enforce-output-budget", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(document.Text)))
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			if err := enforceSourceOutputBudget(seedOutput, limits.MaxOutputBytes); err != nil {
				b.Fatal(err)
			}
		}
	})
	// Encoder.Encode adds one newline but otherwise uses Marshal's default
	// HTML escaping; this experimental comparator checks the original byte cap.
	encoded, err := json.Marshal(seedOutput)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("encode-budget-count-only", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(document.Text)))
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			counter := &sourceSymbolsCountingWriter{}
			if err := json.NewEncoder(counter).Encode(seedOutput); err != nil {
				b.Fatal(err)
			}
			if counter.bytes-1 != len(encoded) {
				b.Fatalf("counted %d JSON bytes; want %d", counter.bytes-1, len(encoded))
			}
		}
	})
	b.Run("full-outline", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(document.Text)))
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			callErr, output, err := h.SourceSymbols(ctx, nil, input)
			if err != nil || callErr != nil || output.SymbolCount != seedOutput.SymbolCount {
				b.Fatalf("handler outline changed: symbols=%d err=%v toolErr=%v", output.SymbolCount, err, callErr)
			}
		}
	})
}

// BenchmarkSourceOutlineProjection isolates tree/compact output construction
// from authorized file traversal, decoding, and language scanning.
func BenchmarkSourceOutlineProjection(b *testing.B) {
	for _, count := range []int{64, 1024} {
		b.Run(fmt.Sprintf("symbols-%d", count), func(b *testing.B) {
			path := "source.go"
			files := []SourceSymbolsFile{{Path: path, Status: "parsed", CoverageComplete: true}}
			symbols := make([]sourceintelligence.NormalizedSymbol, 0, count)
			parent := ""
			for i := 0; i < count; i++ {
				id := fmt.Sprintf("symbol-%04d", i)
				if i%4 == 0 {
					parent = id
				}
				parentID := parent
				if i%4 == 0 {
					parentID = ""
				}
				symbols = append(symbols, sourceintelligence.NormalizedSymbol{
					ID: id, Path: path, Name: id, QualifiedName: id,
					ParentID: parentID, Kind: sourceintelligence.SymbolKindFunction,
					DeclarationRange: sourceintelligence.Range{
						Start: sourceintelligence.Position{Line: i + 1, Column: 1},
						End:   sourceintelligence.Position{Line: i + 1, Column: 10},
					},
				})
			}
			for _, presentation := range []string{"tree", "compact"} {
				b.Run(presentation, func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						output := presentSourceOutline(files, symbols, presentation)
						if len(output) != 1 || len(output[0].Symbols) == 0 {
							b.Fatal("unexpected outline projection")
						}
					}
				})
			}
		})
	}
}

// BenchmarkSourceOutlineProjectionSparsePaths protects cases where the caller
// selected few or no symbols from a much larger cross-file input.
func BenchmarkSourceOutlineProjectionSparsePaths(b *testing.B) {
	const count = 1024
	files := []SourceSymbolsFile{{Path: "selected.go", Status: "parsed", CoverageComplete: true}}
	for _, retained := range []int{0, 1, 32, 512, 1024} {
		b.Run(fmt.Sprintf("retained-%d", retained), func(b *testing.B) {
			symbols := make([]sourceintelligence.NormalizedSymbol, count)
			for index := range symbols {
				path := "skipped.go"
				if index < retained {
					path = "selected.go"
				}
				symbols[index] = sourceintelligence.NormalizedSymbol{
					ID: fmt.Sprintf("symbol-%04d", index), Path: path,
					Name: fmt.Sprintf("Item%04d", index),
					Kind: sourceintelligence.SymbolKindFunction,
				}
			}
			for _, presentation := range []string{"tree", "compact"} {
				b.Run(presentation, func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for iteration := 0; iteration < b.N; iteration++ {
						output := presentSourceOutline(files, symbols, presentation)
						if len(output) != 1 || len(output[0].Symbols) != retained {
							b.Fatalf("projection symbols=%d, want %d", len(output[0].Symbols), retained)
						}
					}
				})
			}
		})
	}
}

// BenchmarkSourceSymbolsOutputAssembly isolates result-slice growth from
// filesystem access, language analysis, and JSON response encoding.
func BenchmarkSourceSymbolsOutputAssembly(b *testing.B) {
	for _, count := range []int{64, 1024} {
		b.Run(fmt.Sprintf("symbols-%d", count), func(b *testing.B) {
			const files = 4
			analyses := make([]sourceFileAnalysis, files)
			for i := range analyses {
				path := fmt.Sprintf("file-%d.go", i)
				analyses[i].file = SourceSymbolsFile{Path: path, Status: "parsed", CoverageComplete: true}
				analyses[i].analysis.Analysis.CoverageComplete = true
				for j := i; j < count; j += files {
					analyses[i].analysis.Analysis.Symbols = append(
						analyses[i].analysis.Analysis.Symbols,
						sourceintelligence.NormalizedSymbol{
							ID: fmt.Sprintf("s-%d", j), Path: path,
							Name: fmt.Sprintf("Function%d", j),
							Kind: sourceintelligence.SymbolKindFunction,
						},
					)
				}
			}
			for _, tc := range []struct {
				name  string
				kinds []string
				want  int
			}{
				{name: "unfiltered", want: count},
				{name: "no-matches", kinds: []string{"method"}, want: 0},
			} {
				b.Run(tc.name, func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						output := buildSourceSymbolsOutput("outline", SourceSymbolsInput{
							Operation: "outline", Kinds: tc.kinds,
						}, files, false, analyses, count+1)
						if len(output.Symbols) != tc.want {
							b.Fatalf("got %d symbols, want %d", len(output.Symbols), tc.want)
						}
					}
				})
			}
		})
	}
}

// BenchmarkSourceFileSelection compares the old all-paths-then-slice strategy
// with bounded retention under the same authorized walk, roots and filters.
// The fixture is created outside the timed sections and no source is executed.
func BenchmarkSourceFileSelection(b *testing.B) {
	root := b.TempDir()
	for directory := 0; directory < 4; directory++ {
		name := filepath.Join(root, fmt.Sprintf("group-%d", directory))
		if err := os.Mkdir(name, 0o700); err != nil {
			b.Fatal(err)
		}
		for index := 0; index < 256; index++ {
			path := filepath.Join(name, fmt.Sprintf("file-%04d.go", index))
			if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
				b.Fatal(err)
			}
		}
	}
	ctx := context.Background()
	h := NewHandler([]string{root})
	paths := []string{filepath.Join(root, "group-3"), root}
	includes := []string{"*.go"}
	all, err := h.collectFiles(ctx, paths, includes, nil, false)
	if err != nil {
		b.Fatal(err)
	}
	sort.Strings(all)
	if len(all) != 1024 {
		b.Fatalf("baseline selected %d files, want 1024", len(all))
	}

	for _, maximum := range []int{32, 256, 1024} {
		b.Run(fmt.Sprintf("maxFiles-%d", maximum), func(b *testing.B) {
			expected := all[:maximum]
			expectedTruncated := len(all) > maximum
			selected, truncated, err := h.collectBoundedSourceFiles(ctx, paths, includes, nil, false, maximum)
			if err != nil || truncated != expectedTruncated || !slices.Equal(selected, expected) {
				b.Fatalf("bounded selection differs from full gather: err=%v, truncated=%v, files=%d", err, truncated, len(selected))
			}
			b.Run("full-gather", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					files, err := h.collectFiles(ctx, paths, includes, nil, false)
					if err != nil {
						b.Fatal(err)
					}
					sort.Strings(files)
					if len(files) < maximum || !slices.Equal(files[:maximum], expected) {
						b.Fatal("full gather returned unexpected file prefix")
					}
				}
			})
			b.Run("bounded", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					files, truncated, err := h.collectBoundedSourceFiles(ctx, paths, includes, nil, false, maximum)
					if err != nil || truncated != expectedTruncated || !slices.Equal(files, expected) {
						b.Fatal("bounded selection returned unexpected prefix or truncation")
					}
				}
			})
		})
	}
}

func BenchmarkSourceSymbolsMixedManySmallFiles(b *testing.B) {
	root := b.TempDir()
	for index := 0; index < 80; index++ {
		var name, content string
		switch index % 4 {
		case 0:
			name = fmt.Sprintf("file-%03d.go", index)
			content = fmt.Sprintf("package p%03d\nfunc Work%03d() {}\n", index, index)
		case 1:
			name = fmt.Sprintf("file-%03d.cs", index)
			content = fmt.Sprintf("namespace N%d { public class C%d { public void Work%d() {} } }\n", index, index, index)
		case 2:
			name = fmt.Sprintf("file-%03d.vb", index)
			content = fmt.Sprintf("Class C%d\nSub Work%d()\nEnd Sub\nEnd Class\n", index, index)
		default:
			name = fmt.Sprintf("file-%03d.py", index)
			content = fmt.Sprintf("def work%d():\n    return %d\n", index, index)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	h := NewHandler([]string{root})
	input := SourceSymbolsInput{Operation: "digest", Paths: []string{root}, Encoding: "utf-8"}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		toolErr, output, err := h.SourceSymbols(context.Background(), nil, input)
		if err != nil || toolErr != nil {
			b.Fatalf("mixed source_symbols failed: err=%v toolErr=%+v", err, toolErr)
		}
		if output.FilesParsed != 80 {
			b.Fatalf("parsed files=%d, want 80", output.FilesParsed)
		}
	}
}
