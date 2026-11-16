package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Direct, same-line packed logic typedefs are authored edit targets, not
// independent module nets. Original Universal Ctags source exercises repeated
// packed dimensions without requiring SystemVerilog elaboration.
func TestSystemVerilogPackedLogicTypedefSourceNavigation(t *testing.T) {
	check := func(t *testing.T, source string, wants map[string]int) {
		t.Helper()
		result, err := (SystemVerilogAnalyzer{}).Analyze(
			context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 128))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
			t.Fatalf("valid SystemVerilog unexpectedly partial: %+v", result.Analysis)
		}
		seen := map[string]bool{}
		for _, symbol := range result.Analysis.Symbols {
			if strings.Contains(symbol.QualifiedName, "Fake") ||
				strings.Contains(symbol.QualifiedName, "Phantom") {
				t.Fatalf("comment/string fabricated declaration: %+v", symbol)
			}
			if line, wanted := wants[symbol.QualifiedName]; wanted {
				if symbol.Kind != SymbolKindType || symbol.NativeKind != "typedef-logic" ||
					symbol.NameRange.Start.Line != line || symbol.ID == "" {
					t.Errorf("wrong type location/kind: %+v", symbol)
				}
				if seen[symbol.QualifiedName] {
					t.Errorf("duplicate typedef %q: %+v", symbol.QualifiedName, symbol)
				}
				seen[symbol.QualifiedName] = true
			}
			if symbol.QualifiedName == "top.cell_t" && symbol.Kind == SymbolKindSignal {
				t.Fatalf("typedef incorrectly reported as a module signal: %+v", symbol)
			}
		}
		for name := range wants {
			if !seen[name] {
				t.Errorf("missing source declaration %s, got %v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
	}
	const fixture = "package demo;\n" +
		"  typedef logic [7:0][1:0] word_t;\n" +
		"  typedef logic signed [3:0] signed_t;\n" +
		"  // typedef logic [2:0] Fake;\n" +
		"endpackage\n" +
		"module top;\n" +
		"  typedef logic [3:0] cell_t;\n" +
		"  logic ready;\n" +
		"endmodule\n"
	wants := map[string]int{"demo.word_t": 2, "demo.signed_t": 3, "top.cell_t": 7}
	for _, tc := range []struct{ name, newline string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			check(t, strings.ReplaceAll(fixture, "\n", tc.newline), wants)
		})
	}
	root := os.Getenv("SCRIPTHOLD_REAL_SYSTEMVERILOG_ROOT")
	if root == "" {
		return
	}
	t.Run("original-ctags", func(t *testing.T) {
		path := filepath.Join(root, "final-ctags-systemverilog-selected", "files",
			"systemverilog", "input.sv")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			t.Fatalf("bounded original source unavailable: %v %v", info, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		if strings.Count(source, "package my_pack;") != 1 ||
			strings.Count(source, "typedef logic") != 2 {
			t.Fatal("original typedef source changed")
		}
		check(t, source, map[string]int{"my_pack.my_t": 5, "my_pack.my_t2": 7})
	})
}
