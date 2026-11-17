package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSystemVerilogModportsTwoOriginalInterfaces(t *testing.T) {
	for _, tc := range []struct {
		env, marker, owner, neighbor string
		line                         map[string]int
	}{
		{
			env:      "SCRIPTHOLD_REAL_SV_MODPORT_VORTEX",
			marker:   "interface VX_txbar_bus_if",
			owner:    "VX_txbar_bus_if",
			neighbor: "VX_txbar_bus_if.valid",
			line:     map[string]int{"master": 22, "slave": 28},
		},
		{
			env:      "SCRIPTHOLD_REAL_SV_MODPORT_SWABIAN",
			marker:   "interface axis_interface #(",
			owner:    "axis_interface",
			neighbor: "axis_interface.tvalid",
			line:     map[string]int{"master": 39, "slave": 40},
		},
	} {
		t.Run(tc.owner, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete original interface not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 500 || fi.Size() > 65536 {
				t.Fatalf("invalid original source: %v %v", fi, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			if strings.Count(source, tc.marker) != 1 {
				t.Fatal("original interface marker drift")
			}
			authored := strings.Split(source, "\n")
			for name, line := range tc.line {
				if len(authored) < line || !strings.Contains(authored[line-1], "modport "+name) {
					t.Fatalf("original authored modport line changed: %s:%d", name, line)
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
				if s.QualifiedName == tc.owner && s.Kind == SymbolKindInterface {
					owner = true
				}
				if s.QualifiedName == tc.neighbor {
					neighbor = true
				}
				for name, line := range tc.line {
					if s.QualifiedName == tc.owner+"."+name && s.Kind == SymbolKindType &&
						s.NativeKind == "modport" && s.ParentQualifiedName == tc.owner &&
						s.NameRange.Start.Line == line {
						found[name] = true
					}
				}
			}
			if !owner || !neighbor {
				t.Errorf("existing interface owner/signal missing: owner=%v neighbor=%v", owner, neighbor)
			}
			for name := range tc.line {
				if !found[name] {
					t.Errorf("direct authored modport omitted: %s.%s", tc.owner, name)
				}
			}
		})
	}
}
