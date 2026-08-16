package main

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/httptransport"
	"github.com/zoster81/scripthold/internal/toolcatalog"
)

func TestSelectRunnerPreservesStdioPolicy(t *testing.T) {
	selection, err := selectRunner(transportStdio, func(string) string { return "" }, 128)
	if err != nil {
		t.Fatalf("selectRunner() error = %v", err)
	}
	if !selection.enableClientRoots || selection.executionPolicy != nil || selection.disableModernDiscovery {
		t.Fatalf("stdio selection = %#v", selection)
	}
	if _, ok := selection.runner.(singleSessionRunner); !ok {
		t.Fatalf("stdio runner type = %T", selection.runner)
	}
}

func TestSelectRunnerCanForceLegacyStdioHandshake(t *testing.T) {
	values := map[string]string{envStdioLegacyHandshake: "1"}
	selection, err := selectRunner(transportStdio, func(name string) string { return values[name] }, 128)
	if err != nil {
		t.Fatalf("selectRunner() error = %v", err)
	}
	if !selection.disableModernDiscovery {
		t.Fatalf("stdio selection = %#v, want modern discovery disabled", selection)
	}

	values[envStdioLegacyHandshake] = "invalid"
	if _, err := selectRunner(transportStdio, func(name string) string { return values[name] }, 128); err == nil {
		t.Fatal("selectRunner() accepted invalid stdio legacy-handshake value")
	}
}

func TestTrimInternalArgumentSeparator(t *testing.T) {
	arguments := trimInternalArgumentSeparator([]string{"--", `C:\workspace`})
	if len(arguments) != 1 || arguments[0] != `C:\workspace` {
		t.Fatalf("trimmed arguments = %#v", arguments)
	}
	arguments = trimInternalArgumentSeparator([]string{`C:\workspace`})
	if len(arguments) != 1 || arguments[0] != `C:\workspace` {
		t.Fatalf("arguments without separator changed: %#v", arguments)
	}
}

func TestSelectRunnerHTTPClearsCredentialEnvironment(t *testing.T) {
	t.Setenv(httptransport.EnvToken, testHTTPToken())
	t.Setenv(httptransport.EnvTokenFile, "")
	t.Setenv(envStdioLegacyHandshake, "invalid-http-ignored")
	if _, err := selectRunner(transportStreamableHTTP, os.Getenv, 7); err != nil {
		t.Fatalf("selectRunner() error = %v", err)
	}
	if value := os.Getenv(httptransport.EnvToken); value != "" {
		t.Fatalf("HTTP token remained in process environment: %q", value)
	}
	if value := os.Getenv(httptransport.EnvTokenFile); value != "" {
		t.Fatalf("HTTP token file remained in process environment: %q", value)
	}
}

func TestSelectRunnerHTTPRequiresDualExecutionOptIn(t *testing.T) {
	values := map[string]string{
		httptransport.EnvToken:   testHTTPToken(),
		"MCP_ENABLE_EXECUTION":   "1",
		httptransport.EnvAddress: "127.0.0.1:8765",
		httptransport.EnvPath:    "/mcp",
	}
	getenv := func(name string) string { return values[name] }

	selection, err := selectRunner(transportStreamableHTTP, getenv, 7)
	if err != nil {
		t.Fatalf("selectRunner() error = %v", err)
	}
	if selection.enableClientRoots || selection.executionPolicy == nil {
		t.Fatalf("HTTP selection = %#v", selection)
	}
	if selection.executionPolicy.AllowRunScript || selection.executionPolicy.AllowShell {
		t.Fatalf("HTTP execution bypassed transport gate: %#v", selection.executionPolicy)
	}
	if _, ok := selection.runner.(httptransport.Runner); !ok {
		t.Fatalf("HTTP runner type = %T", selection.runner)
	}

	values[httptransport.EnvEnableExecution] = "true"
	values["MCP_ENABLE_EXECUTION"] = ""
	values["MCP_ENABLE_RUN_SCRIPT"] = "1"
	selection, err = selectRunner(transportStreamableHTTP, getenv, 7)
	if err != nil {
		t.Fatalf("selectRunner() with execution error = %v", err)
	}
	if !selection.executionPolicy.AllowRunScript || selection.executionPolicy.AllowShell {
		t.Fatalf("tool-specific dual policy = %#v", selection.executionPolicy)
	}
}

func testHTTPToken() string {
	return strings.Repeat("test-token-", 4)
}

func expectedToolCount() int {
	return len(toolcatalog.All())
}

type inFlightPeerCloseTransport struct {
	writeErr error
}

func (transport inFlightPeerCloseTransport) Connect(context.Context) (mcp.Connection, error) {
	request, err := jsonrpc.DecodeMessage([]byte(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"peer-close-test","version":"test"}}}`))
	if err != nil {
		return nil, err
	}
	writeErr := transport.writeErr
	if writeErr == nil {
		writeErr = io.EOF
	}
	return &inFlightPeerCloseConnection{
		request:      request,
		writeErr:     writeErr,
		writeAttempt: make(chan struct{}),
	}, nil
}

type inFlightPeerCloseConnection struct {
	request      jsonrpc.Message
	writeErr     error
	writeAttempt chan struct{}
	writeOnce    sync.Once
	reads        atomic.Int32
}

func (connection *inFlightPeerCloseConnection) Read(context.Context) (jsonrpc.Message, error) {
	if connection.reads.Add(1) == 1 {
		return connection.request, nil
	}
	<-connection.writeAttempt
	return nil, io.EOF
}
func (connection *inFlightPeerCloseConnection) Write(context.Context, jsonrpc.Message) error {
	connection.writeOnce.Do(func() { close(connection.writeAttempt) })
	return connection.writeErr
}
func (*inFlightPeerCloseConnection) Close() error      { return nil }
func (*inFlightPeerCloseConnection) SessionID() string { return "" }

func TestSingleSessionRunnerTreatsInFlightPeerCloseAsNormalTermination(t *testing.T) {
	runner := singleSessionRunner{transport: inFlightPeerCloseTransport{}}
	server := filetoolsserver.BuildServer(filetoolsserver.ServerOptions{
		Version:          "peer-close-test",
		LifecycleContext: context.Background(),
	})
	if err := runner.Run(context.Background(), server); err != nil {
		t.Fatalf("in-flight peer close returned runner error %v, want normal termination", err)
	}
}

type failingTransport struct {
	err error
}

func (transport failingTransport) Connect(context.Context) (mcp.Connection, error) {
	return nil, transport.err
}

func TestSingleSessionRunnerPreservesNonPeerFailure(t *testing.T) {
	want := errors.New("transport failure")
	runner := singleSessionRunner{transport: failingTransport{err: want}}
	server := filetoolsserver.BuildServer(filetoolsserver.ServerOptions{Version: "transport-failure-test"})
	if err := runner.Run(context.Background(), server); !errors.Is(err, want) {
		t.Fatalf("runner error = %v, want %v", err, want)
	}
}

func TestSingleSessionRunnerDoesNotHideTransportFailureAfterPeerClose(t *testing.T) {
	want := errors.New("response write failure")
	runner := singleSessionRunner{transport: inFlightPeerCloseTransport{writeErr: want}}
	server := filetoolsserver.BuildServer(filetoolsserver.ServerOptions{
		Version:          "peer-close-write-failure-test",
		LifecycleContext: context.Background(),
	})
	if err := runner.Run(context.Background(), server); err == nil {
		t.Fatalf("runner hid non-peer transport failure %v after observing peer EOF", want)
	}
}

func TestSingleSessionRunnerUsesSharedServerAndHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	runner := singleSessionRunner{transport: serverTransport}
	server := filetoolsserver.BuildServer(filetoolsserver.ServerOptions{
		Version:            "runner-test",
		AllowedDirectories: []string{t.TempDir()},
		Config:             config.Load(),
		EnableClientRoots:  false,
		LifecycleContext:   ctx,
	})

	errCh := make(chan error, 1)
	go func() { errCh <- runner.Run(ctx, server) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "runner-test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}

	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if got, want := len(tools.Tools), expectedToolCount(); got != want {
		t.Fatalf("tool count = %d, want %d", got, want)
	}

	cancel()
	_ = clientSession.Close()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("runner error = %v, want context cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not stop after context cancellation")
	}
}
