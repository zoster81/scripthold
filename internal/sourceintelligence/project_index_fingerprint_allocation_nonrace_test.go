//go:build !race

package sourceintelligence

import "testing"

// Keep the allocation budget outside race builds because race instrumentation changes allocation behavior.
func TestProjectIndexAnalysisFingerprintAvoidsPerPartHashAllocations(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	config := ProjectIndexAnalysisConfig{
		MaxFileBytes: 8 * 1024 * 1024, MaxDecodedCharacters: 16 * 1024 * 1024,
		MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256,
		MaxDetectorProbes: 4, MaxNesting: 256, MaxProjectEdges: 4096,
	}

	var observed string
	allocations := testing.AllocsPerRun(10, func() {
		fingerprint, fingerprintErr := ProjectIndexAnalysisFingerprint(registry, config)
		if fingerprintErr != nil {
			panic(fingerprintErr)
		}
		observed = fingerprint
	})
	if observed == "" {
		t.Fatal("allocation guard produced no analysis fingerprint")
	}
	if allocations > 3000 {
		t.Fatalf("ProjectIndexAnalysisFingerprint allocations = %.0f, want <= 3000", allocations)
	}
}
