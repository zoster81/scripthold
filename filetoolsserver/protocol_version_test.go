package filetoolsserver

import (
	"context"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/config"
)

const (
	modernProtocolVersion = "2026-07-28"
	legacyProtocolVersion = "2025-11-25"
)

func TestBuildServerNegotiatesModernProtocolWithConfiguredDirectories(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	root := t.TempDir()
	server := BuildServer(ServerOptions{
		Version:            "modern-stdio-test",
		AllowedDirectories: []string{root},
		Config:             config.Load(),
		EnableClientRoots:  true,
		LifecycleContext:   ctx,
	})

	_, session := connectProtocolTestClient(t, ctx, server, "modern-stdio")
	initialization := session.InitializeResult()
	if got := initialization.ProtocolVersion; got != modernProtocolVersion {
		t.Fatalf("protocol version = %q, want %q", got, modernProtocolVersion)
	}
	//lint:ignore SA1019 R20 verifies that modern discovery omits deprecated protocol logging.
	if initialization.Capabilities == nil || initialization.Capabilities.Logging != nil {
		t.Fatalf("modern capabilities unexpectedly advertise protocol logging: %#v", initialization.Capabilities)
	}
	assertProtocolCatalog(t, ctx, session)
	if got := allowedDirectories(t, ctx, session); len(got) != 1 || !samePath(got[0], root) {
		t.Fatalf("allowed directories = %v, want [%q]", got, root)
	}
}

func TestBuildServerCanForceLegacyHandshakeWithConfiguredDirectories(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	root := t.TempDir()
	server := BuildServer(ServerOptions{
		Version:                "legacy-stdio-compat-test",
		AllowedDirectories:     []string{root},
		Config:                 config.Load(),
		EnableClientRoots:      true,
		DisableModernDiscovery: true,
		LifecycleContext:       ctx,
	})

	_, session := connectProtocolTestClient(t, ctx, server, "legacy-stdio-compat")
	initialization := session.InitializeResult()
	if got := initialization.ProtocolVersion; got != legacyProtocolVersion {
		t.Fatalf("protocol version = %q, want %q", got, legacyProtocolVersion)
	}
	//lint:ignore SA1019 compatibility mode intentionally retains legacy protocol logging.
	if initialization.Capabilities == nil || initialization.Capabilities.Logging == nil {
		t.Fatalf("legacy capabilities lost protocol logging compatibility: %#v", initialization.Capabilities)
	}
	assertProtocolCatalog(t, ctx, session)
	if got := allowedDirectories(t, ctx, session); len(got) != 1 || !samePath(got[0], root) {
		t.Fatalf("allowed directories = %v, want [%q]", got, root)
	}
}

func TestBuildServerAllowsModernProtocolWhenClientRootsAreDisabled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := BuildServer(ServerOptions{
		Version:           "modern-no-roots-test",
		Config:            config.Load(),
		EnableClientRoots: false,
		LifecycleContext:  ctx,
	})

	_, session := connectProtocolTestClient(t, ctx, server, "modern-no-roots")
	initialization := session.InitializeResult()
	if got := initialization.ProtocolVersion; got != modernProtocolVersion {
		t.Fatalf("protocol version = %q, want %q", got, modernProtocolVersion)
	}
	//lint:ignore SA1019 R20 verifies that modern discovery omits deprecated protocol logging.
	if initialization.Capabilities == nil || initialization.Capabilities.Logging != nil {
		t.Fatalf("modern capabilities unexpectedly advertise protocol logging: %#v", initialization.Capabilities)
	}
	assertProtocolCatalog(t, ctx, session)
	if got := allowedDirectories(t, ctx, session); len(got) != 0 {
		t.Fatalf("allowed directories = %v, want none", got)
	}
}

func TestBuildServerFallsBackToLegacyProtocolForDynamicClientRoots(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	root := t.TempDir()
	rootURI := fileURIForTest(root)
	server := BuildServer(ServerOptions{
		Version:           "legacy-roots-test",
		Config:            config.Load(),
		EnableClientRoots: true,
		LifecycleContext:  ctx,
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "legacy-roots-client", Version: "test"}, nil)
	//lint:ignore SA1019 R20 intentionally preserves legacy roots during the compatibility window.
	client.AddRoots(&mcp.Root{URI: rootURI})
	session := connectExistingProtocolTestClient(t, ctx, server, client)

	initialization := session.InitializeResult()
	if got := initialization.ProtocolVersion; got != legacyProtocolVersion {
		t.Fatalf("protocol version = %q, want %q", got, legacyProtocolVersion)
	}
	//lint:ignore SA1019 R20 verifies that legacy initialization retains protocol logging compatibility.
	if initialization.Capabilities == nil || initialization.Capabilities.Logging == nil {
		t.Fatalf("legacy capabilities lost protocol logging compatibility: %#v", initialization.Capabilities)
	}
	assertProtocolCatalog(t, ctx, session)
	eventuallyAllowedDirectories(t, ctx, session, []string{root})

	//lint:ignore SA1019 R20 intentionally verifies legacy roots change notifications.
	client.RemoveRoots(rootURI)
	eventuallyAllowedDirectories(t, ctx, session, nil)
}

func connectProtocolTestClient(t *testing.T, ctx context.Context, server *mcp.Server, name string) (*mcp.Client, *mcp.ClientSession) {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: name, Version: "test"}, nil)
	return client, connectExistingProtocolTestClient(t, ctx, server, client)
}

func connectExistingProtocolTestClient(t *testing.T, ctx context.Context, server *mcp.Server, client *mcp.Client) *mcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

func assertProtocolCatalog(t *testing.T, ctx context.Context, session *mcp.ClientSession) {
	t.Helper()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != 30 {
		t.Fatalf("tool count = %d, want 30", len(tools.Tools))
	}
	prompts, err := session.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}
	if len(prompts.Prompts) != 3 {
		t.Fatalf("prompt count = %d, want 3", len(prompts.Prompts))
	}
}

func eventuallyAllowedDirectories(t *testing.T, ctx context.Context, session *mcp.ClientSession, want []string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := allowedDirectories(t, ctx, session)
		if samePaths(got, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("allowed directories = %v, want %v", got, want)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for allowed directories: %v", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func samePaths(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	gotClean := make([]string, len(got))
	wantClean := make([]string, len(want))
	for i := range got {
		gotClean[i] = filepath.Clean(got[i])
	}
	for i := range want {
		wantClean[i] = filepath.Clean(want[i])
	}
	sort.Strings(gotClean)
	sort.Strings(wantClean)
	return reflect.DeepEqual(gotClean, wantClean)
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func fileURIForTest(path string) string {
	uri := "file://" + filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" {
		uri = "file:///" + filepath.ToSlash(path)
	}
	return uri
}
