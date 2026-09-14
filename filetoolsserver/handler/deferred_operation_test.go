package handler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/deferredoperation"
	"github.com/zoster81/scripthold/internal/responsecontinuation"
)

func TestHandleDeferredOperationReadsBoundedChunk(t *testing.T) {
	store := responsecontinuation.NewStore(responsecontinuation.Limits{MaxEntries: 2, MaxTotalBytes: 1024, MaxChunkBytes: 5, Retention: time.Hour}, time.Now)
	handle, err := store.Retain([]byte("abcdefghij"), responsecontinuation.Metadata{OriginalIsError: true, ErrorCode: ErrCodePartialCommit})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{t.TempDir()}, WithResponseContinuationStore(store))
	result, output, err := h.HandleDeferredOperation(context.Background(), nil, DeferredOperationInput{Operation: "get", OperationID: handle.OperationID})
	if err != nil || result.IsError {
		t.Fatalf("get result=%+v err=%v", result, err)
	}
	if output.Data != "abcde" || output.Offset != 0 || output.NextOffset != 5 || output.Complete || !output.OriginalIsError || output.ErrorCode != ErrCodePartialCommit {
		t.Fatalf("unexpected output: %+v", output)
	}
}

func TestHandleDeferredOperationReadsDurableOperationAcrossHandlerInstances(t *testing.T) {
	public := canonicalHandlerTestDir(t)
	store, err := deferredoperation.Initialize(filepath.Join(canonicalHandlerTestDir(t), "deferred"), []string{public}, nil, deferredoperation.Limits{
		MaxConcurrency: 2, MaxQueued: 4, MaxRuntimeSeconds: 30, RetentionSeconds: 3600,
		MaxTerminal: 8, MaxTotalBytes: 8 * 1024 * 1024, MaxResultBytes: 2 * 1024 * 1024, MaxChunkBytes: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := store.Admit(context.Background(), deferredoperation.Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}
	first := NewHandler([]string{public}, WithDeferredOperationStore(store))
	result, running, err := first.HandleDeferredOperation(context.Background(), nil, DeferredOperationInput{Operation: "get", OperationID: operation.OperationID})
	if err != nil || result.IsError || running.Status != string(deferredoperation.StatusRunning) || running.Complete {
		t.Fatalf("running result=%+v output=%+v err=%v", result, running, err)
	}
	payload := []byte(`{"structuredContent":{"ok":true},"isError":false}`)
	if _, err := store.Complete(operation.OperationID, payload, deferredoperation.ResultMetadata{}); err != nil {
		t.Fatal(err)
	}
	second := NewHandler([]string{public}, WithDeferredOperationStore(store))
	result, completed, err := second.HandleDeferredOperation(context.Background(), nil, DeferredOperationInput{Operation: "get", OperationID: operation.OperationID})
	if err != nil || result.IsError || completed.Status != string(deferredoperation.StatusCompleted) || completed.Data == "" {
		t.Fatalf("completed result=%+v output=%+v err=%v", result, completed, err)
	}
}

func TestHandleDeferredOperationCancelsDurableOperation(t *testing.T) {
	public := canonicalHandlerTestDir(t)
	store, err := deferredoperation.Initialize(filepath.Join(canonicalHandlerTestDir(t), "deferred"), []string{public}, nil, deferredoperation.Limits{
		MaxConcurrency: 1, MaxQueued: 2, MaxRuntimeSeconds: 30, RetentionSeconds: 3600,
		MaxTerminal: 4, MaxTotalBytes: 1024 * 1024, MaxResultBytes: 512 * 1024, MaxChunkBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := store.Admit(context.Background(), deferredoperation.Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{public}, WithDeferredOperationStore(store))
	result, output, err := h.HandleDeferredOperation(context.Background(), nil, DeferredOperationInput{Operation: "cancel", OperationID: operation.OperationID})
	if err != nil || result.IsError || output.Status != string(deferredoperation.StatusCancelled) || !output.Complete {
		t.Fatalf("cancel result=%+v output=%+v err=%v", result, output, err)
	}
}

func TestHandleDeferredOperationRejectsInvalidRequests(t *testing.T) {
	store := responsecontinuation.NewStore(responsecontinuation.Limits{MaxEntries: 2, MaxTotalBytes: 1024, MaxChunkBytes: 5, Retention: time.Hour}, time.Now)
	h := NewHandler([]string{t.TempDir()}, WithResponseContinuationStore(store))
	for _, input := range []DeferredOperationInput{
		{Operation: "cancel", OperationID: "op_0000000000000000000000000000000000000000000000000000000000000000"},
		{Operation: "list", OperationID: "op_0000000000000000000000000000000000000000000000000000000000000000"},
		{Operation: "get", OperationID: "bad"},
	} {
		result, _, err := h.HandleDeferredOperation(context.Background(), nil, input)
		if err != nil || result == nil || !result.IsError {
			t.Fatalf("input=%+v result=%+v err=%v", input, result, err)
		}
	}
}
