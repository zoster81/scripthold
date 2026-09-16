package deferredoperation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestEngineSubmitStartsIndependentWorkAndFastWaitCompletes(t *testing.T) {
	store, public := newDeferredTestStore(t)
	started := make(chan struct{})
	engine := newEngineWithLauncher(store, func(operationID string) error {
		go func() {
			_ = store.Execute(context.Background(), operationID, func(context.Context, Request) ([]byte, ResultMetadata, error) {
				close(started)
				return []byte(`{"ok":true}`), ResultMetadata{}, nil
			})
		}()
		return nil
	})
	operation, err := engine.Submit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("executor did not start")
	}
	observed, finished, err := engine.Wait(context.Background(), operation.OperationID, []string{public}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !finished || observed.Status != StatusCompleted {
		t.Fatalf("observed=%+v finished=%v", observed, finished)
	}
}

func TestEngineSlowWaitReturnsRunningWithoutOwningExecution(t *testing.T) {
	store, public := newDeferredTestStore(t)
	release := make(chan struct{})
	started := make(chan struct{})
	engine := newEngineWithLauncher(store, func(operationID string) error {
		go func() {
			_ = store.Execute(context.Background(), operationID, func(context.Context, Request) ([]byte, ResultMetadata, error) {
				close(started)
				<-release
				return []byte(`{"ok":true}`), ResultMetadata{}, nil
			})
		}()
		select {
		case <-started:
			return nil
		case <-time.After(30 * time.Second):
			return errors.New("executor did not start")
		}
	})
	operation, err := engine.Submit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}})
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	observed, finished, err := engine.Wait(context.Background(), operation.OperationID, []string{public}, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if finished || observed.Status != StatusRunning || !observed.Started {
		close(release)
		t.Fatalf("observed=%+v finished=%v", observed, finished)
	}
	if _, err := store.MarkExposed(context.Background(), operation.OperationID, []string{public}); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	observed, finished, err = engine.Wait(context.Background(), operation.OperationID, []string{public}, time.Second)
	if err != nil || !finished || observed.Status != StatusCompleted {
		t.Fatalf("after release observed=%+v finished=%v err=%v", observed, finished, err)
	}
}

func TestEngineLaunchFailureIsTerminal(t *testing.T) {
	store, public := newDeferredTestStore(t)
	engine := newEngineWithLauncher(store, func(string) error { return errors.New("spawn failed") })
	operation, err := engine.Submit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}})
	if err == nil {
		t.Fatal("launch failure unexpectedly succeeded")
	}
	observed, getErr := store.Get(operation.OperationID, []string{public})
	if getErr != nil {
		t.Fatal(getErr)
	}
	if observed.Status != StatusFailed {
		t.Fatalf("status=%s, want %s", observed.Status, StatusFailed)
	}
}

func TestEngineRecoverRelaunchesOnlyUnstartedStaleOperation(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{
		Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}, OriginPaths: []string{public},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkStarting(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}
	baseNow := time.Now().UTC()
	store.now = func() time.Time { return baseNow.Add(30 * time.Second) }
	launches := 0
	engine := newEngineWithLauncher(store, func(operationID string) error {
		launches++
		return store.Execute(context.Background(), operationID, func(context.Context, Request) ([]byte, ResultMetadata, error) {
			return []byte(`{"ok":true}`), ResultMetadata{}, nil
		})
	})
	if err := engine.Recover(context.Background(), []string{public}); err != nil {
		t.Fatal(err)
	}
	if launches != 1 {
		t.Fatalf("recovery launches = %d, want 1", launches)
	}
	observed, err := store.Get(operation.OperationID, []string{public})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Status != StatusCompleted || !observed.Started {
		t.Fatalf("recovered operation = %+v", observed)
	}
}

func TestEngineRecoverDoesNotReplayStartedOperation(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{
		Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}, OriginPaths: []string{public},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkStarting(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}
	baseNow := time.Now().UTC()
	store.now = func() time.Time { return baseNow.Add(30 * time.Second) }
	launches := 0
	engine := newEngineWithLauncher(store, func(string) error {
		launches++
		return nil
	})
	if err := engine.Recover(context.Background(), []string{public}); err != nil {
		t.Fatal(err)
	}
	if launches != 0 {
		t.Fatalf("started operation was relaunched %d times", launches)
	}
	observed, err := store.Get(operation.OperationID, []string{public})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Status != StatusInterrupted {
		t.Fatalf("lost started operation = %+v, want interrupted", observed)
	}
}

func TestEngineRecoverKeepsStartedOperationWhenHeartbeatAdvancesPastObservationSnapshot(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{
		Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}, OriginPaths: []string{public},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkStarting(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}

	observationSnapshot := time.Now().UTC()
	heartbeatPath := filepath.Join(store.operationDir(operation.OperationID), heartbeatName)
	advancedHeartbeat := observationSnapshot.Add(time.Second)
	if err := os.Chtimes(heartbeatPath, advancedHeartbeat, advancedHeartbeat); err != nil {
		t.Fatal(err)
	}
	nowCalls := 0
	store.now = func() time.Time {
		nowCalls++
		if nowCalls == 1 {
			return observationSnapshot
		}
		return observationSnapshot.Add(2 * time.Second)
	}

	launches := 0
	engine := newEngineWithLauncher(store, func(string) error {
		launches++
		return nil
	})
	if err := engine.Recover(context.Background(), []string{public}); err != nil {
		t.Fatal(err)
	}
	if launches != 0 {
		t.Fatalf("live started operation was relaunched %d times", launches)
	}
	observed, err := store.Get(operation.OperationID, []string{public})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Status != StatusRunning || !observed.Started {
		t.Fatalf("live started operation = %+v, want running", observed)
	}
}

func TestEngineRecoverRejectsHeartbeatFarAheadOfObservationSnapshot(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{
		Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}, OriginPaths: []string{public},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkStarting(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}

	observationSnapshot := time.Now().UTC()
	heartbeatPath := filepath.Join(store.operationDir(operation.OperationID), heartbeatName)
	implausibleHeartbeat := observationSnapshot.Add(executorStaleAfter + time.Second)
	if err := os.Chtimes(heartbeatPath, implausibleHeartbeat, implausibleHeartbeat); err != nil {
		t.Fatal(err)
	}
	nowCalls := 0
	store.now = func() time.Time {
		nowCalls++
		if nowCalls == 1 {
			return observationSnapshot
		}
		return observationSnapshot.Add(2 * time.Second)
	}

	engine := newEngineWithLauncher(store, func(string) error {
		t.Fatal("started operation with implausible heartbeat must never be replayed")
		return nil
	})
	if err := engine.Recover(context.Background(), []string{public}); err != nil {
		t.Fatal(err)
	}
	observed, err := store.Get(operation.OperationID, []string{public})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Status != StatusInterrupted || observed.ErrorCode != "EXECUTOR_LOST" {
		t.Fatalf("implausible future heartbeat operation = %+v, want interrupted", observed)
	}
}

func TestEngineRecoveryLoopWaitsForRootsBeforeRecovering(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{
		Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}, OriginPaths: []string{public},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkStarting(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}
	baseNow := time.Now().UTC()
	store.now = func() time.Time { return baseNow.Add(30 * time.Second) }
	var roots atomic.Value
	roots.Store([]string(nil))
	launches := make(chan string, 1)
	engine := newEngineWithLauncher(store, func(operationID string) error {
		launches <- operationID
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time, 2)
	done := make(chan struct{})
	go func() {
		engine.runRecoveryLoop(ctx, func() []string { return roots.Load().([]string) }, ticks, nil)
		close(done)
	}()
	ticks <- time.Now()
	select {
	case operationID := <-launches:
		t.Fatalf("recovery launched %s before roots were available", operationID)
	case <-time.After(50 * time.Millisecond):
	}
	roots.Store([]string{public})
	ticks <- time.Now()
	select {
	case operationID := <-launches:
		if operationID != operation.OperationID {
			t.Fatalf("recovered operation = %s, want %s", operationID, operation.OperationID)
		}
	case <-time.After(time.Second):
		t.Fatal("recovery did not launch after roots became available")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("recovery loop did not stop with its lifecycle context")
	}
}

func TestEngineRecoverFailsClosedWhenRootPolicyChanged(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{
		Tool: "fingerprint_paths", Arguments: json.RawMessage(`{}`), AllowedDirectories: []string{public}, OriginPaths: []string{public},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkStarting(context.Background(), operation.OperationID); err != nil {
		t.Fatal(err)
	}
	baseNow := time.Now().UTC()
	store.now = func() time.Time { return baseNow.Add(30 * time.Second) }
	launches := 0
	engine := newEngineWithLauncher(store, func(string) error {
		launches++
		return nil
	})
	if err := engine.Recover(context.Background(), []string{t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if launches != 0 {
		t.Fatalf("root-revoked operation was relaunched %d times", launches)
	}
	observed, err := store.Get(operation.OperationID, []string{public})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Status != StatusFailed || observed.ErrorCode != "ACCESS_DENIED" {
		t.Fatalf("root-revoked operation = %+v", observed)
	}
}

func TestEngineRecoverClassifiesUnreadableOperationRecord(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operationID := "op_" + strings.Repeat("0", 64)
	directory := store.operationDir(operationID)
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := securePath(directory, true); err != nil {
		t.Fatal(err)
	}

	engine := newEngineWithLauncher(store, func(string) error {
		t.Fatal("unreadable operation must not be launched")
		return nil
	})
	err := engine.Recover(context.Background(), []string{public})
	if err == nil {
		t.Fatal("recovery unexpectedly accepted an incomplete operation record")
	}
	if got := RecoveryFailureReason(err); got != "record_read" {
		t.Fatalf("recovery failure reason = %q, want record_read", got)
	}
}

func TestRecoveryFailureReasonDoesNotExposeUnderlyingError(t *testing.T) {
	err := wrapRecoveryFailure("record_read", errors.New(`private store record failed at C:\sensitive\store`))
	if got := RecoveryFailureReason(err); got != "record_read" {
		t.Fatalf("recovery failure reason = %q, want record_read", got)
	}
	if strings.Contains(RecoveryFailureReason(err), "sensitive") {
		t.Fatal("recovery failure reason exposed the underlying error")
	}
}
