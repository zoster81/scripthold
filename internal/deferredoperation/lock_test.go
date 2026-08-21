package deferredoperation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreGetContextCancelsContendedObservation(t *testing.T) {
	store, public := newDeferredTestStore(t)
	operation, err := store.Admit(context.Background(), Request{Tool: "fingerprint_paths", Arguments: []byte(`{}`), AllowedDirectories: []string{public}})
	if err != nil {
		t.Fatal(err)
	}
	lock, err := acquireStoreLock(context.Background(), filepath.Join(store.root, controlName), true)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = store.GetContext(ctx, operation.OperationID, []string{public})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contended GetContext error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("contended GetContext ignored cancellation for %s", elapsed)
	}
}

func TestStoreLockWaitHonorsContext(t *testing.T) {
	store, _ := newDeferredTestStore(t)
	path := filepath.Join(store.root, controlName)
	first, err := acquireStoreLock(context.Background(), path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer first.close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	second, err := acquireStoreLock(ctx, path, true)
	if second != nil {
		_ = second.close()
		t.Fatal("contended store lock unexpectedly succeeded")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contended store lock error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("contended store lock ignored cancellation for %s", elapsed)
	}
}
