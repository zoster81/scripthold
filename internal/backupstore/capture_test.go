package backupstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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

func TestCaptureEnforcesQuotaManifestVersionAndPinLimits(t *testing.T) {
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
			name:       "versions per target",
			limits:     Limits{MaxTotalBytes: 1024, MaxObjectBytes: 64, MaxManifests: 10, MaxVersionsPerTarget: 1, MaxPinned: 10},
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
			if tc.name == "versions per target" {
				firstTarget = filepath.Join(base, "same.txt")
			}
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
			if tc.name == "versions per target" {
				secondTarget = firstTarget
			}
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
	if _, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}); err != nil {
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
	_, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("second unpinned capture error = %v, want LIMIT", err)
	}
	index := store.Index()
	if index.ManifestCount != 2 || index.PinnedCount != 1 || index.Targets[0].ManifestCount != 2 || index.Targets[0].PinnedCount != 1 {
		t.Fatalf("separate pinned/unpinned accounting = %#v", index)
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
