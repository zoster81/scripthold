package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

func TestR27Phase15SourceQuerySkipsStableDecodeFailureWithoutFalseConflict(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	validPath := filepath.Join(root, "Valid.java")
	brokenPath := filepath.Join(root, "Broken.java")
	if err := os.WriteFile(validPath, []byte("package demo;\npublic class Valid {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(brokenPath, []byte{0xc3, 0x28}, 0o600); err != nil {
		t.Fatal(err)
	}

	h := NewHandler([]string{root})
	result, output, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{root}, Query: "Valid", Mode: "structural",
		Language: "java", Encoding: "utf-8", MaxResults: 32,
	})
	if err != nil || (result != nil && result.IsError) {
		t.Fatalf("query result=%+v output=%+v err=%v", result, output, err)
	}
	if output.Index.Generation == 0 || output.Index.Fingerprint == "" || output.Index.Staleness != sourceintelligence.IndexCurrent {
		t.Fatalf("index evidence = %+v", output.Index)
	}
	if output.Coverage.FilesConsidered != 2 || output.Coverage.FilesParsed != 1 || output.Coverage.FilesSkipped != 1 || output.Coverage.CoverageComplete {
		t.Fatalf("coverage = %+v", output.Coverage)
	}
	if output.Search == nil || len(output.Search.Matches) != 1 || output.Search.Matches[0].Path != validPath {
		t.Fatalf("search = %+v", output.Search)
	}
}

func TestR27Phase15SourceQueryIncrementalGenerationAndStaleBinding(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "Box.java")
	if err := os.WriteFile(path, []byte("package demo;\npublic class Box {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	query := SourceQueryInput{Operation: "search", Paths: []string{root}, Query: "Box", Mode: "structural", Language: "java", Encoding: "utf-8", MaxResults: 32}

	firstResult, first, err := h.SourceQuery(context.Background(), nil, query)
	if err != nil || (firstResult != nil && firstResult.IsError) {
		t.Fatalf("first query result=%+v output=%+v err=%v", firstResult, first, err)
	}
	if first.Index.Generation == 0 || first.Index.Fingerprint == "" || first.Index.Staleness != sourceintelligence.IndexCurrent {
		t.Fatalf("first index evidence = %+v", first.Index)
	}
	if first.Search == nil || len(first.Search.Matches) != 1 || first.Search.Matches[0].SourceFingerprint == "" {
		t.Fatalf("first search = %+v", first.Search)
	}

	warmResult, warm, err := h.SourceQuery(context.Background(), nil, query)
	if err != nil || (warmResult != nil && warmResult.IsError) {
		t.Fatalf("warm query result=%+v output=%+v err=%v", warmResult, warm, err)
	}
	if warm.Index != first.Index {
		t.Fatalf("warm query changed generation: first=%+v warm=%+v", first.Index, warm.Index)
	}

	if err := os.WriteFile(path, []byte("package demo;\npublic class Crate {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	currentResult, current, err := h.SourceQuery(context.Background(), nil, query)
	if err != nil || (currentResult != nil && currentResult.IsError) {
		t.Fatalf("current query result=%+v output=%+v err=%v", currentResult, current, err)
	}
	if current.Index.Generation == first.Index.Generation || current.Index.Fingerprint == first.Index.Fingerprint || current.Index.Staleness != sourceintelligence.IndexCurrent {
		t.Fatalf("changed source did not advance current generation: first=%+v current=%+v", first.Index, current.Index)
	}
	if current.Search == nil || len(current.Search.Matches) != 0 {
		t.Fatalf("current search retained stale Box match: %+v", current.Search)
	}

	oldGeneration := first.Index.Generation
	reject := query
	reject.Index = &SourceIndexBindingInput{Generation: &oldGeneration}
	rejectResult, _, rejectErr := h.SourceQuery(context.Background(), nil, reject)
	if rejectErr != nil {
		t.Fatalf("stale reject transport error: %v", rejectErr)
	}
	if rejectResult == nil || !rejectResult.IsError || rejectResult.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale reject result = %+v", rejectResult)
	}

	allow := query
	allow.Index = &SourceIndexBindingInput{Generation: &oldGeneration, Fingerprint: first.Index.Fingerprint, StalePolicy: "allow"}
	allowResult, stale, allowErr := h.SourceQuery(context.Background(), nil, allow)
	if allowErr != nil || (allowResult != nil && allowResult.IsError) {
		t.Fatalf("stale allow result=%+v output=%+v err=%v", allowResult, stale, allowErr)
	}
	if stale.Index.Generation != first.Index.Generation || stale.Index.Fingerprint != first.Index.Fingerprint || stale.Index.Staleness != sourceintelligence.IndexStale {
		t.Fatalf("stale allow evidence = %+v", stale.Index)
	}
	if stale.Search == nil || len(stale.Search.Matches) != 1 || stale.Search.Matches[0].SymbolID != first.Search.Matches[0].SymbolID {
		t.Fatalf("stale generation search = %+v, first=%+v", stale.Search, first.Search)
	}

	selector := SourceSelectorInput{
		Kind:              "symbol",
		Path:              path,
		SymbolID:          stale.Search.Matches[0].SymbolID,
		SourceFingerprint: stale.Search.Matches[0].SourceFingerprint,
	}
	contextResult, _, contextErr := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "context", Paths: []string{root}, Targets: []SourceSelectorInput{selector}, BudgetBytes: 4096,
		BodyPolicy: "prefer", Language: "java", Encoding: "utf-8", MaxItems: 8, MaxDepth: 2,
		Index: &SourceIndexBindingInput{Generation: &oldGeneration, Fingerprint: first.Index.Fingerprint, StalePolicy: "allow"},
	})
	if contextErr != nil {
		t.Fatalf("stale context transport error: %v", contextErr)
	}
	if contextResult == nil || !contextResult.IsError || contextResult.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale historical body should conflict with current source: %+v", contextResult)
	}
}
