package sourceintelligence

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"testing"
)

func TestProjectIndexAnalysisFingerprintMatchesLegacyEncoding(t *testing.T) {
	defaultRegistry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	customRegistry, err := NewLanguageRegistry([]LanguageDescriptor{
		{ID: "zeta", Aliases: []string{"z"}, ExactBasenames: []string{"Zetafile"}},
		{ID: "alpha", Aliases: []string{"a"}, Extensions: []string{".alpha"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	configs := []ProjectIndexAnalysisConfig{
		{
			MaxFileBytes: 8 * 1024 * 1024, MaxDecodedCharacters: 16 * 1024 * 1024,
			MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256,
			MaxDetectorProbes: 4, MaxNesting: 256, MaxProjectEdges: 4096,
		},
		{
			MaxFileBytes: 17, MaxDecodedCharacters: 31, MaxSymbols: 47,
			MaxSignatureBytes: 61, MaxDiagnostics: 73, MaxDetectorProbes: 89,
			MaxNesting: 101, MaxProjectEdges: 127, IncludeSignatures: true,
		},
	}
	registries := []struct {
		name     string
		registry *LanguageRegistry
	}{
		{name: "default", registry: defaultRegistry},
		{name: "custom", registry: customRegistry},
	}

	for _, registryCase := range registries {
		for configIndex, config := range configs {
			t.Run(fmt.Sprintf("%s/config-%d", registryCase.name, configIndex), func(t *testing.T) {
				got, fingerprintErr := ProjectIndexAnalysisFingerprint(registryCase.registry, config)
				if fingerprintErr != nil {
					t.Fatal(fingerprintErr)
				}
				want := legacyProjectIndexAnalysisFingerprintForTest(registryCase.registry, config)
				if got != want {
					t.Fatalf("analysis fingerprint = %q, want legacy-compatible %q", got, want)
				}
			})
		}
	}
}

func legacyProjectIndexAnalysisFingerprintForTest(registry *LanguageRegistry, config ProjectIndexAnalysisConfig) string {
	hasher := sha256.New()
	writePart := func(value string) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write([]byte(value))
	}
	writeStrings := func(values []string) {
		writePart(fmt.Sprintf("%d", len(values)))
		for _, value := range values {
			writePart(value)
		}
	}

	writePart("scripthold:r27-project-index-analysis-v1")
	writePart(fmt.Sprintf("%d", config.MaxFileBytes))
	writePart(fmt.Sprintf("%d", config.MaxDecodedCharacters))
	writePart(fmt.Sprintf("%d", config.MaxSymbols))
	writePart(fmt.Sprintf("%d", config.MaxSignatureBytes))
	writePart(fmt.Sprintf("%d", config.MaxDiagnostics))
	writePart(fmt.Sprintf("%d", config.MaxDetectorProbes))
	writePart(fmt.Sprintf("%d", config.MaxNesting))
	writePart(fmt.Sprintf("%d", config.MaxProjectEdges))
	writePart(fmt.Sprintf("%t", config.IncludeSignatures))
	ids := make([]string, 0, len(registry.byID))
	for id := range registry.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		descriptor := registry.byID[id]
		writePart(descriptor.ID)
		writeStrings(descriptor.Aliases)
		writeStrings(descriptor.ExactBasenames)
		writeStrings(descriptor.CompoundSuffixes)
		writeStrings(descriptor.Extensions)
		writeStrings(descriptor.AmbiguousExtensions)
		writeStrings(descriptor.ShebangInterpreters)
		writePart(string(descriptor.Analyzer))
		writePart(fmt.Sprintf("%+v", descriptor.Capabilities))
		writePart(descriptor.Family)
		for _, evidence := range descriptor.DetectionEvidence {
			writePart(string(evidence))
		}
		writePart(descriptor.ScannerProfile)
		writePart(descriptor.CompositeBehavior)
		writePart(descriptor.AnalyzerStrategy)
		writePart(descriptor.AnalyzerVersion)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}
