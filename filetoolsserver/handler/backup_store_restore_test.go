package handler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

func TestBackupStoreRestoreExistingTargetCreatesSafetyBackupBeforeApply(t *testing.T) {
	fixture := newBackupRestoreFixture(t, backupstore.Limits{})
	backupID := fixture.captureOriginal(t, "original bytes\n")
	if err := os.WriteFile(fixture.target, []byte("current bytes\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	previewResult, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:   BackupStoreActionRestorePreview,
		BackupID: backupID,
	})
	if err != nil || previewResult.IsError || preview.Restore == nil {
		t.Fatalf("restore preview result=%+v output=%+v err=%v", previewResult, preview, err)
	}
	if len(preview.Restore.PreviewID) != 64 || preview.Restore.BackupID != backupID || !preview.Restore.TargetExisted ||
		preview.Restore.State != BackupStoreRestoreStatePrepared || preview.Restore.Applied ||
		preview.Restore.CurrentFingerprint != filesystem.FingerprintRegularFileData([]byte("current bytes\n")) ||
		preview.Restore.ResultFingerprint != filesystem.FingerprintRegularFileData([]byte("original bytes\n")) {
		t.Fatalf("restore preview = %+v", preview.Restore)
	}
	assertRestoreTargetBytes(t, fixture.target, "current bytes\n")
	if fixture.store.Index().ManifestCount != 1 {
		t.Fatalf("preview changed store: %+v", fixture.store.Index())
	}

	applyResult, applied, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionRestoreApply,
		PreviewID: preview.Restore.PreviewID,
	})
	if err != nil || applyResult.IsError || applied.Restore == nil {
		t.Fatalf("restore apply result=%+v output=%+v err=%v", applyResult, applied, err)
	}
	if !applied.Restore.Applied || applied.Restore.State != BackupStoreRestoreStateRestored ||
		len(applied.Restore.SafetyBackupID) != 64 || applied.Restore.ActualFingerprint != preview.Restore.ResultFingerprint {
		t.Fatalf("restore apply = %+v", applied.Restore)
	}
	assertRestoreTargetBytes(t, fixture.target, "original bytes\n")
	if fixture.store.Index().ManifestCount != 2 {
		t.Fatalf("manifest count=%d, want 2", fixture.store.Index().ManifestCount)
	}
	safety, err := fixture.store.Inspect(context.Background(), applied.Restore.SafetyBackupID, backupstore.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if safety.Manifest.SourceOperation != backupstore.SourceOperationRestore ||
		safety.Manifest.ContentFingerprint != filesystem.FingerprintRegularFileData([]byte("current bytes\n")) {
		t.Fatalf("safety manifest = %+v", safety.Manifest)
	}

	replay, _, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionRestoreApply,
		PreviewID: preview.Restore.PreviewID,
	})
	if err != nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("restore replay result=%+v err=%v", replay, err)
	}
}

func TestBackupStoreRestoreMissingOriginalTargetUsesNoReplaceWithoutSafetyBackup(t *testing.T) {
	fixture := newBackupRestoreFixture(t, backupstore.Limits{})
	backupID := fixture.captureOriginal(t, "recreated bytes")
	if err := os.Remove(fixture.target); err != nil {
		t.Fatal(err)
	}

	_, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:   BackupStoreActionRestorePreview,
		BackupID: backupID,
	})
	if err != nil || preview.Restore == nil || preview.Restore.TargetExisted || preview.Restore.CurrentFingerprint != "" {
		t.Fatalf("missing preview=%+v err=%v", preview, err)
	}
	result, applied, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionRestoreApply,
		PreviewID: preview.Restore.PreviewID,
	})
	if err != nil || result.IsError || applied.Restore == nil || !applied.Restore.Applied || applied.Restore.SafetyBackupID != "" {
		t.Fatalf("missing apply result=%+v output=%+v err=%v", result, applied, err)
	}
	assertRestoreTargetBytes(t, fixture.target, "recreated bytes")
	if fixture.store.Index().ManifestCount != 1 {
		t.Fatalf("missing-target restore created safety manifest: %+v", fixture.store.Index())
	}
}

func TestBackupStoreRestoreRejectsStaleTargetBeforeSafetyBackup(t *testing.T) {
	fixture := newBackupRestoreFixture(t, backupstore.Limits{})
	backupID := fixture.captureOriginal(t, "original")
	if err := os.WriteFile(fixture.target, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:   BackupStoreActionRestorePreview,
		BackupID: backupID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.target, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, output, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionRestoreApply,
		PreviewID: preview.Restore.PreviewID,
	})
	if err != nil || !result.IsError || output.Restore == nil || output.Restore.SafetyBackupID != "" || output.Restore.Applied {
		t.Fatalf("stale apply result=%+v output=%+v err=%v", result, output, err)
	}
	assertRestoreTargetBytes(t, fixture.target, "external")
	if fixture.store.Index().ManifestCount != 1 {
		t.Fatalf("stale apply created safety backup: %+v", fixture.store.Index())
	}
}

func TestBackupStoreRestoreQuotaFailurePreventsMutation(t *testing.T) {
	limits := backupstore.Limits{MaxTotalBytes: 1024, MaxObjectBytes: 1024, MaxManifests: 1, MaxVersionsPerTarget: 4, MaxPinned: 4}
	fixture := newBackupRestoreFixture(t, limits)
	backupID := fixture.captureOriginal(t, "original")
	if err := os.WriteFile(fixture.target, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, _, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:   BackupStoreActionRestorePreview,
		BackupID: backupID,
	})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("quota preview result=%+v err=%v", result, err)
	}
	assertRestoreTargetBytes(t, fixture.target, "current")
	if fixture.store.Index().ManifestCount != 1 {
		t.Fatalf("quota preview changed store: %+v", fixture.store.Index())
	}
}

func TestBackupStoreRestoreReverifiesObjectAfterPreview(t *testing.T) {
	fixture := newBackupRestoreFixture(t, backupstore.Limits{})
	backupID := fixture.captureOriginal(t, "source bytes")
	if err := os.WriteFile(fixture.target, []byte("current data"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:   BackupStoreActionRestorePreview,
		BackupID: backupID,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := fixture.store.Inspect(context.Background(), backupID, backupstore.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	object := restoreObjectPathForTest(fixture.store.Root(), manifest.Manifest.ObjectDigest)
	corrupt := []byte(strings.ToUpper("source bytes"))
	if err := os.WriteFile(object, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	result, output, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionRestoreApply,
		PreviewID: preview.Restore.PreviewID,
	})
	if err != nil || !result.IsError || output.Restore == nil || output.Restore.SafetyBackupID != "" || output.Restore.Applied {
		t.Fatalf("corrupt source apply result=%+v output=%+v err=%v", result, output, err)
	}
	assertRestoreTargetBytes(t, fixture.target, "current data")
	if fixture.store.Index().ManifestCount != 1 {
		t.Fatalf("corrupt source apply created safety backup: %+v", fixture.store.Index())
	}
}

func TestBackupStoreRestorePostCommitFailurePreservesSafetyBackupAndClassification(t *testing.T) {
	fixture := newBackupRestoreFixture(t, backupstore.Limits{})
	backupID := fixture.captureOriginal(t, "original")
	if err := os.WriteFile(fixture.target, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:   BackupStoreActionRestorePreview,
		BackupID: backupID,
	})
	if err != nil {
		t.Fatal(err)
	}
	originalCommit := fixture.handler.restoreCommitReplacement
	fixture.handler.restoreCommitReplacement = func(staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
		changed, commitErr := originalCommit(staged, options)
		if commitErr != nil {
			return changed, commitErr
		}
		return changed, errors.New("injected post-commit failure")
	}
	result, output, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionRestoreApply,
		PreviewID: preview.Restore.PreviewID,
	})
	if err != nil || !result.IsError || output.Restore == nil || len(output.Restore.SafetyBackupID) != 64 ||
		output.Restore.State != BackupStoreRestoreStateRestored || !output.Restore.Applied {
		t.Fatalf("post-commit result=%+v output=%+v err=%v", result, output, err)
	}
	assertRestoreTargetBytes(t, fixture.target, "original")
}

func TestBackupStoreRestoreDetectsTargetChangeAfterSafetyBackup(t *testing.T) {
	fixture := newBackupRestoreFixture(t, backupstore.Limits{})
	backupID := fixture.captureOriginal(t, "original")
	if err := os.WriteFile(fixture.target, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	wrapper := &restoreCaptureStoreWrapper{Store: fixture.store}
	wrapper.afterCapture = func() error {
		return os.WriteFile(fixture.target, []byte("external"), 0o600)
	}
	fixture.handler = NewHandler([]string{fixture.publicRoot}, WithBackupStore(wrapper))
	stageCalled := false
	originalStage := fixture.handler.restoreStageReplacement
	fixture.handler.restoreStageReplacement = func(ctx context.Context, source *backupstore.ReadSource, target string, mode os.FileMode, modTime *time.Time) (*filesystem.StagedReplacement, error) {
		stageCalled = true
		return originalStage(ctx, source, target, mode, modTime)
	}
	_, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:   BackupStoreActionRestorePreview,
		BackupID: backupID,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, output, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionRestoreApply,
		PreviewID: preview.Restore.PreviewID,
	})
	if err != nil || !result.IsError || output.Restore == nil || len(output.Restore.SafetyBackupID) != 64 || output.Restore.Applied || stageCalled {
		t.Fatalf("post-safety change result=%+v output=%+v stageCalled=%t err=%v", result, output, stageCalled, err)
	}
	assertRestoreTargetBytes(t, fixture.target, "external")
	if fixture.store.Index().ManifestCount != 2 {
		t.Fatalf("safety backup was not durable: %+v", fixture.store.Index())
	}
}

func TestBackupStoreRestoreMissingTargetAppearanceFailsNoReplace(t *testing.T) {
	fixture := newBackupRestoreFixture(t, backupstore.Limits{})
	backupID := fixture.captureOriginal(t, "original")
	if err := os.Remove(fixture.target); err != nil {
		t.Fatal(err)
	}
	_, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:   BackupStoreActionRestorePreview,
		BackupID: backupID,
	})
	if err != nil {
		t.Fatal(err)
	}
	originalStage := fixture.handler.restoreStageReplacement
	fixture.handler.restoreStageReplacement = func(ctx context.Context, source *backupstore.ReadSource, target string, mode os.FileMode, modTime *time.Time) (*filesystem.StagedReplacement, error) {
		staged, stageErr := originalStage(ctx, source, target, mode, modTime)
		if stageErr != nil {
			return nil, stageErr
		}
		if writeErr := os.WriteFile(target, []byte("racer"), 0o600); writeErr != nil {
			_ = staged.Cleanup()
			return nil, writeErr
		}
		return staged, nil
	}
	result, output, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionRestoreApply,
		PreviewID: preview.Restore.PreviewID,
	})
	if err != nil || !result.IsError || output.Restore == nil || output.Restore.SafetyBackupID != "" || output.Restore.Applied {
		t.Fatalf("appeared target result=%+v output=%+v err=%v", result, output, err)
	}
	assertRestoreTargetBytes(t, fixture.target, "racer")
}

func TestBackupStoreRestoreReadOnlyTargetAfterSafetyBackup(t *testing.T) {
	fixture := newBackupRestoreFixture(t, backupstore.Limits{})
	backupID := fixture.captureOriginal(t, "original")
	if err := os.WriteFile(fixture.target, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(fixture.target, 0o444); err != nil {
		t.Fatal(err)
	}
	_, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:   BackupStoreActionRestorePreview,
		BackupID: backupID,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, output, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionRestoreApply,
		PreviewID: preview.Restore.PreviewID,
	})
	if err != nil || result.IsError || output.Restore == nil || !output.Restore.Applied || !output.Restore.ReadOnlyCleared || len(output.Restore.SafetyBackupID) != 64 {
		t.Fatalf("read-only restore result=%+v output=%+v err=%v", result, output, err)
	}
	assertRestoreTargetBytes(t, fixture.target, "original")
}

func TestBackupStoreRestoreAuthorizationChangeConsumesPreviewWithoutMutation(t *testing.T) {
	fixture := newBackupRestoreFixture(t, backupstore.Limits{})
	backupID := fixture.captureOriginal(t, "original")
	if err := os.WriteFile(fixture.target, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:   BackupStoreActionRestorePreview,
		BackupID: backupID,
	})
	if err != nil {
		t.Fatal(err)
	}
	newRoot := filepath.Join(filepath.Dir(fixture.publicRoot), "new-root")
	if err := os.Mkdir(newRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture.handler.UpdateAllowedDirectories([]string{newRoot})
	result, output, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionRestoreApply,
		PreviewID: preview.Restore.PreviewID,
	})
	if err != nil || !result.IsError || output.Restore == nil || output.Restore.SafetyBackupID != "" {
		t.Fatalf("authorization change result=%+v output=%+v err=%v", result, output, err)
	}
	assertRestoreTargetBytes(t, fixture.target, "current")
	replay, _, _ := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionRestoreApply, PreviewID: preview.Restore.PreviewID})
	if !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("authorization-failed preview was replayable: %+v", replay)
	}
}

func TestBackupStoreRestoreQuotaCanChangeAfterPreview(t *testing.T) {
	limits := backupstore.Limits{MaxTotalBytes: 1024, MaxObjectBytes: 1024, MaxManifests: 2, MaxVersionsPerTarget: 4, MaxPinned: 4}
	fixture := newBackupRestoreFixture(t, limits)
	backupID := fixture.captureOriginal(t, "original")
	if err := os.WriteFile(fixture.target, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionRestorePreview, BackupID: backupID})
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(fixture.publicRoot, "other.txt")
	if err := os.WriteFile(other, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.Capture(context.Background(), backupstore.CaptureRequest{TargetPath: other, SourceOperation: backupstore.SourceOperationEdit}); err != nil {
		t.Fatal(err)
	}
	result, output, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionRestoreApply, PreviewID: preview.Restore.PreviewID})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit || output.Restore == nil || output.Restore.SafetyBackupID != "" {
		t.Fatalf("changed quota result=%+v output=%+v err=%v", result, output, err)
	}
	assertRestoreTargetBytes(t, fixture.target, "current")
}

func TestBackupStoreRestoreConcurrentApplyClaimsOnce(t *testing.T) {
	fixture := newBackupRestoreFixture(t, backupstore.Limits{})
	backupID := fixture.captureOriginal(t, "original")
	if err := os.WriteFile(fixture.target, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionRestorePreview, BackupID: backupID})
	if err != nil {
		t.Fatal(err)
	}
	type response struct {
		result *mcp.CallToolResult
		output BackupStoreOutput
		err    error
	}
	start := make(chan struct{})
	responses := make(chan response, 2)
	for range 2 {
		go func() {
			<-start
			result, output, callErr := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionRestoreApply, PreviewID: preview.Restore.PreviewID})
			responses <- response{result: result, output: output, err: callErr}
		}()
	}
	close(start)
	successes := 0
	conflicts := 0
	for range 2 {
		response := <-responses
		if response.err != nil {
			t.Fatal(response.err)
		}
		if response.result.IsError {
			if response.result.Meta[ErrorCodeMetaKey] == ErrCodeConflict {
				conflicts++
			}
		} else if response.output.Restore != nil && response.output.Restore.Applied {
			successes++
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent apply successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestBackupStoreRestoreBinaryPreviewOmitsDiffAndRestoresExactBytes(t *testing.T) {
	fixture := newBackupRestoreFixture(t, backupstore.Limits{})
	original := []byte{0x00, 0xff, 0x10, 0x20}
	if err := os.WriteFile(fixture.target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	captured, err := fixture.store.Capture(context.Background(), backupstore.CaptureRequest{TargetPath: fixture.target, SourceOperation: backupstore.SourceOperationEdit})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.target, []byte{0x01, 0x02, 0x03}, 0o600); err != nil {
		t.Fatal(err)
	}
	_, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionRestorePreview, BackupID: captured.Manifest.BackupID})
	if err != nil || preview.Restore == nil || preview.Restore.Diff != "" {
		t.Fatalf("binary preview=%+v err=%v", preview, err)
	}
	result, output, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionRestoreApply, PreviewID: preview.Restore.PreviewID})
	if err != nil || result.IsError || output.Restore == nil || !output.Restore.Applied {
		t.Fatalf("binary apply result=%+v output=%+v err=%v", result, output, err)
	}
	actual, err := os.ReadFile(fixture.target)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(original) {
		t.Fatalf("binary restored=%v want=%v", actual, original)
	}
}

func TestReadRestoreDiffBytesRejectsInputBeyondLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "growing.txt")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readRestoreDiffBytes(path, 4); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("readRestoreDiffBytes() error=%v, want LIMIT", err)
	}
}

func TestBackupStoreRestoreInputUnionAndOutputLimit(t *testing.T) {
	fixture := newBackupRestoreFixture(t, backupstore.Limits{})
	backupID := fixture.captureOriginal(t, "original")
	for _, input := range []BackupStoreInput{
		{Action: BackupStoreActionRestorePreview},
		{Action: BackupStoreActionRestorePreview, BackupID: backupID, Limit: 1},
		{Action: BackupStoreActionRestoreApply},
		{Action: BackupStoreActionRestoreApply, PreviewID: strings.Repeat("a", 64), BackupID: backupID},
		{Action: BackupStoreActionStatus, PreviewID: strings.Repeat("a", 64)},
	} {
		result, _, err := fixture.handler.HandleBackupStore(context.Background(), nil, input)
		if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid restore input=%+v result=%+v err=%v", input, result, err)
		}
	}

	if err := os.WriteFile(fixture.target, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture.handler.config.Limits.MaxOutputBytes = 1
	result, _, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:   BackupStoreActionRestorePreview,
		BackupID: backupID,
	})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("output-limited preview result=%+v err=%v", result, err)
	}
	if fixture.handler.restorePreviews.len() != 0 {
		t.Fatalf("output-limited preview retained a capability")
	}
}

type restoreCaptureStoreWrapper struct {
	*backupstore.Store
	afterCapture func() error
}

func (wrapper *restoreCaptureStoreWrapper) Capture(ctx context.Context, request backupstore.CaptureRequest) (backupstore.CaptureResult, error) {
	result, err := wrapper.Store.Capture(ctx, request)
	if result.Manifest.BackupID != "" && wrapper.afterCapture != nil {
		err = errors.Join(err, wrapper.afterCapture())
	}
	return result, err
}

type backupRestoreFixture struct {
	handler    *Handler
	store      *backupstore.Store
	publicRoot string
	target     string
}

func newBackupRestoreFixture(t *testing.T, limits backupstore.Limits) backupRestoreFixture {
	t.Helper()
	base := canonicalHandlerTestDir(t)
	publicRoot := filepath.Join(base, "public")
	if err := os.Mkdir(publicRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := backupstore.Open(backupstore.Options{
		Directory:                filepath.Join(base, "store"),
		PublicAllowedDirectories: []string{publicRoot},
		Limits:                   limits,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	h := NewHandler([]string{publicRoot}, WithBackupStore(store))
	return backupRestoreFixture{
		handler:    h,
		store:      store,
		publicRoot: publicRoot,
		target:     filepath.Join(publicRoot, "target.txt"),
	}
}

func (fixture backupRestoreFixture) captureOriginal(t *testing.T, content string) string {
	t.Helper()
	if err := os.WriteFile(fixture.target, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	captured, err := fixture.store.Capture(context.Background(), backupstore.CaptureRequest{
		TargetPath:      fixture.target,
		SourceOperation: backupstore.SourceOperationEdit,
	})
	if err != nil {
		t.Fatal(err)
	}
	return captured.Manifest.BackupID
}

func assertRestoreTargetBytes(t *testing.T, path, want string) {
	t.Helper()
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != want {
		t.Fatalf("target bytes=%q, want %q", actual, want)
	}
}

func restoreObjectPathForTest(root, digest string) string {
	return filepath.Join(root, "objects", backupstore.ObjectAlgorithm, digest[:2], digest)
}
