package sourceintelligence

import (
	"reflect"
	"testing"
)

func TestR27RelationContractVocabulary(t *testing.T) {
	if got, want := []RelationKind{
		RelationDependencies, RelationDependents, RelationReferences, RelationDefinitions,
		RelationInheritance, RelationImplementations, RelationOverrides, RelationCallers,
		RelationCallees, RelationTrace, RelationImpact, RelationCycles,
	}, []RelationKind{
		"dependencies", "dependents", "references", "definitions", "inheritance", "implementations",
		"overrides", "callers", "callees", "trace", "impact", "cycles",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("relation vocabulary = %v, want %v", got, want)
	}

	if got, want := []ResolutionState{
		ResolutionResolved, ResolutionAmbiguous, ResolutionUnresolved, ResolutionExternal,
	}, []ResolutionState{"resolved", "ambiguous", "unresolved", "external"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolution vocabulary = %v, want %v", got, want)
	}

	if got, want := []IndexStaleness{
		IndexCurrent, IndexStale, IndexNotIndexed,
	}, []IndexStaleness{"current", "stale", "not-indexed"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("index staleness vocabulary = %v, want %v", got, want)
	}
}

func TestR27EvidenceStrengthRemainsSeparateFromResolution(t *testing.T) {
	for _, evidence := range []SymbolEvidence{
		SymbolEvidenceTextual, SymbolEvidenceLexical, SymbolEvidenceStructural,
		SymbolEvidenceScopeResolved, SymbolEvidenceProjectResolved, SymbolEvidenceSemantic,
	} {
		if _, ok := symbolEvidenceRank[evidence]; !ok {
			t.Fatalf("evidence %q is missing from the ordered R25 ladder", evidence)
		}
	}
	for _, state := range []SymbolEvidence{"ambiguous", "unresolved", "external", "resolved"} {
		if _, ok := symbolEvidenceRank[state]; ok {
			t.Fatalf("resolution state %q leaked into ordered evidence strength", state)
		}
	}
}

func TestR27ContextPriorityVocabulary(t *testing.T) {
	if got, want := []ContextReason{
		ContextTarget, ContextEnclosing, ContextDirectDependency, ContextDirectRelatedBody,
		ContextDirectRelatedSignature, ContextReverseOrTypeRelation, ContextDeeperRelation,
	}, []ContextReason{
		"target", "enclosing", "direct-dependency", "direct-related-body",
		"direct-related-signature", "reverse-or-type-relation", "deeper-relation",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("context priority vocabulary = %v, want %v", got, want)
	}
}
