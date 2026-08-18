package handler

import (
	"encoding/json"
	"testing"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

func TestSourceQueryStructuredResultContract(t *testing.T) {
	output := SourceQueryOutput{
		Operation:        "relations",
		CoordinateSystem: "unicode-scalar-1-based-half-open",
		Index: sourceintelligence.IndexEvidence{
			Generation:  7,
			Fingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Staleness:   sourceintelligence.IndexStale,
		},
		Relations: &SourceRelationsResult{
			Relation: sourceintelligence.RelationReferences,
			Relations: []sourceintelligence.RelationRecord{{
				Kind:       sourceintelligence.RelationReferences,
				Source:     sourceintelligence.RelationEntity{Path: "src/a.go"},
				Target:     sourceintelligence.RelationEntity{Path: "src/b.go"},
				Evidence:   sourceintelligence.SymbolEvidenceStructural,
				Resolution: sourceintelligence.ResolutionAmbiguous,
			}},
		},
		Coverage: SourceQueryCoverage{FilesConsidered: 2, FilesParsed: 2, CoverageComplete: true},
	}

	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal source_query relation output: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode source_query relation output: %v", err)
	}
	index := decoded["index"].(map[string]any)
	if got := index["staleness"]; got != "stale" {
		t.Fatalf("index.staleness = %#v, want stale", got)
	}
	relations := decoded["relations"].(map[string]any)["relations"].([]any)
	relation := relations[0].(map[string]any)
	if got := relation["evidence"]; got != "structural" {
		t.Fatalf("relation.evidence = %#v, want structural", got)
	}
	if got := relation["resolution"]; got != "ambiguous" {
		t.Fatalf("relation.resolution = %#v, want ambiguous", got)
	}
}

func TestSourceQueryContextResultVocabulary(t *testing.T) {
	output := SourceQueryOutput{
		Operation:        "context",
		CoordinateSystem: "unicode-scalar-1-based-half-open",
		Index:            sourceintelligence.IndexEvidence{Staleness: sourceintelligence.IndexNotIndexed},
		Context: &SourceContextResult{
			Items: []sourceintelligence.ContextItem{{
				Entity:         sourceintelligence.RelationEntity{Path: "src/main.go"},
				Reason:         sourceintelligence.ContextDirectRelatedSignature,
				Representation: sourceintelligence.ContextSignature,
				Priority:       4,
				Text:           "func Run()",
				Evidence:       sourceintelligence.SymbolEvidenceStructural,
				Resolution:     sourceintelligence.ResolutionResolved,
			}},
			UsedBytes:   10,
			BudgetBytes: 4096,
		},
		Coverage: SourceQueryCoverage{FilesConsidered: 1, FilesParsed: 1, CoverageComplete: true},
	}

	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal source_query context output: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode source_query context output: %v", err)
	}
	item := decoded["context"].(map[string]any)["items"].([]any)[0].(map[string]any)
	if got := item["reason"]; got != "direct-related-signature" {
		t.Fatalf("context.reason = %#v, want direct-related-signature", got)
	}
	if got := item["representation"]; got != "signature" {
		t.Fatalf("context.representation = %#v, want signature", got)
	}
}
