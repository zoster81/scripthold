package filetoolsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/security"
)

func TestLegacyHandshakeMakesEquivalentRepeatedInitializeIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := BuildServer(ServerOptions{
		Version:                "legacy-repeated-initialize-test",
		AllowedDirectories:     []string{t.TempDir()},
		Config:                 config.Load(),
		EnableClientRoots:      true,
		DisableModernDiscovery: true,
		LifecycleContext:       ctx,
	})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	connection, err := clientTransport.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })

	params := json.RawMessage(`{
		"protocolVersion":"2025-11-25",
		"capabilities":{},
		"clientInfo":{"name":"openai-tunnel","version":"test"}
	}`)
	initialize := func(id int64, raw json.RawMessage) *jsonrpc.Response {
		t.Helper()
		request, err := jsonrpc.DecodeMessage([]byte(fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%d,"method":"initialize","params":%s}`,
			id, raw,
		)))
		if err != nil {
			t.Fatal(err)
		}
		if err := connection.Write(ctx, request); err != nil {
			t.Fatal(err)
		}
		message, err := connection.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		response, ok := message.(*jsonrpc.Response)
		if !ok {
			t.Fatalf("initialize response type = %T", message)
		}
		return response
	}

	if response := initialize(1, params); response.Error != nil {
		t.Fatalf("first initialize failed: %v", response.Error)
	}
	if response := initialize(2, params); response.Error != nil {
		t.Fatalf("equivalent repeated initialize failed: %v", response.Error)
	}

	different := json.RawMessage(`{
		"protocolVersion":"2024-11-05",
		"capabilities":{},
		"clientInfo":{"name":"different-client","version":"test"}
	}`)
	if response := initialize(3, different); response.Error == nil {
		t.Fatal("different repeated initialize unexpectedly succeeded")
	}
}

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
	matched := make([]bool, len(want))
	for _, actual := range got {
		found := false
		for index, expected := range want {
			if !matched[index] && samePath(actual, expected) {
				matched[index] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func samePath(a, b string) bool {
	if security.PathsEqual(a, b) {
		return true
	}
	first, firstErr := os.Stat(a)
	second, secondErr := os.Stat(b)
	return firstErr == nil && secondErr == nil && os.SameFile(first, second)
}

func fileURIForTest(path string) string {
	uri := "file://" + filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" {
		uri = "file:///" + filepath.ToSlash(path)
	}
	return uri
}
