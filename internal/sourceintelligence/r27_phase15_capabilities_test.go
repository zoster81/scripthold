package sourceintelligence

import (
	"strings"
	"testing"
)

func TestR27Phase15EveryActiveProviderAdvertisesIncrementalIndex(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	for _, row := range registry.CapabilityRows() {
		if !row.Capabilities.SourceAnalysis {
			if row.Capabilities.IncrementalIndex {
				t.Fatalf("inactive provider %q advertises incremental indexing", row.ID)
			}
			continue
		}
		active++
		if !row.Capabilities.IncrementalIndex {
			t.Errorf("active provider %q does not advertise incremental indexing", row.ID)
		}
		for _, limitation := range row.KnownLimitations {
			if strings.Contains(strings.ToLower(limitation), "incremental indexing") {
				t.Errorf("active provider %q still reports incremental indexing as a limitation: %q", row.ID, limitation)
			}
		}
	}
	if active == 0 {
		t.Fatal("default registry unexpectedly has no active source providers")
	}
}
