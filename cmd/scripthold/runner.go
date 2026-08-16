package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/httptransport"
)

const envStdioLegacyHandshake = "MCP_STDIO_LEGACY_HANDSHAKE"

type serverRunner interface {
	Run(context.Context, *mcp.Server) error
}

type singleSessionRunner struct {
	transport mcp.Transport
}

func (runner singleSessionRunner) Run(ctx context.Context, server *mcp.Server) error {
	if runner.transport == nil {
		return fmt.Errorf("transport is required")
	}
	var peerClosed, transportFailed atomic.Bool
	err := server.Run(ctx, peerCloseObservingTransport{
		delegate:        runner.transport,
		peerClosed:      &peerClosed,
		transportFailed: &transportFailed,
	})
	if peerClosed.Load() && !transportFailed.Load() {
		return nil
	}
	return err
}

type peerCloseObservingTransport struct {
	delegate        mcp.Transport
	peerClosed      *atomic.Bool
	transportFailed *atomic.Bool
}

func (transport peerCloseObservingTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	connection, err := transport.delegate.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &peerCloseObservingConnection{
		Connection:      connection,
		peerClosed:      transport.peerClosed,
		transportFailed: transport.transportFailed,
	}, nil
}

type peerCloseObservingConnection struct {
	mcp.Connection
	peerClosed      *atomic.Bool
	transportFailed *atomic.Bool
}

func (connection *peerCloseObservingConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	message, err := connection.Connection.Read(ctx)
	connection.observe(err)
	return message, err
}

func (connection *peerCloseObservingConnection) Write(ctx context.Context, message jsonrpc.Message) error {
	err := connection.Connection.Write(ctx, message)
	connection.observe(err)
	return err
}

func (connection *peerCloseObservingConnection) observe(err error) {
	if err == nil {
		return
	}
	if isPeerCloseError(err) {
		if connection.peerClosed != nil {
			connection.peerClosed.Store(true)
		}
		return
	}
	if connection.transportFailed != nil {
		connection.transportFailed.Store(true)
	}
}

func isPeerCloseError(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe)
}

type runnerSelection struct {
	runner                 serverRunner
	enableClientRoots      bool
	disableModernDiscovery bool
	executionPolicy        *handler.ExecutionPolicy
}

func selectRunner(
	transport transportName,
	getenv func(string) string,
	maxSessions int,
) (runnerSelection, error) {
	switch transport {
	case transportStdio:
		disableModernDiscovery, err := parseStdioLegacyHandshake(getenv(envStdioLegacyHandshake))
		if err != nil {
			return runnerSelection{}, fmt.Errorf("invalid %s: %w", envStdioLegacyHandshake, err)
		}
		return runnerSelection{
			runner:                 singleSessionRunner{transport: &mcp.StdioTransport{}},
			enableClientRoots:      true,
			disableModernDiscovery: disableModernDiscovery,
		}, nil
	case transportStreamableHTTP:
		httpConfig, err := httptransport.LoadConfig(getenv, maxSessions)
		if err != nil {
			return runnerSelection{}, err
		}
		httptransport.ClearCredentialEnvironment()
		basePolicy := handler.ExecutionPolicyFromEnvironment(getenv)
		executionPolicy := &handler.ExecutionPolicy{
			AllowRunScript: httpConfig.EnableExecution && basePolicy.AllowRunScript,
			AllowShell:     httpConfig.EnableExecution && basePolicy.AllowShell,
		}
		return runnerSelection{
			runner: httptransport.Runner{
				Config: httpConfig,
				Logger: slog.Default(),
			},
			enableClientRoots: false,
			executionPolicy:   executionPolicy,
		}, nil
	default:
		return runnerSelection{}, fmt.Errorf("unsupported transport %q", transport)
	}
}

func parseStdioLegacyHandshake(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return false, nil
	case "1", "true", "yes", "on", "enabled":
		return true, nil
	case "0", "false", "no", "off", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("expected boolean value")
	}
}
