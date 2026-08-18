package sourceintelligence

import (
	"strings"
	"testing"
)

func TestIncrementalIndexCapabilityHasNoStaleLimitations(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range registry.CapabilityRows() {
		if !row.Capabilities.SourceAnalysis {
			if row.Capabilities.IncrementalIndex {
				t.Fatalf("inactive provider %q advertises incremental indexing", row.ID)
			}
			continue
		}
		for _, limitation := range row.KnownLimitations {
			if strings.Contains(strings.ToLower(limitation), "incremental indexing") {
				t.Errorf("active provider %q still reports incremental indexing as a limitation: %q", row.ID, limitation)
			}
		}
	}
}
