package main

import (
	"context"
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
