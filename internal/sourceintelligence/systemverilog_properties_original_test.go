package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSystemVerilogPropertiesTwoOriginalModules(t *testing.T) {
	for _, tc := range []struct {
		env, owner, neighbor, marker string
		lines                        map[string]int
	}{
		{
			env: "SCRIPTHOLD_REAL_SV_PROPERTY_TINY_TPU", owner: "pe_assertions",
			neighbor: "pe_assertions.pe_valid_out", marker: "module pe_assertions (",
			lines: map[string]int{"p_rst_clears_psum": 43, "p_rst_clears_valid": 47, "p_rst_clears_switch": 51, "p_overflow_is_sticky": 218},
		},
		{
			env: "SCRIPTHOLD_REAL_SV_PROPERTY_OPENNOC", owner: "assert_checker",
			neighbor: "assert_checker.clk", marker: "module assert_checker #(",
			lines: map[string]int{"check_condition": 41},
		},
	} {
		t.Run(tc.owner, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independent original source not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 1000 || fi.Size() > 65536 {
				t.Fatalf("invalid original source: %v %v", fi, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			src := string(raw)
			if strings.Count(src, tc.marker) != 1 {
				t.Fatal("original module marker changed")
			}
			authored := strings.Split(src, "\n")
			for name, line := range tc.lines {
				if len(authored) < line || !strings.Contains(authored[line-1], "property "+name+";") {
					t.Fatalf("authored original property drift: %s:%d", name, line)
				}
			}
			doc := sourceDocumentForScanner(src)
			doc.Path = path
			result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			owner, neighbor := false, false
			found := map[string]bool{}
			for _, sym := range result.Analysis.Symbols {
				if sym.QualifiedName == tc.owner && sym.Kind == SymbolKindModule {
					owner = true
				}
				if sym.QualifiedName == tc.neighbor {
					neighbor = true
				}
				for name, line := range tc.lines {
					if sym.QualifiedName == tc.owner+"."+name && sym.Kind == SymbolKindEntity &&
						sym.NativeKind == "property" && sym.ParentQualifiedName == tc.owner &&
						sym.NameRange.Start.Line == line {
						found[name] = true
					}
				}
			}
			if !owner || !neighbor {
				t.Errorf("existing module/neighbor absent: owner=%v neighbor=%v", owner, neighbor)
			}
			for name := range tc.lines {
				if !found[name] {
					t.Errorf("directly authored property missing: %s.%s", tc.owner, name)
				}
			}
		})
	}
}
