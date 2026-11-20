package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// This fully annotates the original FFmpeg CRC public header, not the whole
// FFmpeg project. Unsupported sources elsewhere remain a qualification gap.
func TestSourceSymbolsOriginalFFmpegCRCHeaderOracle(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	project := filepath.Join(root, "real-projects", "c-ffmpeg-ffmpeg", "libavutil")
	path := filepath.Join(project, "crc.h")
	source := filepath.Join(project, "crc.c")
	h := NewHandler([]string{root})
	fail, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 50,
	})
	if err != nil || fail != nil || !outline.CoverageComplete ||
		len(outline.Files) != 1 {
		t.Fatalf("FFmpeg original CRC header unavailable: %v %+v", err, fail)
	}
	type expected struct {
		kind string
		line int
	}
	want := map[string]expected{
		"AVCRC":            {"alias", 46},
		"AVCRCId":          {"alias", 58}, // Original balanced anonymous enum typedef.
		"av_crc_init":      {"function", 76},
		"av_crc_get_table": {"function", 83},
		"av_crc":           {"function", 95},
	}
	if len(outline.Symbols) != len(want) {
		t.Errorf("FFmpeg crc.h declaration count %d != %d", len(outline.Symbols), len(want))
	}
	for _, symbol := range outline.Symbols {
		expected, present := want[symbol.QualifiedName]
		if !present || string(symbol.Kind) != expected.kind ||
			symbol.NameRange.Start.Line != expected.line ||
			symbol.ParentQualifiedName != "" || symbol.ID == "" {
			t.Errorf("FFmpeg CRC header unexpected/incorrect declaration: %+v want=%+v present=%v", symbol, expected, present)
		}
		delete(want, symbol.QualifiedName)
	}
	for missing := range want {
		t.Errorf("FFmpeg CRC public header missing %s", missing)
	}

	fail, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Mode: "structural", Query: "av_crc_get_table", Match: "exact",
		Paths: []string{path, source}, Language: "c", Encoding: "utf-8", MaxFiles: 2, MaxResults: 10,
	})
	if err != nil || fail != nil || !search.Coverage.CoverageComplete ||
		search.Search == nil || len(search.Search.Matches) != 2 {
		t.Fatalf("FFmpeg header/definition cross-file search failed: %v %+v", err, fail)
	}
	expectedLocations := map[string]int{path: 83, source: 389}
	for _, match := range search.Search.Matches {
		line, present := expectedLocations[match.Path]
		if !present || match.Range.Start.Line != line ||
			string(match.Kind) != "function" || match.QualifiedName != "av_crc_get_table" {
			t.Errorf("FFmpeg CRC original API mismatch: %+v", match)
		}
		delete(expectedLocations, match.Path)
	}
	for missing := range expectedLocations {
		t.Errorf("FFmpeg API origin missing %s", missing)
	}
}
