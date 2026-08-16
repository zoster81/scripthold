package httptransport

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

func TestR27SourceQueryMatchesDirectAndHTTPAdapters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	root := canonicalHTTPTempDir(t)
	sourcePath := filepath.Join(root, "Box.java")
	if err := os.WriteFile(sourcePath, []byte("package demo; public class Box { public int value() { return 1; } }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := sourceintelligence.OpenSourceDocument(ctx, sourcePath, sourceintelligence.OpenDocumentOptions{
		RequestedEncoding: "utf-8", MaxFileBytes: 1024 * 1024, MaxDecodedCharacters: 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := filetoolsserver.BuildServer(filetoolsserver.ServerOptions{
		Version:            "r27-source-query-transport-contract",
		AllowedDirectories: []string{root},
		Config:             config.Load(),
		EnableClientRoots:  false,
		LifecycleContext:   ctx,
	})

	cfg := validTestConfig(2)
	unstarted := httptest.NewUnstartedServer(nil)
	cfg.AllowedHosts = map[string]struct{}{strings.ToLower(unstarted.Listener.Addr().String()): {}}
	h := NewHandler(cfg, server, nil)
	h.setReady(true)
	unstarted.Config.Handler = h
	unstarted.Start()
	t.Cleanup(func() {
		unstarted.CloseClientConnections()
		unstarted.Close()
	})

	direct := connectDirectClient(t, ctx, server)
	t.Cleanup(func() { _ = direct.Close() })
	httpSession := connectHTTPClient(t, ctx, unstarted.URL+cfg.Path)
	t.Cleanup(func() { _ = httpSession.Close() })

	directTools, err := direct.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("direct list tools: %v", err)
	}
	httpTools, err := httpSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("HTTP list tools: %v", err)
	}

	directSource := r27FindToolJSON(t, directTools.Tools, "source_query")
	httpSource := r27FindToolJSON(t, httpTools.Tools, "source_query")
	if directSource != httpSource {
		t.Fatalf("source_query metadata diverged across direct/HTTP adapters:\ndirect=%s\nhttp=%s", directSource, httpSource)
	}

	requests := []struct {
		name string
		args map[string]any
	}{
		{name: "structural-search", args: map[string]any{"operation": "search", "paths": []string{root}, "query": "Box", "mode": "structural"}},
		{name: "context", args: map[string]any{
			"operation": "context", "paths": []string{root}, "language": "java", "encoding": "utf-8",
			"targets":     []any{map[string]any{"kind": "path", "path": sourcePath, "sourceFingerprint": document.SourceFingerprint}},
			"budgetBytes": 4096, "bodyPolicy": "signatures-only", "maxItems": 8, "maxDepth": 2,
		}},
	}
	for _, request := range requests {
		outputs := make(map[string]string, 2)
		for name, session := range map[string]*mcp.ClientSession{"direct": direct, "http": httpSession} {
			result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_query", Arguments: request.args})
			if callErr != nil {
				t.Fatalf("%s %s source_query call: %v", name, request.name, callErr)
			}
			if result == nil || result.IsError {
				t.Fatalf("%s %s source_query result = %#v, want success", name, request.name, result)
			}
			outputs[name] = marshalJSON(t, result.StructuredContent)
		}
		if outputs["direct"] != outputs["http"] {
			t.Fatalf("%s source_query output diverged across direct/HTTP adapters:\ndirect=%s\nhttp=%s", request.name, outputs["direct"], outputs["http"])
		}
	}
}

func r27FindToolJSON(t *testing.T, tools []*mcp.Tool, name string) string {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return marshalJSON(t, tool)
		}
	}
	t.Fatalf("R27 tool %s is absent", name)
	return ""
}
