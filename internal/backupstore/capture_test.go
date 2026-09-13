package backupstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestCaptureCreatesObjectManifestAndIndex(t *testing.T) {
	base := canonicalTempDir(t)
	target := filepath.Join(base, "target.txt")
	content := []byte("exact backup bytes\r\n")
	if err := os.WriteFile(target, content, 0o640); err != nil {
		t.Fatal(err)
	}
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())

	result, err := store.Capture(context.Background(), CaptureRequest{
		TargetPath:      target,
		SourceOperation: SourceOperationEdit,
		Label:           "before approved edit",
		Pinned:          true,
	})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	manifest := result.Manifest
	if manifest.FormatVersion != ManifestVersion || manifest.StoreID != store.Descriptor().StoreID {
		t.Fatalf("manifest identity = %#v", manifest)
	}
	if manifest.TargetPath != filepath.Clean(target) || manifest.SourceOperation != SourceOperationEdit {
		t.Fatalf("manifest target/operation = %#v", manifest)
	}
	if manifest.ObjectBytes != int64(len(content)) || len(manifest.ObjectDigest) != 64 || len(manifest.ContentFingerprint) != 64 {
		t.Fatalf("manifest object evidence = %#v", manifest)
	}
	if manifest.ManifestChecksum == "" || len(manifest.BackupID) != 64 || !manifest.Pinned {
		t.Fatalf("manifest checksum/id/pin = %#v", manifest)
	}
	objectBytes, err := os.ReadFile(objectPath(store.Root(), manifest.ObjectDigest))
	if err != nil {
		t.Fatal(err)
	}
	if string(objectBytes) != string(content) {
		t.Fatalf("object bytes = %q, want %q", objectBytes, content)
	}
	manifestBytes, err := os.ReadFile(manifestPath(store.Root(), manifest.BackupID))
	if err != nil {
		t.Fatal(err)
	}
	var persisted Manifest
	if err := json.Unmarshal(manifestBytes, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != manifest {
		t.Fatalf("persisted manifest = %#v, want %#v", persisted, manifest)
	}
	index := store.Index()
	if index.ManifestCount != 1 || index.ObjectCount != 1 || index.TotalObjectBytes != int64(len(content)) || index.PinnedCount != 1 {
		t.Fatalf("index = %#v", index)
	}
	if _, err := os.Stat(filepath.Join(store.Root(), "index", "index-v1.json")); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureDeduplicatesIdenticalObjects(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	content := []byte("same exact bytes")
	var results []CaptureResult
	for _, name := range []string{"first.txt", "second.txt"} {
		target := filepath.Join(base, name)
		if err := os.WriteFile(target, content, 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationPatchPackage})
		if err != nil {
			t.Fatal(err)
		}
		results = append(results, result)
	}
	if !results[0].ObjectCreated || results[1].ObjectCreated {
		t.Fatalf("object creation flags = %v, %v", results[0].ObjectCreated, results[1].ObjectCreated)
	}
	if results[0].Manifest.ObjectDigest != results[1].Manifest.ObjectDigest {
		t.Fatal("identical bytes produced different object digests")
	}
	index := store.Index()
	if index.ManifestCount != 2 || index.ObjectCount != 1 || index.TotalObjectBytes != int64(len(content)) {
		t.Fatalf("deduplicated index = %#v", index)
	}
}

func TestCaptureEnforcesByteManifestAndPinLimits(t *testing.T) {
	tests := []struct {
		name        string
		limits      Limits
		first       CaptureRequest
		second      CaptureRequest
		firstBytes  []byte
		secondBytes []byte
	}{
		{
			name:       "total bytes",
			limits:     Limits{MaxTotalBytes: 5, MaxObjectBytes: 64, MaxManifests: 10, MaxVersionsPerTarget: 10, MaxPinned: 10},
			first:      CaptureRequest{SourceOperation: SourceOperationEdit},
			firstBytes: []byte("123456"),
		},
		{
			name:       "manifest count",
			limits:     Limits{MaxTotalBytes: 1024, MaxObjectBytes: 64, MaxManifests: 1, MaxVersionsPerTarget: 10, MaxPinned: 10},
			first:      CaptureRequest{SourceOperation: SourceOperationEdit},
			second:     CaptureRequest{SourceOperation: SourceOperationEdit},
			firstBytes: []byte("first"), secondBytes: []byte("second"),
		},
		{
			name:       "pinned count",
			limits:     Limits{MaxTotalBytes: 1024, MaxObjectBytes: 64, MaxManifests: 10, MaxVersionsPerTarget: 10, MaxPinned: 1},
			first:      CaptureRequest{SourceOperation: SourceOperationEdit, Pinned: true},
			second:     CaptureRequest{SourceOperation: SourceOperationEdit, Pinned: true},
			firstBytes: []byte("first"), secondBytes: []byte("second"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := canonicalTempDir(t)
			store := openBackupTestStore(t, filepath.Join(base, "store"), tc.limits)
			firstTarget := filepath.Join(base, "first.txt")
			if err := os.WriteFile(firstTarget, tc.firstBytes, 0o600); err != nil {
				t.Fatal(err)
			}
			tc.first.TargetPath = firstTarget
			_, firstErr := store.Capture(context.Background(), tc.first)
			if tc.name == "total bytes" {
				if operation.KindOf(firstErr) != operation.KindLimit {
					t.Fatalf("first capture error = %v, want LIMIT", firstErr)
				}
				if store.Index().ManifestCount != 0 {
					t.Fatal("quota failure committed a manifest")
				}
				return
			}
			if firstErr != nil {
				t.Fatalf("first capture: %v", firstErr)
			}
			secondTarget := filepath.Join(base, "second.txt")
			if err := os.WriteFile(secondTarget, tc.secondBytes, 0o600); err != nil {
				t.Fatal(err)
			}
			tc.second.TargetPath = secondTarget
			_, err := store.Capture(context.Background(), tc.second)
			if operation.KindOf(err) != operation.KindLimit {
				t.Fatalf("second capture error = %v, want LIMIT", err)
			}
			if store.Index().ManifestCount != 1 {
				t.Fatalf("limit failure changed manifest count: %#v", store.Index())
			}
		})
	}
}

func TestCaptureRotatesOldestUnpinnedVersionAtTargetLimit(t *testing.T) {
	base := canonicalTempDir(t)
	limits := backupStoreTestLimits()
	limits.MaxVersionsPerTarget = 2
	store := openBackupTestStore(t, filepath.Join(base, "store"), limits)
	target := filepath.Join(base, "target.txt")
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)

	pinned := captureGCFixture(t, store, target, "pinned", true, now.Add(-4*time.Hour))
	oldest := captureGCFixture(t, store, target, "oldest", false, now.Add(-3*time.Hour))
	recent := captureGCFixture(t, store, target, "recent", false, now.Add(-2*time.Hour))
	refreshGCFixture(t, store)

	if err := os.WriteFile(target, []byte("newest"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}
	if err := store.PreflightCaptureBatch(context.Background(), []CaptureRequest{request}); err != nil {
		t.Fatalf("preflight at target retention limit: %v", err)
	}
	newest, err := store.Capture(context.Background(), request)
	if err != nil {
		t.Fatalf("capture at target retention limit: %v", err)
	}

	index := store.Index()
	if index.ManifestCount != 3 || index.PinnedCount != 1 || len(index.Targets) != 1 ||
		index.Targets[0].ManifestCount-index.Targets[0].PinnedCount != 2 {
		t.Fatalf("rotated index = %#v", index)
	}
	for _, retained := range []string{pinned.Manifest.BackupID, recent.Manifest.BackupID, newest.Manifest.BackupID} {
		if !indexContainsManifest(index, retained) {
			t.Fatalf("retained backup %s is missing: %#v", retained, index.Manifests)
		}
	}
	if indexContainsManifest(index, oldest.Manifest.BackupID) {
		t.Fatalf("oldest unpinned backup was retained: %#v", index.Manifests)
	}
	if _, statErr := os.Stat(manifestPath(store.Root(), oldest.Manifest.BackupID)); !os.IsNotExist(statErr) {
		t.Fatalf("oldest manifest still exists: %v", statErr)
	}
	if _, statErr := os.Stat(objectPath(store.Root(), oldest.Manifest.ObjectDigest)); !os.IsNotExist(statErr) {
		t.Fatalf("unreferenced oldest object still exists: %v", statErr)
	}
	report, err := store.Audit(context.Background(), AuditOptions{Mode: AuditQuick})
	if err != nil || !report.Healthy || !report.IndexConsistent {
		t.Fatalf("retention left inconsistent derived state: report=%#v err=%v", report, err)
	}
}

func TestCaptureRetentionNeverBlocksNewBackupForActiveRestore(t *testing.T) {
	base := canonicalTempDir(t)
	limits := backupStoreTestLimits()
	limits.MaxVersionsPerTarget = 1
	store := openBackupTestStore(t, filepath.Join(base, "store"), limits)
	target := filepath.Join(base, "target.txt")

	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	old, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.OpenRestoreSource(context.Background(), old.Manifest.BackupID, RestoreSourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	if err := os.WriteFile(target, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	newest, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if err != nil {
		t.Fatalf("active restore blocked a newer backup: %v", err)
	}
	if newest.Manifest.BackupID == "" || store.Index().ManifestCount != 2 {
		t.Fatalf("active-restore retention state = %#v result=%#v", store.Index(), newest)
	}
}

func indexContainsManifest(index Index, backupID string) bool {
	for _, manifest := range index.Manifests {
		if manifest.BackupID == backupID {
			return true
		}
	}
	return false
}

func TestPinnedCaptureUsesSeparateQuotaFromUnpinnedTargetVersions(t *testing.T) {
	base := canonicalTempDir(t)
	limits := backupStoreTestLimits()
	limits.MaxVersionsPerTarget = 1
	limits.MaxPinned = 2
	store := openBackupTestStore(t, filepath.Join(base, "store"), limits)
	target := filepath.Join(base, "target.txt")
	if err := os.WriteFile(target, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("pinned"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit, Pinned: true}); err != nil {
		t.Fatalf("pinned capture was incorrectly blocked by the unpinned version limit: %v", err)
	}
	if err := os.WriteFile(target, []byte("third"), 0o600); err != nil {
		t.Fatal(err)
	}
	newest, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if err != nil {
		t.Fatalf("second unpinned capture was blocked instead of rotating history: %v", err)
	}
	index := store.Index()
	if index.ManifestCount != 2 || index.PinnedCount != 1 || index.Targets[0].ManifestCount != 2 || index.Targets[0].PinnedCount != 1 ||
		indexContainsManifest(index, first.Manifest.BackupID) || !indexContainsManifest(index, newest.Manifest.BackupID) {
		t.Fatalf("separate pinned/unpinned retention accounting = %#v", index)
	}
}

func TestCaptureRejectsCorruptExistingObjectWithoutManifest(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	content := []byte("object bytes")
	first := filepath.Join(base, "first.txt")
	if err := os.WriteFile(first, content, 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := store.Capture(context.Background(), CaptureRequest{TargetPath: first, SourceOperation: SourceOperationEdit})
	if err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("OBJECT BYTES")
	if len(corrupt) != len(content) {
		t.Fatal("test corruption must preserve size")
	}
	object := objectPath(store.Root(), created.Manifest.ObjectDigest)
	if err := os.WriteFile(object, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(object, false); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(base, "second.txt")
	if err := os.WriteFile(second, content, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = store.Capture(context.Background(), CaptureRequest{TargetPath: second, SourceOperation: SourceOperationEdit})
	if err == nil || !strings.Contains(err.Error(), "object") {
		t.Fatalf("capture error = %v, want object corruption", err)
	}
	if store.Index().ManifestCount != 1 {
		t.Fatalf("corrupt dedup committed a manifest: %#v", store.Index())
	}
}

func TestCaptureRejectsReplacedStoreRootIdentity(t *testing.T) {
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
	if err := os.WriteFile(target, []byte("root replacement"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("Capture() error = %v, want CONFLICT for replaced store root", err)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("capture wrote into replacement root: %v", entries)
	}
}

func TestCaptureRejectsReplacedInternalObjectDirectory(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	algorithmRoot := filepath.Join(store.Root(), "objects", ObjectAlgorithm)
	external := filepath.Join(base, "external-objects")
	if err := os.Mkdir(external, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(algorithmRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, algorithmRoot); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	target := filepath.Join(base, "target.txt")
	if err := os.WriteFile(target, []byte("must not escape"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if err == nil {
		t.Fatal("Capture() accepted a replaced internal object directory")
	}
	entries, readErr := os.ReadDir(external)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("capture wrote through the replaced internal directory: %v", entries)
	}
}

func TestCaptureRejectsSameContentPathReplacement(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	target := filepath.Join(base, "target.txt")
	content := []byte("same content")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	overrideAfterCaptureStage(store, func() error {
		replacement := filepath.Join(base, "replacement.txt")
		if err := os.WriteFile(replacement, content, 0o600); err != nil {
			return err
		}
		if err := os.Remove(target); err != nil {
			return err
		}
		return os.Rename(replacement, target)
	})
	_, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("replacement error = %v, want CONFLICT", err)
	}
	if store.Index().ManifestCount != 0 {
		t.Fatal("replacement conflict committed a manifest")
	}
}

func TestIndexPersistenceFailureReturnsDurableManifestAndAccurateMemoryState(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	overrideBeforeIndexPersist(store, func() error {
		return errors.New("injected index persistence failure")
	})
	target := filepath.Join(base, "target.txt")
	content := []byte("durable before index failure")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if err == nil || !strings.Contains(err.Error(), "injected index persistence failure") || result.Manifest.BackupID == "" {
		t.Fatalf("Capture() result/error = %#v / %v, want durable manifest with derived-index error", result, err)
	}
	index := store.Index()
	if index.ManifestCount != 1 || index.ObjectCount != 1 || index.TotalObjectBytes != int64(len(content)) {
		t.Fatalf("in-memory index after persistence failure = %#v", index)
	}
	if _, statErr := os.Stat(manifestPath(store.Root(), result.Manifest.BackupID)); statErr != nil {
		t.Fatalf("durable manifest missing after index failure: %v", statErr)
	}
	if _, statErr := os.Stat(objectPath(store.Root(), result.Manifest.ObjectDigest)); statErr != nil {
		t.Fatalf("durable object missing after index failure: %v", statErr)
	}
}

func TestManifestFailureAccountsForDurableOrphanObject(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	target := filepath.Join(base, "target.txt")
	content := []byte("orphan after manifest failure")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	overrideBeforeManifestCommit(store, func() error {
		return errors.New("injected manifest failure")
	})

	_, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if err == nil || !strings.Contains(err.Error(), "injected manifest failure") {
		t.Fatalf("capture error = %v, want injected manifest failure", err)
	}
	index := store.Index()
	if index.ManifestCount != 0 || index.ObjectCount != 1 || index.TotalObjectBytes != int64(len(content)) || index.Objects[0].References != 0 {
		t.Fatalf("orphan index = %#v", index)
	}
	report, auditErr := store.Audit(context.Background(), AuditOptions{Mode: AuditQuick})
	if auditErr != nil {
		t.Fatal(auditErr)
	}
	if !report.Healthy || report.OrphanObjectCount != 1 || report.OrphanObjectBytes != int64(len(content)) {
		t.Fatalf("orphan audit = %#v", report)
	}
}

func TestConcurrentReservationsPreventQuotaOvercommit(t *testing.T) {
	base := canonicalTempDir(t)
	limits := backupStoreTestLimits()
	limits.MaxTotalBytes = 8
	store := openBackupTestStore(t, filepath.Join(base, "store"), limits)
	firstTarget := filepath.Join(base, "first.txt")
	secondTarget := filepath.Join(base, "second.txt")
	if err := os.WriteFile(firstTarget, []byte("12345678"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondTarget, []byte("abcdefgh"), 0o600); err != nil {
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

	firstDone := make(chan error, 1)
	go func() {
		_, err := store.Capture(context.Background(), CaptureRequest{TargetPath: firstTarget, SourceOperation: SourceOperationEdit})
		firstDone <- err
	}()
	<-staged

	_, secondErr := store.Capture(context.Background(), CaptureRequest{TargetPath: secondTarget, SourceOperation: SourceOperationEdit})
	if operation.KindOf(secondErr) != operation.KindLimit {
		t.Fatalf("second capture error = %v, want LIMIT", secondErr)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first capture: %v", err)
	}
	if store.Index().ManifestCount != 1 || store.Index().TotalObjectBytes != 8 {
		t.Fatalf("index after reservation test = %#v", store.Index())
	}
}

func TestConcurrentCapturesKeepDerivedIndexConsistent(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	const captures = 8
	start := make(chan struct{})
	errorsCh := make(chan error, captures)
	var wait sync.WaitGroup
	for index := 0; index < captures; index++ {
		target := filepath.Join(base, fmt.Sprintf("concurrent-%02d.txt", index))
		if err := os.WriteFile(target, []byte(fmt.Sprintf("payload-%02d", index)), 0o600); err != nil {
			t.Fatal(err)
		}
		wait.Add(1)
		go func(path string) {
			defer wait.Done()
			<-start
			_, err := store.Capture(context.Background(), CaptureRequest{TargetPath: path, SourceOperation: SourceOperationEdit})
			errorsCh <- err
		}(target)
	}
	close(start)
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent capture: %v", err)
		}
	}
	index := store.Index()
	if index.ManifestCount != captures || index.ObjectCount != captures {
		t.Fatalf("concurrent index = %#v", index)
	}
	report, err := store.Audit(context.Background(), AuditOptions{Mode: AuditQuick})
	if err != nil || !report.Healthy || !report.IndexConsistent {
		t.Fatalf("concurrent captures left inconsistent derived state: report=%#v err=%v", report, err)
	}
}

func TestCaptureFallsBackWhenDerivedIndexIsInconsistent(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	first := filepath.Join(base, "first.txt")
	if err := os.WriteFile(first, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(context.Background(), CaptureRequest{TargetPath: first, SourceOperation: SourceOperationEdit}); err != nil {
		t.Fatal(err)
	}
	store.stateMu.Lock()
	store.index.ManifestCount++
	store.stateMu.Unlock()

	second := filepath.Join(base, "second.txt")
	if err := os.WriteFile(second, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(context.Background(), CaptureRequest{TargetPath: second, SourceOperation: SourceOperationEdit}); err != nil {
		t.Fatalf("capture did not recover through authoritative scan: %v", err)
	}
	report, err := store.Audit(context.Background(), AuditOptions{Mode: AuditQuick})
	if err != nil || !report.Healthy || !report.IndexConsistent || report.ManifestCount != 2 {
		t.Fatalf("fallback did not restore derived state: report=%#v err=%v", report, err)
	}
}

func TestCaptureCancellationLeavesNoCommittedState(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	target := filepath.Join(base, "target.txt")
	if err := os.WriteFile(target, []byte(strings.Repeat("x", 1024*1024)), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	overrideAfterCaptureStage(store, func() error {
		cancel()
		return nil
	})
	_, err := store.Capture(ctx, CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if !errors.Is(err, context.Canceled) && operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("cancellation error = %v", err)
	}
	if store.Index().ManifestCount != 0 {
		t.Fatal("cancelled capture committed a manifest")
	}
}

func backupStoreTestLimits() Limits {
	return Limits{
		MaxTotalBytes:        16 * 1024 * 1024,
		MaxObjectBytes:       8 * 1024 * 1024,
		MaxManifests:         128,
		MaxVersionsPerTarget: 16,
		MaxPinned:            16,
		RetentionDays:        defaultRetentionDays,
		PlanTTLSeconds:       defaultPlanTTLSeconds,
	}
}

func openBackupTestStore(t *testing.T, root string, limits Limits) *Store {
	t.Helper()
	store, err := Open(Options{Directory: root, Limits: limits})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return store
}
