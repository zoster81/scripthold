package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/marksplice"
	"github.com/zoster81/marksplice/workspacefs"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestPrepareMarkdownWorkspaceRepairApplyTargetsRevalidatesSemanticAndPhysicalState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	source := "# Root\r\n\r\n## Contents\r\n\r\n- [Root](#old-root)\r\n- [Child](#child)\r\n\r\n## Child\r\nbody\r\n"
	original := encodeUTF16LEWithBOM(t, source)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	h := NewHandler([]string{root})
	repairs := markdownWorkspaceRepairFixture(t, h, root)
	prepared, failure := h.prepareMarkdownWorkspaceRepairPreview(context.Background(), root, repairs, "")
	if failure != nil {
		t.Fatalf("prepare failure=%+v", failure)
	}
	defer prepared.close()

	plans, failure := h.prepareMarkdownWorkspaceRepairApplyTargets(context.Background(), &prepared)
	if failure != nil {
		t.Fatalf("apply revalidation failure=%+v", failure)
	}
	if len(plans) != 1 {
		t.Fatalf("plans=%d, want 1", len(plans))
	}
	plan := plans[0]
	target := &prepared.targets[0]
	if plan.document.Charset != target.encoding || plan.document.BOM.HasBOM != target.hasBOM || plan.document.BOM.Type != target.bomType {
		t.Fatalf("physical metadata changed: document=%+v target=%+v", plan.document, target)
	}
	if !bytes.Equal(plan.replacement.resultData, target.resultData) ||
		plan.replacement.targetFingerprint != target.targetFingerprint ||
		plan.replacement.resultFingerprint != target.resultFingerprint ||
		plan.replacement.identity() != target.identityFile {
		t.Fatalf("replacement binding diverged: %+v", plan.replacement)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("apply revalidation mutated source: bytes=%x err=%v", got, err)
	}
}

func TestPrepareMarkdownWorkspaceRepairApplyTargetsRejectsStaleSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	source := "# Root\n\n## Contents\n\n- [Root](#old-root)\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	repairs := markdownWorkspaceRepairFixture(t, h, root)
	prepared, failure := h.prepareMarkdownWorkspaceRepairPreview(context.Background(), root, repairs, "")
	if failure != nil {
		t.Fatalf("prepare failure=%+v", failure)
	}
	defer prepared.close()

	external := []byte(source + "\nexternal\n")
	if err := os.WriteFile(path, external, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, failure := h.prepareMarkdownWorkspaceRepairApplyTargets(context.Background(), &prepared); failure == nil || !failure.IsError {
		t.Fatalf("stale source unexpectedly revalidated: %+v", failure)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, external) {
		t.Fatalf("stale-source check mutated file: bytes=%q err=%v", got, err)
	}
}

func TestPrepareMarkdownWorkspaceRepairApplyTargetsRejectsTamperedPreparedResult(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	if err := os.WriteFile(path, []byte("# Root\n\n## Contents\n\n- [Root](#old-root)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	repairs := markdownWorkspaceRepairFixture(t, h, root)
	prepared, failure := h.prepareMarkdownWorkspaceRepairPreview(context.Background(), root, repairs, "")
	if failure != nil {
		t.Fatalf("prepare failure=%+v", failure)
	}
	defer prepared.close()

	prepared.targets[0].resultData = append(prepared.targets[0].resultData, []byte("tampered")...)
	if _, failure := h.prepareMarkdownWorkspaceRepairApplyTargets(context.Background(), &prepared); failure == nil || !failure.IsError {
		t.Fatalf("tampered result unexpectedly revalidated: %+v", failure)
	}
}

func TestApplyPreparedMarkdownWorkspaceRepairCommitsInPlanOrder(t *testing.T) {
	root := t.TempDir()
	writeMarkdownWorkspaceRepairApplyDocs(t, root, "a.md", "b.md")
	h := NewHandler([]string{root})
	prepared := prepareMarkdownWorkspaceRepairApplyFixture(t, h, root, "")
	defer prepared.close()

	originalCommit := h.existingFileReplacementOps.commit
	var commitOrder []int
	h.existingFileReplacementOps.commit = func(index int, staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
		commitOrder = append(commitOrder, index)
		return originalCommit(index, staged, options)
	}

	applied, failure := h.applyPreparedMarkdownWorkspaceRepair(context.Background(), &prepared)
	if failure != nil {
		t.Fatalf("apply failure=%+v output=%+v", failure, applied)
	}
	if applied.CommittedCount != 2 || applied.UnchangedCount != 0 || applied.UnknownCount != 0 || applied.PartialCommit {
		t.Fatalf("apply output=%+v", applied)
	}
	if len(commitOrder) != 2 || commitOrder[0] != 0 || commitOrder[1] != 1 {
		t.Fatalf("commit order=%v", commitOrder)
	}
	for index := range prepared.targets {
		if applied.Results[index].State != string(existingFileReplacementStateCommitted) || !applied.Results[index].Applied {
			t.Fatalf("result %d=%+v", index, applied.Results[index])
		}
		assertMarkdownWorkspaceRepairResultFingerprint(t, prepared.targets[index])
	}
}

func TestApplyPreparedMarkdownWorkspaceRepairCapturesAllBackupsBeforeCommit(t *testing.T) {
	fixture := newBackupStoreHandlerFixture(t)
	writeMarkdownWorkspaceRepairApplyDocs(t, fixture.publicRoot, "a.md", "b.md")
	prepared := prepareMarkdownWorkspaceRepairApplyFixture(t, fixture.handler, fixture.publicRoot, editBackupPolicyRequired)
	defer prepared.close()

	originalCommit := fixture.handler.existingFileReplacementOps.commit
	fixture.handler.existingFileReplacementOps.commit = func(index int, staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
		if index == 0 && fixture.store.Index().ManifestCount != 2 {
			t.Fatalf("first commit began with %d durable manifests, want 2", fixture.store.Index().ManifestCount)
		}
		return originalCommit(index, staged, options)
	}

	applied, failure := fixture.handler.applyPreparedMarkdownWorkspaceRepair(context.Background(), &prepared)
	if failure != nil {
		t.Fatalf("apply failure=%+v output=%+v", failure, applied)
	}
	if applied.BackupCount != 2 || fixture.store.Index().ManifestCount != 2 {
		t.Fatalf("backup state output=%+v index=%+v", applied, fixture.store.Index())
	}
	for index := range applied.Results {
		if len(applied.Results[index].BackupID) != 64 {
			t.Fatalf("result %d backupId=%q", index, applied.Results[index].BackupID)
		}
	}
}

func TestApplyPreparedMarkdownWorkspaceRepairIncompleteBackupPreservesDurablePrefixWithoutCommit(t *testing.T) {
	fixture := newBackupStoreHandlerFixture(t)
	writeMarkdownWorkspaceRepairApplyDocs(t, fixture.publicRoot, "a.md", "b.md")
	prepared := prepareMarkdownWorkspaceRepairApplyFixture(t, fixture.handler, fixture.publicRoot, editBackupPolicyRequired)
	defer prepared.close()

	wrapped := &patchPackageBackupStoreWrapper{Store: fixture.store}
	wrapped.captureBatch = func(ctx context.Context, requests []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error) {
		results, err := fixture.store.CaptureBatch(ctx, requests[:1])
		return results, errors.Join(err, errors.New("injected incomplete backup batch"))
	}
	fixture.handler.backupBatchCapture = wrapped
	commits := 0
	fixture.handler.existingFileReplacementOps.commit = func(int, *filesystem.StagedReplacement, filesystem.ReplaceOptions) (bool, error) {
		commits++
		return false, errors.New("commit must not be reached")
	}

	applied, failure := fixture.handler.applyPreparedMarkdownWorkspaceRepair(context.Background(), &prepared)
	if failure == nil || !failure.IsError {
		t.Fatalf("incomplete backup failure=%+v output=%+v", failure, applied)
	}
	if commits != 0 || applied.BackupCount != 1 || applied.CommittedCount != 0 || applied.UnchangedCount != 2 || applied.UnknownCount != 0 {
		t.Fatalf("incomplete backup state commits=%d output=%+v", commits, applied)
	}
	if len(applied.Results[0].BackupID) != 64 || applied.Results[1].BackupID != "" {
		t.Fatalf("incomplete backup IDs=%+v", applied.Results)
	}
}

func TestApplyPreparedMarkdownWorkspaceRepairPostBackupChangePreservesBackupsAndObservedState(t *testing.T) {
	fixture := newBackupStoreHandlerFixture(t)
	writeMarkdownWorkspaceRepairApplyDocs(t, fixture.publicRoot, "a.md", "b.md")
	prepared := prepareMarkdownWorkspaceRepairApplyFixture(t, fixture.handler, fixture.publicRoot, editBackupPolicyRequired)
	defer prepared.close()

	changedPath := prepared.targets[1].resolvedPath
	wrapped := &patchPackageBackupStoreWrapper{Store: fixture.store}
	wrapped.captureBatch = func(ctx context.Context, requests []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error) {
		results, err := fixture.store.CaptureBatch(ctx, requests)
		if err == nil {
			err = os.WriteFile(changedPath, []byte("external"), 0o600)
		}
		return results, err
	}
	fixture.handler.backupBatchCapture = wrapped
	commits := 0
	fixture.handler.existingFileReplacementOps.commit = func(int, *filesystem.StagedReplacement, filesystem.ReplaceOptions) (bool, error) {
		commits++
		return false, errors.New("commit must not be reached")
	}

	applied, failure := fixture.handler.applyPreparedMarkdownWorkspaceRepair(context.Background(), &prepared)
	if failure == nil || !failure.IsError {
		t.Fatalf("post-backup change failure=%+v output=%+v", failure, applied)
	}
	if commits != 0 || applied.BackupCount != 2 || applied.CommittedCount != 0 || applied.UnchangedCount != 1 || applied.UnknownCount != 1 {
		t.Fatalf("post-backup state commits=%d output=%+v", commits, applied)
	}
	for index := range applied.Results {
		if len(applied.Results[index].BackupID) != 64 {
			t.Fatalf("result %d backupId=%q", index, applied.Results[index].BackupID)
		}
	}
}

func TestApplyPreparedMarkdownWorkspaceRepairOutputLimitPreventsStaging(t *testing.T) {
	root := t.TempDir()
	writeMarkdownWorkspaceRepairApplyDocs(t, root, "doc.md")
	h := NewHandler([]string{root})
	prepared := prepareMarkdownWorkspaceRepairApplyFixture(t, h, root, "")
	defer prepared.close()
	original, err := os.ReadFile(prepared.targets[0].resolvedPath)
	if err != nil {
		t.Fatal(err)
	}
	h.config.Limits.MaxOutputBytes = 1
	staged := 0
	originalStage := h.existingFileReplacementOps.stage
	h.existingFileReplacementOps.stage = func(ctx context.Context, path string, data []byte, mode os.FileMode) (*filesystem.StagedReplacement, error) {
		staged++
		return originalStage(ctx, path, data, mode)
	}

	applied, failure := h.applyPreparedMarkdownWorkspaceRepair(context.Background(), &prepared)
	if failure == nil || !failure.IsError || failure.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("output-limit failure=%+v output=%+v", failure, applied)
	}
	if staged != 0 {
		t.Fatalf("staged=%d, want 0", staged)
	}
	if got, err := os.ReadFile(prepared.targets[0].resolvedPath); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("output-limit apply mutated source: bytes=%q err=%v", got, err)
	}
}

func TestApplyPreparedMarkdownWorkspaceRepairClassifiesPartialCommit(t *testing.T) {
	root := t.TempDir()
	writeMarkdownWorkspaceRepairApplyDocs(t, root, "a.md", "b.md")
	h := NewHandler([]string{root})
	prepared := prepareMarkdownWorkspaceRepairApplyFixture(t, h, root, "")
	defer prepared.close()

	originalCommit := h.existingFileReplacementOps.commit
	h.existingFileReplacementOps.commit = func(index int, staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
		if index == 1 {
			return false, errors.New("injected second commit failure")
		}
		return originalCommit(index, staged, options)
	}

	applied, failure := h.applyPreparedMarkdownWorkspaceRepair(context.Background(), &prepared)
	if failure == nil || !failure.IsError {
		t.Fatalf("partial apply failure=%+v output=%+v", failure, applied)
	}
	if !applied.PartialCommit || applied.CommittedCount != 1 || applied.UnchangedCount != 1 || applied.UnknownCount != 0 {
		t.Fatalf("partial output=%+v", applied)
	}
	if applied.Results[0].State != string(existingFileReplacementStateCommitted) || applied.Results[1].State != string(existingFileReplacementStateUnchanged) {
		t.Fatalf("partial results=%+v", applied.Results)
	}
}

func TestApplyPreparedMarkdownWorkspaceRepairStagingFailureCleansAllStagesWithoutWrites(t *testing.T) {
	root := t.TempDir()
	writeMarkdownWorkspaceRepairApplyDocs(t, root, "a.md", "b.md")
	h := NewHandler([]string{root})
	prepared := prepareMarkdownWorkspaceRepairApplyFixture(t, h, root, "")
	defer prepared.close()

	original := make([][]byte, len(prepared.targets))
	for index := range prepared.targets {
		data, err := os.ReadFile(prepared.targets[index].resolvedPath)
		if err != nil {
			t.Fatal(err)
		}
		original[index] = data
	}

	originalStage := h.existingFileReplacementOps.stage
	originalCleanup := h.existingFileReplacementOps.cleanup
	stageCalls := 0
	cleanupCalls := 0
	h.existingFileReplacementOps.stage = func(ctx context.Context, path string, data []byte, mode os.FileMode) (*filesystem.StagedReplacement, error) {
		stageCalls++
		if stageCalls == 2 {
			return nil, errors.New("injected staging failure")
		}
		return originalStage(ctx, path, data, mode)
	}
	h.existingFileReplacementOps.cleanup = func(staged *filesystem.StagedReplacement) error {
		cleanupCalls++
		return originalCleanup(staged)
	}

	applied, failure := h.applyPreparedMarkdownWorkspaceRepair(context.Background(), &prepared)
	if failure == nil || !failure.IsError {
		t.Fatalf("staging failure=%+v output=%+v", failure, applied)
	}
	if stageCalls != 2 || cleanupCalls != 1 || applied.CommittedCount != 0 || applied.UnchangedCount != 2 || applied.UnknownCount != 0 {
		t.Fatalf("staging state stages=%d cleanups=%d output=%+v", stageCalls, cleanupCalls, applied)
	}
	for index := range prepared.targets {
		got, err := os.ReadFile(prepared.targets[index].resolvedPath)
		if err != nil || !bytes.Equal(got, original[index]) {
			t.Fatalf("target %d changed after staging failure: bytes=%q err=%v", index, got, err)
		}
	}
}

func TestApplyPreparedMarkdownWorkspaceRepairCancellationAfterStagingCleansWithoutCommit(t *testing.T) {
	root := t.TempDir()
	writeMarkdownWorkspaceRepairApplyDocs(t, root, "a.md", "b.md")
	h := NewHandler([]string{root})
	prepared := prepareMarkdownWorkspaceRepairApplyFixture(t, h, root, "")
	defer prepared.close()

	original := make([][]byte, len(prepared.targets))
	for index := range prepared.targets {
		data, err := os.ReadFile(prepared.targets[index].resolvedPath)
		if err != nil {
			t.Fatal(err)
		}
		original[index] = data
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	originalStage := h.existingFileReplacementOps.stage
	originalCleanup := h.existingFileReplacementOps.cleanup
	stageCalls := 0
	cleanupCalls := 0
	commitCalls := 0
	h.existingFileReplacementOps.stage = func(ctx context.Context, path string, data []byte, mode os.FileMode) (*filesystem.StagedReplacement, error) {
		stageCalls++
		staged, err := originalStage(ctx, path, data, mode)
		if err == nil && stageCalls == len(prepared.targets) {
			cancel()
		}
		return staged, err
	}
	h.existingFileReplacementOps.cleanup = func(staged *filesystem.StagedReplacement) error {
		cleanupCalls++
		return originalCleanup(staged)
	}
	h.existingFileReplacementOps.commit = func(int, *filesystem.StagedReplacement, filesystem.ReplaceOptions) (bool, error) {
		commitCalls++
		return false, errors.New("commit must not be reached")
	}

	applied, failure := h.applyPreparedMarkdownWorkspaceRepair(ctx, &prepared)
	if failure == nil || !failure.IsError || failure.Meta[ErrorCodeMetaKey] != ErrCodeCancelled {
		t.Fatalf("cancellation failure=%+v output=%+v", failure, applied)
	}
	if stageCalls != 2 || cleanupCalls != 2 || commitCalls != 0 || applied.CommittedCount != 0 || applied.UnchangedCount != 2 || applied.UnknownCount != 0 {
		t.Fatalf("cancellation state stages=%d cleanups=%d commits=%d output=%+v", stageCalls, cleanupCalls, commitCalls, applied)
	}
	for index := range prepared.targets {
		got, err := os.ReadFile(prepared.targets[index].resolvedPath)
		if err != nil || !bytes.Equal(got, original[index]) {
			t.Fatalf("target %d changed after cancellation: bytes=%q err=%v", index, got, err)
		}
	}
}

func TestApplyPreparedMarkdownWorkspaceRepairFinalVerificationDetectsExternalChange(t *testing.T) {
	root := t.TempDir()
	writeMarkdownWorkspaceRepairApplyDocs(t, root, "a.md", "b.md")
	h := NewHandler([]string{root})
	prepared := prepareMarkdownWorkspaceRepairApplyFixture(t, h, root, "")
	defer prepared.close()

	external := []byte("external after commits")
	originalCommit := h.existingFileReplacementOps.commit
	h.existingFileReplacementOps.commit = func(index int, staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
		changed, err := originalCommit(index, staged, options)
		if err == nil && index == len(prepared.targets)-1 {
			err = os.WriteFile(prepared.targets[0].resolvedPath, external, 0o600)
		}
		return changed, err
	}

	applied, failure := h.applyPreparedMarkdownWorkspaceRepair(context.Background(), &prepared)
	if failure == nil || !failure.IsError || failure.Meta[ErrorCodeMetaKey] != ErrCodePartialCommit {
		t.Fatalf("final verification failure=%+v output=%+v", failure, applied)
	}
	if !applied.PartialCommit || applied.CommittedCount != 1 || applied.UnchangedCount != 0 || applied.UnknownCount != 1 {
		t.Fatalf("final verification output=%+v", applied)
	}
	if applied.Results[0].State != string(existingFileReplacementStateUnknown) ||
		applied.Results[1].State != string(existingFileReplacementStateCommitted) {
		t.Fatalf("final verification results=%+v", applied.Results)
	}
	got, err := os.ReadFile(prepared.targets[0].resolvedPath)
	if err != nil || !bytes.Equal(got, external) {
		t.Fatalf("external mutation not preserved: bytes=%q err=%v", got, err)
	}
}

func TestApplyMarkdownWorkspaceRepairPreviewConsumesCapabilityOnSuccess(t *testing.T) {
	root := t.TempDir()
	writeMarkdownWorkspaceRepairApplyDocs(t, root, "doc.md")
	h := NewHandler([]string{root})
	prepared := prepareMarkdownWorkspaceRepairApplyFixture(t, h, root, "")
	preview, err := h.markdownPreviews.putWorkspaceRepair(prepared)
	if err != nil {
		prepared.close()
		t.Fatal(err)
	}

	applied, failure := h.applyMarkdownWorkspaceRepairPreview(context.Background(), preview.id)
	if failure != nil || applied.CommittedCount != 1 || applied.UnknownCount != 0 {
		t.Fatalf("apply failure=%+v output=%+v", failure, applied)
	}
	if _, replay := h.applyMarkdownWorkspaceRepairPreview(context.Background(), preview.id); replay == nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("replay=%+v, want consumed-preview conflict", replay)
	}
	assertMarkdownWorkspaceRepairResultFingerprint(t, prepared.targets[0])
}

func TestApplyMarkdownWorkspaceRepairPreviewConsumesCapabilityOnFailedAttempt(t *testing.T) {
	root := t.TempDir()
	writeMarkdownWorkspaceRepairApplyDocs(t, root, "doc.md")
	h := NewHandler([]string{root})
	prepared := prepareMarkdownWorkspaceRepairApplyFixture(t, h, root, "")
	preview, err := h.markdownPreviews.putWorkspaceRepair(prepared)
	if err != nil {
		prepared.close()
		t.Fatal(err)
	}
	path := prepared.targets[0].resolvedPath
	external := []byte("external before apply")
	if err := os.WriteFile(path, external, 0o600); err != nil {
		t.Fatal(err)
	}

	applied, failure := h.applyMarkdownWorkspaceRepairPreview(context.Background(), preview.id)
	if failure == nil || !failure.IsError || applied.CommittedCount != 0 {
		t.Fatalf("failed apply failure=%+v output=%+v", failure, applied)
	}
	if _, replay := h.applyMarkdownWorkspaceRepairPreview(context.Background(), preview.id); replay == nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("replay=%+v, want consumed-preview conflict", replay)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, external) {
		t.Fatalf("failed attempt changed source: bytes=%q err=%v", got, err)
	}
}

func TestMarkdownApplyOutputMarshalPreservesSingleFileWireShape(t *testing.T) {
	output := MarkdownApplyOutput{
		Path: "doc.md", TargetFingerprint: "before", ResultFingerprint: "after",
		Encoding: "utf-8", HasBOM: false, LineEndingStyle: "lf",
		State: editApplyStateUnchanged, Changed: false, Applied: false,
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"path":"doc.md","targetFingerprint":"before","resultFingerprint":"after","encoding":"utf-8","hasBOM":false,"lineEndingStyle":"lf","state":"unchanged","changed":false,"applied":false}`
	if string(encoded) != want {
		t.Fatalf("single-file wire shape=%s want=%s", encoded, want)
	}
}

func TestHandleMarkdownApplyProjectsWorkspaceRepairWithoutLegacyZeroFields(t *testing.T) {
	root := t.TempDir()
	writeMarkdownWorkspaceRepairApplyDocs(t, root, "doc.md")
	h := NewHandler([]string{root})
	prepared := prepareMarkdownWorkspaceRepairApplyFixture(t, h, root, "")
	preview, err := h.markdownPreviews.putWorkspaceRepair(prepared)
	if err != nil {
		prepared.close()
		t.Fatal(err)
	}

	result, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.id})
	if err != nil || result == nil || result.IsError || output.Workspace == nil {
		t.Fatalf("workspace apply result=%+v output=%+v err=%v", result, output, err)
	}
	if output.Workspace.CommittedCount != 1 || output.Workspace.TotalTargets != 1 || len(output.Workspace.Results) != 1 {
		t.Fatalf("workspace output=%+v", output.Workspace)
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope) != 1 || envelope["workspace"] == nil {
		t.Fatalf("workspace wire shape=%s", encoded)
	}
	replay, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.id})
	if err != nil || replay == nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("replay result=%+v err=%v", replay, err)
	}
}

func writeMarkdownWorkspaceRepairApplyDocs(t *testing.T, root string, names ...string) {
	t.Helper()
	source := "# Root\n\n## Contents\n\n- [Root](#old-root)\n"
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func prepareMarkdownWorkspaceRepairApplyFixture(t *testing.T, h *Handler, root, backupPolicy string) preparedMarkdownWorkspaceRepair {
	t.Helper()
	adapter, err := newMarkdownWorkspaceFS(context.Background(), h, root, "")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := workspacefs.Scan(adapter, ".", workspacefs.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	documents := workspace.Documents()
	managed := make([]marksplice.ManagedTOC, 0, len(documents))
	for _, item := range documents {
		target, ok := item.Document.ResolveFragment("#contents")
		if !ok {
			t.Fatalf("%s managed TOC did not resolve", item.Key)
		}
		managed = append(managed, marksplice.ManagedTOC{Document: item.Key, HeadingID: target.NodeID()})
	}
	report, err := workspace.Validate(marksplice.WorkspaceValidationOptions{ManagedTOCs: managed})
	if err != nil {
		t.Fatal(err)
	}
	prepared, failure := h.prepareMarkdownWorkspaceRepairPreview(context.Background(), root, report.RepairPlan().Repairs(), backupPolicy)
	if failure != nil {
		t.Fatalf("prepare failure=%+v", failure)
	}
	return prepared
}

func assertMarkdownWorkspaceRepairResultFingerprint(t *testing.T, target preparedMarkdownWorkspaceRepairTarget) {
	t.Helper()
	snapshot, err := filesystem.CaptureRegularFileSnapshotBounded(context.Background(), target.resolvedPath, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := filesystem.FingerprintRegularFileSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if actual != target.resultFingerprint {
		t.Fatalf("actual fingerprint=%q want %q", actual, target.resultFingerprint)
	}
}
