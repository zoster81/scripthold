//go:build !race

package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestHDLEndStaticTerminatorsAvoidPerCallRegexpCompilation(t *testing.T) {
	text := "notendmodule\nENDMODULE // close\n"
	want := strings.Index(text, "ENDMODULE") + len("ENDMODULE")
	var got int
	allocations := testing.AllocsPerRun(20, func() {
		got = hdlEnd(text, 0, "endmodule")
	})
	if got != want {
		t.Fatalf("hdlEnd = %d, want %d", got, want)
	}
	if allocations > 2 {
		t.Fatalf("hdlEnd allocations = %.0f, want <= 2 for static terminator", allocations)
	}
}

func TestSystemVerilogSingleModuleAllocationBudget(t *testing.T) {
	document := sourceDocumentForScanner("module demo(input logic clk); always_ff @(posedge clk) begin end endmodule\n")
	document.Path = "allocation.sv"
	options := AnalyzeOptions{IncludeSignatures: true, MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256}}
	allocations := testing.AllocsPerRun(20, func() {
		result, err := (SystemVerilogAnalyzer{}).Analyze(context.Background(), document, options)
		if err != nil {
			panic(err)
		}
		if len(result.Analysis.Symbols) != 2 || !result.Analysis.CoverageComplete {
			panic("unexpected SystemVerilog allocation-guard result")
		}
	})
	if allocations > 16 {
		t.Fatalf("single-module SystemVerilog allocations = %.0f, want <= 16", allocations)
	}
}
