package handler

import (
	"context"
	"encoding/json"
	"os"
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
