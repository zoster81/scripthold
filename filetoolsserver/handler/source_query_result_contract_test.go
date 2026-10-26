package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestSourceQuerySearchIdentifiesDeclarationsWithoutAdditionalLookup(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "Service.java")
	if err := os.WriteFile(path, []byte("package demo; public class Service { public void run() {} }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	result, output, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Query: "Service", Mode: "structural",
		Match: "exact", Language: "java", Encoding: "utf-8", MaxResults: 8,
	})
	if err != nil || (result != nil && result.IsError) || output.Search == nil || len(output.Search.Matches) != 1 {
		t.Fatalf("source query result=%+v output=%+v err=%v", result, output, err)
	}
	encoded, err := json.Marshal(output.Search.Matches[0])
	if err != nil {
		t.Fatal(err)
	}
	var match map[string]any
	if err := json.Unmarshal(encoded, &match); err != nil {
		t.Fatal(err)
	}
	if match["name"] != "Service" || match["kind"] != "class" {
		t.Fatalf("declaration identity missing in search output: %v", match)
	}
	if name, ok := match["qualifiedName"].(string); !ok || name == "" {
		t.Fatalf("declaration qualified name missing in search output: %v", match)
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
				Reason:         sourceintelligence.ContextDirectDependency,
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
	if got := item["reason"]; got != "direct-dependency" {
		t.Fatalf("context.reason = %#v, want direct-dependency", got)
	}
	if got := item["representation"]; got != "signature" {
		t.Fatalf("context.representation = %#v, want signature", got)
	}
}
