package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSystemVerilogSequencesTwoOriginalModules(t *testing.T) {
	for _, tc := range []struct {
		env, owner, neighbor, marker string
		lines                        map[string]int
	}{
		{
			env: "SCRIPTHOLD_REAL_SV_SEQUENCE_GENTLEMAN", owner: "sequence_triggered",
			neighbor: "sequence_triggered.request", marker: "module sequence_triggered(input bit clk);",
			lines: map[string]int{"accepted_request": 12},
		},
		{
			env: "SCRIPTHOLD_REAL_SV_SEQUENCE_OPENFORMAL", owner: "sequence_operators_tb",
			neighbor: "sequence_operators_tb.clock", marker: "module sequence_operators_tb();",
			lines: map[string]int{"grant_1_5_S": 57, "grant_2_6_S": 61, "grant_1_6_S": 79, "grant_1_4_5_S": 112, "request_6_fell_S": 165},
		},
	} {
		t.Run(tc.owner, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independent original source not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 500 || fi.Size() > 65536 {
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
				if len(authored) < line || !strings.Contains(authored[line-1], "sequence "+name+";") {
					t.Fatalf("authored sequence changed: %s:%d", name, line)
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
						sym.NativeKind == "sequence" && sym.ParentQualifiedName == tc.owner &&
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
					t.Errorf("directly authored sequence missing: %s.%s", tc.owner, name)
				}
			}
		})
	}
}
