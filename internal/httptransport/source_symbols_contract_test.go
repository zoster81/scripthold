package httptransport

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zoster81/scripthold/filetoolsserver"
	"github.com/zoster81/scripthold/internal/config"
)

func TestSourceSymbolsSchemaMatchesDirectAndHTTPAdapters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	root := canonicalHTTPTempDir(t)
	server := filetoolsserver.BuildServer(filetoolsserver.ServerOptions{
		Version:            "r25-source-symbols-transport-contract",
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

	var directSource, httpSource string
	for _, tool := range directTools.Tools {
		if tool.Name == "source_symbols" {
			directSource = marshalJSON(t, tool)
			break
		}
	}
	for _, tool := range httpTools.Tools {
		if tool.Name == "source_symbols" {
			httpSource = marshalJSON(t, tool)
			break
		}
	}
	if directSource == "" || httpSource == "" {
		t.Fatalf("R25 source_symbols implementation is absent: direct=%v http=%v", directSource != "", httpSource != "")
	}
	if directSource != httpSource {
		t.Fatalf("source_symbols metadata diverged across direct/HTTP adapters:\ndirect=%s\nhttp=%s", directSource, httpSource)
	}
}
