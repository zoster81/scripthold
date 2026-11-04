package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Independent source reads/grep establish maintenance targets, neighbors and
// alternatives before checking source_* on original read-only VHDL projects.
func TestSourceSymbolsOriginalVHDLTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original VHDL corpus not configured")
	}
	type target struct {
		file, name, kind, excerpt string
		line                      int
	}
	type evidence struct{ file, text string }
	cases := []struct {
		project, query, unrelated string
		files                     []string
		evidence                  []evidence
		targets                   []target
	}{
		{
			project: "vhdl-stnolting-neorv32", query: "IO_GPTMR_NUM",
			files:     []string{"rtl/core/neorv32_top.vhd", "rtl/core/neorv32_gptmr.vhd", "rtl/core/neorv32_clint.vhd"},
			unrelated: "rtl/core/neorv32_clint.vhd",
			evidence: []evidence{
				{"rtl/core/neorv32_top.vhd", "IO_GPTMR_NUM        : natural range 0 to 16          := 0;"},
				{"rtl/core/neorv32_top.vhd", "io_gptmr_en_c   : boolean := boolean(IO_GPTMR_NUM > 0);"},
				{"rtl/core/neorv32_top.vhd", "NUM_SLICES => IO_GPTMR_NUM"},
				{"rtl/core/neorv32_gptmr.vhd", "for i in 0 to NUM_SLICES-1 generate"},
				{"rtl/core/neorv32_package.vhd", "IO_GPTMR_NUM        : natural range 0 to 16          := 0;"},
				{"sw/example/demo_gptmr/main.c", "neorv32_gptmr_get_num_slices()"},
			},
			targets: []target{
				{"rtl/core/neorv32_top.vhd", "IO_GPTMR_NUM", "constant", "IO_GPTMR_NUM        : natural range 0 to 16", 165},
				{"rtl/core/neorv32_gptmr.vhd", "NUM_SLICES", "constant", "NUM_SLICES : natural range 0 to 16", 22},
			},
		},
		{
			project: "vhdl-open-logic-open-logic", query: "AlmFullLevel_g",
			files:     []string{"src/base/vhdl/olo_base_fifo_sync.vhd", "src/base/vhdl/olo_base_fifo_async.vhd", "test/base/olo_base_fifo_sync/olo_base_fifo_sync_tb.vhd"},
			unrelated: "src/base/vhdl/olo_base_crc.vhd",
			evidence: []evidence{
				{"src/base/vhdl/olo_base_fifo_sync.vhd", "AlmFullLevel_g  : natural   := 0;"},
				{"src/base/vhdl/olo_base_fifo_sync.vhd", "if AlmFullOn_g and unsigned(r.WrLevel) >= AlmFullLevel_g then"},
				{"src/base/vhdl/olo_base_fifo_async.vhd", "only power of two Depth_g is allowed"},
				{"test/base/olo_base_fifo_sync/olo_base_fifo_sync_tb.vhd", "constant AlmFullLevel_c  : natural := Depth_g - 3;"},
				{"test/base/olo_base_fifo_sync/olo_base_fifo_sync_tb.vhd", "AlmFullLevel_g      => AlmFullLevel_c,"},
			},
			targets: []target{
				{"src/base/vhdl/olo_base_fifo_sync.vhd", "AlmFullLevel_g", "constant", "AlmFullLevel_g  : natural", 37},
				{"src/base/vhdl/olo_base_fifo_sync.vhd", "Depth_g", "constant", "Depth_g         : positive", 35},
				{"src/base/vhdl/olo_base_fifo_async.vhd", "AlmFullLevel_g", "constant", "AlmFullLevel_g  : natural", 39},
				{"test/base/olo_base_fifo_sync/olo_base_fifo_sync_tb.vhd", "Depth_g", "constant", "Depth_g         : natural", 31},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			h := NewHandler([]string{root})
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original file unavailable %s: %v", rel, err)
				}
				paths = append(paths, path)
			}
			for _, oracle := range tc.evidence {
				data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(oracle.file)))
				if err != nil || !strings.Contains(string(data), oracle.text) {
					t.Fatalf("original source evidence changed %s %q: %v", oracle.file, oracle.text, err)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "vhdl", Encoding: "utf-8",
				Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("bounded compact digest: %v failed=%+v result=%+v", err, failed, digest)
			}
			data, err := json.Marshal(digest)
			if err != nil || len(data) > 8192 {
				t.Fatalf("compact digest too large: bytes=%d err=%v", len(data), err)
			}
			t.Logf("originalProjects=1 files=%d symbols=%d compactBytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(data), digest.CoverageComplete)
			for _, want := range tc.targets {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Query: want.name, Match: "exact",
					Language: "vhdl", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("find %s: %v failed=%+v result=%+v", want.name, err, failed, found)
				}
				var id string
				for _, sym := range found.Symbols {
					if sym.Name == want.name && string(sym.Kind) == want.kind && sym.NativeKind == "generic" &&
						sym.NameRange.Start.Line == want.line {
						id = sym.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("missing original entity generic %s:%d %s results=%+v", want.file, want.line, want.name, found.Symbols)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint,
					Language: "vhdl", Encoding: "utf-8", MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.excerpt) {
					t.Fatalf("verified show %s failed: %v failed=%+v show=%+v", want.name, err, failed, shown.Show)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Language: "vhdl", Encoding: "utf-8",
				Query: tc.query, Match: "exact", Mode: "structural", MaxResults: 20,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
				t.Fatalf("structural search %s: %v failed=%+v result=%+v", tc.query, err, failed, search.Search)
			}
			failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))},
				Language: "vhdl", Encoding: "utf-8", Query: tc.query, Match: "exact", MaxFiles: 1, MaxSymbols: 16,
			})
			if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated file invented %s: %v failed=%+v symbols=%+v", tc.query, err, failed, negative.Symbols)
			}
		})
	}
}
