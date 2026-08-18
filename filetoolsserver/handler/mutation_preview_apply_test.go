package handler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

func TestByteMutationPreviewStoreIsBoundedOneShotAndExpiring(t *testing.T) {
	store := newByteMutationPreviewStore(1, 4096, time.Minute)
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	first, err := store.put(preparedByteMutation{
		kind: byteMutationKindBOM,
		targets: []preparedByteMutationTarget{{
			requestedPath: "/first",
			resolvedPath:  "/first",
			data:          []byte("first"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.put(preparedByteMutation{
		kind: byteMutationKindEncoding,
		targets: []preparedByteMutationTarget{{
			requestedPath: "/second",
			resolvedPath:  "/second",
			data:          []byte("second"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.claim(first.id, byteMutationKindBOM); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("claim(evicted) error=%v, want CONFLICT", err)
	}
	if _, err := store.claim(second.id, byteMutationKindBOM); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("claim(wrong kind) error=%v, want CONFLICT", err)
	}
	if _, err := store.claim(second.id, byteMutationKindEncoding); err != nil {
		t.Fatalf("claim(correct kind): %v", err)
	}
	if _, err := store.claim(second.id, byteMutationKindEncoding); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("replay error=%v, want CONFLICT", err)
	}

	expiring, err := store.put(preparedByteMutation{
		kind: byteMutationKindBOM,
		targets: []preparedByteMutationTarget{{
			requestedPath: "/expiring",
			resolvedPath:  "/expiring",
			data:          []byte("value"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := store.claim(expiring.id, byteMutationKindBOM); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("expired claim error=%v, want CONFLICT", err)
	}
}

func TestByteMutationPreviewConcurrentClaimSucceedsOnce(t *testing.T) {
	store := newByteMutationPreviewStore(2, 4096, time.Minute)
	preview, err := store.put(preparedByteMutation{
		kind: byteMutationKindEncoding,
		targets: []preparedByteMutationTarget{{
			requestedPath: "/concurrent",
			resolvedPath:  "/concurrent",
			data:          []byte("value"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, claimErr := store.claim(preview.id, byteMutationKindEncoding)
			results <- claimErr
		}()
	}
	close(start)

	successes := 0
	conflicts := 0
	for range 2 {
		claimErr := <-results
		if claimErr == nil {
			successes++
			continue
		}
		if operation.KindOf(claimErr) == operation.KindConflict {
			conflicts++
			continue
		}
		t.Fatalf("unexpected concurrent claim error: %v", claimErr)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent claims successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}
}
func TestManageBOMRequiredDefaultNoOpNeedsNoStoreAndChangedPreviewFailsClosed(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "plain.txt")
	original := []byte("alpha\r\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.LoadFromEnvironment(func(string) string { return "" })
	cfg.Backup.DefaultPolicy = config.BackupPolicyRequired
	h := NewHandler([]string{root}, WithConfig(cfg))

	previewResult, preview, err := h.HandleManageBOMRead(context.Background(), nil, ManageBOMReadInput{
		Path: path, Action: manageBOMActionStripPreview,
	})
	if err != nil || previewResult.IsError || preview.Changed || preview.BackupPolicy != editBackupPolicyRequired || len(preview.PreviewID) != 64 {
		t.Fatalf("no-op preview result=%+v output=%+v err=%v", previewResult, preview, err)
	}
	applyResult, applied, err := h.HandleManageBOMApply(context.Background(), nil, PreviewApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || applied.Applied || applied.Changed || applied.BackupID != "" {
		t.Fatalf("no-op apply result=%+v output=%+v err=%v", applyResult, applied, err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, original) || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("no-op changed target: bytes=%x before=%s after=%s", actual, before.ModTime(), after.ModTime())
	}

	changedResult, changed, err := h.HandleManageBOMRead(context.Background(), nil, ManageBOMReadInput{
		Path: path, Action: manageBOMActionAddPreview, Encoding: "utf-8",
	})
	if err != nil || !changedResult.IsError || changedResult.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput || changed.PreviewID != "" {
		t.Fatalf("required changed preview result=%+v output=%+v err=%v", changedResult, changed, err)
	}
	actual, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, original) {
		t.Fatalf("failed preview changed target=%x err=%v", actual, err)
	}
}

func TestManageBOMCapabilityBindsKindIdentityBytesAndReplay(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "target.txt")
	original := []byte("alpha\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})

	_, preview, err := h.HandleManageBOMRead(context.Background(), nil, ManageBOMReadInput{
		Path: path, Action: manageBOMActionAddPreview, Encoding: "utf-8",
	})
	if err != nil || len(preview.PreviewID) != 64 || !preview.Changed {
		t.Fatalf("BOM preview=%+v err=%v", preview, err)
	}
	wrongKind, _, err := h.HandleConvertEncodingApply(context.Background(), nil, PreviewApplyInput{PreviewID: preview.PreviewID})
	if err != nil || !wrongKind.IsError || wrongKind.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("wrong-kind apply result=%+v err=%v", wrongKind, err)
	}

	replacement := filepath.Join(root, "replacement.txt")
	if err := os.WriteFile(replacement, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	stale, staleOutput, err := h.HandleManageBOMApply(context.Background(), nil, PreviewApplyInput{PreviewID: preview.PreviewID})
	if err != nil || !stale.IsError || stale.Meta[ErrorCodeMetaKey] != ErrCodeConflict || staleOutput.Applied {
		t.Fatalf("same-content replacement apply result=%+v output=%+v err=%v", stale, staleOutput, err)
	}

	_, fresh, err := h.HandleManageBOMRead(context.Background(), nil, ManageBOMReadInput{
		Path: path, Action: manageBOMActionAddPreview, Encoding: "utf-8",
	})
	if err != nil {
		t.Fatal(err)
	}
	applyResult, applied, err := h.HandleManageBOMApply(context.Background(), nil, PreviewApplyInput{PreviewID: fresh.PreviewID})
	if err != nil || applyResult.IsError || !applied.Applied {
		t.Fatalf("fresh apply result=%+v output=%+v err=%v", applyResult, applied, err)
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte{0xef, 0xbb, 0xbf}, original...)
	if !bytes.Equal(actual, want) || filesystem.FingerprintRegularFileData(actual) != fresh.ResultFingerprint {
		t.Fatalf("applied BOM bytes=%x fingerprint=%s wantFingerprint=%s", actual, filesystem.FingerprintRegularFileData(actual), fresh.ResultFingerprint)
	}
	replay, _, err := h.HandleManageBOMApply(context.Background(), nil, PreviewApplyInput{PreviewID: fresh.PreviewID})
	if err != nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("replay result=%+v err=%v", replay, err)
	}
}

func TestConvertEncodingPreviewIsSideEffectFreeAndApplyUsesExactBytes(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "encoding.txt")
	original := []byte("hello\r\n€\r\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	previewResult, preview, err := h.HandleConvertEncodingPreview(context.Background(), nil, ConvertEncodingPreviewInput{
		Path: path, From: "utf-8", To: "utf-16-le", BOM: "auto", DryRun: true, Backup: true,
	})
	if err != nil || previewResult.IsError || len(preview.PreviewID) != 64 || !preview.Changed || len(preview.ResultFingerprint) != 64 {
		t.Fatalf("conversion preview result=%+v output=%+v err=%v", previewResult, preview, err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, original) {
		t.Fatalf("preview changed target=%x err=%v", actual, err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("preview created adjacent backup: %v", err)
	}

	applyResult, applied, err := h.HandleConvertEncodingApply(context.Background(), nil, PreviewApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !applied.Applied || applied.CommittedCount != 1 {
		t.Fatalf("conversion apply result=%+v output=%+v err=%v", applyResult, applied, err)
	}
	converted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if filesystem.FingerprintRegularFileData(converted) != preview.ResultFingerprint {
		t.Fatalf("converted fingerprint=%s want=%s", filesystem.FingerprintRegularFileData(converted), preview.ResultFingerprint)
	}
	backupBytes, err := os.ReadFile(path + ".bak")
	if err != nil || !bytes.Equal(backupBytes, original) {
		t.Fatalf("adjacent backup=%x err=%v", backupBytes, err)
	}
	readResult, read, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: path, Encoding: "utf-16-le"})
	if err != nil || readResult.IsError || read.Content != string(original) || !read.HasBOM {
		t.Fatalf("converted read result=%+v output=%+v err=%v", readResult, read, err)
	}
	replay, _, err := h.HandleConvertEncodingApply(context.Background(), nil, PreviewApplyInput{PreviewID: preview.PreviewID})
	if err != nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("conversion replay result=%+v err=%v", replay, err)
	}
}

func TestBackupHistoryAndCompareAreAuthorizedAndBounded(t *testing.T) {
	fixture := newBackupStoreHandlerFixture(t)
	target := filepath.Join(fixture.publicRoot, "versioned.txt")
	first := fixture.capture(t, target, "alpha\n", false)
	second := fixture.capture(t, target, "beta\n", false)
	other := fixture.capture(t, filepath.Join(fixture.publicRoot, "other.txt"), "other\n", false)

	firstPageResult, firstPage, err := fixture.handler.HandleBackupStoreRead(context.Background(), nil, BackupStoreReadInput{
		Action: BackupStoreActionHistory, TargetPath: target, Limit: 1,
	})
	if err != nil || firstPageResult.IsError || len(firstPage.Items) != 1 || firstPage.NextCursor == "" {
		t.Fatalf("history first page result=%+v output=%+v err=%v", firstPageResult, firstPage, err)
	}
	secondPageResult, secondPage, err := fixture.handler.HandleBackupStoreRead(context.Background(), nil, BackupStoreReadInput{
		Action: BackupStoreActionHistory, TargetPath: target, Limit: 1, Cursor: firstPage.NextCursor,
	})
	if err != nil || secondPageResult.IsError || len(secondPage.Items) != 1 || secondPage.Items[0].BackupID == firstPage.Items[0].BackupID {
		t.Fatalf("history second page result=%+v output=%+v err=%v", secondPageResult, secondPage, err)
	}
	for _, item := range append(firstPage.Items, secondPage.Items...) {
		if item.TargetPath != target {
			t.Fatalf("history leaked another target: %+v", item)
		}
	}

	currentResult, current, err := fixture.handler.HandleBackupStoreRead(context.Background(), nil, BackupStoreReadInput{
		Action: BackupStoreActionCompare, BackupID: first.Manifest.BackupID,
	})
	if err != nil || currentResult.IsError || current.Compare == nil || current.Compare.Equal ||
		!current.Compare.BackupObjectVerified || !current.Compare.OtherObjectVerified || !current.Compare.DiffAvailable || current.Compare.OtherKind != "current" {
		t.Fatalf("backup/current compare result=%+v output=%+v err=%v", currentResult, current, err)
	}
	backupResult, compared, err := fixture.handler.HandleBackupStoreRead(context.Background(), nil, BackupStoreReadInput{
		Action: BackupStoreActionCompare, BackupID: first.Manifest.BackupID, OtherBackupID: second.Manifest.BackupID,
	})
	if err != nil || backupResult.IsError || compared.Compare == nil || compared.Compare.Equal || !compared.Compare.DiffAvailable || compared.Compare.OtherKind != "backup" {
		t.Fatalf("backup/backup compare result=%+v output=%+v err=%v", backupResult, compared, err)
	}
	crossResult, _, err := fixture.handler.HandleBackupStoreRead(context.Background(), nil, BackupStoreReadInput{
		Action: BackupStoreActionCompare, BackupID: first.Manifest.BackupID, OtherBackupID: other.Manifest.BackupID,
	})
	if err != nil || !crossResult.IsError || crossResult.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("cross-target compare result=%+v err=%v", crossResult, err)
	}

	outside := filepath.Join(filepath.Dir(fixture.publicRoot), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideCapture, err := fixture.store.Capture(context.Background(), backupstore.CaptureRequest{TargetPath: outside, SourceOperation: backupstore.SourceOperationEdit})
	if err != nil {
		t.Fatal(err)
	}
	denied, _, err := fixture.handler.HandleBackupStoreRead(context.Background(), nil, BackupStoreReadInput{
		Action: BackupStoreActionCompare, BackupID: outsideCapture.Manifest.BackupID,
	})
	if err != nil || !denied.IsError || denied.Meta[ErrorCodeMetaKey] != ErrCodeAccessDenied {
		t.Fatalf("outside compare result=%+v err=%v", denied, err)
	}
}

func TestBackupCompareBinaryKeepsFingerprintsWithoutDiff(t *testing.T) {
	fixture := newBackupStoreHandlerFixture(t)
	target := filepath.Join(fixture.publicRoot, "binary.bin")
	firstBytes := []byte{0x00, 0x01, 0x02, 0xff}
	secondBytes := []byte{0x00, 0x01, 0x03, 0xff}
	if err := os.WriteFile(target, firstBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := fixture.store.Capture(context.Background(), backupstore.CaptureRequest{TargetPath: target, SourceOperation: backupstore.SourceOperationEdit})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, secondBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	result, output, err := fixture.handler.HandleBackupStoreRead(context.Background(), nil, BackupStoreReadInput{
		Action: BackupStoreActionCompare, BackupID: first.Manifest.BackupID,
	})
	if err != nil || result.IsError || output.Compare == nil || output.Compare.Equal || output.Compare.DiffAvailable || output.Compare.Diff != "" ||
		len(output.Compare.BackupFingerprint) != 64 || len(output.Compare.OtherFingerprint) != 64 {
		t.Fatalf("binary compare result=%+v output=%+v err=%v", result, output, err)
	}
}

func TestPreviewApplyInputsRejectMutationOverrides(t *testing.T) {
	for _, raw := range []string{
		`{"previewId":"` + strings.Repeat("a", 64) + `","path":"target"}`,
		`{"previewId":"` + strings.Repeat("a", 64) + `","backupPolicy":"required"}`,
		`{"previewId":"` + strings.Repeat("a", 64) + `"}{}`,
	} {
		var input PreviewApplyInput
		if err := input.UnmarshalJSON([]byte(raw)); err == nil {
			t.Fatalf("apply input accepted forbidden data: %s", raw)
		}
	}
}
