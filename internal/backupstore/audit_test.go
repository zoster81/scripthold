package backupstore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestFullAuditDetectsSameSizeObjectCorruption(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	target := filepath.Join(base, "target.txt")
	content := []byte("audit object")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if err != nil {
		t.Fatal(err)
	}
	object := objectPath(store.Root(), result.Manifest.ObjectDigest)
	corrupt := []byte("AUDIT OBJECT")
	if len(corrupt) != len(content) {
		t.Fatal("corruption fixture must preserve size")
	}
	if err := os.WriteFile(object, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(object, false); err != nil {
		t.Fatal(err)
	}

	quick, err := store.Audit(context.Background(), AuditOptions{Mode: AuditQuick})
	if err != nil {
		t.Fatal(err)
	}
	if !quick.Healthy {
		t.Fatalf("quick audit unexpectedly hashed object: %#v", quick)
	}
	full, err := store.Audit(context.Background(), AuditOptions{Mode: AuditFull})
	if err != nil {
		t.Fatal(err)
	}
	if full.Healthy || len(full.Issues) == 0 {
		t.Fatalf("full audit missed corruption: %#v", full)
	}
	if !strings.Contains(full.Issues[0].Code, "OBJECT") {
		t.Fatalf("unexpected audit issue = %#v", full.Issues[0])
	}
}

func TestAuditHonorsObjectAndByteBounds(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	for index, content := range [][]byte{[]byte("first"), []byte("second")} {
		target := filepath.Join(base, string(rune('a'+index))+".txt")
		if err := os.WriteFile(target, content, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}); err != nil {
			t.Fatal(err)
		}
	}
	for _, options := range []AuditOptions{
		{Mode: AuditFull, MaxObjects: 1, MaxBytes: 1024},
		{Mode: AuditFull, MaxObjects: 10, MaxBytes: 1},
	} {
		report, err := store.Audit(context.Background(), options)
		if err != nil {
			t.Fatal(err)
		}
		if report.Healthy || len(report.Issues) == 0 || report.Issues[0].Code != AuditIssueLimit {
			t.Fatalf("bounded audit report = %#v", report)
		}
	}
}

func TestAuditReportsUnexpectedRootEntryWithoutDeletingIt(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	unexpected := filepath.Join(store.Root(), "unexpected.txt")
	if err := os.WriteFile(unexpected, []byte("preserve me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(unexpected, false); err != nil {
		t.Fatal(err)
	}

	report, err := store.Audit(context.Background(), AuditOptions{Mode: AuditQuick})
	if err != nil {
		t.Fatal(err)
	}
	if report.Healthy || len(report.Issues) == 0 || report.Issues[0].Code != AuditIssueStoreEntry {
		t.Fatalf("audit report = %#v", report)
	}
	if _, err := os.Stat(unexpected); err != nil {
		t.Fatalf("audit deleted unexpected root entry: %v", err)
	}
}

func TestAuditRejectsClosedStore(t *testing.T) {
	base := canonicalTempDir(t)
	store, err := Open(Options{Directory: filepath.Join(base, "store"), Limits: backupStoreTestLimits()})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = store.Audit(context.Background(), AuditOptions{Mode: AuditQuick})
	if operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("closed-store audit error = %v, want CONFLICT", err)
	}
}

func TestAuditCancellationIsTypedAndReadOnly(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	target := filepath.Join(base, "target.txt")
	if err := os.WriteFile(target, []byte(strings.Repeat("x", 1024*1024)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}); err != nil {
		t.Fatal(err)
	}
	before := store.Index()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := store.Audit(ctx, AuditOptions{Mode: AuditFull})
	if err == nil {
		t.Fatal("cancelled audit unexpectedly succeeded")
	}
	after := store.Index()
	if before.Generation != after.Generation || before.ManifestCount != after.ManifestCount {
		t.Fatalf("audit cancellation changed index: before=%#v after=%#v", before, after)
	}
}
