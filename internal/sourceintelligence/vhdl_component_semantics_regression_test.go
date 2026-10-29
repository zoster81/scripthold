package sourceintelligence

import (
	"context"
	"fmt"
	"os"
	"testing"
)

func TestVHDLArchitectureComponentsAreDeclaredTypes(t *testing.T) {
	source := `library ieee;
use ieee.std_logic_1164.all;
entity Core is
end entity;
architecture rtl of Core is
  -- component Fake
  signal state : std_logic;
  component ADC
    port (clk : in std_logic);
  end component;
  component UART
    port (clk : in std_logic);
  end component;
begin
  ADC_i : ADC
    port map(clk => state);
end architecture;`
	result, err := (VHDLAnalyzer{}).Analyze(context.Background(), testSourceDocument("core.vhd", source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("unexpected incomplete VHDL coverage: %+v", result.Analysis)
	}
	got := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, tc := range []struct {
		name string
		kind SymbolKind
		line int
	}{
		{"Core", SymbolKindEntity, 3},
		{"Core.rtl", SymbolKindImplementation, 5},
		{"Core.rtl.state", SymbolKindSignal, 7},
		{"Core.rtl.ADC", SymbolKindType, 8},
		{"Core.rtl.UART", SymbolKindType, 11},
	} {
		symbol, ok := got[tc.name]
		if !ok || symbol.Kind != tc.kind || symbol.DeclarationRange.Start.Line != tc.line {
			t.Errorf("VHDL declaration %q: present=%v, kind=%q line=%d; want %q line %d", tc.name, ok, symbol.Kind, symbol.DeclarationRange.Start.Line, tc.kind, tc.line)
		}
		if ok && (tc.kind == SymbolKindType && symbol.NativeKind != "component") {
			t.Errorf("component native kind: got %q", symbol.NativeKind)
		}
	}
	for _, name := range []string{"Core.rtl.ADC_i", "Core.rtl.Fake"} {
		if sym, ok := got[name]; ok {
			t.Errorf("false VHDL component %q: %+v", name, sym)
		}
	}
}

func TestVHDLArchitectureDoesNotCloseAtNestedFunctionEnd(t *testing.T) {
	for _, finalEnd := range []string{"end rtl;", "end;"} {
		t.Run(finalEnd, func(t *testing.T) {
			source := "entity Core is\nend;\narchitecture rtl of Core is\n" +
				"  function Helper return integer is\n  begin\n    return 1;\n  end;\n" +
				"  signal after_function : std_logic;\n" +
				"  component UART\n    port (clk : in std_logic);\n  end component;\n" +
				"begin\n  after_function <= '0';\n" + finalEnd + "\n"
			result, err := (VHDLAnalyzer{}).Analyze(context.Background(), testSourceDocument("nested.vhd", source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Errorf("valid nested VHDL structure must remain complete: %+v", result.Analysis.Diagnostics)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range map[string]SymbolKind{
				"Core":                    SymbolKindEntity,
				"Core.rtl":                SymbolKindImplementation,
				"Core.rtl.after_function": SymbolKindSignal,
				"Core.rtl.UART":           SymbolKindType,
			} {
				sym, ok := got[name]
				if !ok || sym.Kind != kind {
					t.Errorf("%q: found=%v kind=%q; want %q", name, ok, sym.Kind, kind)
				}
			}
		})
	}
}

func TestVHDLNestedFunctionEndDoesNotHideUnclosedArchitecture(t *testing.T) {
	source := "entity Core is\nend;\narchitecture rtl of Core is\n" +
		"  function Helper return integer is\n  begin\n    return 1;\n  end;\n" +
		"  signal after_function : std_logic;\nbegin\n  after_function <= '0';\n" +
		"entity Following is\nend Following;\n"
	result, err := (VHDLAnalyzer{}).Analyze(context.Background(), testSourceDocument("unclosed-nested.vhd", source), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete {
		t.Fatal("nested function's bare end must not make a missing outer architecture complete")
	}
	found := false
	for _, diagnostic := range result.Analysis.Diagnostics {
		if diagnostic.Code == "vhdl-unclosed-architecture" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing architecture closure diagnostic: %+v", result.Analysis.Diagnostics)
	}
}

func TestVHDLBareNestedFunctionEndAtEOFIsNotArchitectureEnd(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprintf("nested-functions-%d", count), func(t *testing.T) {
			source := "entity Core is\nend entity;\narchitecture rtl of Core is\n"
			for n := 0; n < count; n++ {
				source += fmt.Sprintf("  function F%d return integer is\n  begin\n    return %d;\n  end;\n", n, n)
			}
			result, err := (VHDLAnalyzer{}).Analyze(context.Background(), testSourceDocument("unfinished.vhd", source), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete {
				t.Fatal("nested function termination is not a closing architecture delimiter")
			}
			found := false
			for _, diagnostic := range result.Analysis.Diagnostics {
				if diagnostic.Code == "vhdl-unclosed-architecture" {
					found = true
				}
			}
			if !found {
				t.Errorf("missing unclosed architecture diagnostic: %+v", result.Analysis.Diagnostics)
			}
		})
	}
}

func TestVHDLBareArchitectureEndAfterNamedSubprogramOrPrototype(t *testing.T) {
	for _, tc := range []struct {
		name       string
		subprogram string
	}{
		{"end-function", "function Helper return integer is\nbegin\n  return 1;\nend function;"},
		{"end-function-name", "function Helper return integer is\nbegin\n  return 1;\nend function Helper;"},
		{"end-name", "function Helper return integer is\nbegin\n  return 1;\nend Helper;"},
		{"function-prototype", "function Helper return integer;"},
		{"end-procedure", "procedure Helper is\nbegin\n  null;\nend procedure;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "entity Core is\nend entity;\narchitecture rtl of Core is\n" +
				tc.subprogram + "\nsignal live : std_logic;\nbegin\nlive <= '0';\nend;\n"
			result, err := (VHDLAnalyzer{}).Analyze(context.Background(), testSourceDocument("named-nested.vhd", source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Errorf("valid architecture should remain complete: %+v", result.Analysis.Diagnostics)
			}
			sym, ok := symbolsByQualifiedName(result.Analysis.Symbols)["Core.rtl.live"]
			if !ok || sym.Kind != SymbolKindSignal {
				t.Errorf("subprogram closure hid a later architecture signal: present=%v symbol=%+v", ok, sym)
			}
		})
	}
}

func TestVHDLUnclosedComponentDoesNotForgeDeclaration(t *testing.T) {
	source := "entity Core is\nend entity;\narchitecture rtl of Core is\n component Ghost\n begin\nend architecture;\n"
	result, err := (VHDLAnalyzer{}).Analyze(context.Background(), testSourceDocument("broken.vhd", source), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)["Core.rtl.Ghost"]; ok {
		t.Fatal("unclosed component promoted to a structural declaration")
	}
	if result.Analysis.CoverageComplete {
		t.Fatal("unclosed VHDL component was reported as complete")
	}
}

func TestVHDLComponentRealCorpusOracle(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_VHDL_COMPONENT_ORACLE")
	if path == "" {
		t.Skip("set SCRIPTHOLD_VHDL_COMPONENT_ORACLE for the real source oracle")
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (VHDLAnalyzer{}).Analyze(context.Background(), testSourceDocument("vhdl-component.vhd", string(bytes)), testAnalyzeOptions(false, 128))
	if err != nil {
		t.Fatal(err)
	}
	got := symbolsByQualifiedName(result.Analysis.Symbols)
	for name, line := range map[string]int{"ENTITY_TOP.arch.ENTITY_1": 18, "ENTITY_TOP.arch.ENTITY_2": 27} {
		symbol, ok := got[name]
		if !ok || symbol.Kind != SymbolKindType || symbol.DeclarationRange.Start.Line != line {
			t.Errorf("real VHDL component %q: present=%v kind=%q line=%d, expected type at %d", name, ok, symbol.Kind, symbol.DeclarationRange.Start.Line, line)
		}
	}
	if !result.Analysis.CoverageComplete {
		t.Errorf("valid VHDL document unexpectedly incomplete: %+v", result.Analysis)
	}
}
