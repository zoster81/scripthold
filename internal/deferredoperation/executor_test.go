package deferredoperation

import (
	"context"
	"encoding/json"
	"sync"
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
	base := t.TempDir()
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
	var active atomic.Int32
	var peak atomic.Int32
	release := make(chan struct{})
	var wg sync.WaitGroup
	for _, id := range []string{first.OperationID, second.OperationID} {
		wg.Add(1)
		go func(operationID string) {
			defer wg.Done()
			_ = store.Execute(context.Background(), operationID, func(ctx context.Context, request Request) ([]byte, ResultMetadata, error) {
				current := active.Add(1)
				for {
					old := peak.Load()
					if current <= old || peak.CompareAndSwap(old, current) {
						break
					}
				}
				<-release
				active.Add(-1)
				return []byte(`{"ok":true}`), ResultMetadata{}, nil
			})
		}(id)
	}
	time.Sleep(250 * time.Millisecond)
	if got := peak.Load(); got != 1 {
		close(release)
		wg.Wait()
		t.Fatalf("peak concurrency = %d, want 1", got)
	}
	close(release)
	wg.Wait()
}
