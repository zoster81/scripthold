package sourceintelligence

import (
	"context"
	"testing"
)

func TestR27Phase3RegistryActivatesOnlyC_CPP_Java_KotlinProviders(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]AnalyzerID{"c": AnalyzerC, "cpp": AnalyzerCPP, "java": AnalyzerJava, "kotlin": AnalyzerKotlin}
	for language, analyzerID := range expected {
		descriptor, ok := registry.Resolve(language)
		if !ok {
			t.Fatalf("missing %s", language)
		}
		analyzer, available := AnalyzerFor(descriptor)
		if !available || analyzer.ID() != analyzerID || analyzer.Language() != language {
			t.Fatalf("%s analyzer = %#v available=%v descriptor=%+v", language, analyzer, available, descriptor)
		}
		caps := descriptor.Capabilities
		if !caps.SourceAnalysis || !caps.Declarations || !caps.Hierarchy || !caps.Signatures || !caps.Ranges || !caps.Dependencies {
			t.Fatalf("%s incomplete declaration capability row: %+v", language, caps)
		}
		if (language == "cpp" || language == "java" || language == "kotlin") && !caps.InheritanceRelations {
			t.Fatalf("%s must expose structural inheritance relations", language)
		}
		if caps.ProjectResolvedReferences || caps.ProjectResolvedDefinitions || caps.SemanticRelations || caps.IncrementalIndex {
			t.Fatalf("%s overclaims later R27 semantics: %+v", language, caps)
		}
	}
}

func TestR27Phase3HeaderDetectionRemainsAmbiguousUnlessCPPContentDisambiguates(t *testing.T) {
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
