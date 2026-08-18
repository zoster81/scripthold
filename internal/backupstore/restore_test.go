package backupstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

func TestOpenRestoreSourceReadsAndStagesExactVerifiedBytes(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	target := filepath.Join(base, "target.txt")
	content := []byte("restore source bytes\r\n")
	if err := os.WriteFile(target, content, 0o640); err != nil {
		t.Fatal(err)
	}
	captured, err := store.Capture(context.Background(), CaptureRequest{
		TargetPath:      target,
		SourceOperation: SourceOperationEdit,
	})
	if err != nil {
		t.Fatal(err)
	}

	source, err := store.OpenRestoreSource(context.Background(), captured.Manifest.BackupID, RestoreSourceOptions{})
	if err != nil {
		t.Fatalf("OpenRestoreSource() error = %v", err)
	}
	defer source.Close()
	if source.Manifest() != captured.Manifest {
		t.Fatalf("Manifest() = %#v, want %#v", source.Manifest(), captured.Manifest)
	}
	if err := source.Verify(context.Background()); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	read, err := source.ReadAll(context.Background(), int64(len(content)))
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(read) != string(content) {
		t.Fatalf("ReadAll() = %q, want %q", read, content)
	}

	destination := filepath.Join(base, "restored.txt")
	modTime := time.Unix(1_700_000_000, 0).UTC()
	staged, err := source.Stage(context.Background(), destination, 0o640, &modTime)
	if err != nil {
		t.Fatalf("Stage() error = %v", err)
	}
	expected, err := filesystem.CaptureSnapshot(destination)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := staged.Commit(filesystem.ReplaceOptions{Expected: &expected})
	if err != nil || !changed {
		t.Fatalf("Commit() changed=%v error=%v", changed, err)
	}
	restored, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(content) {
		t.Fatalf("restored bytes = %q, want %q", restored, content)
	}
}

func TestOpenRestoreSourceAuthorizesBeforeObjectHash(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	capture := captureManagementFixture(t, store, filepath.Join(base, "target.txt"), "authorization first", false)
	object := objectPath(store.Root(), capture.Manifest.ObjectDigest)
	corrupt := []byte(strings.ToUpper("authorization first"))
	if len(corrupt) != len("authorization first") {
		t.Fatal("corruption fixture must preserve size")
	}
	if err := os.WriteFile(object, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(object, false); err != nil {
		t.Fatal(err)
	}

	_, err := store.OpenRestoreSource(context.Background(), capture.Manifest.BackupID, RestoreSourceOptions{
		AuthorizeTarget: func(string) error {
			return operation.New(operation.KindAccessDenied, "restore target is not currently authorized")
		},
	})
	if operation.KindOf(err) != operation.KindAccessDenied {
		t.Fatalf("OpenRestoreSource() error = %v, want ACCESS_DENIED", err)
	}
}

func TestOpenRestoreSourceRejectsCorruptObject(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	capture := captureManagementFixture(t, store, filepath.Join(base, "target.txt"), "verified restore", false)
	object := objectPath(store.Root(), capture.Manifest.ObjectDigest)
	corrupt := []byte(strings.ToUpper("verified restore"))
	if len(corrupt) != len("verified restore") {
		t.Fatal("corruption fixture must preserve size")
	}
	if err := os.WriteFile(object, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(object, false); err != nil {
		t.Fatal(err)
	}

	if _, err := store.OpenRestoreSource(context.Background(), capture.Manifest.BackupID, RestoreSourceOptions{}); err == nil {
		t.Fatal("OpenRestoreSource() accepted a corrupt object")
	}
}

func TestRestoreSourceDetectsManifestChangeAfterOpen(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	capture := captureManagementFixture(t, store, filepath.Join(base, "target.txt"), "stable restore bytes", false)
	source, err := store.OpenRestoreSource(context.Background(), capture.Manifest.BackupID, RestoreSourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	manifestFile := manifestPath(store.Root(), capture.Manifest.BackupID)
	manifestBytes, err := os.ReadFile(manifestFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestFile, append(manifestBytes, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(manifestFile, false); err != nil {
		t.Fatal(err)
	}
	if operation.KindOf(source.Verify(context.Background())) != operation.KindConflict {
		t.Fatal("Verify() did not reject a changed manifest")
	}
}

func TestRestoreSourceDetectsObjectChangeAfterOpen(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	capture := captureManagementFixture(t, store, filepath.Join(base, "target.txt"), "stable restore bytes", false)
	source, err := store.OpenRestoreSource(context.Background(), capture.Manifest.BackupID, RestoreSourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	object := objectPath(store.Root(), capture.Manifest.ObjectDigest)
	corrupt := []byte(strings.ToUpper("stable restore bytes"))
	if len(corrupt) != len("stable restore bytes") {
		t.Fatal("corruption fixture must preserve size")
	}
	if err := os.WriteFile(object, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(object, false); err != nil {
		t.Fatal(err)
	}
	if err := source.Verify(context.Background()); err == nil {
		t.Fatal("Verify() did not reject changed object bytes")
	}
}

func TestRestoreSourceBoundsCancellationAndClose(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	capture := captureManagementFixture(t, store, filepath.Join(base, "target.txt"), "bounded restore", false)
	source, err := store.OpenRestoreSource(context.Background(), capture.Manifest.BackupID, RestoreSourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.ReadAll(context.Background(), int64(len("bounded restore")-1)); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("ReadAll() error = %v, want LIMIT", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := source.Verify(cancelled); operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("Verify(cancelled) error = %v, want CANCELLED", err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if _, err := source.ReadAll(context.Background(), 1024); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("ReadAll(after close) error = %v, want CONFLICT", err)
	}
	if _, err := source.Stage(context.Background(), filepath.Join(base, "closed.txt"), 0o600, nil); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("Stage(after close) error = %v, want CONFLICT", err)
	}
	if err := source.Verify(context.Background()); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("Verify(after close) error = %v, want CONFLICT", err)
	}
}

func TestRestorePlanTTLUsesConfiguredLimit(t *testing.T) {
	base := canonicalTempDir(t)
	limits := backupStoreTestLimits()
	limits.PlanTTLSeconds = 37
	store := openBackupTestStore(t, filepath.Join(base, "store"), limits)
	if got := store.RestorePlanTTL(); got != 37*time.Second {
		t.Fatalf("RestorePlanTTL() = %v, want 37s", got)
	}
}

func TestOpenRestoreSourceRejectsMissingAndMalformedIdentifiers(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	for _, backupID := range []string{"invalid", strings.Repeat("a", 64)} {
		_, err := store.OpenRestoreSource(context.Background(), backupID, RestoreSourceOptions{})
		if operation.KindOf(err) != operation.KindInvalidInput {
			t.Fatalf("OpenRestoreSource(%q) error = %v, want INVALID_INPUT", backupID, err)
		}
	}
}

func TestRestoreSourceCloseJoinsNoUnexpectedErrors(t *testing.T) {
	var source *RestoreSource
	if err := source.Close(); err != nil && !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("nil Close() error = %v", err)
	}
}
