package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These original VHDL projects are immutable fixtures; only their authored
// structural declarations are asserted, never compiler or synthesis behavior.
func TestVHDLPinnedWholeOriginalSignalSpans(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_VHDL_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete independent VHDL originals are not configured")
	}
	for _, tc := range []struct {
		file    string
		count   int
		signals map[string]int
	}{
		{"full-adder.vhd", 4, map[string]int{"sig_tmp_0": 35, "sig_tmp_1": 36}},
		{"tt09-counter.vhdl", 9, map[string]int{
			"reset": 26, "led_out": 27, "second_counter": 28, "digit": 29, "compare": 30,
		}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original VHDL file unavailable or unsafe: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			lines := strings.Split(text, "\n")
			result, err := (VHDLAnalyzer{}).Analyze(context.Background(),
				testSourceDocument(path, text), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated ||
				len(result.Analysis.Symbols) != tc.count {
				t.Fatalf("entire original source coverage: %+v", result.Analysis)
			}
			seen := map[string]bool{}
			for _, symbol := range result.Analysis.Symbols {
				line, exists := tc.signals[symbol.Name]
				if !exists {
					continue
				}
				seen[symbol.Name] = true
				authored := strings.TrimSuffix(lines[line-1], "\r")
				if symbol.Kind != SymbolKindSignal || symbol.NativeKind != "signal" ||
					symbol.NameRange.Start.Line != line ||
					symbol.DeclarationRange.End.Line != line ||
					symbol.DeclarationRange.End.Column != len(authored)+1 ||
					symbol.SignatureRange == nil ||
					symbol.SignatureRange.End.Line != line ||
					symbol.SignatureRange.End.Column != len(authored)+1 ||
					symbol.Signature != authored {
					t.Errorf("authored complete signal declaration %q: %+v, want full %q", symbol.Name, symbol, authored)
				}
			}
			for name := range tc.signals {
				if !seen[name] {
					t.Errorf("original signal %q missing", name)
				}
			}
		})
	}
}

func TestVHDLSignalSemicolonInQuotedValuesAndMultilineLFCRLF(t *testing.T) {
	for _, tc := range []struct{ name, eol string }{
		{"LF", "\n"}, {"CRLF", "\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := []string{
				"entity Core is",
				"end Core;",
				"architecture rtl of Core is",
				"  signal banner : string := \"a;b\";",
				"  signal delimiter : character := ';';",
				"  signal bits : std_logic_vector(",
				"    7 downto 0",
				"  );",
				"begin",
				"end rtl;",
				"",
			}
			text := strings.Join(lines, tc.eol)
			result, err := (VHDLAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("guard.vhd", text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 5 {
				t.Fatalf("valid quoted/multiline VHDL must be complete: %+v", result.Analysis)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, tc := range []struct {
				name    string
				endLine int
				endCol  int
			}{
				{"Core.rtl.banner", 4, len(lines[3]) + 1},
				{"Core.rtl.delimiter", 5, len(lines[4]) + 1},
				{"Core.rtl.bits", 8, len(lines[7]) + 1},
			} {
				sym, ok := got[tc.name]
				if !ok || sym.Kind != SymbolKindSignal || sym.DeclarationRange.End.Line != tc.endLine ||
					sym.DeclarationRange.End.Column != tc.endCol || sym.SignatureRange == nil ||
					sym.SignatureRange.End != sym.DeclarationRange.End ||
					!strings.HasSuffix(sym.Signature, ";") {
					t.Errorf("valid literal/multiline signal %s: %+v, found=%v", tc.name, sym, ok)
				}
			}
		})
	}
}

func TestVHDLUnterminatedSignalDoesNotConsumeArchitectureBody(t *testing.T) {
	source := "entity Core is\nend Core;\narchitecture rtl of Core is\n" +
		"  signal broken : std_logic\nbegin\n  null;\nend rtl;\n"
	result, err := (VHDLAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("broken.vhd", source), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete {
		t.Fatalf("missing signal semicolon must report incomplete coverage: %+v", result.Analysis)
	}
	for _, sym := range result.Analysis.Symbols {
		if sym.Name == "broken" && sym.DeclarationRange.End.Line >= 5 {
			t.Fatalf("malformed signal swallowed architecture begin/body: %+v", sym)
		}
	}
}
