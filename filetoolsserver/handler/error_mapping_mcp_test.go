package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestStructuredErrorOutputHandlesNilTypedOutput(t *testing.T) {
	type output struct {
		State string `json:"state"`
	}
	wrapped := WithStructuredErrorOutput(func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, *output, error) {
		return errorResultWithCode(ErrCodeIO, "write failed"), nil, nil
	})
	result, structured, err := wrapped(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("result = %#v, want MCP error", result)
	}
	values, ok := structured.(map[string]any)
	if !ok {
		t.Fatalf("structured output type = %T, want map[string]any", structured)
	}
	if values["errorCode"] != ErrCodeIO || values["message"] != "write failed" {
		t.Fatalf("structured output = %#v", values)
	}
}

func TestMCPErrorResultPreservesStructuredDiagnostics(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "error-envelope-test", Version: "test"}, nil)
	type output struct {
		State string `json:"state"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "failing_tool"}, WithStructuredErrorOutput(func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, output, error) {
		return errorResultWithCode(ErrCodeConflict, "stale preview"), output{State: "unchanged"}, nil
	}))
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "error-envelope-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "failing_tool", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("result = %#v, want MCP error", result)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		ErrorCode string `json:"errorCode"`
		Message   string `json:"message"`
		State     string `json:"state"`
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.ErrorCode != ErrCodeConflict || envelope.Message != "stale preview" || envelope.State != "unchanged" {
		t.Fatalf("StructuredContent = %s, want errorCode=%q message=%q state=unchanged", encoded, ErrCodeConflict, "stale preview")
	}
}
