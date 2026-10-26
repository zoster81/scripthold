package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// This opt-in probe measures a real two-call search/context path without an LLM.
func TestSourceQueryRealCorpusTaskSearchContextWorkflow(t *testing.T) {
	task, want := realSourceTask(t)
	input := realSourceCorpusInput(t)
	input.Query, input.Match, input.Language, input.Encoding = want.query, "exact", want.language, ""
	input.MaxFiles, input.MaxResults = 256, 128
	h := NewHandler(input.Paths)
	search, searchTime := runRealSourceCorpusSearch(t, h, input)
	var target *SourceSearchMatch
	for i := range search.Search.Matches {
		match := &search.Search.Matches[i]
		if match.Name == want.query && string(match.Kind) == want.kind &&
			filepath.Base(match.Path) == want.file && match.SymbolID != "" {
			target = match
			break
		}
	}
	if target == nil {
		t.Fatalf("%s: search did not identify the expected declaration", task)
	}
	request := SourceQueryInput{
		Operation: "context", Paths: input.Paths, Language: want.language,
		Targets: []SourceSelectorInput{{
			Kind: "symbol", Path: target.Path, SymbolID: target.SymbolID,
			SourceFingerprint: target.SourceFingerprint,
		}},
		BudgetBytes: 18000, MaxItems: 8, BodyPolicy: "prefer",
	}
	started := time.Now()
	result, output, err := h.SourceQuery(context.Background(), nil, request)
	contextTime := time.Since(started)
	if err != nil || (result != nil && result.IsError) || output.Context == nil {
		t.Fatalf("%s: source context failed: result=%+v err=%v", task, result, err)
	}
	if len(output.Context.Items) == 0 {
		t.Fatalf("%s: source context returned no items", task)
	}
	item := output.Context.Items[0]
	if item.Entity.SymbolID != target.SymbolID || string(item.Representation) != "body" {
		t.Fatalf("%s: first context item is not the verified target body", task)
	}
	expected := map[string][]string{
		"go-router":        {"return NewMux()"},
		"python-api-key":   {"HTTP_401_UNAUTHORIZED", "WWW-Authenticate", "if self.auto_error", "return None"},
		"csharp-validator": {"DuplicateTypeMapConfigurationException", "GetUnmappedPropertyNames()", "PassesCtorValidation"},
	}
	for _, fact := range expected[task] {
		if !strings.Contains(item.Text, fact) {
			t.Fatalf("%s: target context lacks verified fact %q", task, fact)
		}
	}
	searchBytes, contextBytes := sourceCorpusOutputBytes(t, search), sourceCorpusOutputBytes(t, output)
	t.Logf("task=%s calls=2 searchBytes=%d contextBytes=%d totalBytes=%d contextItems=%d targetBodyBytes=%d searchDuration=%s contextDuration=%s",
		task, searchBytes, contextBytes, searchBytes+contextBytes,
		len(output.Context.Items), len(item.Text), searchTime, contextTime)
}

// Compare opt-in file-evidence size on read-only third-party source trees.
func TestSourceSymbolsRealCorpusRelevantFileDetails(t *testing.T) {
	task, want := realSourceTask(t)
	corpus := realSourceCorpusInput(t)
	h := NewHandler(corpus.Paths)
	request := SourceSymbolsInput{
		Operation: "find", Paths: corpus.Paths, Query: want.query,
		Match: "exact", Language: want.language, MaxFiles: 256, MaxSymbols: 128,
	}
	toolErr, full, err := h.SourceSymbols(context.Background(), nil, request)
	if err != nil || toolErr != nil {
		t.Fatalf("%s: full source symbols failed: result=%+v err=%v", task, toolErr, err)
	}
	request.FileDetails = "relevant"
	toolErr, compact, err := h.SourceSymbols(context.Background(), nil, request)
	if err != nil || toolErr != nil {
		t.Fatalf("%s: compact source symbols failed: result=%+v err=%v", task, toolErr, err)
	}
	if !reflect.DeepEqual(full.Symbols, compact.Symbols) ||
		full.FilesConsidered != compact.FilesConsidered || full.FilesParsed != compact.FilesParsed ||
		full.FilesSkipped != compact.FilesSkipped || full.CoverageComplete != compact.CoverageComplete ||
		full.Truncated != compact.Truncated || full.Ambiguous != compact.Ambiguous {
		t.Fatalf("%s: relevant file detail changed symbols or coverage", task)
	}
	needed := make(map[string]bool)
	var targetFound bool
	for _, symbol := range full.Symbols {
		needed[symbol.Path] = true
		if symbol.Name == want.query && string(symbol.Kind) == want.kind && filepath.Base(symbol.Path) == want.file {
			targetFound = true
		}
	}
	for _, file := range full.Files {
		if file.Status != "parsed" || !file.CoverageComplete {
			needed[file.Path] = true
		}
	}
	if !targetFound || len(needed) != len(compact.Files) {
		t.Fatalf("%s: missing oracle or relevant file evidence: matches=%d files=%d expected=%d", task, len(full.Symbols), len(compact.Files), len(needed))
	}
	for _, file := range compact.Files {
		if !needed[file.Path] {
			t.Fatalf("%s: unexpected irrelevant file record", task)
		}
	}
	fullJSON, err := json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	compactJSON, err := json.Marshal(compact)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("task=%s fullBytes=%d relevantBytes=%d fullFiles=%d relevantFiles=%d matches=%d coverageComplete=%t",
		task, len(fullJSON), len(compactJSON), len(full.Files), len(compact.Files), len(full.Symbols), compact.CoverageComplete)
}

// This opt-in comparison uses unchanged third-party source files and identical analyzer settings.
func TestSourceSymbolsRealCorpusOutlinePresentations(t *testing.T) {
	task, want := realSourceTask(t)
	corpus := realSourceCorpusInput(t)
	path := filepath.Join(corpus.Paths[0], want.file)
	h := NewHandler(corpus.Paths)
	read := func(presentation string) SourceSymbolsOutput {
		t.Helper()
		result, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "outline", Paths: []string{path}, Language: want.language,
			IncludeSignatures: true, Presentation: presentation,
		})
		if err != nil || result != nil {
			t.Fatalf("%s: %s returned %+v err=%v", task, presentation, result, err)
		}
		return out
	}
	detailed := read("")
	if len(detailed.Files) != 1 || !detailed.CoverageComplete || detailed.SymbolCount == 0 {
		t.Fatalf("%s: unexpected detailed coverage", task)
	}
	fullJSON, err := json.Marshal(detailed)
	if err != nil {
		t.Fatal(err)
	}
	for _, presentation := range []string{"compact", "tree"} {
		out := read(presentation)
		if out.SymbolCount != detailed.SymbolCount ||
			out.FilesConsidered != detailed.FilesConsidered || out.FilesParsed != detailed.FilesParsed ||
			out.FilesSkipped != detailed.FilesSkipped || out.CoverageComplete != detailed.CoverageComplete ||
			len(out.OutlineFiles) != 1 || out.OutlineFiles[0].SourceFingerprint != detailed.Files[0].SourceFingerprint {
			t.Fatalf("%s: %s lost coverage or file identity", task, presentation)
		}
		var walk func([]*SourceOutlineNode)
		ids := make(map[string]bool)
		walk = func(nodes []*SourceOutlineNode) {
			for _, node := range nodes {
				if ids[node.SymbolID] {
					t.Fatalf("%s: duplicated symbol %s", task, node.Name)
				}
				ids[node.SymbolID] = true
				walk(node.Children)
			}
		}
		walk(out.OutlineFiles[0].Symbols)
		if len(ids) != detailed.SymbolCount {
			t.Fatalf("%s: %s retained %d/%d declarations", task, presentation, len(ids), detailed.SymbolCount)
		}
		payload, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		if len(payload) >= len(fullJSON) {
			t.Fatalf("%s: %s did not reduce response bytes: %d >= %d", task, presentation, len(payload), len(fullJSON))
		}
		t.Logf("task=%s presentation=%s detailedBytes=%d presentedBytes=%d symbols=%d", task, presentation, len(fullJSON), len(payload), out.SymbolCount)
	}
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
