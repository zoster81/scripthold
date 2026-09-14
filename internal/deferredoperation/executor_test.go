package deferredoperation

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestExecuteCompletesExactlyOnce(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}, MaxRuntimeSeconds: 10})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	err = store.Execute(context.Background(), operation.OperationID, func(ctx context.Context, request Request) ([]byte, ResultMetadata, error) {
		calls.Add(1)
		return []byte(`{"ok":true}`), ResultMetadata{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("executor calls = %d, want 1", calls.Load())
	}
	observed, err := store.Get(operation.OperationID, []string{public})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Status != StatusCompleted || !observed.ResultAvailable {
		t.Fatalf("operation = %+v", observed)
	}
	if err := store.Execute(context.Background(), operation.OperationID, func(context.Context, Request) ([]byte, ResultMetadata, error) {
		calls.Add(1)
		return []byte(`{"duplicate":true}`), ResultMetadata{}, nil
	}); err == nil {
		t.Fatal("second execution unexpectedly succeeded")
	}
	if calls.Load() != 1 {
		t.Fatalf("duplicate executor calls = %d, want 1", calls.Load())
	}
}

func TestExecuteObservesCancellationAfterStart(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}, MaxRuntimeSeconds: 10})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- store.Execute(context.Background(), operation.OperationID, func(ctx context.Context, request Request) ([]byte, ResultMetadata, error) {
			close(started)
			<-ctx.Done()
			return nil, ResultMetadata{}, ctx.Err()
		})
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("executor did not start")
	}
	if _, err := store.Cancel(context.Background(), operation.OperationID, []string{public}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("execute returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("executor did not stop after cancellation")
	}
	observed, err := store.Get(operation.OperationID, []string{public})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Status != StatusCancelled {
		t.Fatalf("status = %s, want %s", observed.Status, StatusCancelled)
	}
}

func TestExecuteEnforcesOperationRuntime(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}, MaxRuntimeSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := store.Execute(context.Background(), operation.OperationID, func(ctx context.Context, request Request) ([]byte, ResultMetadata, error) {
		<-ctx.Done()
		return nil, ResultMetadata{}, ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("runtime deadline took %s", elapsed)
	}
	observed, err := store.Get(operation.OperationID, []string{public})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Status != StatusTimedOut {
		t.Fatalf("status = %s, want %s", observed.Status, StatusTimedOut)
	}
}

func TestExecuteBoundsCrossProcessConcurrency(t *testing.T) {
	limits := testDeferredLimits()
	limits.MaxConcurrency = 1
	base := canonicalDeferredTestDir(t)
	public := base
	store, err := Initialize(base+"-private", []string{public}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Admit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Admit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}})
	if err != nil {
		t.Fatal(err)
	}

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- store.Execute(context.Background(), first.OperationID, func(context.Context, Request) ([]byte, ResultMetadata, error) {
			close(firstStarted)
			<-releaseFirst
			return []byte(`{"ok":true}`), ResultMetadata{}, nil
		})
	}()
	select {
	case <-firstStarted:
	case <-time.After(30 * time.Second):
		close(releaseFirst)
		t.Fatal("first executor did not acquire the only execution slot")
	}

	var secondCalls atomic.Int32
	secondCtx, cancelSecond := context.WithTimeout(context.Background(), 300*time.Millisecond)
	err = store.Execute(secondCtx, second.OperationID, func(context.Context, Request) ([]byte, ResultMetadata, error) {
		secondCalls.Add(1)
		return []byte(`{"ok":true}`), ResultMetadata{}, nil
	})
	cancelSecond()
	if !errors.Is(err, context.DeadlineExceeded) {
		close(releaseFirst)
		<-firstDone
		t.Fatalf("second execution while slot occupied = %v, want context deadline exceeded", err)
	}
	if calls := secondCalls.Load(); calls != 0 {
		close(releaseFirst)
		<-firstDone
		t.Fatalf("second execution entered callback %d times while only slot was occupied", calls)
	}

	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first execution failed: %v", err)
	}
	if err := store.Execute(context.Background(), second.OperationID, func(context.Context, Request) ([]byte, ResultMetadata, error) {
		secondCalls.Add(1)
		return []byte(`{"ok":true}`), ResultMetadata{}, nil
	}); err != nil {
		t.Fatalf("second execution after slot release failed: %v", err)
	}
	if calls := secondCalls.Load(); calls != 1 {
		t.Fatalf("second execution callback calls = %d, want 1", calls)
	}
}
