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
			if tc.name == "c-obsproject-obs-studio" {
				// A known, source-verified conditional linkage mismatch is not
				// silently promoted to completeness; any new gap is a regression.
				if len(outstanding) > 1 || len(outstanding) == 1 && outstanding[0] != "deps/w32-pthreads/implement.h" {
					t.Errorf("unreviewed OBS original incomplete sources: %v", outstanding)
				}
			}
			if tc.name == "c-ffmpeg-ffmpeg" {
				if len(partial) != 0 {
					t.Fatalf("FFmpeg still has %d partially analyzed sources: %v", len(partial), outstanding)
				}
				// In the same complete 4,670-file original scope, distinguish
				// the 61 grep occurrences (mostly calls) from two declarations.
				expected := map[string]int{
					filepath.Join(project, "libavutil", "crc.h"): 83,
					filepath.Join(project, "libavutil", "crc.c"): 389,
				}
				matched := make(map[string]int)
				for start := 0; start < len(files); start += batchSize {
					end := min(start+batchSize, len(files))
					failed, found, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
						Operation: "search", Mode: "structural", Match: "exact",
						Query: "av_crc_get_table", Paths: files[start:end],
						Language: "c", Encoding: "utf-8", MaxFiles: batchSize, MaxResults: 32,
					})
					if err != nil || failed != nil || !found.Coverage.CoverageComplete ||
						found.Coverage.FilesParsed != end-start || found.Search == nil {
						t.Fatalf("original FFmpeg project-wide structural search incomplete at %d:%d: %v %+v %+v", start, end, err, failed, found.Coverage)
					}
					for _, match := range found.Search.Matches {
						if string(match.Kind) != "function" || match.QualifiedName != "av_crc_get_table" ||
							match.SymbolID == "" || match.SourceFingerprint != fingerprints[match.Path] {
							t.Errorf("unexpected unbound FFmpeg structural match: %+v", match)
						}
						if _, exists := matched[match.Path]; exists {
							t.Errorf("duplicate FFmpeg project-wide API in %s", match.Path)
						}
						matched[match.Path] = match.Range.Start.Line
					}
				}
				if len(matched) != len(expected) {
					t.Errorf("FFmpeg expected two structural declarations, found %v", matched)
				}
				for path, line := range expected {
					if matched[path] != line {
						t.Errorf("FFmpeg API expected %s:%d; found %d", path, line, matched[path])
					}
				}
			}
			t.Logf("original %s: selected/parsed=%d/%d; complete=%d; partial=%d (see selected semantic scope separately): %v",
				tc.name, len(seen), len(files), len(seen)-len(outstanding), len(outstanding), outstanding)
		})
	}
}
