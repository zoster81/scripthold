package handler

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/updater"
)

// CheckUpdateInput is the input for check_for_updates.
type CheckUpdateInput struct {
	Force bool `json:"force,omitempty"`
}

// CheckUpdateOutput returns current and latest version info.
type CheckUpdateOutput struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	UpdateMessage  string `json:"updateMessage,omitempty"`
}

// NewCheckUpdateHandler returns a handler that checks for newer versions.
// Uses cached result by default (max 1 GitHub API call per 30 min).
// Set force=true to bypass cache.
func NewCheckUpdateHandler(version string) mcp.ToolHandlerFor[CheckUpdateInput, CheckUpdateOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input CheckUpdateInput) (*mcp.CallToolResult, CheckUpdateOutput, error) {
		msg := updater.Check(ctx, version, input.Force)
		latest := updater.CachedLatestVersion()
		if latest == "" {
			latest = version
		}

		return &mcp.CallToolResult{}, CheckUpdateOutput{
			CurrentVersion: version,
			LatestVersion:  latest,
			UpdateMessage:  msg,
		}, nil
	}
}

// CheckForUpdatesAsync checks for updates in the background and notifies via MCP logging.
// Called once on server initialization and cancelled with the server lifecycle.
func CheckForUpdatesAsync(parent context.Context, session *mcp.ServerSession, version string) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()

	if msg := updater.Check(ctx, version, false); msg != "" {
		//lint:ignore SA1019 Legacy sessions retain MCP logging during the R20 compatibility window.
		_ = session.Log(ctx, &mcp.LoggingMessageParams{
			Level:  "notice",
			Logger: "update-checker",
			Data:   msg,
		})
	}
}
