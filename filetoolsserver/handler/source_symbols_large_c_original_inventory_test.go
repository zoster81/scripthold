package handler

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// The full original-project source set must remain observable when individual
// files have unsupported syntax. Only the fully analyzed files count toward
// complete structural screening; partial files are explicit and not dropped.
func TestSourceSymbolsOriginalLargeCProjectInventory(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus not configured")
	}
	for _, tc := range []struct {
		name  string
		count int
	}{
		{"c-obsproject-obs-studio", 1024},
		{"c-ffmpeg-ffmpeg", 4670},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := filepath.Join(root, "real-projects", tc.name)
			var files []string
			err := filepath.WalkDir(project, func(path string, e fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if e.IsDir() && e.Name() == ".git" {
					return filepath.SkipDir
				}
				if e.Type().IsRegular() && (filepath.Ext(path) == ".c" || filepath.Ext(path) == ".h") {
					files = append(files, path)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			sort.Strings(files)
			if len(files) != tc.count {
				t.Fatalf("original source count changed: %d vs %d", len(files), tc.count)
			}
			h := NewHandler([]string{root})
			seen := make(map[string]bool, tc.count)
			fingerprints := make(map[string]string, tc.count)
			partial := make(map[string]bool)
			const batchSize = 32
			for start := 0; start < len(files); start += batchSize {
				end := min(start+batchSize, len(files))
				fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "digest", Presentation: "compact", Paths: files[start:end],
					Language: "c", Encoding: "utf-8", MaxFiles: batchSize,
				})
				if err != nil || fail != nil || out.Truncated || out.FilesConsidered != end-start ||
					out.FilesParsed != end-start || out.FilesSkipped != 0 || len(out.OverviewFiles) != end-start {
					t.Fatalf("incomplete source file selection batch [%d:%d]: err=%v fail=%+v considered=%d parsed=%d skipped=%d", start, end, err, fail, out.FilesConsidered, out.FilesParsed, out.FilesSkipped)
				}
				part := false
				for _, f := range out.OverviewFiles {
					if seen[f.Path] || f.Path == "" || f.Status != "parsed" || f.SourceFingerprint == "" || f.Language != "c" {
						t.Errorf("unexpected/duplicate/unparsed original source: %+v", f)
					}
					seen[f.Path] = true
					fingerprints[f.Path] = f.SourceFingerprint
					if !f.CoverageComplete {
						part = true
						partial[f.Path] = true
					}
				}
				if out.CoverageComplete == part {
					t.Errorf("source-level and project batch completeness disagree at [%d:%d]", start, end)
				}
			}
			if len(seen) != len(files) {
				t.Errorf("indexed %d/%d original files", len(seen), len(files))
			}
			for _, p := range files {
				if !seen[p] {
					t.Errorf("original file omitted: %s", p)
				}
			}
			var outstanding []string
			for path := range partial {
				rel, err := filepath.Rel(project, path)
				if err != nil {
					t.Fatal(err)
				}
				outstanding = append(outstanding, filepath.ToSlash(rel))
			}
			sort.Strings(outstanding)
			if len(partial) != 0 {
				t.Fatalf("%s has %d partially analyzed original sources: %v", tc.name, len(partial), outstanding)
			}
			// The whole-project source search must distinguish independently
			// verified declarations from text calls found by grep.
			{
				query := "av_crc_get_table"
				expected := map[string]int{
					filepath.Join(project, "libavutil", "crc.h"): 83,
					filepath.Join(project, "libavutil", "crc.c"): 389,
				}
				if tc.name == "c-obsproject-obs-studio" {
					query = "calc_crc32"
					expected = map[string]int{
						filepath.Join(project, "libobs", "util", "crc32.h"): 25,
						filepath.Join(project, "libobs", "util", "crc32.c"): 53,
					}
				}
				matched := make(map[string]int)
				for start := 0; start < len(files); start += batchSize {
					end := min(start+batchSize, len(files))
					failed, found, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
						Operation: "search", Mode: "structural", Match: "exact",
						Query: query, Paths: files[start:end],
						Language: "c", Encoding: "utf-8", MaxFiles: batchSize, MaxResults: 32,
					})
					if err != nil || failed != nil || !found.Coverage.CoverageComplete ||
						found.Coverage.FilesParsed != end-start || found.Search == nil {
						t.Fatalf("original FFmpeg project-wide structural search incomplete at %d:%d: %v %+v %+v", start, end, err, failed, found.Coverage)
					}
					for _, match := range found.Search.Matches {
						if string(match.Kind) != "function" || match.QualifiedName != query ||
							match.SymbolID == "" || match.SourceFingerprint != fingerprints[match.Path] {
							t.Errorf("unexpected unbound %s structural match: %+v", tc.name, match)
						}
						if _, exists := matched[match.Path]; exists {
							t.Errorf("duplicate %s project-wide API in %s", tc.name, match.Path)
						}
						matched[match.Path] = match.Range.Start.Line
					}
				}
				if len(matched) != len(expected) {
					t.Errorf("%s expected two structural declarations, found %v", tc.name, matched)
				}
				for path, line := range expected {
					if matched[path] != line {
						t.Errorf("%s API expected %s:%d; found %d", tc.name, path, line, matched[path])
					}
				}
			}
			t.Logf("original %s: selected/parsed=%d/%d; complete=%d; partial=%d (see selected semantic scope separately): %v",
				tc.name, len(seen), len(files), len(seen)-len(outstanding), len(outstanding), outstanding)
		})
	}
}
