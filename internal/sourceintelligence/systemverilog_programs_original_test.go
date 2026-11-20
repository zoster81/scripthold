package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSystemVerilogProgramsTwoOriginalTopLevel(t *testing.T) {
	for _, tc := range []struct {
		env, neighbor, marker, name string
		line                        int
	}{
		{env: "SCRIPTHOLD_REAL_SV_PROGRAM_MBITS", neighbor: "TB_using_Module", marker: "module TB_using_Module();", name: "TB_using_Program", line: 22},
		{env: "SCRIPTHOLD_REAL_SV_PROGRAM_VENGINEER", neighbor: "tb", marker: "module tb ;", name: "prog1", line: 17},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independent original not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 450 || fi.Size() > 65536 {
				t.Fatalf("invalid original %v %v", fi, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			src := string(raw)
			if strings.Count(src, tc.marker) != 1 {
				t.Fatal("original module marker changed")
			}
			lines := strings.Split(src, "\n")
			if len(lines) < tc.line || !strings.Contains(lines[tc.line-1], "program "+tc.name) {
				t.Fatal("original authored program changed")
			}
			doc := sourceDocumentForScanner(src)
			doc.Path = path
			r, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			neighbor, found := false, false
			for _, s := range r.Analysis.Symbols {
				if s.QualifiedName == tc.neighbor && s.Kind == SymbolKindModule {
					neighbor = true
				}
				if s.Name == tc.name && s.QualifiedName == tc.name && s.Kind == SymbolKindModule &&
					s.NativeKind == "program" && s.ParentQualifiedName == "" && s.NameRange.Start.Line == tc.line {
					found = true
				}
			}
			if !neighbor {
				t.Error("neighboring module missing")
			}
			if !found {
				t.Errorf("authored top-level program missing: %s", tc.name)
			}
		})
	}
}
