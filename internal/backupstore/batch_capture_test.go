package backupstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestPreflightCaptureBatchIsConservativeAndSideEffectFree(t *testing.T) {
	base := canonicalTempDir(t)
	limits := backupStoreTestLimits()
	limits.MaxTotalBytes = 8
	store := openBackupTestStore(t, filepath.Join(base, "store"), limits)
	content := []byte("12345678")
	requests := make([]CaptureRequest, 2)
	for index, name := range []string{"first.txt", "second.txt"} {
		path := filepath.Join(base, name)
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
		requests[index] = CaptureRequest{TargetPath: path, SourceOperation: SourceOperationPatchPackage}
	}

	err := store.PreflightCaptureBatch(context.Background(), requests)
	if operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("PreflightCaptureBatch() error = %v, want LIMIT", err)
	}
	if index := store.Index(); index.ManifestCount != 0 || index.ObjectCount != 0 || index.TotalObjectBytes != 0 {
		t.Fatalf("preflight changed store state: %#v", index)
	}

	result, err := store.Capture(context.Background(), requests[0])
	if err != nil || result.Manifest.BackupID == "" {
		t.Fatalf("single capture after preflight = %#v / %v", result, err)
	}
}

func TestCaptureBatchCommitsOrderedManifests(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	requests := make([]CaptureRequest, 2)
	contents := [][]byte{[]byte("alpha"), []byte("beta")}
	for index, name := range []string{"first.txt", "second.txt"} {
		path := filepath.Join(base, name)
		if err := os.WriteFile(path, contents[index], 0o600); err != nil {
			t.Fatal(err)
		}
		requests[index] = CaptureRequest{
			TargetPath:      path,
			SourceOperation: SourceOperationPatchPackage,
			Label:           "approved package",
		}
	}

	results, err := store.CaptureBatch(context.Background(), requests)
	if err != nil {
		t.Fatalf("CaptureBatch() error = %v", err)
	}
	if len(results) != len(requests) {
		t.Fatalf("result count = %d, want %d", len(results), len(requests))
	}
	for index, result := range results {
		manifest := result.Manifest
		if manifest.TargetPath != requests[index].TargetPath || manifest.SourceOperation != SourceOperationPatchPackage || manifest.Label != "approved package" {
			t.Fatalf("result %d manifest = %#v", index, manifest)
		}
		if manifest.BackupID == "" {
			t.Fatalf("result %d has no backup ID", index)
		}
	}
	index := store.Index()
	if index.ManifestCount != 2 || index.ObjectCount != 2 || index.TotalObjectBytes != int64(len(contents[0])+len(contents[1])) {
		t.Fatalf("batch index = %#v", index)
	}
}

func TestCaptureBatchReturnsDurablePrefixAndReleasesRemainingReservations(t *testing.T) {
	base := canonicalTempDir(t)
	limits := backupStoreTestLimits()
	limits.MaxManifests = 3
	store := openBackupTestStore(t, filepath.Join(base, "store"), limits)
	requests := make([]CaptureRequest, 2)
	for index, name := range []string{"first.txt", "second.txt"} {
		path := filepath.Join(base, name)
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		requests[index] = CaptureRequest{TargetPath: path, SourceOperation: SourceOperationPatchPackage}
	}
	var commits atomic.Int32
	restoreCommit := overrideBeforeManifestCommit(store, func() error {
		if commits.Add(1) == 2 {
			return errors.New("injected second manifest failure")
		}
		return nil
	})

	results, err := store.CaptureBatch(context.Background(), requests)
	if err == nil || len(results) != 1 || results[0].Manifest.BackupID == "" {
		t.Fatalf("CaptureBatch() results/error = %#v / %v", results, err)
	}
	restoreCommit()
	third := filepath.Join(base, "third.txt")
	if err := os.WriteFile(third, []byte("third"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, captureErr := store.Capture(context.Background(), CaptureRequest{TargetPath: third, SourceOperation: SourceOperationPatchPackage})
	if captureErr != nil || result.Manifest.BackupID == "" {
		t.Fatalf("capture after batch failure = %#v / %v", result, captureErr)
	}
	if store.Index().ManifestCount != 2 {
		t.Fatalf("manifest count = %d, want 2", store.Index().ManifestCount)
	}
}

func TestCaptureBatchReservationBlocksConcurrentOvercommit(t *testing.T) {
	base := canonicalTempDir(t)
	limits := backupStoreTestLimits()
	limits.MaxTotalBytes = 8
	store := openBackupTestStore(t, filepath.Join(base, "store"), limits)
	requests := make([]CaptureRequest, 2)
	for index, name := range []string{"first.txt", "second.txt"} {
		path := filepath.Join(base, name)
		if err := os.WriteFile(path, []byte("1234"), 0o600); err != nil {
			t.Fatal(err)
		}
		requests[index] = CaptureRequest{TargetPath: path, SourceOperation: SourceOperationPatchPackage}
	}
	outside := filepath.Join(base, "outside.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	staged := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	overrideAfterCaptureStage(store, func() error {
		once.Do(func() {
			close(staged)
			<-release
		})
		return nil
	})
	batchDone := make(chan error, 1)
	go func() {
		_, err := store.CaptureBatch(context.Background(), requests)
		batchDone <- err
	}()
	<-staged

	_, concurrentErr := store.Capture(context.Background(), CaptureRequest{TargetPath: outside, SourceOperation: SourceOperationEdit})
	if operation.KindOf(concurrentErr) != operation.KindLimit {
		t.Fatalf("concurrent capture error = %v, want LIMIT", concurrentErr)
	}
	close(release)
	if err := <-batchDone; err != nil {
		t.Fatalf("CaptureBatch() error = %v", err)
	}
	if store.Index().ManifestCount != 2 || store.Index().ObjectCount != 1 || store.Index().TotalObjectBytes != 4 {
		t.Fatalf("batch index = %#v", store.Index())
	}
}

func TestPreflightCaptureBatchRejectsReplacedStoreRootIdentity(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "store")
	store := openBackupTestStore(t, root, backupStoreTestLimits())
	moved := filepath.Join(base, "store-moved")
	if err := os.Rename(root, moved); err != nil {
		t.Skipf("open store root cannot be renamed on this filesystem: %v", err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(root, true); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "target.txt")
	if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := store.PreflightCaptureBatch(context.Background(), []CaptureRequest{{TargetPath: target, SourceOperation: SourceOperationPatchPackage}})
	if operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("PreflightCaptureBatch() error = %v, want CONFLICT", err)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("preflight wrote into replacement root: %v", entries)
	}
}

func TestCaptureBatchRejectsDuplicateTargets(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	target := filepath.Join(base, "target.txt")
	if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := CaptureRequest{TargetPath: target, SourceOperation: SourceOperationPatchPackage}
	_, err := store.CaptureBatch(context.Background(), []CaptureRequest{request, request})
	if operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("duplicate batch error = %v, want INVALID_INPUT", err)
	}
	if store.Index().ManifestCount != 0 {
		t.Fatal("duplicate batch committed a manifest")
	}
}
