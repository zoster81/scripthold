package handler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/markdownintelligence"
)

func TestMarkdownEditPreviewApplyLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Old\r\n\r\nBody.\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandler([]string{dir})
	if h.markdownEditPreviews == nil {
		t.Fatal("markdown edit preview store is not initialized")
	}
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"heading"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 1 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}

	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
		Path:       path,
		Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: read.Nodes[0].TargetID, Text: "New"}},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if !validMarkdownEditPreviewID(preview.PreviewID) || !preview.Changed || preview.TargetFingerprint == preview.ResultFingerprint {
		t.Fatalf("preview=%+v", preview)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("preview mutated target: %q err=%v", got, err)
	}

	applyResult, applied, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError {
		t.Fatalf("apply=%+v result=%+v err=%v", applied, applyResult, err)
	}
	if !applied.Applied || applied.State != editApplyStateCommitted || applied.ActualFingerprint != preview.ResultFingerprint {
		t.Fatalf("apply=%+v", applied)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "# New\r\n\r\nBody.\n" {
		t.Fatalf("applied bytes=%q err=%v", got, err)
	}

	replayResult, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || replayResult == nil || !replayResult.IsError {
		t.Fatalf("replay result=%+v err=%v", replayResult, err)
	}
}

func TestMarkdownEditComposesIndependentHeadingRenames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# One\r\n\r\n## Two\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"heading"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 2 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operations := []MarkdownEditOperation{
		{Action: "rename", Subject: "heading", TargetID: read.Nodes[0].TargetID, Text: "First"},
		{Action: "rename", Subject: "heading", TargetID: read.Nodes[1].TargetID, Text: "Second"},
	}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: operations})
	if err != nil || previewResult.IsError || !preview.Changed || len(preview.Operations) != 2 {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("multi-edit preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "# First\r\n\r\n## Second\n" {
		t.Fatalf("multi-edit target=%q err=%v", got, err)
	}
}

func TestMarkdownEditComposesRenameAndHeadingLevelChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# One\r\n\r\n## Two\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"heading"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 2 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operations := []MarkdownEditOperation{
		{Action: "rename", Subject: "heading", TargetID: read.Nodes[0].TargetID, Text: "First"},
		{Action: "set", Subject: "heading", TargetID: read.Nodes[1].TargetID, Level: 3},
	}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: operations})
	if err != nil || previewResult.IsError || !preview.Changed || len(preview.Operations) != 2 {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("mixed preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "# First\r\n\r\n### Two\n" {
		t.Fatalf("mixed target=%q err=%v", got, err)
	}
}

func TestMarkdownEditRejectsInvalidHeadingLevelBeforeFilesystemWork(t *testing.T) {
	result := validateMarkdownEditInput(MarkdownEditInput{Path: "unused.md", Operations: []MarkdownEditOperation{{
		Action: "set", Subject: "heading", TargetID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Level: 7,
	}}})
	if result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("invalid level result=%+v", result)
	}
}

func TestMarkdownEditRejectsOverlappingHeadingRenamesWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Old\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	targetID := markdownHeadingTargetID(t, h, path)
	result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{
		{Action: "rename", Subject: "heading", TargetID: targetID, Text: "First"},
		{Action: "rename", Subject: "heading", TargetID: targetID, Text: "Second"},
	}})
	if err != nil || result == nil || !result.IsError {
		t.Fatalf("overlap result=%+v err=%v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("overlap target=%q err=%v", got, err)
	}
}

func TestMarkdownEditRejectsOperationCountAboveFixedLimit(t *testing.T) {
	operations := make([]MarkdownEditOperation, markdownintelligence.MaxEditOperations+1)
	for index := range operations {
		operations[index] = MarkdownEditOperation{
			Action: "rename", Subject: "heading",
			TargetID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Text: "New",
		}
	}
	result, _, err := NewHandler(nil).HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: "unused.md", Operations: operations})
	if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("over-limit result=%+v err=%v", result, err)
	}
}

func TestMarkdownApplyConsumesPreviewBeforeCancellation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("# Old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	targetID := markdownHeadingTargetID(t, h, path)
	_, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "New"}}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, _, err := h.HandleMarkdownApply(ctx, nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || result == nil || !result.IsError {
		t.Fatalf("cancelled apply result=%+v err=%v", result, err)
	}
	replay, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || replay == nil || !replay.IsError {
		t.Fatalf("replay result=%+v err=%v", replay, err)
	}
}

func TestMarkdownApplyRejectsReadOnlyTargetAfterPreviewAndConsumesPreview(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Old\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	targetID := markdownHeadingTargetID(t, h, path)
	_, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "New"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	replaceCalled := false
	h.replaceFile = func(string, []byte, filesystem.ReplaceOptions) error {
		replaceCalled = true
		return nil
	}

	result, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodePermission {
		t.Fatalf("read-only apply result=%+v err=%v", result, err)
	}
	if replaceCalled {
		t.Fatal("read-only apply reached durable replacement without explicit writable approval")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("read-only target=%q err=%v", got, err)
	}
	replay, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || replay == nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("read-only replay result=%+v err=%v", replay, err)
	}
}

func TestMarkdownApplyRejectsStaleSourceAndConsumesPreview(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("# Old\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	targetID := markdownHeadingTargetID(t, h, path)
	_, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "New"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Old\n\nExternal.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale apply result=%+v err=%v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "# Old\n\nExternal.\n" {
		t.Fatalf("stale target=%q err=%v", got, err)
	}
	replay, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || replay == nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale replay result=%+v err=%v", replay, err)
	}
}

func TestMarkdownApplyRequiredBackupCapturesApprovedPreState(t *testing.T) {
	h, store, path := newEditBackupFixture(t, backupstore.Limits{})
	original := []byte("# Old\r\n\nBody.\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	targetID := markdownHeadingTargetID(t, h, path)
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
		Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "New"}}, BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil || previewResult.IsError || store.Index().ManifestCount != 0 {
		t.Fatalf("preview result=%+v output=%+v manifests=%d err=%v", previewResult, preview, store.Index().ManifestCount, err)
	}

	applyResult, applied, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !applied.Applied || len(applied.BackupID) != 64 || applied.BackupPolicy != editBackupPolicyRequired {
		t.Fatalf("apply result=%+v output=%+v err=%v", applyResult, applied, err)
	}
	inspected, err := store.Inspect(context.Background(), applied.BackupID, backupstore.InspectOptions{})
	if err != nil || !inspected.ObjectVerified || inspected.Manifest.TargetPath != path || inspected.Manifest.SourceOperation != backupstore.SourceOperationEdit || inspected.Manifest.ContentFingerprint != filesystem.FingerprintRegularFileData(original) {
		t.Fatalf("backup=%+v err=%v", inspected, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "# New\r\n\nBody.\n" {
		t.Fatalf("applied target=%q err=%v", got, err)
	}
}

func TestMarkdownApplyWriteFailureIsTerminalAndPreservesBackup(t *testing.T) {
	h, store, path := newEditBackupFixture(t, backupstore.Limits{})
	original := []byte("# Old\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	targetID := markdownHeadingTargetID(t, h, path)
	_, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
		Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "New"}}, BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.replaceFile = func(string, []byte, filesystem.ReplaceOptions) error { return errors.New("injected write failure") }

	result, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || result == nil || !result.IsError || output.Applied || output.State != editApplyStateUnchanged || len(output.BackupID) != 64 {
		t.Fatalf("failed apply result=%+v output=%+v err=%v", result, output, err)
	}
	if store.Index().ManifestCount != 1 {
		t.Fatalf("manifest count=%d, want 1", store.Index().ManifestCount)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("failed target=%q err=%v", got, err)
	}
	replay, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || replay == nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("failed replay result=%+v err=%v", replay, err)
	}
}

func TestMarkdownApplyNoOpIsTerminalWithoutBackupOrWrite(t *testing.T) {
	h, store, path := newEditBackupFixture(t, backupstore.Limits{})
	original := []byte("# Old\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	targetID := markdownHeadingTargetID(t, h, path)
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
		Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "Old"}}, BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil || previewResult.IsError || preview.Changed || store.Index().ManifestCount != 0 {
		t.Fatalf("no-op preview result=%+v output=%+v manifests=%d err=%v", previewResult, preview, store.Index().ManifestCount, err)
	}
	replaceCalled := false
	h.replaceFile = func(string, []byte, filesystem.ReplaceOptions) error {
		replaceCalled = true
		return errors.New("unexpected durable replace")
	}

	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || output.Applied || output.Changed || output.State != editApplyStateUnchanged || output.ActualFingerprint != preview.TargetFingerprint {
		t.Fatalf("no-op apply result=%+v output=%+v err=%v", applyResult, output, err)
	}
	if replaceCalled || store.Index().ManifestCount != 0 {
		t.Fatalf("no-op apply replaceCalled=%v manifests=%d", replaceCalled, store.Index().ManifestCount)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("no-op target=%q err=%v", got, err)
	}
	replay, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || replay == nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("no-op replay result=%+v err=%v", replay, err)
	}
}

func TestMarkdownApplyConcurrentClaimHasOneWinner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("# Old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	targetID := markdownHeadingTargetID(t, h, path)
	_, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "New"}}})
	if err != nil {
		t.Fatal(err)
	}

	type outcome struct {
		code    string
		success bool
		err     error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, _, callErr := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
			if result == nil {
				outcomes <- outcome{err: errors.New("markdown_apply returned nil result")}
				return
			}
			code, _ := result.Meta[ErrorCodeMetaKey].(string)
			outcomes <- outcome{code: code, success: !result.IsError, err: callErr}
		}()
	}
	close(start)
	wg.Wait()
	close(outcomes)

	successes, conflicts := 0, 0
	for got := range outcomes {
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.success {
			successes++
		} else if got.code == ErrCodeConflict {
			conflicts++
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "# New\n" {
		t.Fatalf("concurrent apply target=%q err=%v", got, err)
	}
}

func TestMarkdownApplyOutputLimitIsTerminalAndNonMutating(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Old\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	targetID := markdownHeadingTargetID(t, h, path)
	_, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "New"}}})
	if err != nil {
		t.Fatal(err)
	}
	replaceCalled := false
	h.replaceFile = func(string, []byte, filesystem.ReplaceOptions) error {
		replaceCalled = true
		return errors.New("unexpected durable replace")
	}
	h.config.Limits.MaxOutputBytes = 1

	limited, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || limited == nil || !limited.IsError || limited.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("output-limited apply result=%+v err=%v", limited, err)
	}
	if replaceCalled {
		t.Fatal("output-limited apply reached durable replacement")
	}
	h.config.Limits.MaxOutputBytes = config.DefaultMaxOutputBytes
	replay, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || replay == nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("output-limited replay result=%+v err=%v", replay, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("output-limited target=%q err=%v", got, err)
	}
}

func markdownHeadingTargetID(t *testing.T, h *Handler, path string) string {
	t.Helper()
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"heading"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 1 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	return read.Nodes[0].TargetID
}
