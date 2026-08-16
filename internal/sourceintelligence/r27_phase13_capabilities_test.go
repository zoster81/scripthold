package sourceintelligence

import "testing"

func TestR27Phase13CapabilityMatrixPromotesOnlyVerifiedProjectSubset(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	verified := []string{"cpp", "java", "kotlin", "typescript", "rust", "ruby", "delphi"}
	for _, language := range verified {
		descriptor, ok := registry.Resolve(language)
		if !ok {
			t.Fatalf("missing verified Phase 13 language %s", language)
		}
		caps := descriptor.Capabilities
		if !caps.ProjectResolvedReferences || !caps.ProjectResolvedDefinitions {
			t.Fatalf("%s project query capabilities = %+v, want project-ref/project-def", language, caps)
		}
		if caps.SemanticRelations || caps.SyntacticCalls || caps.Overrides {
			t.Fatalf("%s overclaims unproven project/semantic Phase 13 capabilities: %+v", language, caps)
		}
	}
	java, _ := registry.Resolve("java")
	if !java.Capabilities.Implementations {
		t.Fatalf("java capabilities = %+v, want proven implementation queries", java.Capabilities)
	}
	for _, language := range []string{"c", "javascript", "php", "pascal"} {
		descriptor, _ := registry.Resolve(language)
		if descriptor.Capabilities.ProjectResolvedReferences || descriptor.Capabilities.ProjectResolvedDefinitions || descriptor.Capabilities.Implementations {
			t.Fatalf("%s unexpectedly promoted beyond Phase 13 evidence: %+v", language, descriptor.Capabilities)
		}
	}
}
