package handler

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

func (h *Handler) handleBackupStoreHistory(ctx context.Context, input BackupStoreReadInput, visibility backupVisibilitySnapshot) (*mcp.CallToolResult, BackupStoreOutput, error) {
	if input.TargetPath == "" {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "history requires targetPath")), BackupStoreOutput{}, nil
	}
	validatedTarget, err := visibility.validate(input.TargetPath)
	if err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	listed, err := h.backupStore.List(ctx, backupstore.ListOptions{
		Cursor:          input.Cursor,
		Limit:           input.Limit,
		TargetPath:      validatedTarget,
		Pinned:          input.Pinned,
		VisibilityScope: visibility.scope,
		TargetVisible: func(path string) bool {
			_, visibilityErr := visibility.validate(path)
			return visibilityErr == nil
		},
	})
	if err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	output := BackupStoreOutput{
		Action:     BackupStoreActionHistory,
		Enabled:    true,
		State:      BackupStoreStateReady,
		Generation: listed.Generation,
		NextCursor: listed.NextCursor,
		Items:      make([]BackupStoreManifestItem, len(listed.Items)),
	}
	for index, item := range listed.Items {
		output.Items[index] = mapBackupStoreManifest(item)
	}
	return h.finishBackupStoreOutput(output, fmt.Sprintf("Listed %d backup version(s) for the authorized target.", len(output.Items)))
}

func (h *Handler) handleBackupStoreCompare(ctx context.Context, input BackupStoreReadInput, visibility backupVisibilitySnapshot) (*mcp.CallToolResult, BackupStoreOutput, error) {
	if h.backupRestoreReader == nil {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store does not provide comparison read authority")), BackupStoreOutput{}, nil
	}
	validatedTarget := ""
	left, err := h.backupRestoreReader.OpenReadSource(ctx, input.BackupID, backupstore.RestoreSourceOptions{
		AuthorizeTarget: func(path string) error {
			var authorizationErr error
			validatedTarget, authorizationErr = visibility.validate(path)
			return authorizationErr
		},
	})
	if err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	defer left.Close()
	if err := left.Verify(ctx); err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	leftManifest := left.Manifest()
	compare := &BackupStoreCompareOutput{
		BackupID:             leftManifest.BackupID,
		TargetPath:           validatedTarget,
		BackupFingerprint:    leftManifest.ContentFingerprint,
		BackupObjectVerified: true,
	}

	if input.OtherBackupID != "" {
		return h.compareBackupToBackup(ctx, input.OtherBackupID, visibility, left, compare)
	}
	return h.compareBackupToCurrent(ctx, left, compare)
}

func (h *Handler) compareBackupToBackup(ctx context.Context, otherBackupID string, visibility backupVisibilitySnapshot, left *backupstore.ReadSource, compare *BackupStoreCompareOutput) (*mcp.CallToolResult, BackupStoreOutput, error) {
	otherTarget := ""
	right, err := h.backupRestoreReader.OpenReadSource(ctx, otherBackupID, backupstore.RestoreSourceOptions{
		AuthorizeTarget: func(path string) error {
			var authorizationErr error
			otherTarget, authorizationErr = visibility.validate(path)
			return authorizationErr
		},
	})
	if err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	defer right.Close()
	if err := right.Verify(ctx); err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	if !sameResolvedRestoreTarget(compare.TargetPath, otherTarget) {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "backup/backup compare requires versions of the same authorized target")), BackupStoreOutput{}, nil
	}
	rightManifest := right.Manifest()
	compare.OtherKind = "backup"
	compare.OtherBackupID = rightManifest.BackupID
	compare.OtherExists = true
	compare.OtherFingerprint = rightManifest.ContentFingerprint
	compare.OtherObjectVerified = true
	compare.Equal = compare.BackupFingerprint == compare.OtherFingerprint
	compare.Diff = boundedBackupCompareDiff(ctx, left, left.Manifest().ObjectBytes, right, rightManifest.ObjectBytes, compare.TargetPath)
	compare.DiffAvailable = compare.Diff != ""
	output := BackupStoreOutput{Action: BackupStoreActionCompare, Enabled: true, State: BackupStoreStateReady, Compare: compare}
	return h.finishBackupStoreOutput(output, fmt.Sprintf("Compared backup %s with backup %s; equal=%t.", compare.BackupID, compare.OtherBackupID, compare.Equal))
}

func (h *Handler) compareBackupToCurrent(ctx context.Context, left *backupstore.ReadSource, compare *BackupStoreCompareOutput) (*mcp.CallToolResult, BackupStoreOutput, error) {
	compare.OtherKind = "current"
	info, err := os.Lstat(compare.TargetPath)
	if os.IsNotExist(err) {
		output := BackupStoreOutput{Action: BackupStoreActionCompare, Enabled: true, State: BackupStoreStateReady, Compare: compare}
		return h.finishBackupStoreOutput(output, fmt.Sprintf("Compared backup %s with current target; current target is missing.", compare.BackupID))
	}
	if err != nil {
		return errorResultFromError(operation.WrapFilesystem("inspect_backup_compare_target", compare.TargetPath, err)), BackupStoreOutput{}, nil
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errorResultFromError(operation.New(operation.KindConflict, "current comparison target is not a real regular file")), BackupStoreOutput{}, nil
	}
	objectLimit := h.backupRestoreReader.RestoreObjectLimit()
	current, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, compare.TargetPath, objectLimit)
	if err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	fingerprint, err := filesystem.FingerprintRegularFileSnapshot(current)
	if err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	compare.OtherExists = true
	compare.OtherFingerprint = fingerprint
	compare.OtherObjectVerified = true
	compare.Equal = compare.BackupFingerprint == fingerprint
	if left.Manifest().ObjectBytes <= restoreDiffInputMaxBytes && current.Size <= restoreDiffInputMaxBytes {
		currentBytes, readErr := readRestoreDiffBytes(compare.TargetPath, restoreDiffInputMaxBytes)
		if readErr == nil && int64(len(currentBytes)) == current.Size && current.Verify(compare.TargetPath) == nil {
			leftBytes, leftErr := left.ReadAll(ctx, restoreDiffInputMaxBytes)
			if leftErr == nil {
				compare.Diff = backupCompareTextDiff(leftBytes, currentBytes, compare.TargetPath)
				compare.DiffAvailable = compare.Diff != ""
			}
		}
	}
	output := BackupStoreOutput{Action: BackupStoreActionCompare, Enabled: true, State: BackupStoreStateReady, Compare: compare}
	return h.finishBackupStoreOutput(output, fmt.Sprintf("Compared backup %s with current target; equal=%t.", compare.BackupID, compare.Equal))
}

func boundedBackupCompareDiff(ctx context.Context, left *backupstore.ReadSource, leftBytes int64, right *backupstore.ReadSource, rightBytes int64, path string) string {
	if leftBytes > restoreDiffInputMaxBytes || rightBytes > restoreDiffInputMaxBytes {
		return ""
	}
	leftData, err := left.ReadAll(ctx, restoreDiffInputMaxBytes)
	if err != nil {
		return ""
	}
	rightData, err := right.ReadAll(ctx, restoreDiffInputMaxBytes)
	if err != nil {
		return ""
	}
	return backupCompareTextDiff(leftData, rightData, path)
}

func backupCompareTextDiff(left, right []byte, path string) string {
	leftText, leftOK := decodeRestoreText(left)
	rightText, rightOK := decodeRestoreText(right)
	if !leftOK || !rightOK {
		return ""
	}
	diff := createUnifiedDiff(ConvertLineEndings(leftText, LineEndingLF), ConvertLineEndings(rightText, LineEndingLF), path)
	if int64(len(diff)) > restoreDiffInputMaxBytes {
		return ""
	}
	return diff
}
