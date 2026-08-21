package filetoolsserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/responsecontinuation"
)

func TestWithSafeToolResponsePreservesSmallResponse(t *testing.T) {
	store := responsecontinuation.NewStore(responsecontinuation.Limits{MaxEntries: 4, MaxTotalBytes: 1 << 20, MaxChunkBytes: 1024, Retention: time.Hour}, time.Now)
	wrapped := withSafeToolResponse(16*1024, store, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, map[string]any{"message": "small"}, nil
	})
	result, output, err := wrapped(context.Background(), &mcp.CallToolRequest{}, struct{}{})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("small response result=%+v err=%v", result, err)
	}
	object, ok := output.(map[string]any)
	if !ok || object["message"] != "small" {
		t.Fatalf("small output=%#v", output)
	}
}

func TestWithSafeToolResponseSegmentsOversizedResponse(t *testing.T) {
	store := responsecontinuation.NewStore(responsecontinuation.Limits{MaxEntries: 4, MaxTotalBytes: 1 << 20, MaxChunkBytes: 512, Retention: time.Hour}, time.Now)
	original := map[string]any{"content": strings.Repeat("x", 8192), "count": 7}
	wrapped := withSafeToolResponse(1024, store, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, original, nil
	})
	result, output, err := wrapped(context.Background(), &mcp.CallToolRequest{}, struct{}{})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("segmented response result=%+v err=%v", result, err)
	}
	handoff, ok := output.(responseContinuationOutput)
	if !ok || !handoff.Deferred || handoff.OperationID == "" || handoff.Status != responsecontinuation.StatusCompleted || handoff.OriginalIsError {
		t.Fatalf("handoff=%#v", output)
	}

	var payload []byte
	offset := 0
	for {
		chunk, getErr := store.Get(handoff.OperationID, offset, 0)
		if getErr != nil {
			t.Fatal(getErr)
		}
		payload = append(payload, []byte(chunk.Data)...)
		offset = chunk.NextOffset
		if chunk.Complete {
			break
		}
	}
	var retained retainedToolResponse
	if err := json.Unmarshal(payload, &retained); err != nil {
		t.Fatalf("decode retained response: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(retained.StructuredContent, &got); err != nil {
		t.Fatal(err)
	}
	if got["content"] != original["content"] || got["count"] != float64(7) {
		t.Fatalf("retained structured output=%#v", got)
	}
}

func TestWithSafeToolResponsePreservesOriginalErrorClassification(t *testing.T) {
	store := responsecontinuation.NewStore(responsecontinuation.Limits{MaxEntries: 4, MaxTotalBytes: 1 << 20, MaxChunkBytes: 512, Retention: time.Hour}, time.Now)
	wrapped := withSafeToolResponse(512, store, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{IsError: true, Meta: mcp.Meta{handler.ErrorCodeMetaKey: handler.ErrCodePartialCommit}}, map[string]any{"details": strings.Repeat("y", 4096)}, nil
	})
	result, output, err := wrapped(context.Background(), &mcp.CallToolRequest{}, struct{}{})
	if err != nil || result == nil || !result.IsError || result.Meta[handler.ErrorCodeMetaKey] != handler.ErrCodePartialCommit {
		t.Fatalf("segmented error result=%+v err=%v", result, err)
	}
	handoff, ok := output.(responseContinuationOutput)
	if !ok || !handoff.OriginalIsError || handoff.ErrorCode != handler.ErrCodePartialCommit {
		t.Fatalf("error handoff=%#v", output)
	}
}

func TestWithSafeToolResponseFailsClosedWhenRetentionCannotStoreResult(t *testing.T) {
	store := responsecontinuation.NewStore(responsecontinuation.Limits{MaxEntries: 1, MaxTotalBytes: 32, MaxChunkBytes: 16, Retention: time.Hour}, time.Now)
	wrapped := withSafeToolResponse(64, store, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, map[string]any{"details": strings.Repeat("z", 4096)}, nil
	})
	result, output, err := wrapped(context.Background(), &mcp.CallToolRequest{}, struct{}{})
	if err != nil || result == nil || !result.IsError || result.Meta[handler.ErrorCodeMetaKey] != handler.ErrCodeLimit {
		t.Fatalf("retention failure result=%+v output=%#v err=%v", result, output, err)
	}
	failure, ok := output.(responseContinuationOutput)
	if !ok || !failure.OperationCompleted || failure.ResultRetained {
		t.Fatalf("retention failure output=%#v", output)
	}
}
