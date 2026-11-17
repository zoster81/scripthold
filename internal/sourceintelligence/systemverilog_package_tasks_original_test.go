package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSystemVerilogPackageTasksTwoOriginalProjects(t *testing.T) {
	tests := []struct {
		env, marker, owner, neighbor string
		tasks                        map[string]int
	}{
		{
			env:      "SCRIPTHOLD_REAL_SV_TASK_VORTEX",
			marker:   "package VX_dxa_pkg;",
			owner:    "VX_dxa_pkg",
			neighbor: "VX_dxa_pkg.DXA_LMEM_WORD_SIZE",
			tasks:    map[string]int{"trace_ex_op": 171},
		},
		{
			env:      "SCRIPTHOLD_REAL_SV_TASK_PULP",
			marker:   "package tb_package;",
			owner:    "tb_package",
			neighbor: "tb_package.ADDR_WIDTH",
			tasks:    map[string]int{"periph_write": 60, "periph_read": 106, "check_output": 151},
		},
	}
	for _, tc := range tests {
		t.Run(tc.owner, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete pinned original source not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 1000 || fi.Size() > 65536 {
				t.Fatalf("invalid original source: %v %v", fi, err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(original)
			if strings.Count(source, tc.marker) != 1 {
				t.Fatal("original package marker changed")
			}
			lines := strings.Split(source, "\n")
			for task, line := range tc.tasks {
				if line > len(lines) || !strings.Contains(lines[line-1], task+"(") &&
					!strings.Contains(lines[line-1], task+" (") {
					t.Fatalf("original task line changed: %s:%d", task, line)
				}
			}
			doc := sourceDocumentForScanner(source)
			doc.Path = path
			res, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			neighbor := false
			found := map[string]bool{}
			for _, sym := range res.Analysis.Symbols {
				if sym.QualifiedName == tc.neighbor {
					neighbor = true
				}
				for name, line := range tc.tasks {
					if sym.QualifiedName == tc.owner+"."+name && sym.Kind == SymbolKindFunction &&
						sym.NativeKind == "package-task" && sym.ParentQualifiedName == tc.owner &&
						sym.NameRange.Start.Line == line {
						found[name] = true
					}
				}
			}
			if !neighbor {
				t.Errorf("neighboring original package parameter missing: %s", tc.neighbor)
			}
			for name := range tc.tasks {
				if !found[name] {
					t.Errorf("missing authored package task %s.%s (symbols=%d)", tc.owner, name, len(res.Analysis.Symbols))
				}
			}
		})
	}
}
