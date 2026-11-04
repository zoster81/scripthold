package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// VHDL configuration generics are declarations owned by an entity, not the
// similarly named associations in component instantiations or generic maps.
func TestVHDLEntityGenericsNavigation(t *testing.T) {
	source := `entity Queue is
  generic (
    WIDTH, HEIGHT : positive := 32;
    MODE : string := "semi; and ) inside literal";
    MASK : std_logic_vector(7 downto 0) := (others => '0');
    constant ENABLE : boolean := true
  );
  port (clk : in std_logic);
end entity;
architecture rtl of Queue is
  component Other
    generic (NOT_ENTITY : integer := 10);
  end component;
  signal ready : std_logic;
begin
  other_i : Other generic map (NOT_ENTITY => WIDTH);
end architecture;
-- entity Comment is generic (FALSE : natural := 1);
entity Second is
  generic (DEPTH : natural := 3);
end entity;`
	for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(eol.name, func(t *testing.T) {
			doc := testSourceDocument("queue.vhd", strings.ReplaceAll(source, "\n", eol.value))
			got, err := (VHDLAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 96))
			if err != nil {
				t.Fatal(err)
			}
			if !got.Analysis.CoverageComplete || got.Analysis.Truncated {
				t.Fatalf("supported scalar generics reported incomplete: %+v", got.Analysis.Diagnostics)
			}
			byName := symbolsByQualifiedName(got.Analysis.Symbols)
			for name, line := range map[string]int{
				"Queue.WIDTH": 3, "Queue.HEIGHT": 3, "Queue.MODE": 4, "Queue.MASK": 5,
				"Queue.ENABLE": 6, "Second.DEPTH": 20,
			} {
				sym, ok := byName[name]
				if !ok || sym.Kind != SymbolKindConstant || sym.NativeKind != "generic" ||
					sym.NameRange.Start.Line != line || sym.ParentQualifiedName != strings.Split(name, ".")[0] {
					t.Errorf("incorrect/missing VHDL generic %s: ok=%v sym=%+v", name, ok, sym)
				}
			}
			for _, forbidden := range []string{"Queue.NOT_ENTITY", "Queue.FALSE", "Other.NOT_ENTITY", "Queue.rtl.NOT_ENTITY"} {
				if v, ok := byName[forbidden]; ok {
					t.Errorf("nonentity generic reported as entity declaration: %+v", v)
				}
			}
		})
	}
}

func TestVHDLUnrecognizedGenericTypeIsExplicitlyPartial(t *testing.T) {
	doc := testSourceDocument("type_generic.vhd", `entity Core is
  generic (
    type T;
    DEPTH : positive := 8
  );
end entity;`)
	got, err := (VHDLAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if got.Analysis.CoverageComplete {
		t.Fatalf("unsupported generic type reported complete: %+v", got.Analysis)
	}
	byName := symbolsByQualifiedName(got.Analysis.Symbols)
	if _, ok := byName["Core.DEPTH"]; !ok {
		t.Error("known scalar generic must remain indexed")
	}
	if _, ok := byName["Core.T"]; ok {
		t.Error("unsupported type generic must not be fabricated")
	}
}
