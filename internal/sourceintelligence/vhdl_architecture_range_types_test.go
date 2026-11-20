package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Architecture-owned range types are useful configuration edit targets.
// Process- and subprogram-local types must not be assigned to the architecture.
func TestVHDLArchitectureRangeTypeNavigation(t *testing.T) {
	cases := []struct {
		name      string
		source    string
		want      map[string]int
		forbidden []string
	}{
		{
			name: "architecture-before-begin",
			source: "entity Probe is\n" +
				"end entity;\n" +
				"architecture rtl of Probe is\n" +
				"  -- type fake_t is range 0 to 4;\n" +
				"  type blink_t is range 0 to 119;\n" +
				"  type signed_t is range -3 to 15;\n" +
				"  signal counter: blink_t;\n" +
				"begin\n" +
				"  p: process\n" +
				"    type local_t is range 0 to 2;\n" +
				"  begin\n" +
				"    null;\n" +
				"  end process;\n" +
				"end rtl;\n",
			want: map[string]int{
				"Probe.rtl.blink_t":  5,
				"Probe.rtl.signed_t": 6,
			},
			forbidden: []string{"Probe.rtl.fake_t", "Probe.rtl.local_t"},
		},
		{
			name: "subprogram-declarations-not-architecture-owned",
			source: "entity Probe is\n" +
				"end entity;\n" +
				"architecture rtl of Probe is\n" +
				"  type before_fn_t is range 0 to 3;\n" +
				"  function inner return natural is\n" +
				"    type function_local_t is range 0 to 2;\n" +
				"  begin\n" +
				"    return 1;\n" +
				"  end function;\n" +
				"  type after_fn_t is range 0 to 8;\n" +
				"begin\n" +
				"end rtl;\n",
			want: map[string]int{"Probe.rtl.before_fn_t": 4},
			forbidden: []string{
				"Probe.rtl.function_local_t",
				"Probe.rtl.after_fn_t", // unsupported after nested subprograms, never forged
			},
		},
	}
	for _, tc := range cases {
		for _, eol := range []struct{ name, text string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"-"+eol.name, func(t *testing.T) {
				doc := sourceDocumentForScanner(strings.ReplaceAll(tc.source, "\n", eol.text))
				result, err := (VHDLAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 128))
				if err != nil {
					t.Fatal(err)
				}
				if result.Analysis.Truncated {
					t.Fatalf("unexpected truncation: %+v", result.Analysis)
				}
				found := make(map[string]bool)
				for _, symbol := range result.Analysis.Symbols {
					for _, bad := range tc.forbidden {
						if symbol.QualifiedName == bad {
							t.Errorf("false architecture-owned range type %+v", symbol)
						}
					}
					line, ok := tc.want[symbol.QualifiedName]
					if !ok {
						continue
					}
					if symbol.Kind != SymbolKindType || symbol.NativeKind != "range-type" ||
						symbol.NameRange.Start.Line != line {
						t.Errorf("unexpected architecture range type %+v, line=%d", symbol, line)
					}
					if found[symbol.QualifiedName] {
						t.Errorf("duplicate %s", symbol.QualifiedName)
					}
					found[symbol.QualifiedName] = true
				}
				for name := range tc.want {
					if !found[name] {
						t.Errorf("missing independent VHDL range type %s", name)
					}
				}
			})
		}
	}
}
