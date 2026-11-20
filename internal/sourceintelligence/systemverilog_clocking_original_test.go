package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSystemVerilogClockingTwoOriginalInterfaces(t *testing.T) {
	for _, tc := range []struct {
		env, owner, neighbor string
		clocking             map[string]int
	}{
		{"SCRIPTHOLD_REAL_SV_CLOCKING_TVIP", "tvip_axi_if", "tvip_axi_if.awvalid",
			map[string]int{"master_cb": 50, "slave_cb": 88, "monitor_cb": 126}},
		{"SCRIPTHOLD_REAL_SV_CLOCKING_CESNET", "mfb_if", "mfb_if.DATA",
			map[string]int{"driver_rx_cb": 42, "driver_tx_cb": 47, "monitor_cb": 54}},
	} {
		t.Run(tc.owner, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independently authored original not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 1000 || fi.Size() > 65536 {
				t.Fatalf("invalid original source: %v %v", fi, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(raw), "\n")
			for name, line := range tc.clocking {
				if line > len(lines) || !strings.Contains(lines[line-1], "clocking "+name) {
					t.Fatalf("original authored clocking declaration drift: %s:%d", name, line)
				}
			}
			doc := sourceDocumentForScanner(string(raw))
			doc.Path = path
			result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			owner, neighbor := false, false
			found := map[string]bool{}
			for _, sym := range result.Analysis.Symbols {
				if sym.QualifiedName == tc.owner && sym.Kind == SymbolKindInterface {
					owner = true
				}
				if sym.QualifiedName == tc.neighbor {
					neighbor = true
				}
				for name, line := range tc.clocking {
					if sym.QualifiedName == tc.owner+"."+name &&
						sym.Kind == SymbolKindEntity && sym.NativeKind == "clocking-block" &&
						sym.ParentQualifiedName == tc.owner && sym.NameRange.Start.Line == line {
						found[name] = true
					}
				}
			}
			if !owner || !neighbor {
				t.Errorf("existing interface owner/neighbor absent: %v %v", owner, neighbor)
			}
			for name := range tc.clocking {
				if !found[name] {
					t.Errorf("missing directly authored clocking block %s.%s", tc.owner, name)
				}
			}
		})
	}
}
