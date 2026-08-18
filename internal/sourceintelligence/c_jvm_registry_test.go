package sourceintelligence

import (
	"context"
	"testing"
)

func TestCJVMCapabilityCeilings(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"c", "cpp", "java", "kotlin"} {
		descriptor, _ := registry.Resolve(language)
		caps := descriptor.Capabilities
		if (language == "cpp" || language == "java" || language == "kotlin") && !caps.InheritanceRelations {
			t.Fatalf("%s must expose structural inheritance relations", language)
		}
		if caps.SemanticRelations {
			t.Fatalf("%s overclaims semantic relations: %+v", language, caps)
		}
	}
}

func TestHeaderDetectionRemainsAmbiguousUnlessCPPContentDisambiguates(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	plain, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "api.h", Text: "struct Item;\n"})
	if err != nil {
		t.Fatal(err)
	}
	if plain.State != DetectionAmbiguous || plain.Language != "" {
		t.Fatalf("plain .h = %+v, want ambiguous", plain)
	}
	cpp, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "api.h", Text: "namespace demo { template <class T> class Box {}; }\n"})
	if err != nil {
		t.Fatal(err)
	}
	if cpp.State != DetectionProbable || cpp.Language != "cpp" || !hasDetectionEvidence(cpp, EvidenceContentMarker) {
		t.Fatalf("C++ header = %+v, want probable cpp with content evidence", cpp)
	}
}
