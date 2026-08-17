package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

func TestR27Phase13SourceQueryStructuralSearchAndCoverage(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	basePath := filepath.Join(root, "Base.java")
	childPath := filepath.Join(root, "Child.java")
	writePhase13Source(t, basePath, "package demo; public class Base {}\n")
	writePhase13Source(t, childPath, "package demo; public class Child extends Base {}\n")
	h := NewHandler([]string{root})

	result, output, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{root}, Query: "Base", Mode: "structural", Match: "exact", Language: "java", MaxResults: 16,
	})
	if err != nil || (result != nil && result.IsError) {
		t.Fatalf("structural search result=%+v output=%+v err=%v", result, output, err)
	}
	if output.Operation != "search" || output.Search == nil || len(output.Search.Matches) < 2 {
		t.Fatalf("structural search output = %+v", output)
	}
	if output.Index.Staleness != sourceintelligence.IndexCurrent || output.Index.Generation == 0 || output.Index.Fingerprint == "" {
		t.Fatalf("index = %+v, want current coherent generation", output.Index)
	}
	if output.Coverage.FilesConsidered != 2 || output.Coverage.FilesParsed != 2 || output.Coverage.FilesSkipped != 0 || !output.Coverage.CoverageComplete {
		t.Fatalf("coverage = %+v", output.Coverage)
	}
	foundDefinition := false
	foundRelation := false
	for _, match := range output.Search.Matches {
		if match.Path == basePath && match.SymbolID != "" && match.Evidence == sourceintelligence.SymbolEvidenceStructural {
			foundDefinition = true
		}
		if match.Path == childPath && match.SymbolID == "" && match.Evidence == sourceintelligence.SymbolEvidenceProjectResolved {
			foundRelation = true
		}
	}
	if !foundDefinition || !foundRelation {
		t.Fatalf("structural search matches = %+v", output.Search.Matches)
	}
}

func TestR27Phase13SourceQueryProjectRelationsAndStaleSelector(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	basePath := filepath.Join(root, "base.ts")
	childPath := filepath.Join(root, "child.ts")
	writePhase13Source(t, basePath, "export class Base {}\n")
	childBytes := []byte("import { Base } from \"./base\";\nexport class Child extends Base {}\n")
	if err := os.WriteFile(childPath, childBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	childFingerprint := phase13FileFingerprint(t, childPath)

	result, output, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "relations", Paths: []string{root}, Relation: "dependencies", Language: "typescript",
		Subject: &SourceSelectorInput{Kind: "path", Path: childPath, SourceFingerprint: childFingerprint}, MaxResults: 16, MaxNodes: 16, MaxEdges: 32, MaxDepth: 4,
	})
	if err != nil || (result != nil && result.IsError) {
		t.Fatalf("dependencies result=%+v output=%+v err=%v", result, output, err)
	}
	if output.Relations == nil || output.Relations.Relation != sourceintelligence.RelationDependencies || len(output.Relations.Relations) != 1 {
		t.Fatalf("dependencies output = %+v", output)
	}
	edge := output.Relations.Relations[0]
	if edge.Source.Path != childPath || edge.Target.Path != basePath || edge.Evidence != sourceintelligence.SymbolEvidenceProjectResolved || edge.Resolution != sourceintelligence.ResolutionResolved {
		t.Fatalf("dependency edge = %+v", edge)
	}

	filteredResult, filteredOutput, filteredErr := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "relations", Paths: []string{root}, Relation: "dependencies", Language: "typescript", Evidence: []string{"structural"},
		Subject: &SourceSelectorInput{Kind: "path", Path: childPath, SourceFingerprint: childFingerprint}, MaxResults: 16, MaxNodes: 16, MaxEdges: 32, MaxDepth: 4,
	})
	if filteredErr != nil || (filteredResult != nil && filteredResult.IsError) {
		t.Fatalf("filtered dependencies result=%+v output=%+v err=%v", filteredResult, filteredOutput, filteredErr)
	}
	if filteredOutput.Relations == nil || len(filteredOutput.Relations.Relations) != 0 {
		t.Fatalf("structural evidence filter retained project-resolved dependency: %+v", filteredOutput)
	}

	staleResult, _, staleErr := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "relations", Paths: []string{root}, Relation: "dependencies", Language: "typescript",
		Subject: &SourceSelectorInput{Kind: "path", Path: childPath, SourceFingerprint: phase13ZeroDigestHandler()}, MaxResults: 16, MaxNodes: 16, MaxEdges: 32, MaxDepth: 4,
	})
	if staleErr != nil {
		t.Fatalf("stale selector transport error: %v", staleErr)
	}
	if staleResult == nil || !staleResult.IsError || staleResult.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale selector result = %+v", staleResult)
	}
}

func TestR27Phase13SourceQueryGraphAndDeferredModes(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	aPath := filepath.Join(root, "a.ts")
	bPath := filepath.Join(root, "b.ts")
	cPath := filepath.Join(root, "c.ts")
	aBytes := []byte("import { B } from \"./b\";\nexport class A extends B {}\n")
	bBytes := []byte("import { C } from \"./c\";\nexport class B extends C {}\n")
	cBytes := []byte("import { A } from \"./a\";\nexport class C {}\n")
	writePhase13Bytes(t, aPath, aBytes)
	writePhase13Bytes(t, bPath, bBytes)
	writePhase13Bytes(t, cPath, cBytes)
	h := NewHandler([]string{root})

	result, output, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "relations", Paths: []string{root}, Relation: "trace", Language: "typescript",
		Subject:    &SourceSelectorInput{Kind: "path", Path: aPath, SourceFingerprint: phase13FileFingerprint(t, aPath)},
		Target:     &SourceSelectorInput{Kind: "path", Path: cPath, SourceFingerprint: phase13FileFingerprint(t, cPath)},
		MaxResults: 16, MaxNodes: 16, MaxEdges: 32, MaxDepth: 4,
	})
	if err != nil || (result != nil && result.IsError) || output.Relations == nil || len(output.Relations.Relations) != 2 {
		t.Fatalf("trace result=%+v output=%+v err=%v", result, output, err)
	}

	filteredTraceResult, _, filteredTraceErr := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "relations", Paths: []string{root}, Relation: "trace", Language: "typescript", Evidence: []string{"structural"},
		Subject:    &SourceSelectorInput{Kind: "path", Path: aPath, SourceFingerprint: phase13FileFingerprint(t, aPath)},
		Target:     &SourceSelectorInput{Kind: "path", Path: cPath, SourceFingerprint: phase13FileFingerprint(t, cPath)},
		MaxResults: 16, MaxNodes: 16, MaxEdges: 32, MaxDepth: 4,
	})
	if filteredTraceErr != nil {
		t.Fatalf("filtered trace transport error: %v", filteredTraceErr)
	}
	if filteredTraceResult == nil || !filteredTraceResult.IsError || filteredTraceResult.Meta[ErrorCodeMetaKey] != ErrCodeNotFound {
		t.Fatalf("filtered trace result = %+v, want NOT_FOUND on evidence-filtered graph", filteredTraceResult)
	}

	cyclesResult, cyclesOutput, cyclesErr := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "relations", Paths: []string{root}, Relation: "cycles", Language: "typescript", MaxResults: 16, MaxNodes: 16, MaxEdges: 32,
	})
	if cyclesErr != nil || (cyclesResult != nil && cyclesResult.IsError) || cyclesOutput.Relations == nil || len(cyclesOutput.Relations.Relations) != 3 {
		t.Fatalf("cycles result=%+v output=%+v err=%v", cyclesResult, cyclesOutput, cyclesErr)
	}

	for _, input := range []SourceQueryInput{
		{Operation: "search", Paths: []string{root}, Query: "Base", Mode: "textual"},
		{Operation: "search", Paths: []string{root}, Query: "Base", Mode: "lexical"},
	} {
		deferredResult, _, deferredErr := h.SourceQuery(context.Background(), nil, input)
		if deferredErr != nil {
			t.Fatalf("deferred %s/%s transport error: %v", input.Operation, input.Mode, deferredErr)
		}
		if deferredResult == nil || !deferredResult.IsError || deferredResult.Meta[ErrorCodeMetaKey] != ErrCodeUnsupported {
			t.Fatalf("deferred %s/%s result = %+v", input.Operation, input.Mode, deferredResult)
		}
	}

	missingIndexResult, _, missingIndexErr := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{root}, Query: "Base", Mode: "structural",
		Index: &SourceIndexBindingInput{Fingerprint: phase13ZeroDigestHandler()},
	})
	if missingIndexErr != nil {
		t.Fatalf("missing index transport error: %v", missingIndexErr)
	}
	if missingIndexResult == nil || !missingIndexResult.IsError || missingIndexResult.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("missing index result = %+v, want CONFLICT", missingIndexResult)
	}
}

func writePhase13Source(t *testing.T, path, text string) {
	t.Helper()
	writePhase13Bytes(t, path, []byte(text))
}

func writePhase13Bytes(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func phase13FileFingerprint(t *testing.T, path string) string {
	t.Helper()
	document, err := sourceintelligence.OpenSourceDocument(context.Background(), path, sourceintelligence.OpenDocumentOptions{
		MaxFileBytes: 1024 * 1024, MaxDecodedCharacters: 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	return document.SourceFingerprint
}

func phase13ZeroDigestHandler() string {
	return "0000000000000000000000000000000000000000000000000000000000000000"
}
