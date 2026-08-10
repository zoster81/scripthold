package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type testInput struct {
	Value string `json:"value"`
}

type testOutput struct {
	Result string `json:"result"`
}

func TestWithRecovery_NoPanic(t *testing.T) {
	handler := func(ctx context.Context, req *mcp.CallToolRequest, input testInput) (*mcp.CallToolResult, testOutput, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "success"}},
		}, testOutput{Result: "ok"}, nil
	}

	wrapped := WithRecovery(handler)
	result, output, err := wrapped(context.Background(), &mcp.CallToolRequest{}, testInput{Value: "test"})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Error("expected non-error result")
	}
	if output.Result != "ok" {
		t.Errorf("expected output 'ok', got %q", output.Result)
	}
}

func TestWithRecovery_Panic(t *testing.T) {
	handler := func(ctx context.Context, req *mcp.CallToolRequest, input testInput) (*mcp.CallToolResult, testOutput, error) {
		panic("test panic")
	}

	wrapped := WithRecovery(handler)
	result, _, err := wrapped(context.Background(), &mcp.CallToolRequest{}, testInput{Value: "test"})

	if err != nil {
		t.Errorf("expected no error (panic handled via result), got %v", err)
	}
	if result == nil || !result.IsError {
		t.Error("expected error result")
	}
}

func TestWithRecovery_PanicWithNilValue(t *testing.T) {
	handler := func(ctx context.Context, req *mcp.CallToolRequest, input testInput) (*mcp.CallToolResult, testOutput, error) {
		panic(nil)
	}

	wrapped := WithRecovery(handler)
	result, _, err := wrapped(context.Background(), &mcp.CallToolRequest{}, testInput{Value: "test"})

	if err != nil {
		t.Errorf("expected no error (panic handled via result), got %v", err)
	}
	if result == nil || !result.IsError {
		t.Error("expected error result")
	}
}

func TestWithLogging_Success(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	handler := func(ctx context.Context, req *mcp.CallToolRequest, input testInput) (*mcp.CallToolResult, testOutput, error) {
		return &mcp.CallToolResult{}, testOutput{Result: "ok"}, nil
	}

	wrapped := WithLogging(logger, "test_tool", handler)
	_, _, _ = wrapped(context.Background(), &mcp.CallToolRequest{}, testInput{})

	logOutput := buf.String()
	if !strings.Contains(logOutput, "tool_call_start") {
		t.Error("expected tool_call_start log")
	}
	if !strings.Contains(logOutput, "tool_call_success") {
		t.Error("expected tool_call_success log")
	}
	if !strings.Contains(logOutput, "test_tool") {
		t.Error("expected tool name in log")
	}
}

func TestWithLogging_ToolError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	handler := func(ctx context.Context, req *mcp.CallToolRequest, input testInput) (*mcp.CallToolResult, testOutput, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "something went wrong"}},
			IsError: true,
		}, testOutput{}, nil
	}

	wrapped := WithLogging(logger, "test_tool", handler)
	_, _, _ = wrapped(context.Background(), &mcp.CallToolRequest{}, testInput{})

	logOutput := buf.String()
	if !strings.Contains(logOutput, "tool_call_failed") {
		t.Error("expected tool_call_failed log")
	}
	if !strings.Contains(logOutput, "something went wrong") {
		t.Error("expected error message in log")
	}
}

func TestWithLogging_NilLogger(t *testing.T) {
	handler := func(ctx context.Context, req *mcp.CallToolRequest, input testInput) (*mcp.CallToolResult, testOutput, error) {
		return &mcp.CallToolResult{}, testOutput{Result: "ok"}, nil
	}

	// Should not panic with nil logger
	wrapped := WithLogging(nil, "test_tool", handler)
	result, output, err := wrapped(context.Background(), &mcp.CallToolRequest{}, testInput{})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if output.Result != "ok" {
		t.Errorf("expected output 'ok', got %q", output.Result)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestWrap_CombinesMiddleware(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	handler := func(ctx context.Context, req *mcp.CallToolRequest, input testInput) (*mcp.CallToolResult, testOutput, error) {
		panic("test panic in wrapped handler")
	}

	wrapped := Wrap(logger, "test_tool", handler)
	result, _, err := wrapped(context.Background(), &mcp.CallToolRequest{}, testInput{})

	// Should recover from panic
	if err != nil {
		t.Errorf("expected no error (panic handled via result), got %v", err)
	}
	if result == nil || !result.IsError {
		t.Error("expected error result")
	}

	// Logging middleware sees IsError result, logs as warning
	logOutput := buf.String()
	if !strings.Contains(logOutput, "tool_call_start") {
		t.Error("expected tool_call_start log")
	}
	if !strings.Contains(logOutput, "tool_call_failed") {
		t.Error("expected tool_call_failed log")
	}
}

func TestUnstringifyJSONArgs(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		in       string
		want     string
	}{
		{
			name:     "edits sent as JSON string",
			toolName: "edit_file",
			in:       `{"path":"a.txt","edits":"[{\"oldText\":\"x\",\"newText\":\"y\"}]"}`,
			want:     `{"edits":[{"oldText":"x","newText":"y"}],"path":"a.txt"}`,
		},
		{
			name:     "paths sent as JSON string",
			toolName: "read_multiple_files",
			in:       `{"paths":"[\"a\",\"b\"]"}`,
			want:     `{"paths":["a","b"]}`,
		},
		{
			name:     "proper array left unchanged",
			toolName: "read_multiple_files",
			in:       `{"paths":["a","b"]}`,
			want:     `{"paths":["a","b"]}`,
		},
		{
			name:     "plain string field left unchanged",
			toolName: "write_whole_file",
			in:       `{"path":"C:/dir/file.txt"}`,
			want:     `{"path":"C:/dir/file.txt"}`,
		},
		{
			name:     "invalid json returned as-is",
			toolName: "read_multiple_files",
			in:       `not json`,
			want:     `not json`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(unstringifyJSONArgs(tt.toolName, json.RawMessage(tt.in)))
			if !jsonEqual(got, tt.want) {
				t.Errorf("unstringifyJSONArgs(%s) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestStringifiedJSONArgumentFieldAllowlist(t *testing.T) {
	tests := []struct {
		toolName string
		field    string
		wrapped  string
	}{
		{toolName: "read_multiple_files", field: "paths", wrapped: `["a"]`},
		{toolName: "grep_text_files", field: "patterns", wrapped: `["x"]`},
		{toolName: "grep_text_files", field: "paths", wrapped: `["a"]`},
		{toolName: "grep_text_files", field: "includes", wrapped: `["*.go"]`},
		{toolName: "grep_text_files", field: "excludes", wrapped: `["vendor/**"]`},
		{toolName: "tree", field: "exclude", wrapped: `[".git"]`},
		{toolName: "search_files", field: "excludePatterns", wrapped: `["*.tmp"]`},
		{toolName: "fingerprint_paths", field: "paths", wrapped: `["a"]`},
		{toolName: "verify_state", field: "checks", wrapped: `[{"type":"json"}]`},
		{toolName: "edit_file", field: "edits", wrapped: `[{"oldText":"x","newText":"y"}]`},
		{toolName: "patch_package", field: "manifest", wrapped: `{"formatVersion":"patch-package-v1"}`},
		{toolName: "convert_encoding", field: "paths", wrapped: `["a"]`},
		{toolName: "task_run", field: "args", wrapped: `["--flag"]`},
		{toolName: "task_run", field: "tags", wrapped: `["build"]`},
		{toolName: "task_run", field: "lockKeys", wrapped: `["workspace"]`},
		{toolName: "task_list", field: "statuses", wrapped: `["running"]`},
		{toolName: "task_list", field: "kinds", wrapped: `["shell"]`},
		{toolName: "task_list", field: "tags", wrapped: `["build"]`},
	}

	allowedCount := 0
	for _, fields := range stringifiedJSONArgumentFields {
		allowedCount += len(fields)
	}
	if allowedCount != len(tests) {
		t.Fatalf("allowlist contains %d fields, test covers %d", allowedCount, len(tests))
	}

	for _, test := range tests {
		t.Run(test.toolName+"/"+test.field, func(t *testing.T) {
			if _, ok := stringifiedJSONArgumentFields[test.toolName][test.field]; !ok {
				t.Fatalf("missing allowlist entry for %s.%s", test.toolName, test.field)
			}
			raw, err := json.Marshal(map[string]string{test.field: test.wrapped})
			if err != nil {
				t.Fatal(err)
			}
			repaired := unstringifyJSONArgs(test.toolName, raw)
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(repaired, &fields); err != nil {
				t.Fatal(err)
			}
			if string(fields[test.field]) != test.wrapped {
				t.Fatalf("repaired %s.%s = %s, want %s", test.toolName, test.field, fields[test.field], test.wrapped)
			}
		})
	}
}

func TestRepairStringifiedArrayArgs(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    string
		want     string
	}{
		{
			name:     "repairs declared array argument",
			toolName: "read_multiple_files",
			input:    `{"paths":"[\"a\",\"b\"]"}`,
			want:     `{"paths":["a","b"]}`,
		},
		{
			name:     "preserves JSON document string content",
			toolName: "write_whole_file",
			input:    `{"path":"state.json","content":"{\"ok\":true}","encoding":"utf-8"}`,
			want:     `{"path":"state.json","content":"{\"ok\":true}","encoding":"utf-8"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Name:      test.toolName,
					Arguments: json.RawMessage(test.input),
				},
			}

			var seen json.RawMessage
			next := func(ctx context.Context, method string, r mcp.Request) (mcp.Result, error) {
				seen = r.(*mcp.CallToolRequest).Params.Arguments
				return nil, nil
			}

			if _, err := RepairStringifiedArrayArgs(next)(context.Background(), "tools/call", req); err != nil {
				t.Fatalf("middleware returned error: %v", err)
			}
			if !jsonEqual(string(seen), test.want) {
				t.Errorf("downstream saw %s, want %s", seen, test.want)
			}
		})
	}
}

// jsonEqual compares two JSON documents ignoring key order, falling back to raw
// string comparison for non-JSON inputs.
func jsonEqual(a, b string) bool {
	var va, vb any
	if json.Unmarshal([]byte(a), &va) != nil || json.Unmarshal([]byte(b), &vb) != nil {
		return a == b
	}
	na, _ := json.Marshal(va)
	nb, _ := json.Marshal(vb)
	return string(na) == string(nb)
}
