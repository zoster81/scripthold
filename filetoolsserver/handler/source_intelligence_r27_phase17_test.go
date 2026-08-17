package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

func TestR27Phase17ManySmallFilesWarmConcurrencyAndGenerationSwap(t *testing.T) {
	root := t.TempDir()
	const fileCount = 96
	for index := 0; index < fileCount; index++ {
		name := fmt.Sprintf("Item%03d", index)
		content := fmt.Sprintf("package scale; public class %s { public int value() { return %d; } }\n", name, index)
		if err := os.WriteFile(filepath.Join(root, name+".java"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler([]string{root})
	query := SourceQueryInput{
		Operation: "search", Paths: []string{root}, Query: "Item095", Mode: "structural", Match: "exact",
		Language: "java", Encoding: "utf-8", MaxFiles: fileCount, MaxResults: 8,
	}
	firstResult, first, err := h.SourceQuery(context.Background(), nil, query)
	if err != nil || (firstResult != nil && firstResult.IsError) {
		t.Fatalf("cold scale query result=%+v output=%+v err=%v", firstResult, first, err)
	}
	if first.Coverage.FilesConsidered != fileCount || first.Coverage.FilesParsed != fileCount || !first.Coverage.CoverageComplete || first.Search == nil || len(first.Search.Matches) != 1 {
		t.Fatalf("cold scale output=%+v", first)
	}
	if first.Index.Staleness != sourceintelligence.IndexCurrent || first.Index.Generation == 0 || first.Index.Fingerprint == "" {
		t.Fatalf("cold scale index=%+v", first.Index)
	}

	warmResult, warm, err := h.SourceQuery(context.Background(), nil, query)
	if err != nil || (warmResult != nil && warmResult.IsError) {
		t.Fatalf("warm scale query result=%+v output=%+v err=%v", warmResult, warm, err)
	}
	if warm.Index != first.Index || warm.Search == nil || len(warm.Search.Matches) != 1 || warm.Search.Matches[0].SymbolID != first.Search.Matches[0].SymbolID {
		t.Fatalf("warm query changed coherent generation: cold=%+v warm=%+v", first.Index, warm.Index)
	}

	const workers = 12
	type concurrentResult struct {
		output SourceQueryOutput
		err    error
		tool   bool
	}
	results := make(chan concurrentResult, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			result, output, queryErr := h.SourceQuery(context.Background(), nil, query)
			results <- concurrentResult{output: output, err: queryErr, tool: result != nil && result.IsError}
		}()
	}
	group.Wait()
	close(results)
	for current := range results {
		if current.err != nil || current.tool {
			t.Fatalf("concurrent warm query err=%v toolError=%v output=%+v", current.err, current.tool, current.output)
		}
		if current.output.Index != first.Index || current.output.Search == nil || len(current.output.Search.Matches) != 1 || current.output.Search.Matches[0].SymbolID != first.Search.Matches[0].SymbolID {
			t.Fatalf("concurrent warm query observed mixed generation: want=%+v got=%+v", first.Index, current.output.Index)
		}
	}

	changedPath := filepath.Join(root, "Item095.java")
	if err := os.WriteFile(changedPath, []byte("package scale; public class Item095Changed { public int value() { return 9500; } }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changedQuery := query
	changedQuery.Query = "Item095Changed"
	changedResult, changed, err := h.SourceQuery(context.Background(), nil, changedQuery)
	if err != nil || (changedResult != nil && changedResult.IsError) {
		t.Fatalf("changed scale query result=%+v output=%+v err=%v", changedResult, changed, err)
	}
	if changed.Index.Generation <= first.Index.Generation || changed.Index.Fingerprint == first.Index.Fingerprint || changed.Search == nil || len(changed.Search.Matches) != 1 {
		t.Fatalf("generation swap failed: before=%+v after=%+v search=%+v", first.Index, changed.Index, changed.Search)
	}
	oldResult, oldOutput, err := h.SourceQuery(context.Background(), nil, query)
	if err != nil || (oldResult != nil && oldResult.IsError) {
		t.Fatalf("old-name post-swap query result=%+v output=%+v err=%v", oldResult, oldOutput, err)
	}
	if oldOutput.Index != changed.Index || oldOutput.Search == nil || len(oldOutput.Search.Matches) != 0 {
		t.Fatalf("post-swap query retained stale symbol: index=%+v search=%+v", oldOutput.Index, oldOutput.Search)
	}
}

func TestR27Phase17AllowedRootCancellationAndNoUnrelatedSourceLeakage(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	sentinel := "unrelated-source-body-sentinel"
	safePath := filepath.Join(root, "Safe.java")
	secretPath := filepath.Join(root, "Secret.java")
	if err := os.WriteFile(safePath, []byte("package safe; public class Safe { public int run(int value) { return value + 1; } }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretPath, []byte("package secret; public class Secret { public String hidden() { return \""+sentinel+"\"; } }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "Outside.java"), []byte("public class Outside {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})

	denied, _, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{outside}, Query: "Outside", Mode: "structural", Match: "exact", Language: "java",
	})
	if err != nil {
		t.Fatalf("outside-root transport error: %v", err)
	}
	if denied == nil || !denied.IsError || denied.Meta[ErrorCodeMetaKey] != ErrCodeAccessDenied {
		t.Fatalf("outside-root query=%+v want %s", denied, ErrCodeAccessDenied)
	}

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled, _, err := h.SourceQuery(cancelledCtx, nil, SourceQueryInput{
		Operation: "search", Paths: []string{root}, Query: "Safe", Mode: "structural", Match: "exact", Language: "java",
	})
	if err != nil {
		t.Fatalf("cancelled query transport error: %v", err)
	}
	if cancelled == nil || !cancelled.IsError || cancelled.Meta[ErrorCodeMetaKey] != ErrCodeCancelled {
		t.Fatalf("cancelled query=%+v want %s", cancelled, ErrCodeCancelled)
	}

	searchResult, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{root}, Query: "Safe", Mode: "structural", Match: "exact", Language: "java", MaxResults: 8,
	})
	if err != nil || (searchResult != nil && searchResult.IsError) || search.Search == nil || len(search.Search.Matches) != 1 {
		t.Fatalf("safe search result=%+v output=%+v err=%v", searchResult, search, err)
	}
	encodedSearch, err := json.Marshal(search)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedSearch), sentinel) {
		t.Fatalf("structural search leaked unrelated source body: %s", encodedSearch)
	}
	match := search.Search.Matches[0]
	selector := SourceSelectorInput{
		Kind: "symbol", Path: match.Path, SymbolID: match.SymbolID, SourceFingerprint: match.SourceFingerprint,
	}
	contextResult, contextOutput, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "context", Paths: []string{root}, Targets: []SourceSelectorInput{selector}, BudgetBytes: 4096,
		BodyPolicy: "prefer", Language: "java", Encoding: "utf-8", MaxItems: 8, MaxDepth: 2,
	})
	if err != nil || (contextResult != nil && contextResult.IsError) || contextOutput.Context == nil || len(contextOutput.Context.Items) == 0 {
		t.Fatalf("safe context result=%+v output=%+v err=%v", contextResult, contextOutput, err)
	}
	encodedContext, err := json.Marshal(contextOutput)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedContext), sentinel) {
		t.Fatalf("context leaked unrelated source body: %s", encodedContext)
	}
	if contextOutput.Context.UsedBytes > contextOutput.Context.BudgetBytes || contextOutput.Context.BudgetBytes != 4096 {
		t.Fatalf("context budget not bounded: %+v", contextOutput.Context)
	}
}

func TestR27Phase17SourceQueryFilesHaveNoHiddenExecutionOrNetworkImports(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate handler package")
	}
	directory := filepath.Dir(currentFile)
	files := []string{
		"source_intelligence_r27.go",
		"source_query_engine.go",
		"source_query_index.go",
		"source_symbols.go",
		"source_symbols_helpers.go",
	}
	forbidden := map[string]bool{"os/exec": true, "log/slog": true, "net": true, "net/http": true, "net/rpc": true, "net/smtp": true}
	fileset := token.NewFileSet()
	for _, name := range files {
		path := filepath.Join(directory, name)
		file, err := parser.ParseFile(fileset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports %s: %v", name, err)
		}
		for _, imported := range file.Imports {
			value := strings.Trim(imported.Path.Value, "\"")
			if forbidden[value] {
				t.Errorf("source-query production file %s imports forbidden execution/network package %q", name, value)
			}
		}
	}
}
