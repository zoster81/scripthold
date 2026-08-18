package sourceintelligence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCapabilityMatrixDocumentationMatchesRegistry(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	expected := RenderLanguageCapabilityMatrixMarkdown(registry)
	path := filepath.Join("..", "..", "docs", "LANGUAGE_CAPABILITIES.md")
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read capability matrix documentation: %v", err)
	}
	assertProviderContractRegistryAndDocumentation(t, loadProviderContractManifest(t), registry, string(actual))
	if string(actual) != expected {
		firstMismatch := min(len(actual), len(expected))
		for index := 0; index < firstMismatch; index++ {
			if actual[index] != expected[index] {
				firstMismatch = index
				break
			}
		}
		t.Fatalf("%s is out of sync with the authoritative registry capability rows: actualBytes=%d expectedBytes=%d firstMismatch=%d actual=%q expected=%q", path, len(actual), len(expected), firstMismatch, byteAt(actual, firstMismatch), byteAt([]byte(expected), firstMismatch))
	}
}

func byteAt(value []byte, index int) []byte {
	if index < 0 || index >= len(value) {
		return nil
	}
	end := min(len(value), index+16)
	return value[index:end]
}
