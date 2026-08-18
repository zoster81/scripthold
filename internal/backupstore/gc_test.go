package backupstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

func TestPlanGCDeterministicRetentionFloorPinsAndReferenceCounts(t *testing.T) {
	base := canonicalTempDir(t)
	limits := backupStoreTestLimits()
	limits.RetentionDays = 30
	limits.MaxVersionsPerTarget = 8
	store := openBackupTestStore(t, filepath.Join(base, "store"), limits)
	now := time.Date(2026, 8, 5, 7, 30, 0, 0, time.UTC)
	target := filepath.Join(base, "target.txt")
	otherTarget := filepath.Join(base, "other.txt")

	oldest := captureGCFixture(t, store, target, "shared old bytes", false, now.AddDate(0, 0, -50))
	middle := captureGCFixture(t, store, target, "shared old bytes", false, now.AddDate(0, 0, -40))
	pinned := captureGCFixture(t, store, target, "pinned bytes", true, now.AddDate(0, 0, -35))
	newest := captureGCFixture(t, store, target, "newest bytes", false, now.AddDate(0, 0, -1))
	other := captureGCFixture(t, store, otherTarget, "only old version", false, now.AddDate(0, 0, -60))
	refreshGCFixture(t, store)

	plan, err := store.PlanGC(context.Background(), GCOptions{Now: now})
	if err != nil {
		t.Fatalf("PlanGC() error = %v", err)
	}
	if plan.Generation == "" || plan.PlannedAt != utcTimestamp(now) || plan.RetentionDays != 30 || plan.MinimumVersionsPerTarget != 1 {
		t.Fatalf("plan identity/policy = %#v", plan)
	}
	if len(plan.Manifests) != 2 {
		t.Fatalf("manifest candidates = %#v", plan.Manifests)
	}
	if plan.Manifests[0].BackupID != oldest.Manifest.BackupID || plan.Manifests[1].BackupID != middle.Manifest.BackupID {
		t.Fatalf("candidate order = %#v", plan.Manifests)
	}
	for _, candidate := range plan.Manifests {
		if len(candidate.Reasons) != 1 || candidate.Reasons[0] != GCReasonRetention {
			t.Fatalf("candidate reasons = %#v", candidate)
		}
		if candidate.Pinned {
			t.Fatalf("pinned candidate = %#v", candidate)
		}
	}
	for _, retained := range []CaptureResult{pinned, newest, other} {
		if gcPlanContainsManifest(plan, retained.Manifest.BackupID) {
			t.Fatalf("retained manifest selected: %s", retained.Manifest.BackupID)
		}
	}
	if len(plan.Objects) != 1 || plan.Objects[0].Digest != oldest.Manifest.ObjectDigest ||
		plan.Objects[0].ReferencesBefore != 2 || plan.Objects[0].Bytes != int64(len("shared old bytes")) {
		t.Fatalf("object candidates = %#v", plan.Objects)
	}
	if plan.ReclaimableBytes != int64(len("shared old bytes")) || plan.ManifestCount != 2 || plan.ObjectCount != 1 {
		t.Fatalf("plan counts = %#v", plan)
	}

	repeated, err := store.PlanGC(context.Background(), GCOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !gcPlansEqual(plan, repeated) {
		t.Fatalf("PlanGC() is nondeterministic:\nfirst=%#v\nsecond=%#v", plan, repeated)
	}
}

func TestPlanGCUsesVersionLimitAfterReopen(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "store")
	limits := backupStoreTestLimits()
	limits.RetentionDays = 365
	limits.MaxVersionsPerTarget = 8
	store := openBackupTestStore(t, root, limits)
	now := time.Date(2026, 8, 5, 7, 30, 0, 0, time.UTC)
	target := filepath.Join(base, "target.txt")
	captures := make([]CaptureResult, 0, 4)
	for index := 0; index < 4; index++ {
		captures = append(captures, captureGCFixture(t, store, target, string(rune('a'+index)), false, now.Add(-time.Duration(4-index)*time.Hour)))
	}
	refreshGCFixture(t, store)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	limits.MaxVersionsPerTarget = 2
	store, err := Open(Options{Directory: root, Limits: limits})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	plan, err := store.PlanGC(context.Background(), GCOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Manifests) != 2 || plan.Manifests[0].BackupID != captures[0].Manifest.BackupID ||
		plan.Manifests[1].BackupID != captures[1].Manifest.BackupID {
		t.Fatalf("version-limit candidates = %#v", plan.Manifests)
	}
	for _, candidate := range plan.Manifests {
		if len(candidate.Reasons) != 1 || candidate.Reasons[0] != GCReasonVersionLimit {
			t.Fatalf("version-limit reason = %#v", candidate)
		}
	}
}

func TestApplyGCIsGenerationBoundManifestFirstAndReferenceCounted(t *testing.T) {
	base := canonicalTempDir(t)
	limits := backupStoreTestLimits()
	limits.RetentionDays = 30
	limits.MaxVersionsPerTarget = 8
	store := openBackupTestStore(t, filepath.Join(base, "store"), limits)
	now := time.Date(2026, 8, 5, 7, 30, 0, 0, time.UTC)
	target := filepath.Join(base, "target.txt")
	sharedTarget := filepath.Join(base, "shared.txt")

	exclusive := captureGCFixture(t, store, target, "exclusive old", false, now.AddDate(0, 0, -50))
	_ = captureGCFixture(t, store, target, "current", false, now.AddDate(0, 0, -1))
	sharedOld := captureGCFixture(t, store, sharedTarget, "shared live", false, now.AddDate(0, 0, -50))
	sharedLive := captureGCFixture(t, store, sharedTarget, "shared live", true, now.AddDate(0, 0, -1))
	orphanDigest := installGCOrphanObject(t, store, []byte("orphan bytes"))
	refreshGCFixture(t, store)

	plan, err := store.PlanGC(context.Background(), GCOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !gcPlanContainsManifest(plan, exclusive.Manifest.BackupID) || !gcPlanContainsManifest(plan, sharedOld.Manifest.BackupID) {
		t.Fatalf("missing expected candidates: %#v", plan.Manifests)
	}
	if gcPlanContainsObject(plan, sharedLive.Manifest.ObjectDigest) {
		t.Fatalf("shared live object selected: %#v", plan.Objects)
	}
	if !gcPlanContainsObject(plan, exclusive.Manifest.ObjectDigest) || !gcPlanContainsObject(plan, orphanDigest) {
		t.Fatalf("exclusive/orphan object missing: %#v", plan.Objects)
	}

	result, err := store.ApplyGC(context.Background(), plan)
	if err != nil {
		t.Fatalf("ApplyGC() error = %v, result=%#v", err, result)
	}
	if result.PreviousGeneration != plan.Generation || result.Generation == "" || result.Generation == plan.Generation ||
		result.ManifestsRemoved != len(plan.Manifests) || result.ObjectsRemoved != len(plan.Objects) ||
		result.BytesReclaimed != plan.ReclaimableBytes {
		t.Fatalf("apply result = %#v", result)
	}
	for _, candidate := range plan.Manifests {
		if _, err := os.Stat(manifestPath(store.Root(), candidate.BackupID)); !os.IsNotExist(err) {
			t.Fatalf("manifest still live: %s err=%v", candidate.BackupID, err)
		}
	}
	for _, candidate := range plan.Objects {
		if _, err := os.Stat(objectPath(store.Root(), candidate.Digest)); !os.IsNotExist(err) {
			t.Fatalf("object still live: %s err=%v", candidate.Digest, err)
		}
	}
	if _, err := os.Stat(objectPath(store.Root(), sharedLive.Manifest.ObjectDigest)); err != nil {
		t.Fatalf("shared live object removed: %v", err)
	}
	if store.Index().Generation != result.Generation || store.Index().ManifestCount != 2 {
		t.Fatalf("derived index = %#v", store.Index())
	}
	status, err := store.Status(context.Background())
	if err != nil || !status.Healthy || status.TrashEntryCount != 0 {
		t.Fatalf("status after GC = %#v err=%v", status, err)
	}
}

func TestApplyGCRejectsStaleGenerationReservationsAndActiveRestore(t *testing.T) {
	base := canonicalTempDir(t)
	limits := backupStoreTestLimits()
	limits.RetentionDays = 30
	limits.MaxVersionsPerTarget = 8
	store := openBackupTestStore(t, filepath.Join(base, "store"), limits)
	now := time.Date(2026, 8, 5, 7, 30, 0, 0, time.UTC)
	target := filepath.Join(base, "target.txt")
	candidate := captureGCFixture(t, store, target, "old", false, now.AddDate(0, 0, -50))
	_ = captureGCFixture(t, store, target, "new", false, now.AddDate(0, 0, -1))
	refreshGCFixture(t, store)

	reserved, err := store.reserve(1, CaptureRequest{TargetPath: filepath.Join(base, "future.txt"), SourceOperation: SourceOperationEdit})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PlanGC(context.Background(), GCOptions{Now: now}); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("PlanGC(active reservation) error=%v, want CONFLICT", err)
	}
	store.release(reserved)

	plan, err := store.PlanGC(context.Background(), GCOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.OpenRestoreSource(context.Background(), candidate.Manifest.BackupID, RestoreSourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyGC(context.Background(), plan); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("ApplyGC(active restore) error=%v, want CONFLICT", err)
	}
	if _, err := os.Stat(manifestPath(store.Root(), candidate.Manifest.BackupID)); err != nil {
		t.Fatalf("active restore manifest changed: %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}

	stale, err := store.PlanGC(context.Background(), GCOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	_ = captureGCFixture(t, store, filepath.Join(base, "other.txt"), "new state", false, now)
	if _, err := store.ApplyGC(context.Background(), stale); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("ApplyGC(stale generation) error=%v, want CONFLICT", err)
	}
	if _, err := os.Stat(manifestPath(store.Root(), candidate.Manifest.BackupID)); err != nil {
		t.Fatalf("stale plan changed candidate: %v", err)
	}
}

func TestGCFailureAfterManifestTrashPreservesObjectAndRefreshesIndex(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "store")
	limits := backupStoreTestLimits()
	limits.RetentionDays = 30
	limits.MaxVersionsPerTarget = 8
	store := openBackupTestStore(t, root, limits)
	now := time.Date(2026, 8, 5, 7, 30, 0, 0, time.UTC)
	target := filepath.Join(base, "target.txt")
	candidate := captureGCFixture(t, store, target, "old", false, now.AddDate(0, 0, -50))
	_ = captureGCFixture(t, store, target, "new", false, now.AddDate(0, 0, -1))
	refreshGCFixture(t, store)
	plan, err := store.PlanGC(context.Background(), GCOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	restoreMove := overrideAfterGCMove(store, "backup manifest", func() error {
		return operation.New(operation.KindFilesystem, "injected failure after manifest trash")
	})
	result, err := store.ApplyGC(context.Background(), plan)
	if operation.KindOf(err) != operation.KindFilesystem || result.ManifestsRemoved != 1 || result.ObjectsRemoved != 0 {
		t.Fatalf("ApplyGC() result=%#v error=%v", result, err)
	}
	if _, err := os.Stat(manifestPath(root, candidate.Manifest.BackupID)); !os.IsNotExist(err) {
		t.Fatalf("candidate manifest remained live: %v", err)
	}
	if _, err := os.Stat(gcManifestTrashPath(root, candidate.Manifest.BackupID)); err != nil {
		t.Fatalf("manifest trash missing: %v", err)
	}
	if _, err := os.Stat(objectPath(root, candidate.Manifest.ObjectDigest)); err != nil {
		t.Fatalf("object was removed before object phase: %v", err)
	}
	if store.Index().ManifestCount != 1 {
		t.Fatalf("index was not refreshed after partial manifest phase: %#v", store.Index())
	}
	restoreMove()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(Options{Directory: root, Limits: limits})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := os.Stat(gcManifestTrashPath(root, candidate.Manifest.BackupID)); !os.IsNotExist(err) {
		t.Fatalf("startup did not recover manifest trash: %v", err)
	}
	if _, err := os.Stat(objectPath(root, candidate.Manifest.ObjectDigest)); err != nil {
		t.Fatalf("startup removed orphan object implicitly: %v", err)
	}
}

func TestGCFailureAfterObjectTrashRecoversWithoutLiveReferenceLoss(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "store")
	limits := backupStoreTestLimits()
	limits.RetentionDays = 30
	limits.MaxVersionsPerTarget = 8
	store := openBackupTestStore(t, root, limits)
	now := time.Date(2026, 8, 5, 7, 30, 0, 0, time.UTC)
	target := filepath.Join(base, "target.txt")
	candidate := captureGCFixture(t, store, target, "old", false, now.AddDate(0, 0, -50))
	_ = captureGCFixture(t, store, target, "new", false, now.AddDate(0, 0, -1))
	refreshGCFixture(t, store)
	plan, err := store.PlanGC(context.Background(), GCOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	restoreMove := overrideAfterGCMove(store, "backup object", func() error {
		return operation.New(operation.KindFilesystem, "injected failure after object trash")
	})
	result, err := store.ApplyGC(context.Background(), plan)
	if operation.KindOf(err) != operation.KindFilesystem || result.ManifestsRemoved != 1 || result.ObjectsRemoved != 1 {
		t.Fatalf("ApplyGC() result=%#v error=%v", result, err)
	}
	if _, err := os.Stat(manifestPath(root, candidate.Manifest.BackupID)); !os.IsNotExist(err) {
		t.Fatalf("candidate manifest remained live: %v", err)
	}
	if _, err := os.Stat(objectPath(root, candidate.Manifest.ObjectDigest)); !os.IsNotExist(err) {
		t.Fatalf("candidate object remained live: %v", err)
	}
	if _, err := os.Stat(gcObjectTrashPath(root, candidate.Manifest.ObjectDigest)); err != nil {
		t.Fatalf("object trash missing: %v", err)
	}
	restoreMove()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(Options{Directory: root, Limits: limits})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, path := range []string{gcManifestTrashPath(root, candidate.Manifest.BackupID), gcObjectTrashPath(root, candidate.Manifest.ObjectDigest)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("startup did not recover %s: %v", filepath.Base(path), err)
		}
	}
	if store.Index().ManifestCount != 1 || store.Index().ObjectCount != 1 {
		t.Fatalf("recovered index = %#v", store.Index())
	}
}

func TestGCCleanupFailureSurfacesTrashAndStartupCompletesCleanup(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "store")
	limits := backupStoreTestLimits()
	limits.RetentionDays = 30
	limits.MaxVersionsPerTarget = 8
	store := openBackupTestStore(t, root, limits)
	now := time.Date(2026, 8, 5, 7, 30, 0, 0, time.UTC)
	target := filepath.Join(base, "target.txt")
	candidate := captureGCFixture(t, store, target, "old", false, now.AddDate(0, 0, -50))
	_ = captureGCFixture(t, store, target, "new", false, now.AddDate(0, 0, -1))
	refreshGCFixture(t, store)
	plan, err := store.PlanGC(context.Background(), GCOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	restoreRemoval := overrideGCTrashRemoval(store, func(path string) error {
		if strings.HasPrefix(filepath.Base(path), gcObjectTrashPrefix) {
			return operation.New(operation.KindFilesystem, "injected object cleanup failure")
		}
		return nil
	})
	result, err := store.ApplyGC(context.Background(), plan)
	if operation.KindOf(err) != operation.KindFilesystem || result.ManifestsRemoved != 1 || result.ObjectsRemoved != 1 ||
		result.TrashCleanupFailures != 1 || result.TrashEntriesRemaining != 1 || result.BytesReclaimed != 0 {
		t.Fatalf("ApplyGC() result=%#v error=%v", result, err)
	}
	if _, err := os.Stat(gcObjectTrashPath(root, candidate.Manifest.ObjectDigest)); err != nil {
		t.Fatalf("object trash missing after cleanup failure: %v", err)
	}
	restoreRemoval()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(Options{Directory: root, Limits: limits})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := os.Stat(gcObjectTrashPath(root, candidate.Manifest.ObjectDigest)); !os.IsNotExist(err) {
		t.Fatalf("startup did not finish object cleanup: %v", err)
	}
}

func TestCaptureAdmissionRejectsActiveGC(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	target := filepath.Join(base, "target.txt")
	if err := os.WriteFile(target, []byte("capture"), 0o600); err != nil {
		t.Fatal(err)
	}
	store.stateMu.Lock()
	store.gcActive = true
	store.stateMu.Unlock()
	defer func() {
		store.stateMu.Lock()
		store.gcActive = false
		store.stateMu.Unlock()
	}()
	request := CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}
	if _, err := store.Capture(context.Background(), request); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("Capture(active GC) error=%v, want CONFLICT", err)
	}
	if err := store.PreflightCaptureBatch(context.Background(), []CaptureRequest{request}); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("PreflightCaptureBatch(active GC) error=%v, want CONFLICT", err)
	}
	if store.Index().ManifestCount != 0 || store.Index().ObjectCount != 0 {
		t.Fatalf("active-GC rejection changed store: %#v", store.Index())
	}
}

func TestPinnedCaptureDoesNotLeakTargetReservationAccounting(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	captureManagementFixture(t, store, filepath.Join(base, "target.txt"), "pinned", true)
	store.stateMu.RLock()
	defer store.stateMu.RUnlock()
	if store.hasReservationsLocked() || len(store.reservedTargets) != 0 {
		t.Fatalf("reservation accounting leaked: bytes=%d manifests=%d pinned=%d targets=%#v", store.reservedBytes, store.reservedManifests, store.reservedPinned, store.reservedTargets)
	}
}

func TestOpenRecoversRecognizedGCTrashAndPreservesUnknownEntries(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "store")
	store := openBackupTestStore(t, root, backupStoreTestLimits())
	target := filepath.Join(base, "target.txt")
	captured := captureManagementFixture(t, store, target, "trash recovery", false)

	manifestTrash := gcManifestTrashPath(root, captured.Manifest.BackupID)
	if err := filesystem.MoveNoReplace(manifestPath(root, captured.Manifest.BackupID), manifestTrash); err != nil {
		t.Fatal(err)
	}
	objectTrash := gcObjectTrashPath(root, captured.Manifest.ObjectDigest)
	if err := filesystem.MoveNoReplace(objectPath(root, captured.Manifest.ObjectDigest), objectTrash); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(root, "trash", "operator-unknown.tmp")
	if err := os.WriteFile(unknown, []byte("unknown"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(unknown, false); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(Options{Directory: root, Limits: backupStoreTestLimits()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, path := range []string{manifestTrash, objectTrash} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("recognized GC trash was not recovered: %s err=%v", filepath.Base(path), err)
		}
	}
	if _, err := os.Stat(unknown); err != nil {
		t.Fatalf("unknown trash entry was removed: %v", err)
	}
	status, err := store.Status(context.Background())
	if err != nil || !status.Healthy || status.TrashEntryCount != 1 || store.Index().ManifestCount != 0 || store.Index().ObjectCount != 0 {
		t.Fatalf("recovered status=%#v index=%#v err=%v", status, store.Index(), err)
	}
}

func TestOpenPreservesMalformedRecognizedGCTrash(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "store")
	store := openBackupTestStore(t, root, backupStoreTestLimits())
	manifestTrash := gcManifestTrashPath(root, strings.Repeat("a", 64))
	if err := os.WriteFile(manifestTrash, []byte("not a manifest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(manifestTrash, false); err != nil {
		t.Fatal(err)
	}
	claimedDigest := strings.Repeat("b", 64)
	objectTrash := gcObjectTrashPath(root, claimedDigest)
	if err := os.WriteFile(objectTrash, []byte("digest mismatch"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(objectTrash, false); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(Options{Directory: root, Limits: backupStoreTestLimits()})
	if err != nil {
		t.Fatalf("Open() rejected uncertain trash instead of preserving it: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, path := range []string{manifestTrash, objectTrash} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("uncertain trash was removed: %s err=%v", filepath.Base(path), err)
		}
	}
	status, err := store.Status(context.Background())
	if err != nil || status.TrashEntryCount != 2 {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func captureGCFixture(t *testing.T, store *Store, target, content string, pinned bool, createdAt time.Time) CaptureResult {
	t.Helper()
	result := captureManagementFixture(t, store, target, content, pinned)
	manifest := result.Manifest
	manifest.CreatedAt = utcTimestamp(createdAt)
	manifest, err := finalizeManifestChecksum(manifest)
	if err != nil {
		t.Fatal(err)
	}
	data, err := encodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := manifestPath(store.Root(), manifest.BackupID)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(path, false); err != nil {
		t.Fatal(err)
	}
	result.Manifest = manifest
	return result
}

func refreshGCFixture(t *testing.T, store *Store) {
	t.Helper()
	store.transactionMu.Lock()
	defer store.transactionMu.Unlock()
	if err := store.refreshDerivedIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func installGCOrphanObject(t *testing.T, store *Store, data []byte) string {
	t.Helper()
	digest := sha256.Sum256(data)
	digestText := hex.EncodeToString(digest[:])
	path := objectPath(store.Root(), digestText)
	if err := ensureDirectory(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(path, false); err != nil {
		t.Fatal(err)
	}
	return digestText
}

func gcPlanContainsManifest(plan GCPlan, backupID string) bool {
	for _, candidate := range plan.Manifests {
		if candidate.BackupID == backupID {
			return true
		}
	}
	return false
}

func gcPlanContainsObject(plan GCPlan, digest string) bool {
	for _, candidate := range plan.Objects {
		if candidate.Digest == digest {
			return true
		}
	}
	return false
}

func FuzzValidateGCPlan(f *testing.F) {
	validID := strings.Repeat("a", 64)
	f.Add("2026-08-05T08:00:00Z", validID, validID, validID, "retention", int64(1), 1)
	f.Add("not-a-time", "bad", "bad", "bad", "unknown", int64(-1), -1)
	f.Fuzz(func(t *testing.T, plannedAt, generation, backupID, digest, reason string, objectBytes int64, references int) {
		if len(plannedAt) > 4096 || len(generation) > 4096 || len(backupID) > 4096 || len(digest) > 4096 || len(reason) > 4096 {
			t.Skip()
		}
		plan := GCPlan{
			PlannedAt:                plannedAt,
			Generation:               generation,
			RetentionDays:            30,
			MinimumVersionsPerTarget: 1,
			ManifestCount:            1,
			ObjectCount:              1,
			ReclaimableBytes:         objectBytes,
			Manifests: []GCManifestCandidate{{
				BackupID:     backupID,
				CreatedAt:    plannedAt,
				TargetPath:   "target",
				ObjectDigest: digest,
				ObjectBytes:  objectBytes,
				Reasons:      []GCReason{GCReason(reason)},
			}},
			Objects: []GCObjectCandidate{{Digest: digest, Bytes: objectBytes, ReferencesBefore: references}},
		}
		parsed, err := validateGCPlan(plan)
		if err == nil {
			if parsed.Location() != time.UTC || plan.ManifestCount != len(plan.Manifests) || plan.ObjectCount != len(plan.Objects) || plan.ReclaimableBytes < 0 {
				t.Fatalf("accepted inconsistent plan: %#v", plan)
			}
		}
	})
}
