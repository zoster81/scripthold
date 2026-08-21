package filetoolsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/deferredoperation"
)

func newDeferredReadOnlyExecutionFixture(t *testing.T) (string, string, string, *config.Config) {
	t.Helper()
	root := t.TempDir()
	textPath := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(textPath, []byte("alpha\nneedle\nomega\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	goPath := filepath.Join(root, "fixture.go")
	goSource := "package fixture\n\ntype Store struct{}\n\nfunc (Store) Hello() string { return \"needle\" }\n"
	if err := os.WriteFile(goPath, []byte(goSource), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.LoadFromEnvironment(func(string) string { return "" })
	cfg.Reliability.DeferredMaxRuntimeSeconds = 30
	cfg.Source.MaxRequestSeconds = 30
	return root, textPath, goPath, cfg
}

func assertDeferredExecutionEquivalent[In, Out any](t *testing.T, root, toolName string, cfg *config.Config, input In, direct mcp.ToolHandlerFor[In, Out]) Out {
	t.Helper()
	directResult, directOutput, directErr := direct(context.Background(), nil, input)
	if directErr != nil {
		t.Fatalf("direct %s transport error: %v", toolName, directErr)
	}
	arguments, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	payload, metadata, err := ExecuteDeferredOperation(context.Background(), deferredoperation.Request{
		Tool:               toolName,
		Arguments:          arguments,
		AllowedDirectories: []string{root},
		OriginPaths:        []string{root},
		Config: deferredoperation.ExecutionConfig{
			DefaultEncoding: cfg.DefaultEncoding,
			Limits:          cfg.Limits,
			Source:          cfg.Source,
		},
		MaxRuntimeSeconds: 30,
	})
	if err != nil {
		t.Fatalf("deferred %s execution error: %v", toolName, err)
	}
	deferredResult, deferredOutput, err := decodeDurableToolEnvelope[Out](payload)
	if err != nil {
		t.Fatalf("decode deferred %s result: %v", toolName, err)
	}
	if directResult == nil {
		directResult = &mcp.CallToolResult{}
	}
	if deferredResult == nil {
		deferredResult = &mcp.CallToolResult{}
	}
	if directResult.IsError != deferredResult.IsError || metadata.OriginalIsError != directResult.IsError || metadata.ErrorCode != resultErrorCode(directResult) {
		t.Fatalf("%s result metadata diverged: direct=%+v deferred=%+v metadata=%+v", toolName, directResult, deferredResult, metadata)
	}
	// The durable result is intentionally finalized to the public MCP shape,
	// whereas directHandler returns the pre-SDK result. Compare only handler
	// semantics here; TestDeferredResultMatchesPublicMCPWireShape owns final
	// Content/StructuredContent equivalence.
	if resultErrorCode(directResult) != resultErrorCode(deferredResult) {
		t.Fatalf("%s errorCode diverged: direct=%q deferred=%q", toolName, resultErrorCode(directResult), resultErrorCode(deferredResult))
	}
	directOutputJSON, err := json.Marshal(directOutput)
	if err != nil {
		t.Fatal(err)
	}
	deferredOutputJSON, err := json.Marshal(deferredOutput)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(directOutputJSON, deferredOutputJSON) {
		t.Fatalf("%s output diverged:\ndirect=%s\ndeferred=%s", toolName, directOutputJSON, deferredOutputJSON)
	}
	return deferredOutput
}

func TestExecuteDeferredOperationPreservesEligibleReadOnlyResults(t *testing.T) {
	root, textPath, goPath, cfg := newDeferredReadOnlyExecutionFixture(t)
	h := handler.NewHandler([]string{root}, handler.WithConfig(cfg))

	t.Run("grep_text_files", func(t *testing.T) {
		output := assertDeferredExecutionEquivalent(t, root, "grep_text_files", cfg,
			handler.GrepInput{Pattern: "needle", Paths: []string{textPath}, MaxMatches: 8}, h.HandleGrep)
		if output.TotalMatches != 1 || len(output.Matches) != 1 {
			t.Fatalf("unexpected grep output: %+v", output)
		}
	})

	t.Run("search_files", func(t *testing.T) {
		output := assertDeferredExecutionEquivalent(t, root, "search_files", cfg,
			handler.SearchFilesInput{Path: root, Pattern: "*.txt", MaxResults: 8}, h.HandleSearchFiles)
		if len(output.Files) != 1 || output.Files[0] != textPath {
			t.Fatalf("unexpected search output: %+v", output)
		}
	})

	t.Run("tree", func(t *testing.T) {
		output := assertDeferredExecutionEquivalent(t, root, "tree", cfg,
			handler.TreeInput{Path: root, MaxFiles: 16}, h.HandleTree)
		if !strings.Contains(output.Tree, "notes.txt") || !strings.Contains(output.Tree, "fixture.go") {
			t.Fatalf("unexpected tree output: %+v", output)
		}
	})

	t.Run("source_symbols", func(t *testing.T) {
		output := assertDeferredExecutionEquivalent(t, root, "source_symbols", cfg,
			handler.SourceSymbolsInput{Operation: "outline", Paths: []string{goPath}, Language: "go", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16}, h.SourceSymbols)
		if output.FilesParsed != 1 || output.SymbolCount == 0 {
			t.Fatalf("unexpected source_symbols output: %+v", output)
		}
	})

}

func TestExecuteDeferredOperationRejectsSourceQueryWhileIndexEvidenceIsProcessLocal(t *testing.T) {
	root, _, goPath, cfg := newDeferredReadOnlyExecutionFixture(t)
	arguments, err := json.Marshal(handler.SourceQueryInput{
		Operation: "search", Paths: []string{goPath}, Query: "Store", Mode: "structural", Match: "exact", Language: "go", Encoding: "utf-8", MaxFiles: 1, MaxResults: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = ExecuteDeferredOperation(context.Background(), deferredoperation.Request{
		Tool:               "source_query",
		Arguments:          arguments,
		AllowedDirectories: []string{root},
		OriginPaths:        []string{goPath},
		Config: deferredoperation.ExecutionConfig{
			DefaultEncoding: cfg.DefaultEncoding,
			Limits:          cfg.Limits,
			Source:          cfg.Source,
		},
		MaxRuntimeSeconds: 30,
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported deferred tool") {
		t.Fatalf("source_query deferred execution error = %v, want fail-closed unsupported result", err)
	}
}

func TestDeferredResultMatchesPublicMCPWireShape(t *testing.T) {
	root, _, goPath, cfg := newDeferredReadOnlyExecutionFixture(t)
	textPath := filepath.Join(root, "notes.txt")

	cases := []struct {
		name      string
		tool      string
		input     any
		wantError bool
	}{
		{name: "fingerprint success", tool: "fingerprint_paths", input: handler.FingerprintPathsInput{Paths: []string{textPath}}, wantError: false},
		{name: "source symbols success", tool: "source_symbols", input: handler.SourceSymbolsInput{Operation: "outline", Paths: []string{goPath}, Language: "go", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16}, wantError: false},
		{name: "grep tool error", tool: "grep_text_files", input: handler.GrepInput{Pattern: "[", Paths: []string{textPath}, MaxMatches: 8}, wantError: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			publicResult := callPublicTool(t, root, cfg, test.tool, test.input)
			if publicResult.IsError != test.wantError {
				t.Fatalf("public isError=%v, want %v", publicResult.IsError, test.wantError)
			}
			arguments, err := json.Marshal(test.input)
			if err != nil {
				t.Fatal(err)
			}
			payload, metadata, err := ExecuteDeferredOperation(context.Background(), deferredoperation.Request{
				Tool:               test.tool,
				Arguments:          arguments,
				AllowedDirectories: []string{root},
				OriginPaths:        []string{root},
				Config: deferredoperation.ExecutionConfig{
					DefaultEncoding: cfg.DefaultEncoding,
					Limits:          cfg.Limits,
					Source:          cfg.Source,
				},
				MaxRuntimeSeconds: 30,
			})
			if err != nil {
				t.Fatal(err)
			}
			var envelope durableToolEnvelope
			if err := json.Unmarshal(payload, &envelope); err != nil {
				t.Fatal(err)
			}
			var deferredResult mcp.CallToolResult
			if err := json.Unmarshal(envelope.Result, &deferredResult); err != nil {
				t.Fatal(err)
			}
			assertEquivalentCallToolResult(t, publicResult, &deferredResult)
			if metadata.OriginalIsError != publicResult.IsError || metadata.ErrorCode != resultErrorCode(publicResult) {
				t.Fatalf("metadata=%+v public=%+v", metadata, publicResult)
			}
		})
	}
}

func callPublicTool(t *testing.T, root string, cfg *config.Config, toolName string, input any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	server := BuildServer(ServerOptions{AllowedDirectories: []string{root}, Config: cfg, LifecycleContext: ctx})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "deferred-wire-equivalence", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var arguments map[string]any
	if err := json.Unmarshal(encoded, &arguments); err != nil {
		t.Fatal(err)
	}
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: arguments})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func assertEquivalentCallToolResult(t *testing.T, expected, actual *mcp.CallToolResult) {
	t.Helper()
	canonical := func(result *mcp.CallToolResult) []byte {
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(encoded, &value); err != nil {
			t.Fatal(err)
		}
		// These fields belong to the enclosing MCP method/transport response and
		// are deliberately not persisted inside the underlying tool result.
		delete(value, "resultType")
		if meta, ok := value["_meta"].(map[string]any); ok {
			delete(meta, "io.modelcontextprotocol/serverInfo")
			if len(meta) == 0 {
				delete(value, "_meta")
			}
		}
		resultJSON, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return resultJSON
	}
	expectedCanonical := canonical(expected)
	actualCanonical := canonical(actual)
	if !bytes.Equal(expectedCanonical, actualCanonical) {
		t.Fatalf("CallToolResult diverged:\nexpected=%s\nactual=%s", expectedCanonical, actualCanonical)
	}
}

func TestDeferredRuntimePreservesSourceExecutionCeiling(t *testing.T) {
	cfg := config.LoadFromEnvironment(func(string) string { return "" })
	cfg.Reliability.DeferredMaxRuntimeSeconds = 300
	cfg.Source.MaxRequestSeconds = 17
	if got := deferredRuntimeSeconds(cfg, true); got != 17 {
		t.Fatalf("source deferred runtime = %d, want 17", got)
	}
	if got := deferredRuntimeSeconds(cfg, false); got != 300 {
		t.Fatalf("ordinary deferred runtime = %d, want 300", got)
	}
}
