//go:build !race

package sourceintelligence

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestApexAnalyzerAvoidsDuplicateModifierCollection(t *testing.T) {
	var source strings.Builder
	source.WriteString("public class Demo {\n")
	for index := 0; index < 400; index++ {
		fmt.Fprintf(&source, "  public static Integer method%d(Integer value) { return value; }\n", index)
	}
	source.WriteString("}\n")

	document := sourceDocumentForScanner(source.String())
	document.Path = "apex-modifier-allocation.apex"
	options := AnalyzeOptions{
		MaxNesting: 512,
		Limits: SymbolBuilderLimits{
			MaxSymbols:        5000,
			MaxSignatureBytes: 8192,
			MaxDiagnostics:    256,
		},
	}

	var observed int
	allocations := testing.AllocsPerRun(5, func() {
		result, err := (ApexAnalyzer{}).Analyze(context.Background(), document, options)
		if err != nil {
			panic(err)
		}
		observed += len(result.Analysis.Symbols)
	})
	if observed == 0 {
		t.Fatal("allocation guard produced no Apex symbols")
	}
	if allocations > 8400 {
		t.Fatalf("Apex modifier-path allocations = %.0f, want <= 8400", allocations)
	}
}
