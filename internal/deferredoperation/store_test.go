package deferredoperation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testDeferredLimits() Limits {
	return Limits{
		MaxConcurrency:    2,
		MaxQueued:         4,
		MaxRuntimeSeconds: 30,
		RetentionSeconds:  3600,
		MaxTerminal:       8,
		MaxTotalBytes:     8 * 1024 * 1024,
		MaxResultBytes:    2 * 1024 * 1024,
		MaxChunkBytes:     64 * 1024,
	}
}

func newDeferredTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	base := t.TempDir()
	resolved, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	public := filepath.Join(resolved, "public")
	if err := os.Mkdir(public, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := Initialize(filepath.Join(resolved, "deferred"), []string{public}, nil, testDeferredLimits())
	if err != nil {
		t.Fatal(err)
	}
	return store, public
}

func TestStoreRejectsPublicOverlap(t *testing.T) {
	base := t.TempDir()
	resolved, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	public := filepath.Join(resolved, "public")
	if err := os.Mkdir(public, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(filepath.Join(public, "deferred"), []string{public}, nil, testDeferredLimits()); err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("overlap error = %v", err)
	}
}

func TestStoreAdmissionLifecycleAndChunkRoundTrip(t *testing.T) {
	store, public := newDeferredTestStore(t)
	origin := filepath.Join(public, "source.txt")
	if err := os.WriteFile(origin, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	arguments := json.RawMessage(`{"paths":["source.txt"]}`)
	operation, err := store.Admit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: arguments, AllowedDirectories: []string{public}, OriginPaths: []string{origin}, MaxRuntimeSeconds: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !ValidOperationID(operation.OperationID) || operation.Status != StatusQueued || operation.Revision != 1 {
		t.Fatalf("admitted operation = %+v", operation)
	}

	claimed, err := store.Claim(context.Background(), operation.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Request.Tool != "fingerprint_paths" || claimed.Operation.Status != StatusRunning || !claimed.Operation.Started {
		t.Fatalf("claim = %+v", claimed)
	}

	payload := []byte(`{"structuredContent":{"message":"héllo"},"isError":false}`)
	completed, err := store.Complete(operation.OperationID, payload, ResultMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != StatusCompleted || !completed.ResultAvailable || completed.TotalBytes != int64(len(payload)) {
		t.Fatalf("completed operation = %+v", completed)
	}

	var reconstructed []byte
	offset := int64(0)
	for {
		chunk, err := store.ResultChunk(operation.OperationID, []string{public}, offset, 11)
		if err != nil {
			t.Fatal(err)
		}
		reconstructed = append(reconstructed, []byte(chunk.Data)...)
		if chunk.Complete {
			break
		}
		if chunk.NextOffset <= offset {
			t.Fatalf("chunk did not advance: %+v", chunk)
		}
		offset = chunk.NextOffset
	}
	if string(reconstructed) != string(payload) {
		t.Fatalf("reconstructed payload = %q, want %q", reconstructed, payload)
	}
}

func TestStoreGetDoesNotExposeStartedBeforeRunningState(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkStarting(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}
	if err := writeMarkerExclusive(filepath.Join(store.operationDir(operation.OperationID), startedName)); err != nil {
		t.Fatal(err)
	}

	observed, err := store.Get(operation.OperationID, []string{public})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Status != StatusStarting || observed.Started || observed.StartedAt != nil {
		t.Fatalf("in-progress claim became externally started: %+v", observed)
	}
}

func TestStoreGetWaitsForClaimTransitionPublication(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkStarting(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}

	lock, err := acquireStoreLock(context.Background(), filepath.Join(store.root, controlName), true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if lock != nil {
			_ = lock.close()
		}
	}()
	if err := writeMarkerExclusive(filepath.Join(store.operationDir(operation.OperationID), startedName)); err != nil {
		t.Fatal(err)
	}

	type getResult struct {
		operation Operation
		err       error
	}
	resultCh := make(chan getResult, 1)
	go func() {
		observed, getErr := store.Get(operation.OperationID, []string{public})
		resultCh <- getResult{operation: observed, err: getErr}
	}()

	select {
	case result := <-resultCh:
		t.Fatalf("Get observed a state transition before publication completed: operation=%+v err=%v", result.operation, result.err)
	case <-time.After(100 * time.Millisecond):
	}

	current, err := store.latestState(operation.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	now := store.now().UTC()
	started := now
	running := stateRecord{Status: StatusRunning, Revision: current.Revision + 1, UpdatedAt: now, StartedAt: &started}
	if err := store.writeStateExclusive(operation.OperationID, running); err != nil {
		t.Fatal(err)
	}
	if err := touch(filepath.Join(store.operationDir(operation.OperationID), heartbeatName)); err != nil {
		t.Fatal(err)
	}
	if err := lock.close(); err != nil {
		t.Fatal(err)
	}
	lock = nil

	select {
	case result := <-resultCh:
		if result.err != nil || result.operation.Status != StatusRunning || !result.operation.Started || result.operation.StartedAt == nil {
			t.Fatalf("coherent Get result = %+v err=%v", result.operation, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Get did not resume after state publication completed")
	}
}

func TestStoreContextResultReadersAcceptNilContext(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"ok":true}`)
	if _, err := store.Complete(operation.OperationID, payload, ResultMetadata{}); err != nil {
		t.Fatal(err)
	}
	var nilContext context.Context
	read, _, err := store.ReadResultContext(nilContext, operation.OperationID, []string{public})
	if err != nil || string(read) != string(payload) {
		t.Fatalf("nil-context read result = %q err=%v", read, err)
	}
	chunk, err := store.ResultChunkContext(nilContext, operation.OperationID, []string{public}, 0, 0)
	if err != nil || !chunk.Complete || chunk.Data != string(payload) {
		t.Fatalf("nil-context result chunk = %+v err=%v", chunk, err)
	}
}

func TestStoreResultVisibilityRevalidatesCurrentRoots(t *testing.T) {
	store, public := newDeferredTestStore(t)
	origin := filepath.Join(public, "source.txt")
	if err := os.WriteFile(origin, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	operation, err := store.Admit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}, OriginPaths: []string{origin}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Complete(operation.OperationID, []byte(`{"ok":true}`), ResultMetadata{}); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(public), "other")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResultChunk(operation.OperationID, []string{other}, 0, 1024); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("visibility error = %v, want %v", err, ErrAccessDenied)
	}
}

func TestStoreStartedMarkerPreventsReplay(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), operation.OperationID); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("second claim error = %v, want %v", err, ErrAlreadyStarted)
	}
}

func TestStoreCancelBeforeStartIsTerminal(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.Cancel(context.Background(), operation.OperationID, []string{public})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != StatusCancelled || !cancelled.Status.Terminal() {
		t.Fatalf("cancelled operation = %+v", cancelled)
	}
	if _, err := store.Claim(context.Background(), operation.OperationID); !errors.Is(err, ErrTerminal) {
		t.Fatalf("claim after cancel error = %v, want %v", err, ErrTerminal)
	}
}

func TestStoreMarksLostStartedExecutorInterruptedWithoutReplay(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}
	heartbeat, err := os.Stat(filepath.Join(store.operationDir(operation.OperationID), heartbeatName))
	if err != nil {
		t.Fatal(err)
	}
	now := heartbeat.ModTime().UTC().Add(executorStaleAfter + time.Second)
	store.now = func() time.Time { return now }
	observed, err := store.Get(operation.OperationID, []string{public})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Status != StatusInterrupted {
		t.Fatalf("lost executor status = %s, want %s", observed.Status, StatusInterrupted)
	}
	if _, err := store.Claim(context.Background(), operation.OperationID); !errors.Is(err, ErrTerminal) {
		t.Fatalf("replay claim error = %v, want terminal", err)
	}
}
