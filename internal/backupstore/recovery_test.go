package backupstore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenRebuildsMissingAndCorruptDerivedIndex(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(string) error
	}{
		{
			name: "missing",
			mutate: func(path string) error {
				return os.Remove(path)
			},
		},
		{
			name: "corrupt",
			mutate: func(path string) error {
				if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
					return err
				}
				return restrictPathPermissions(path, false)
			},
		},
		{
			name: "stale projection",
			mutate: func(path string) error {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				var index persistedIndex
				if err := json.Unmarshal(data, &index); err != nil {
					return err
				}
				index.TotalObjectBytes++
				data, err = json.Marshal(index)
				if err != nil {
					return err
				}
				if err := os.WriteFile(path, data, 0o600); err != nil {
					return err
				}
				return restrictPathPermissions(path, false)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := canonicalTempDir(t)
			root := filepath.Join(base, "store")
			store := openBackupTestStore(t, root, backupStoreTestLimits())
			target := filepath.Join(base, "target.txt")
			if err := os.WriteFile(target, []byte("indexed bytes"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}); err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			indexPath := filepath.Join(root, "index", "index-v1.json")
			if err := tc.mutate(indexPath); err != nil {
				t.Fatal(err)
			}

			reopened, err := Open(Options{Directory: root, Limits: backupStoreTestLimits()})
			if err != nil {
				t.Fatalf("reopen: %v", err)
			}
			defer reopened.Close()
			if reopened.Index().ManifestCount != 1 || reopened.Index().ObjectCount != 1 || reopened.Index().TotalObjectBytes != int64(len("indexed bytes")) {
				t.Fatalf("rebuilt index = %#v", reopened.Index())
			}
			data, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			var persisted persistedIndex
			if err := json.Unmarshal(data, &persisted); err != nil {
				t.Fatalf("rebuilt index is invalid JSON: %v", err)
			}
			if persisted.Generation != reopened.Index().Generation {
				t.Fatalf("persisted generation = %q, want %q", persisted.Generation, reopened.Index().Generation)
			}
		})
	}
}

func TestOpenRejectsHardLinkedInternalFiles(t *testing.T) {
	for _, entry := range []string{"store.json", "store.lock", "manifest", "object", "index"} {
		t.Run(entry, func(t *testing.T) {
			base := canonicalTempDir(t)
			root := filepath.Join(base, "store")
			store := openBackupTestStore(t, root, backupStoreTestLimits())
			target := filepath.Join(base, "target.txt")
			if err := os.WriteFile(target, []byte("hard-link fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}

			var source string
			switch entry {
			case "store.json":
				source = filepath.Join(root, "store.json")
			case "store.lock":
				source = filepath.Join(root, "store.lock")
			case "manifest":
				source = manifestPath(root, result.Manifest.BackupID)
			case "object":
				source = objectPath(root, result.Manifest.ObjectDigest)
			case "index":
				source = indexPath(root)
			}
			alias := filepath.Join(base, entry+"-alias")
			if err := os.Link(source, alias); err != nil {
				t.Skipf("hard links unavailable: %v", err)
			}

			reopened, err := Open(Options{Directory: root, Limits: backupStoreTestLimits()})
			if reopened != nil {
				_ = reopened.Close()
			}
			if err == nil {
				t.Fatalf("Open() accepted hard-linked internal %s", entry)
			}
			if strings.Contains(err.Error(), root) || strings.Contains(err.Error(), source) || strings.Contains(err.Error(), alias) {
				t.Fatalf("error exposed an internal path: %v", err)
			}
		})
	}
}

func TestOpenRejectsUnexpectedIndexEntry(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "store")
	store := openBackupTestStore(t, root, backupStoreTestLimits())
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	unexpected := filepath.Join(root, "index", "unexpected.json")
	if err := os.WriteFile(unexpected, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(unexpected, false); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(Options{Directory: root, Limits: backupStoreTestLimits()})
	if reopened != nil {
		_ = reopened.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "index directory") {
		t.Fatalf("reopen error = %v, want unexpected index entry", err)
	}
}

func TestOpenRejectsManifestWithInconsistentContentFingerprint(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "store")
	store := openBackupTestStore(t, root, backupStoreTestLimits())
	target := filepath.Join(base, "target.txt")
	if err := os.WriteFile(target, []byte("fingerprint bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	path := manifestPath(root, result.Manifest.BackupID)
	manifest := result.Manifest
	manifest.ContentFingerprint = strings.Repeat("0", 64)
	manifest, err = finalizeManifestChecksum(manifest)
	if err != nil {
		t.Fatal(err)
	}
	data, err := encodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(path, false); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(Options{Directory: root, Limits: backupStoreTestLimits()})
	if reopened != nil {
		_ = reopened.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("reopen error = %v, want invalid manifest", err)
	}
	if strings.Contains(err.Error(), root) || strings.Contains(err.Error(), path) {
		t.Fatalf("reopen error exposed an internal path: %v", err)
	}
}

func TestOpenRejectsMissingReferencedObject(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "store")
	store := openBackupTestStore(t, root, backupStoreTestLimits())
	target := filepath.Join(base, "target.txt")
	if err := os.WriteFile(target, []byte("object required"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(objectPath(root, result.Manifest.ObjectDigest)); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(Options{Directory: root, Limits: backupStoreTestLimits()})
	if reopened != nil {
		_ = reopened.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "referenced object") {
		t.Fatalf("reopen error = %v, want missing referenced object", err)
	}
	if strings.Contains(err.Error(), root) {
		t.Fatalf("error exposed store path: %v", err)
	}
}

func TestOpenRejectsTamperedManifestChecksum(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "store")
	store := openBackupTestStore(t, root, backupStoreTestLimits())
	target := filepath.Join(base, "target.txt")
	if err := os.WriteFile(target, []byte("manifest bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	path := manifestPath(root, result.Manifest.BackupID)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["label"] = "tampered"
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(path, false); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(Options{Directory: root, Limits: backupStoreTestLimits()})
	if reopened != nil {
		_ = reopened.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "manifest checksum") {
		t.Fatalf("reopen error = %v, want checksum failure", err)
	}
}

func TestOpenPreservesAndReportsOrphanStagingAndTrashEntries(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "store")
	store := openBackupTestStore(t, root, backupStoreTestLimits())
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	digest := strings.Repeat("a", 64)
	shard := filepath.Join(root, "objects", "sha256", digest[:2])
	if err := ensureDirectory(shard); err != nil {
		t.Fatal(err)
	}
	orphan := objectPath(root, digest)
	if err := os.WriteFile(orphan, []byte("orphan"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(orphan, false); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{filepath.Join("staging", "capture.tmp"), filepath.Join("trash", "old.tmp")} {
		path := filepath.Join(root, relative)
		if err := os.WriteFile(path, []byte("leftover"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := restrictPathPermissions(path, false); err != nil {
			t.Fatal(err)
		}
	}

	reopened, err := Open(Options{Directory: root, Limits: backupStoreTestLimits()})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	report, err := reopened.Audit(context.Background(), AuditOptions{Mode: AuditQuick})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Healthy || report.OrphanObjectCount != 1 || report.StagingEntryCount != 1 || report.TrashEntryCount != 1 {
		t.Fatalf("audit report = %#v", report)
	}
	for _, path := range []string{orphan, filepath.Join(root, "staging", "capture.tmp"), filepath.Join(root, "trash", "old.tmp")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("recovery deleted %s: %v", filepath.Base(path), err)
		}
	}
}
