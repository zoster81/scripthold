package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSystemVerilogPackageFunctionsTwoOriginalProjects(t *testing.T) {
	for _, tc := range []struct {
		env, packageName, neighbor, marker string
		functions                          map[string]int
	}{
		{
			env:         "SCRIPTHOLD_REAL_SV_VORTEX",
			packageName: "VX_tlb_pkg",
			neighbor:    "VX_tlb_pkg.TLB_FLAG_R",
			marker:      "package VX_tlb_pkg;",
			functions:   map[string]int{"tlb_perm_ok": 61, "tlb_vpn_mask": 76},
		},
		{
			env:         "SCRIPTHOLD_REAL_SV_CORAL",
			packageName: "top_racl_pkg",
			neighbor:    "top_racl_pkg.NrRaclBits",
			marker:      "package top_racl_pkg;",
			functions:   map[string]int{"tlul_extract_racl_role_bits": 67, "tlul_extract_ctn_uid_bits": 76},
		},
	} {
		t.Run(tc.packageName, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("full independent original source not configured")
			}
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() < 1000 || stat.Size() > 65536 {
				t.Fatalf("original source invalid: %v %v", stat, err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(original), tc.marker) != 1 {
				t.Fatal("package marker drift")
			}
			doc := sourceDocumentForScanner(string(original))
			doc.Path = path
			result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			found := make(map[string]bool)
			for _, s := range result.Analysis.Symbols {
				if s.QualifiedName == tc.neighbor {
					found[tc.neighbor] = true
				}
				for name, line := range tc.functions {
					if s.QualifiedName == tc.packageName+"."+name && s.Kind == SymbolKindFunction &&
						s.NameRange.Start.Line == line && s.ParentQualifiedName == tc.packageName {
						found[name] = true
					}
				}
			}
			if !found[tc.neighbor] {
				t.Errorf("adjacent package declaration missing: %s", tc.neighbor)
			}
			for name := range tc.functions {
				if !found[name] {
					t.Errorf("direct authored package function missing: %s.%s (symbols=%d)", tc.packageName, name, len(result.Analysis.Symbols))
				}
			}
		})
	}
}
