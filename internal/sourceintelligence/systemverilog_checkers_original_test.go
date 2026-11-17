package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSystemVerilogCheckersTwoOriginalTopLevel(t *testing.T) {
	for _, tc := range []struct {
		env, marker, neighbor string
		lines                 map[string]int
	}{
		{
			env: "SCRIPTHOLD_REAL_SV_CHECKER_SBY", marker: "module top(", neighbor: "top",
			lines: map[string]int{"mark_secret": 255, "leak_check": 267, "initial_reset": 274},
		},
		{
			env: "SCRIPTHOLD_REAL_SV_CHECKER_AVA", marker: "module checker_demo (", neighbor: "checker_demo",
			lines: map[string]int{"my_check": 3},
		},
	} {
		t.Run(tc.neighbor, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete original source not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 800 || fi.Size() > 65536 {
				t.Fatalf("invalid original file: %v %v", fi, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			src := string(raw)
			if strings.Count(src, tc.marker) != 1 {
				t.Fatal("original neighboring module drift")
			}
			lines := strings.Split(src, "\n")
			for name, line := range tc.lines {
				if len(lines) < line || !strings.Contains(lines[line-1], "checker "+name) {
					t.Fatalf("original authored checker line drift %s:%d", name, line)
				}
			}
			doc := sourceDocumentForScanner(src)
			doc.Path = path
			result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			neighbor := false
			for _, sym := range result.Analysis.Symbols {
				if sym.QualifiedName == tc.neighbor && sym.Kind == SymbolKindModule {
					neighbor = true
				}
				for name, line := range tc.lines {
					if sym.Name == name && sym.QualifiedName == name && sym.Kind == SymbolKindType &&
						sym.NativeKind == "checker" && sym.ParentQualifiedName == "" &&
						sym.NameRange.Start.Line == line {
						found[name] = true
					}
				}
			}
			if !neighbor {
				t.Error("known module neighbor missing")
			}
			for name := range tc.lines {
				if !found[name] {
					t.Errorf("original top-level checker missing: %s", name)
				}
			}
		})
	}
}
