package backupstore

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestStatusReportsVerifiedBoundedStateWithoutStorePath(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	captureManagementFixture(t, store, filepath.Join(base, "target.txt"), "status bytes", true)

	status, err := store.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Healthy || status.FormatVersion != FormatVersion || status.ManifestVersion != ManifestVersion ||
		status.IndexVersion != IndexVersion || status.ObjectAlgorithm != ObjectAlgorithm {
		t.Fatalf("status identity/health = %#v", status)
	}
	if status.ManifestCount != 1 || status.ObjectCount != 1 || status.PinnedCount != 1 || status.TotalObjectBytes != int64(len("status bytes")) {
		t.Fatalf("status counts = %#v", status)
	}
	if status.Limits != backupStoreTestLimits() {
		t.Fatalf("status limits = %#v, want %#v", status.Limits, backupStoreTestLimits())
	}
	encoded := status.String()
	if strings.Contains(encoded, store.Root()) || strings.Contains(encoded, filepath.Join(store.Root(), "objects")) {
		t.Fatalf("status exposed an internal path: %s", encoded)
	}
}

func TestListIsGenerationBoundFilteredAndCursorProtected(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	firstPath := filepath.Join(base, "first.txt")
	secondPath := filepath.Join(base, "second.txt")
	first := captureManagementFixture(t, store, firstPath, "first", false)
	second := captureManagementFixture(t, store, secondPath, "second", true)
	third := captureManagementFixture(t, store, firstPath, "third", false)
	newestFirst := newestFirstManagementCaptures(first, second, third)

	page, err := store.List(context.Background(), ListOptions{Limit: 1})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].BackupID != newestFirst[0].Manifest.BackupID || page.NextCursor == "" {
		t.Fatalf("first page = %#v, want newest backup %s", page, newestFirst[0].Manifest.BackupID)
	}
	next, err := store.List(context.Background(), ListOptions{Limit: 1, Cursor: page.NextCursor})
	if err != nil {
		t.Fatalf("next List() error = %v", err)
	}
	if len(next.Items) != 1 || next.Items[0].BackupID != newestFirst[1].Manifest.BackupID {
		t.Fatalf("second page = %#v, want backup %s", next, newestFirst[1].Manifest.BackupID)
	}

	pinned := true
	filtered, err := store.List(context.Background(), ListOptions{Limit: 10, TargetPath: secondPath, Pinned: &pinned})
	if err != nil {
		t.Fatalf("filtered List() error = %v", err)
	}
	if len(filtered.Items) != 1 || filtered.Items[0].BackupID != second.Manifest.BackupID {
		t.Fatalf("filtered page = %#v", filtered)
	}
	unpinned := false
	filtered, err = store.List(context.Background(), ListOptions{Limit: 10, TargetPath: firstPath, Pinned: &unpinned})
	if err != nil {
		t.Fatalf("target List() error = %v", err)
	}
	firstPathNewest := newestFirstManagementCaptures(first, third)
	if len(filtered.Items) != 2 || filtered.Items[0].BackupID != firstPathNewest[0].Manifest.BackupID || filtered.Items[1].BackupID != firstPathNewest[1].Manifest.BackupID {
		t.Fatalf("target page = %#v, want backups %s then %s", filtered, firstPathNewest[0].Manifest.BackupID, firstPathNewest[1].Manifest.BackupID)
	}

	tampered := page.NextCursor[:len(page.NextCursor)-1] + differentCursorCharacter(page.NextCursor[len(page.NextCursor)-1])
	if _, err := store.List(context.Background(), ListOptions{Limit: 1, Cursor: tampered}); operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("tampered cursor error = %v, want INVALID_INPUT", err)
	}
	if _, err := store.List(context.Background(), ListOptions{Limit: 1, Cursor: page.NextCursor, TargetPath: firstPath}); operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("filter-swapped cursor error = %v, want INVALID_INPUT", err)
	}

	captureManagementFixture(t, store, filepath.Join(base, "fourth.txt"), "fourth", false)
	if _, err := store.List(context.Background(), ListOptions{Limit: 1, Cursor: page.NextCursor}); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("stale cursor error = %v, want CONFLICT", err)
	}
}

func TestListVisibilityPredicateRunsOutsideStoreTransaction(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	captureManagementFixture(t, store, filepath.Join(base, "target.txt"), "visible", false)

	result := make(chan error, 1)
	go func() {
		_, err := store.List(context.Background(), ListOptions{
			Limit: 1,
			TargetVisible: func(string) bool {
				_, statusErr := store.Status(context.Background())
				return statusErr == nil
			},
		})
		result <- err
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("List() invoked the visibility predicate while holding the store transaction lock")
	}
}

func TestListVisibilityPredicateAndScopeAreCursorBound(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	visiblePath := filepath.Join(base, "visible.txt")
	hiddenPath := filepath.Join(base, "hidden.txt")
	visibleFirst := captureManagementFixture(t, store, visiblePath, "visible", false)
	captureManagementFixture(t, store, hiddenPath, "hidden", false)
	visibleSecond := captureManagementFixture(t, store, visiblePath, "visible second", false)
	visibleNewestFirst := newestFirstManagementCaptures(visibleFirst, visibleSecond)

	visibleOnly := func(path string) bool { return path == visiblePath }
	page, err := store.List(context.Background(), ListOptions{
		Limit:           1,
		VisibilityScope: "scope-a",
		TargetVisible:   visibleOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.NextCursor == "" || page.Items[0].TargetPath != visiblePath {
		t.Fatalf("visible page = %#v", page)
	}
	next, err := store.List(context.Background(), ListOptions{
		Limit:           1,
		Cursor:          page.NextCursor,
		VisibilityScope: "scope-a",
		TargetVisible:   visibleOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 || next.Items[0].BackupID != visibleNewestFirst[1].Manifest.BackupID {
		t.Fatalf("visible next page = %#v, want backup %s", next, visibleNewestFirst[1].Manifest.BackupID)
	}
	if _, err := store.List(context.Background(), ListOptions{
		Limit:           1,
		Cursor:          page.NextCursor,
		VisibilityScope: "scope-b",
		TargetVisible:   visibleOnly,
	}); operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("scope-swapped cursor error = %v, want INVALID_INPUT", err)
	}
}

func TestListCursorUsesLastReturnedManifestInsteadOfVisibleOffset(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	firstPath := filepath.Join(base, "first.txt")
	secondPath := filepath.Join(base, "second.txt")
	thirdPath := filepath.Join(base, "third.txt")
	captures := newestFirstManagementCaptures(
		captureManagementFixture(t, store, firstPath, "first", false),
		captureManagementFixture(t, store, secondPath, "second", false),
		captureManagementFixture(t, store, thirdPath, "third", false),
	)
	newest := captures[0]
	oldest := captures[2]
	newestPath := newest.Manifest.TargetPath
	oldestPath := oldest.Manifest.TargetPath

	visible := map[string]bool{newestPath: true, oldestPath: true}
	predicate := func(path string) bool { return visible[path] }
	page, err := store.List(context.Background(), ListOptions{
		Limit:           1,
		VisibilityScope: "stable-scope",
		TargetVisible:   predicate,
	})
	if err != nil || len(page.Items) != 1 || page.Items[0].TargetPath != newestPath || page.NextCursor == "" {
		t.Fatalf("first page = %#v, err=%v", page, err)
	}

	visible[newestPath] = false
	next, err := store.List(context.Background(), ListOptions{
		Limit:           1,
		Cursor:          page.NextCursor,
		VisibilityScope: "stable-scope",
		TargetVisible:   predicate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 || next.Items[0].BackupID != oldest.Manifest.BackupID {
		t.Fatalf("next page after visibility change = %#v", next)
	}
}

func TestInspectAuthorizesOutsideTransactionBeforeObjectHash(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	capture := captureManagementFixture(t, store, filepath.Join(base, "target.txt"), "authorization before hash", false)
	object := objectPath(store.Root(), capture.Manifest.ObjectDigest)
	corrupt := []byte(strings.ToUpper("authorization before hash"))
	if len(corrupt) != len("authorization before hash") {
		t.Fatal("corruption fixture must preserve size")
	}
	if err := os.WriteFile(object, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(object, false); err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := store.Inspect(context.Background(), capture.Manifest.BackupID, InspectOptions{
			AuthorizeTarget: func(string) error {
				if _, statusErr := store.Status(context.Background()); statusErr != nil {
					return statusErr
				}
				return operation.New(operation.KindAccessDenied, "backup target is not currently authorized")
			},
		})
		result <- err
	}()
	select {
	case err := <-result:
		if operation.KindOf(err) != operation.KindAccessDenied {
			t.Fatalf("Inspect() error = %v, want ACCESS_DENIED before object verification", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Inspect() invoked authorization while holding the store transaction lock")
	}
}

func TestInspectReturnsVerifiedMetadataWithoutBytes(t *testing.T) {
	base := canonicalTempDir(t)
	store := openBackupTestStore(t, filepath.Join(base, "store"), backupStoreTestLimits())
	content := "inspect secret bytes"
	capture := captureManagementFixture(t, store, filepath.Join(base, "target.txt"), content, false)

	inspected, err := store.Inspect(context.Background(), capture.Manifest.BackupID, InspectOptions{})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if !inspected.ObjectVerified || inspected.Manifest != capture.Manifest {
		t.Fatalf("inspect result = %#v", inspected)
	}
	if strings.Contains(inspected.String(), content) || strings.Contains(inspected.String(), store.Root()) {
		t.Fatalf("inspect exposed bytes or store path: %s", inspected.String())
	}

	object := objectPath(store.Root(), capture.Manifest.ObjectDigest)
	corrupt := []byte(strings.ToUpper(content))
	if len(corrupt) != len(content) {
		t.Fatal("corruption fixture must preserve size")
	}
	if err := os.WriteFile(object, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(object, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Inspect(context.Background(), capture.Manifest.BackupID, InspectOptions{}); err == nil {
		t.Fatal("Inspect() accepted a corrupt object")
	}
}

func TestManagementOperationsValidateBoundsAndIdentifiers(t *testing.T) {
	base := canonicalTempDir(t)
	limits := backupStoreTestLimits()
	store := openBackupTestStore(t, filepath.Join(base, "store"), limits)

	if _, err := store.List(context.Background(), ListOptions{Limit: maxListPageSize + 1}); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("oversized list error = %v, want LIMIT", err)
	}
	if _, err := store.List(context.Background(), ListOptions{Limit: 1, Cursor: strings.Repeat("x", maxListCursorBytes+1)}); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("oversized cursor error = %v, want LIMIT", err)
	}
	if _, err := store.Inspect(context.Background(), "not-an-id", InspectOptions{}); operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("invalid backup ID error = %v, want INVALID_INPUT", err)
	}
	if _, err := store.Audit(context.Background(), AuditOptions{Mode: AuditFull, MaxObjects: limits.MaxManifests + 1}); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("oversized audit object limit error = %v, want LIMIT", err)
	}
	if _, err := store.Audit(context.Background(), AuditOptions{Mode: AuditFull, MaxBytes: limits.MaxTotalBytes + 1}); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("oversized audit byte limit error = %v, want LIMIT", err)
	}
}

func captureManagementFixture(t *testing.T, store *Store, target, content string, pinned bool) CaptureResult {
	t.Helper()
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := store.Capture(context.Background(), CaptureRequest{
		TargetPath:      target,
		SourceOperation: SourceOperationEdit,
		Pinned:          pinned,
	})
	if err != nil {
		t.Fatalf("Capture(%s) error = %v", filepath.Base(target), err)
	}
	return result
}

func newestFirstManagementCaptures(captures ...CaptureResult) []CaptureResult {
	ordered := append([]CaptureResult(nil), captures...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Manifest.CreatedAt != ordered[j].Manifest.CreatedAt {
			return ordered[i].Manifest.CreatedAt > ordered[j].Manifest.CreatedAt
		}
		return ordered[i].Manifest.BackupID > ordered[j].Manifest.BackupID
	})
	return ordered
}

func differentCursorCharacter(value byte) string {
	if value == 'A' {
		return "B"
	}
	return "A"
}
