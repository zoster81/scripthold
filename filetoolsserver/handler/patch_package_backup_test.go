package handler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestPatchPackageRequiredBackupCapturesAllChangedTargetsBeforeCommit(t *testing.T) {
	h, store, paths, manifest := newPatchPackageBackupFixture(t, backupstore.Limits{}, []patchPackageApplyFixture{
		{oldText: "alpha", newText: "omega"},
		{oldText: "beta", newText: "gamma"},
	})

	dryResult, dryRun, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	if err != nil || dryResult.IsError {
		t.Fatalf("dryRun result=%+v output=%+v err=%v", dryResult, dryRun, err)
	}
	if dryRun.BackupPolicy != editBackupPolicyRequired || dryRun.BackupCount != 0 || store.Index().ManifestCount != 0 {
		t.Fatalf("dryRun backup state output=%+v index=%+v", dryRun, store.Index())
	}

	originalCommit := h.patchPackageCommitReplacement
	var commits atomic.Int32
	h.patchPackageCommitReplacement = func(index int, staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
		if commits.Add(1) == 1 && store.Index().ManifestCount != 2 {
			t.Fatalf("first commit began with %d durable manifests, want 2", store.Index().ManifestCount)
		}
		return originalCommit(index, staged, options)
	}
	applyResult, applied, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || applyResult.IsError {
		t.Fatalf("apply result=%+v output=%+v err=%v", applyResult, applied, err)
	}
	if !applied.Applied || applied.BackupPolicy != editBackupPolicyRequired || applied.BackupCount != 2 {
		t.Fatalf("apply backup summary=%+v", applied)
	}
	for index := range applied.Results {
		if len(applied.Results[index].BackupID) != 64 {
			t.Fatalf("result %d backup ID=%q", index, applied.Results[index].BackupID)
		}
		inspected, inspectErr := store.Inspect(context.Background(), applied.Results[index].BackupID, backupstore.InspectOptions{})
		if inspectErr != nil || inspected.Manifest.TargetPath != paths[index] || inspected.Manifest.SourceOperation != backupstore.SourceOperationPatchPackage ||
			inspected.Manifest.ContentFingerprint != manifest.Targets[index].ExpectedFingerprint {
			t.Fatalf("result %d backup=%+v err=%v", index, inspected, inspectErr)
		}
	}
	assertFileBytes(t, paths[0], []byte("omega"))
	assertFileBytes(t, paths[1], []byte("gamma"))
}

func TestPatchPackageRequiredBackupStrictPolicyAndAvailability(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: path, oldText: "alpha", newText: "omega"}})
	h := NewHandler([]string{root})

	required := base
	required.BackupPolicy = editBackupPolicyRequired
	result, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: required})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("missing store result=%+v err=%v", result, err)
	}
	for _, policy := range []string{"optional", "Required", " required "} {
		invalid := base
		invalid.BackupPolicy = policy
		result, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionInspect, Manifest: invalid})
		if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("policy %q result=%+v err=%v", policy, result, err)
		}
	}
	omitted, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: base})
	if err != nil || omitted.IsError {
		t.Fatalf("omitted policy result=%+v err=%v", omitted, err)
	}
}

func TestPatchPackageRequiredBackupPreflightQuotaFailureCreatesNoState(t *testing.T) {
	limits := backupstore.Limits{MaxTotalBytes: 9, MaxObjectBytes: 16, MaxManifests: 8, MaxVersionsPerTarget: 8, MaxPinned: 8}
	h, store, paths, manifest := newPatchPackageBackupFixture(t, limits, []patchPackageApplyFixture{
		{oldText: "12345", newText: "first"},
		{oldText: "abcde", newText: "second"},
	})
	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("quota result=%+v output=%+v err=%v", result, output, err)
	}
	if store.Index().ManifestCount != 0 || store.Index().ObjectCount != 0 {
		t.Fatalf("quota preflight changed store: %+v", store.Index())
	}
	assertFileBytes(t, paths[0], []byte("12345"))
	assertFileBytes(t, paths[1], []byte("abcde"))
}

func TestPatchPackageRequiredBackupNoOpCapturesOnlyChangedTargets(t *testing.T) {
	h, store, _, manifest := newPatchPackageBackupFixture(t, backupstore.Limits{}, []patchPackageApplyFixture{
		{oldText: "alpha", newText: "alpha"},
		{oldText: "beta", newText: "gamma"},
	})
	_, dryRun, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	if err != nil {
		t.Fatal(err)
	}
	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || result.IsError {
		t.Fatalf("apply result=%+v output=%+v err=%v", result, output, err)
	}
	if output.BackupCount != 1 || output.Results[0].BackupID != "" || len(output.Results[1].BackupID) != 64 || store.Index().ManifestCount != 1 {
		t.Fatalf("no-op backup state output=%+v index=%+v", output, store.Index())
	}
}

func TestPatchPackageRequiredBackupFailureReturnsDurablePrefixWithoutCommit(t *testing.T) {
	_, store, paths, manifest := newPatchPackageBackupFixture(t, backupstore.Limits{}, []patchPackageApplyFixture{
		{oldText: "alpha", newText: "omega"},
		{oldText: "beta", newText: "gamma"},
	})
	wrapped := &patchPackageBackupStoreWrapper{Store: store}
	wrapped.captureBatch = func(ctx context.Context, requests []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error) {
		result, err := store.Capture(ctx, requests[0])
		if err != nil {
			return nil, err
		}
		return []backupstore.CaptureResult{result}, errors.New("injected second backup failure")
	}
	h := NewHandler([]string{filepath.Dir(paths[0])}, WithBackupStore(wrapped))
	_, dryRun, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	if err != nil {
		t.Fatal(err)
	}
	var commits atomic.Int32
	h.patchPackageCommitReplacement = func(int, *filesystem.StagedReplacement, filesystem.ReplaceOptions) (bool, error) {
		commits.Add(1)
		return false, errors.New("commit must not be reached")
	}
	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !result.IsError || output.Applied || output.PartialCommit || output.BackupCount != 1 || commits.Load() != 0 {
		t.Fatalf("backup failure result=%+v output=%+v commits=%d err=%v", result, output, commits.Load(), err)
	}
	if len(output.Results[0].BackupID) != 64 || output.Results[1].BackupID != "" || store.Index().ManifestCount != 1 {
		t.Fatalf("durable prefix output=%+v index=%+v", output, store.Index())
	}
	assertFileBytes(t, paths[0], []byte("alpha"))
	assertFileBytes(t, paths[1], []byte("beta"))
}

func TestPatchPackageRequiredBackupOutputLimitPreventsCapture(t *testing.T) {
	h, store, paths, manifest := newPatchPackageBackupFixture(t, backupstore.Limits{}, []patchPackageApplyFixture{{oldText: "alpha", newText: "omega"}})
	_, dryRun, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	if err != nil {
		t.Fatal(err)
	}
	h.config.Limits.MaxOutputBytes = 1
	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit || output.BackupCount != 0 {
		t.Fatalf("output limit result=%+v output=%+v err=%v", result, output, err)
	}
	if store.Index().ManifestCount != 0 {
		t.Fatalf("output-limited apply created manifests: %+v", store.Index())
	}
	assertFileBytes(t, paths[0], []byte("alpha"))
}

func TestPatchPackageRequiredBackupRevalidatesAllTargetsBeforeFirstCommit(t *testing.T) {
	_, store, paths, manifest := newPatchPackageBackupFixture(t, backupstore.Limits{}, []patchPackageApplyFixture{
		{oldText: "alpha", newText: "omega"},
		{oldText: "beta", newText: "gamma"},
	})
	wrapped := &patchPackageBackupStoreWrapper{Store: store}
	wrapped.captureBatch = func(ctx context.Context, requests []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error) {
		results, err := store.CaptureBatch(ctx, requests)
		if err != nil {
			return results, err
		}
		if writeErr := os.WriteFile(paths[1], []byte("external"), 0o600); writeErr != nil {
			return results, writeErr
		}
		return results, nil
	}
	h := NewHandler([]string{filepath.Dir(paths[0])}, WithBackupStore(wrapped))
	_, dryRun, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	if err != nil {
		t.Fatal(err)
	}
	var commits atomic.Int32
	h.patchPackageCommitReplacement = func(int, *filesystem.StagedReplacement, filesystem.ReplaceOptions) (bool, error) {
		commits.Add(1)
		return false, errors.New("commit must not be reached")
	}
	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !result.IsError || output.Applied || output.BackupCount != 2 || commits.Load() != 0 {
		t.Fatalf("post-backup change result=%+v output=%+v commits=%d err=%v", result, output, commits.Load(), err)
	}
	if store.Index().ManifestCount != 2 {
		t.Fatalf("manifest count=%d, want 2", store.Index().ManifestCount)
	}
	assertFileBytes(t, paths[0], []byte("alpha"))
	assertFileBytes(t, paths[1], []byte("external"))
}

func TestPatchPackageRequiredBackupContinuesAfterDerivedIndexFailure(t *testing.T) {
	_, store, paths, manifest := newPatchPackageBackupFixture(t, backupstore.Limits{}, []patchPackageApplyFixture{
		{oldText: "alpha", newText: "omega"},
		{oldText: "beta", newText: "gamma"},
	})
	wrapped := &patchPackageBackupStoreWrapper{Store: store}
	wrapped.captureBatch = func(ctx context.Context, requests []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error) {
		results, err := store.CaptureBatch(ctx, requests)
		if err != nil {
			return results, err
		}
		return results, errors.New("injected derived index persistence failure")
	}
	h := NewHandler([]string{filepath.Dir(paths[0])}, WithBackupStore(wrapped))
	_, dryRun, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	if err != nil {
		t.Fatal(err)
	}
	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || result.IsError || !output.Applied || output.BackupCount != 2 {
		t.Fatalf("derived-index result=%+v output=%+v err=%v", result, output, err)
	}
	assertFileBytes(t, paths[0], []byte("omega"))
	assertFileBytes(t, paths[1], []byte("gamma"))
}

func TestPatchPackageRequiredBackupRejectsUnexpectedExtraCaptureResult(t *testing.T) {
	_, store, paths, manifest := newPatchPackageBackupFixture(t, backupstore.Limits{}, []patchPackageApplyFixture{{oldText: "alpha", newText: "omega"}})
	wrapped := &patchPackageBackupStoreWrapper{Store: store}
	wrapped.captureBatch = func(ctx context.Context, requests []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error) {
		results, err := store.CaptureBatch(ctx, requests)
		if err != nil {
			return results, err
		}
		return append(results, results[0]), nil
	}
	h := NewHandler([]string{filepath.Dir(paths[0])}, WithBackupStore(wrapped))
	_, dryRun, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	if err != nil {
		t.Fatal(err)
	}
	var commits atomic.Int32
	h.patchPackageCommitReplacement = func(int, *filesystem.StagedReplacement, filesystem.ReplaceOptions) (bool, error) {
		commits.Add(1)
		return false, errors.New("commit must not be reached")
	}
	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !result.IsError || output.Applied || output.BackupCount != 1 || commits.Load() != 0 {
		t.Fatalf("extra capture result=%+v output=%+v commits=%d err=%v", result, output, commits.Load(), err)
	}
	assertFileBytes(t, paths[0], []byte("alpha"))
}

func TestPatchPackageRequiredBackupPartialCommitPreservesEveryBackupID(t *testing.T) {
	h, store, _, manifest := newPatchPackageBackupFixture(t, backupstore.Limits{}, []patchPackageApplyFixture{
		{oldText: "alpha", newText: "omega"},
		{oldText: "beta", newText: "gamma"},
	})
	_, dryRun, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	if err != nil {
		t.Fatal(err)
	}
	originalCommit := h.patchPackageCommitReplacement
	h.patchPackageCommitReplacement = func(index int, staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
		if index == 1 {
			return false, errors.New("injected second commit failure")
		}
		return originalCommit(index, staged, options)
	}
	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodePartialCommit || !output.PartialCommit || output.BackupCount != 2 {
		t.Fatalf("partial result=%+v output=%+v err=%v", result, output, err)
	}
	for index := range output.Results {
		if len(output.Results[index].BackupID) != 64 {
			t.Fatalf("result %d lost backup ID: %+v", index, output.Results[index])
		}
	}
	if store.Index().ManifestCount != 2 {
		t.Fatalf("manifest count=%d, want 2", store.Index().ManifestCount)
	}
}

type patchPackageBackupStoreWrapper struct {
	*backupstore.Store
	captureBatch func(context.Context, []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error)
}

func (store *patchPackageBackupStoreWrapper) CaptureBatch(ctx context.Context, requests []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error) {
	return store.captureBatch(ctx, requests)
}

func (store *patchPackageBackupStoreWrapper) PreflightCaptureBatch(ctx context.Context, requests []backupstore.CaptureRequest) error {
	return store.Store.PreflightCaptureBatch(ctx, requests)
}

func newPatchPackageBackupFixture(t *testing.T, limits backupstore.Limits, fixtures []patchPackageApplyFixture) (*Handler, *backupstore.Store, []string, PatchPackageManifest) {
	t.Helper()
	base := canonicalHandlerTestDir(t)
	publicRoot := filepath.Join(base, "public")
	if err := os.Mkdir(publicRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := backupstore.Open(backupstore.Options{
		Directory:                filepath.Join(base, "backup-store"),
		PublicAllowedDirectories: []string{publicRoot},
		Limits:                   limits,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	paths := make([]string, len(fixtures))
	for index := range fixtures {
		paths[index] = filepath.Join(publicRoot, string(rune('a'+index))+".txt")
		fixtures[index].path = paths[index]
		if err := os.WriteFile(paths[index], []byte(fixtures[index].oldText), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := patchPackageManifestForApplyTest(t, fixtures)
	manifest.BackupPolicy = editBackupPolicyRequired
	return NewHandler([]string{publicRoot}, WithBackupStore(store)), store, paths, manifest
}
