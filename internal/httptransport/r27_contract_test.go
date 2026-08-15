package httptransport

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/config"
)

func TestR27SourceQueryMatchesDirectAndHTTPAdapters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	root := canonicalHTTPTempDir(t)
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

	args := map[string]any{"operation": "search", "paths": []string{root}, "query": "Box", "mode": "structural"}
	for name, session := range map[string]*mcp.ClientSession{"direct": direct, "http": httpSession} {
		result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_query", Arguments: args})
		if callErr != nil {
			t.Fatalf("%s source_query call: %v", name, callErr)
		}
		if result == nil || !result.IsError || result.Meta[handler.ErrorCodeMetaKey] != handler.ErrCodeUnsupported {
			t.Fatalf("%s source_query result = %#v, want %s", name, result, handler.ErrCodeUnsupported)
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
