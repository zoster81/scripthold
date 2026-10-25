package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// These measurements require an opt-in directory of source files from a real
// project. The benchmark never executes code from that directory or writes to it.
func realSourceCorpusInput(tb testing.TB) SourceQueryInput {
	tb.Helper()
	root := os.Getenv("SCRIPTHOLD_REAL_CORPUS_DIR")
	if root == "" {
		tb.Skip("set SCRIPTHOLD_REAL_CORPUS_DIR to opt into real-source measurement")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		tb.Fatalf("the configured corpus directory is unavailable: %v", err)
	}
	query := os.Getenv("SCRIPTHOLD_REAL_CORPUS_QUERY")
	if query == "" {
		query = "a"
	}
	return SourceQueryInput{
		Operation: "search", Paths: []string{root}, Query: query,
		Mode: "structural", Match: "contains",
		Language: os.Getenv("SCRIPTHOLD_REAL_CORPUS_LANGUAGE"), Encoding: "utf-8",
		MaxFiles: 32, MaxResults: 10_000,
	}
}

func runRealSourceCorpusSearch(tb testing.TB, h *Handler, input SourceQueryInput) (SourceQueryOutput, time.Duration) {
	tb.Helper()
	started := time.Now()
	result, output, err := h.SourceQuery(context.Background(), nil, input)
	duration := time.Since(started)
	if err != nil || (result != nil && result.IsError) || output.Search == nil {
		tb.Fatalf("source search failed: result=%+v err=%v", result, err)
	}
	return output, duration
}

func sourceCorpusOutputBytes(tb testing.TB, output SourceQueryOutput) int {
	tb.Helper()
	encoded, err := json.Marshal(output)
	if err != nil {
		tb.Fatal(err)
	}
	return len(encoded)
}

func TestSourceQueryRealCorpusPagesMatchCompleteSearch(t *testing.T) {
	fullInput := realSourceCorpusInput(t)
	h := NewHandler(fullInput.Paths)
	full, fullTime := runRealSourceCorpusSearch(t, h, fullInput)
	if full.Coverage.ResultsTruncated {
		t.Fatal("full-search baseline exceeded maxResults; select a narrower corpus or query")
	}
	if len(full.Search.Matches) <= 20 {
		t.Skipf("real corpus returned %d matches; require more than 20 for meaningful pagination", len(full.Search.Matches))
	}
	fullBytes := sourceCorpusOutputBytes(t, full)
	pageInput := fullInput
	pageInput.MaxResults = 20
	var combined []SourceSearchMatch
	var firstBytes, totalBytes, pageCount int
	var totalLatency time.Duration
	for {
		page, elapsed := runRealSourceCorpusSearch(t, h, pageInput)
		pageCount++
		if pageCount > 500 {
			t.Fatal("pagination did not terminate within the bounded request count")
		}
		if page.Index != full.Index {
			t.Fatalf("index generation changed on page %d", pageCount)
		}
		if page.Coverage.FilesConsidered != full.Coverage.FilesConsidered ||
			page.Coverage.FilesParsed != full.Coverage.FilesParsed ||
			page.Coverage.FilesSkipped != full.Coverage.FilesSkipped ||
			page.Coverage.FileSelectionTruncated != full.Coverage.FileSelectionTruncated ||
			page.Coverage.AnalysisComplete != full.Coverage.AnalysisComplete {
			t.Fatalf("source file coverage changed on page %d", pageCount)
		}
		if len(page.Search.Matches) > 20 {
			t.Fatalf("page %d exceeded the result limit", pageCount)
		}
		combined = append(combined, page.Search.Matches...)
		size := sourceCorpusOutputBytes(t, page)
		totalBytes += size
		totalLatency += elapsed
		if pageCount == 1 {
			firstBytes = size
		}
		if page.Search.NextCursor == "" {
			if page.Coverage.ResultsTruncated {
				t.Fatal("last page remains truncated without a continuation")
			}
			break
		}
		if !page.Coverage.ResultsTruncated {
			t.Fatal("page has continuation despite reporting no truncation")
		}
		pageInput.Cursor = page.Search.NextCursor
	}
	if !reflect.DeepEqual(combined, full.Search.Matches) {
		t.Fatalf("paged results differ from complete search: paged=%d complete=%d", len(combined), len(full.Search.Matches))
	}
	t.Logf("real corpus: considered=%d parsed=%d skipped=%d selectionTruncated=%t analysisComplete=%t",
		full.Coverage.FilesConsidered, full.Coverage.FilesParsed, full.Coverage.FilesSkipped,
		full.Coverage.FileSelectionTruncated, full.Coverage.AnalysisComplete)
	t.Logf("real corpus: matches=%d pages=%d fullBytes=%d firstPageBytes=%d allPageBytes=%d fullLatency=%s allPagesLatency=%s",
		len(full.Search.Matches), pageCount, fullBytes, firstBytes, totalBytes, fullTime, totalLatency)
}

// Real-source tasks are fixed probes for later paired client trials, not
// simulated LLM behavior or evidence of reduced tool calls.
type realSourceTaskSpec struct {
	language, query, file, kind string
}

var realSourceTaskSpecs = map[string]realSourceTaskSpec{
	"go-router":        {language: "go", query: "NewRouter", file: "chi.go", kind: "function"},
	"python-api-key":   {language: "python", query: "APIKeyBase", file: "api_key.py", kind: "class"},
	"csharp-validator": {language: "csharp", query: "ConfigurationValidator", file: "ConfigurationValidator.cs", kind: "class"},
}

func realSourceTask(tb testing.TB) (string, realSourceTaskSpec) {
	tb.Helper()
	task := os.Getenv("SCRIPTHOLD_REAL_CORPUS_TASK")
	if task == "" {
		tb.Skip("set SCRIPTHOLD_REAL_CORPUS_TASK and SCRIPTHOLD_REAL_CORPUS_DIR to opt in")
	}
	want, ok := realSourceTaskSpecs[task]
	if !ok {
		tb.Fatalf("unsupported task oracle %q", task)
	}
	return task, want
}

// This single-file oracle checks known evidence without testing file discovery.
func TestSourceQueryRealCorpusTaskOracle(t *testing.T) {
	task, want := realSourceTask(t)
	input := realSourceCorpusInput(t)
	root := input.Paths[0]
	// The oracle checks known source evidence, not the agent's ability to discover
	// this file within a larger truncated project search.
	input.Paths = []string{filepath.Join(root, want.file)}
	input.Language = want.language
	input.Query = want.query
	input.Match = "exact"
	input.MaxResults = 128
	h := NewHandler([]string{root})
	output, duration := runRealSourceCorpusSearch(t, h, input)
	if output.Coverage.FilesConsidered != 1 || output.Coverage.FilesSkipped != 0 ||
		output.Coverage.FilesParsed != 1 || output.Coverage.FileSelectionTruncated {
		t.Fatalf("selected-file analysis incomplete: %+v", output.Coverage)
	}
	var declarations []SourceSearchMatch
	for _, match := range output.Search.Matches {
		if match.Name == want.query && filepath.Base(match.Path) == want.file &&
			match.SymbolID != "" && string(match.Kind) == want.kind && match.SourceFingerprint != "" {
			declarations = append(declarations, match)
		}
	}
	if len(declarations) != 1 {
		t.Fatalf("%s: wanted one identified declaration in %s, got %d from %d matches (coverage=%+v)",
			task, want.file, len(declarations), len(output.Search.Matches), output.Coverage)
	}
	if declarations[0].Range.Start.Line <= 0 {
		t.Fatalf("task declaration has invalid source location: %+v", declarations[0])
	}
	t.Logf("task=%s declaration=%s kind=%s range=%+v matches=%d files=%d bytes=%d duration=%s",
		task, want.query, declarations[0].Kind, declarations[0].Range,
		len(output.Search.Matches), output.Coverage.FilesParsed,
		sourceCorpusOutputBytes(t, output), duration)
	t.Logf("fileSelectionTruncated=%t analysisComplete=%t", output.Coverage.FileSelectionTruncated, output.Coverage.AnalysisComplete)
}

// This probe tests whether a client can locate the known declaration when the
// selected project contains multiple files, rather than naming the file first.
// Require complete file selection; analyzer coverage is reported separately.
func TestSourceQueryRealCorpusTaskDiscovery(t *testing.T) {
	task, want := realSourceTask(t)
	input := realSourceCorpusInput(t)
	input.Query = want.query
	input.Match = "exact"
	input.Language = want.language
	input.MaxFiles = 256
	input.MaxResults = 128
	output, elapsed := runRealSourceCorpusSearch(t, NewHandler(input.Paths), input)
	if output.Coverage.FileSelectionTruncated || output.Coverage.FilesParsed <= 1 ||
		output.Coverage.FilesSkipped != 0 {
		t.Fatalf("%s: multi-file scope has incomplete file selection: %+v", task, output.Coverage)
	}
	if output.Coverage.ResultsTruncated {
		t.Fatalf("%s: declaration search exhausted the result budget", task)
	}
	target := filepath.Clean(filepath.Join(input.Paths[0], want.file))
	var matching []SourceSearchMatch
	for _, match := range output.Search.Matches {
		if filepath.Clean(match.Path) == target && match.Name == want.query &&
			string(match.Kind) == want.kind && match.SymbolID != "" &&
			match.SourceFingerprint != "" && match.Range.Start.Line > 0 {
			matching = append(matching, match)
		}
	}
	if len(matching) != 1 {
		t.Fatalf("%s: expected one identified declaration at %s; got %d of %d matches (coverage=%+v)",
			task, target, len(matching), len(output.Search.Matches), output.Coverage)
	}
	t.Logf("task=%s filesConsidered=%d filesParsed=%d analysisComplete=%t matches=%d bytes=%d duration=%s",
		task, output.Coverage.FilesConsidered, output.Coverage.FilesParsed,
		output.Coverage.AnalysisComplete, len(output.Search.Matches), sourceCorpusOutputBytes(t, output), elapsed)
}

func BenchmarkSourceQueryRealCorpusWarm(b *testing.B) {
	fullInput := realSourceCorpusInput(b)
	h := NewHandler(fullInput.Paths)
	seed, _ := runRealSourceCorpusSearch(b, h, fullInput)
	if seed.Coverage.ResultsTruncated || len(seed.Search.Matches) <= 20 {
		b.Skip("corpus has no useful untruncated comparison at this query")
	}
	for _, comparison := range []struct {
		name string
		size int
	}{
		{name: "complete", size: 10_000},
		{name: "page20", size: 20},
	} {
		b.Run(comparison.name, func(b *testing.B) {
			request := fullInput
			request.MaxResults = comparison.size
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				out, _ := runRealSourceCorpusSearch(b, h, request)
				if len(out.Search.Matches) == 0 || len(out.Search.Matches) > comparison.size {
					b.Fatal("search returned an unexpected number of matches")
				}
			}
		})
	}
}
