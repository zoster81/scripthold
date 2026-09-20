package backupstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestScanStorePreservesIssuePhaseOrderAndTruncation(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())

	invalidManifest := filepath.Join(store.Root(), "manifests", "invalid.json")
	if err := os.WriteFile(invalidManifest, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(invalidManifest, false); err != nil {
		t.Fatal(err)
	}
	invalidShard := filepath.Join(store.Root(), "objects", "sha256", "zz")
	if err := os.Mkdir(invalidShard, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(invalidShard, true); err != nil {
		t.Fatal(err)
	}

	result, err := scanStore(context.Background(), store.root, store.descriptor, scanOptions{
		mode: AuditQuick, maxObjects: 256, maxBytes: store.limits.MaxTotalBytes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.report.Issues) < 2 ||
		result.report.Issues[0].Code != AuditIssueManifest ||
		result.report.Issues[1].Code != AuditIssueStoreEntry {
		t.Fatalf("issue order = %#v, want manifest before object-store issue", result.report.Issues)
	}

	for index := 0; index < maxAuditIssues+4; index++ {
		path := filepath.Join(store.Root(), "manifests", fmt.Sprintf("bad-%03d", index))
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := restrictPathPermissions(path, false); err != nil {
			t.Fatal(err)
		}
	}
	result, err = scanStore(context.Background(), store.root, store.descriptor, scanOptions{
		mode: AuditQuick, maxObjects: maxAuditIssues + 16, maxBytes: store.limits.MaxTotalBytes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(result.report.Issues), maxAuditIssues+1; got != want {
		t.Fatalf("issue count = %d, want %d", got, want)
	}
	last := result.report.Issues[len(result.report.Issues)-1]
	if last.Code != AuditIssueLimit || last.Message != "additional audit issues were truncated" {
		t.Fatalf("truncation issue = %#v", last)
	}
}

func TestScanStorePreservesCancellationOperationMetadata(t *testing.T) {
	t.Run("manifest phase", func(t *testing.T) {
		base := canonicalTempDir(t)
		store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
		target := filepath.Join(base, "target.txt")
		if err := os.WriteFile(target, []byte("manifest"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}); err != nil {
			t.Fatal(err)
		}

		_, err := scanStore(&cancelAfterErrContext{cancelAt: 2}, store.root, store.descriptor, scanOptions{
			mode: AuditQuick, maxObjects: 32, maxBytes: store.limits.MaxTotalBytes,
		})
		assertScanCancellationOperation(t, err, "scan_backup_manifests")
	})

	t.Run("object phase", func(t *testing.T) {
		base := canonicalTempDir(t)
		store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
		shard := filepath.Join(store.Root(), "objects", "sha256", "aa")
		if err := os.Mkdir(shard, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := restrictPathPermissions(shard, true); err != nil {
			t.Fatal(err)
		}

		_, err := scanStore(&cancelAfterErrContext{cancelAt: 2}, store.root, store.descriptor, scanOptions{
			mode: AuditQuick, maxObjects: 32, maxBytes: store.limits.MaxTotalBytes,
		})
		assertScanCancellationOperation(t, err, "scan_backup_objects")
	})
}

func TestScanStoreCheckIndexOptionRemainsExplicit(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	indexPath := filepath.Join(store.Root(), "index", "index-v1.json")
	if err := os.Remove(indexPath); err != nil {
		t.Fatal(err)
	}

	withoutIndexCheck, err := scanStore(context.Background(), store.root, store.descriptor, scanOptions{
		mode: AuditQuick, maxObjects: 32, maxBytes: store.limits.MaxTotalBytes, checkIndex: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !withoutIndexCheck.report.Healthy || !withoutIndexCheck.report.IndexConsistent {
		t.Fatalf("checkIndex=false report = %#v", withoutIndexCheck.report)
	}

	withIndexCheck, err := scanStore(context.Background(), store.root, store.descriptor, scanOptions{
		mode: AuditQuick, maxObjects: 32, maxBytes: store.limits.MaxTotalBytes, checkIndex: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if withIndexCheck.report.Healthy || withIndexCheck.report.IndexConsistent {
		t.Fatalf("checkIndex=true report = %#v", withIndexCheck.report)
	}
	foundIndexIssue := false
	for _, issue := range withIndexCheck.report.Issues {
		if issue.Code == AuditIssueIndex {
			foundIndexIssue = true
			break
		}
	}
	if !foundIndexIssue {
		t.Fatalf("checkIndex=true omitted index issue: %#v", withIndexCheck.report.Issues)
	}
}

type cancelAfterErrContext struct {
	calls    int
	cancelAt int
}

func (ctx *cancelAfterErrContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *cancelAfterErrContext) Done() <-chan struct{}       { return nil }
func (ctx *cancelAfterErrContext) Value(any) any               { return nil }
func (ctx *cancelAfterErrContext) Err() error {
	ctx.calls++
	if ctx.calls >= ctx.cancelAt {
		return context.Canceled
	}
	return nil
}

func assertScanCancellationOperation(t *testing.T, err error, want string) {
	t.Helper()
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("scan error = %v, want CANCELLED", err)
	}
	var typed *operation.Error
	if !errors.As(err, &typed) || typed.Operation != want {
		t.Fatalf("scan error metadata = %#v, want operation %q", typed, want)
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
