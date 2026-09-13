package backupstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteBackupExplicitlyRemovesPinnedManifestAndObject(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	target := filepath.Join(base, "target.txt")
	if err := os.WriteFile(target, []byte("protected"), 0o600); err != nil {
		t.Fatal(err)
	}
	captured, err := store.Capture(context.Background(), CaptureRequest{
		TargetPath: target, SourceOperation: SourceOperationEdit, Pinned: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.DeleteBackup(context.Background(), captured.Manifest.BackupID)
	if err != nil {
		t.Fatalf("delete pinned backup: %v", err)
	}
	if !result.Pinned || !result.ManifestRemoved || !result.ObjectRemoved || result.BytesReclaimed != int64(len("protected")) {
		t.Fatalf("delete result = %#v", result)
	}
	if store.Index().ManifestCount != 0 || store.Index().PinnedCount != 0 {
		t.Fatalf("index after explicit delete = %#v", store.Index())
	}
	if _, statErr := os.Stat(manifestPath(store.Root(), captured.Manifest.BackupID)); !os.IsNotExist(statErr) {
		t.Fatalf("deleted pinned manifest remains: %v", statErr)
	}
	if _, statErr := os.Stat(objectPath(store.Root(), captured.Manifest.ObjectDigest)); !os.IsNotExist(statErr) {
		t.Fatalf("deleted pinned object remains: %v", statErr)
	}
	report, err := store.Audit(context.Background(), AuditOptions{Mode: AuditQuick})
	if err != nil || !report.Healthy || !report.IndexConsistent {
		t.Fatalf("explicit delete left inconsistent derived state: report=%#v err=%v", report, err)
	}
}

func TestDeleteBackupPreservesSharedOrActiveRestoreObjects(t *testing.T) {
	t.Run("shared object", func(t *testing.T) {
		base := canonicalTempDir(t)
		store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
		firstTarget := filepath.Join(base, "first.txt")
		secondTarget := filepath.Join(base, "second.txt")
		for _, target := range []string{firstTarget, secondTarget} {
			if err := os.WriteFile(target, []byte("shared"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		first, err := store.Capture(context.Background(), CaptureRequest{TargetPath: firstTarget, SourceOperation: SourceOperationEdit, Pinned: true})
		if err != nil {
			t.Fatal(err)
		}
		second, err := store.Capture(context.Background(), CaptureRequest{TargetPath: secondTarget, SourceOperation: SourceOperationEdit})
		if err != nil {
			t.Fatal(err)
		}
		result, err := store.DeleteBackup(context.Background(), first.Manifest.BackupID)
		if err != nil {
			t.Fatal(err)
		}
		if !result.ManifestRemoved || result.ObjectRemoved || result.BytesReclaimed != 0 {
			t.Fatalf("shared-object delete result = %#v", result)
		}
		if _, err := store.Inspect(context.Background(), second.Manifest.BackupID, InspectOptions{}); err != nil {
			t.Fatalf("shared object no longer supports retained backup: %v", err)
		}
	})

	t.Run("active restore", func(t *testing.T) {
		base := canonicalTempDir(t)
		store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
		target := filepath.Join(base, "target.txt")
		if err := os.WriteFile(target, []byte("active"), 0o600); err != nil {
			t.Fatal(err)
		}
		captured, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit, Pinned: true})
		if err != nil {
			t.Fatal(err)
		}
		source, err := store.OpenRestoreSource(context.Background(), captured.Manifest.BackupID, RestoreSourceOptions{})
		if err != nil {
			t.Fatal(err)
		}
		defer source.Close()
		if _, err := store.DeleteBackup(context.Background(), captured.Manifest.BackupID); err == nil {
			t.Fatal("active restore source was explicitly deleted")
		}
		if !indexContainsManifest(store.Index(), captured.Manifest.BackupID) {
			t.Fatal("active restore conflict removed the manifest")
		}
	})
}
