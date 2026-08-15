package taskstore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestTaskStartFailureMessageIncludesOnlyBoundedPublicCause(t *testing.T) {
	const internalPath = `D:\private-task-store\tasks\secret\script.snapshot.ps1`
	if message := taskStartFailureMessage(errors.New("open " + internalPath + ": access denied")); message != "task could not be prepared or started" {
		t.Fatalf("unclassified message = %q, want generic public failure", message)
	}

	publicCause := "PowerShell 7 was not found: " + strings.Repeat("é", 1024)
	message := taskStartFailureMessage(withPublicTaskStartCause(errors.New("open "+internalPath+": access denied"), publicCause))
	if !strings.Contains(message, "PowerShell 7 was not found") {
		t.Fatalf("message = %q, want concrete public start cause", message)
	}
	if strings.Contains(message, internalPath) {
		t.Fatalf("message leaked internal task-store path: %q", message)
	}
	if len(message) > 1024 {
		t.Fatalf("message length = %d, want <= 1024 bytes", len(message))
	}
	if !utf8.ValidString(message) {
		t.Fatalf("message is not valid UTF-8: %q", message)
	}
}

func TestSubmitCancellationWhileWaitingForControlLockDoesNotAdmit(t *testing.T) {
	store := newTestStore(t)
	held, err := acquireControlLock(filepath.Join(store.root, controlLockName))
	if err != nil {
		t.Fatal(err)
	}
	defer held.close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, submitErr := store.Submit(ctx, shellRequest("cancelled-control-lock-admission"))
		done <- submitErr
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case submitErr := <-done:
		if !errors.Is(submitErr, context.Canceled) {
			t.Fatalf("Submit() error = %v, want context.Canceled", submitErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Submit() did not observe cancellation while waiting for control.lock")
	}

	if err := held.close(); err != nil {
		t.Fatal(err)
	}
	held = nil
	queued, err := store.countQueuedUnlocked()
	if err != nil {
		t.Fatal(err)
	}
	if queued != 0 {
		t.Fatalf("queued tasks = %d, want 0 after cancelled admission", queued)
	}
}
