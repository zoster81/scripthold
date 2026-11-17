package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

// The two unexecuted, complete originals are pinned to upstream Git blobs in
// private operator state. This test exercises lexical navigation only.
func TestSystemVerilogCovergroupsTwoOriginalModules(t *testing.T) {
	for _, tc := range []struct {
		env, owner, neighbor, marker string
		lines                        map[string]int
	}{
		{
			env: "SCRIPTHOLD_REAL_SV_COVERGROUP_AHB_LITE", owner: "testbench_top",
			neighbor: "testbench_top.clk", marker: "module testbench_top;",
			lines: map[string]int{"cover_burst": 48, "cover_size": 65, "cover_trans": 72, "cover_address": 90, "cross_cover_HBURST_HSIZE": 102},
		},
		{
			env: "SCRIPTHOLD_REAL_SV_COVERGROUP_CALIPTRA", owner: "ahb_slv_sif",
			neighbor: "ahb_slv_sif.err_f", marker: "module ahb_slv_sif ",
			lines: map[string]int{"ahb_slv_sif_cov_grp": 162},
		},
	} {
		t.Run(tc.owner, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete original project file not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 1500 || fi.Size() > 65536 {
				t.Fatalf("invalid original source: %v %v", fi, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			if strings.Count(source, tc.marker) != 1 {
				t.Fatal("original module marker changed")
			}
			authored := strings.Split(source, "\n")
			for name, line := range tc.lines {
				if len(authored) < line || !strings.Contains(authored[line-1], "covergroup "+name+" ") {
					t.Fatalf("original authored covergroup drift: %s:%d", name, line)
				}
			}
			doc := sourceDocumentForScanner(source)
			doc.Path = path
			result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			owner, neighbor := false, false
			found := map[string]bool{}
			for _, s := range result.Analysis.Symbols {
				if s.QualifiedName == tc.owner && s.Kind == SymbolKindModule {
					owner = true
				}
				if s.QualifiedName == tc.neighbor {
					neighbor = true
				}
				for name, line := range tc.lines {
					if s.QualifiedName == tc.owner+"."+name && s.Kind == SymbolKindType &&
						s.NativeKind == "covergroup" && s.ParentQualifiedName == tc.owner &&
						s.NameRange.Start.Line == line {
						found[name] = true
					}
				}
			}
			if !owner || !neighbor {
				t.Errorf("existing module/neighbor absent: owner=%v neighbor=%v", owner, neighbor)
			}
			for name := range tc.lines {
				if !found[name] {
					t.Errorf("directly authored covergroup missing: %s.%s", tc.owner, name)
				}
			}
		})
	}
}
