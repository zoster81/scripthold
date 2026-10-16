package main

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The failure-only cleanup must request cancellation of the real task ID and
// observe its terminal state; it must not turn a timed-out smoke into a pass.
func TestExternalSmokeFailureCleanupCancelsAndObservesTerminalState(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "smoke-cleanup-server", Version: "test"}, nil)
	type input struct {
		TaskID string `json:"taskId"`
		Reason string `json:"reason,omitempty"`
	}
	type output struct {
		Status string `json:"status"`
	}
	var cancelCalls, statusCalls atomic.Int32
	mcp.AddTool(server, &mcp.Tool{Name: "task_cancel"}, func(_ context.Context, _ *mcp.CallToolRequest, args input) (*mcp.CallToolResult, output, error) {
		if args.TaskID != "task-test" || args.Reason != "failed external smoke test cleanup" {
			t.Errorf("unexpected cancellation arguments: %#v", args)
		}
		cancelCalls.Add(1)
		return &mcp.CallToolResult{}, output{Status: "running"}, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "task_get"}, func(_ context.Context, _ *mcp.CallToolRequest, args input) (*mcp.CallToolResult, output, error) {
		if args.TaskID != "task-test" {
			t.Errorf("unexpected status task ID: %q", args.TaskID)
		}
		statusCalls.Add(1)
		return &mcp.CallToolResult{}, output{Status: "cancelled"}, nil
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "smoke-cleanup-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	cancelExternalTaskAfterFailedWait(t, clientSession, "task-test")
	if cancelCalls.Load() != 1 || statusCalls.Load() != 1 {
		t.Fatalf("cleanup calls: cancel=%d get=%d, want 1 each", cancelCalls.Load(), statusCalls.Load())
	}
}

func TestExternalSmokeFailureCleanupReturnsAfterRejectedCancellation(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "smoke-cleanup-reject", Version: "test"}, nil)
	var cancelCalls atomic.Int32
	mcp.AddTool(server, &mcp.Tool{Name: "task_cancel"}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct {
		TaskID string `json:"taskId"`
		Reason string `json:"reason,omitempty"`
	}) (*mcp.CallToolResult, struct{}, error) {
		cancelCalls.Add(1)
		return &mcp.CallToolResult{IsError: true}, struct{}{}, nil
	})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "smoke-cleanup-reject-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	cancelExternalTaskAfterFailedWait(t, clientSession, "task-test")
	if cancelCalls.Load() != 1 {
		t.Fatalf("task_cancel called %d times, want exactly one", cancelCalls.Load())
	}
}

func TestExternalTaskStateDiagnosticIncludesOnlyBoundedMetadata(t *testing.T) {
	state := parseExternalTaskState(map[string]any{
		"status":       "running",
		"createdAt":    "2026-10-16T16:00:00Z",
		"startedAt":    "2026-10-16T16:00:01Z",
		"updatedAt":    "2026-10-16T16:00:02Z",
		"finishedAt":   "",
		"workerOnline": true,
		"command":      "private-command-must-not-appear",
	})
	got := state.diagnostic()
	for _, part := range []string{
		"status=running", "createdAt=2026-10-16T16:00:00Z",
		"startedAt=2026-10-16T16:00:01Z", "updatedAt=2026-10-16T16:00:02Z",
		"finishedAt=unknown", "workerOnline=true",
	} {
		if !strings.Contains(got, part) {
			t.Errorf("missing %q in diagnostic %q", part, got)
		}
	}
	if strings.Contains(got, "private-command-must-not-appear") {
		t.Fatal("diagnostic exposed arbitrary task metadata")
	}
}

func TestExternalTaskStateDiagnosticHandlesMalformedFields(t *testing.T) {
	for _, payload := range []any{
		nil,
		"not-a-map",
		map[string]any{"status": 7, "createdAt": "bad-time", "workerOnline": "true"},
	} {
		got := parseExternalTaskState(payload).diagnostic()
		if !strings.Contains(got, "createdAt=unknown") || !strings.Contains(got, "workerOnline=unknown") {
			t.Errorf("malformed task state should remain unknown: %q", got)
		}
	}
}
