package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

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
	return server.Run(ctx, runner.transport)
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
