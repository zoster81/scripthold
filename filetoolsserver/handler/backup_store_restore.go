package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

const maxRestoreFailureMessageBytes = 1024

func (h *Handler) handleBackupStoreRestorePreview(ctx context.Context, backupID string, visibility backupVisibilitySnapshot) (*mcp.CallToolResult, BackupStoreOutput, error) {
	if h.backupRestoreReader == nil || h.backupCapturePreflight == nil {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store does not provide restore preview authority")), BackupStoreOutput{}, nil
	}
	validatedTarget := ""
	source, err := h.backupRestoreReader.OpenReadSource(ctx, backupID, backupstore.RestoreSourceOptions{
		AuthorizeTarget: func(path string) error {
			var authorizationErr error
			validatedTarget, authorizationErr = visibility.validate(path)
			return authorizationErr
		},
	})
	if err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	prepared := preparedRestore{source: source}
	keepPrepared := false
	defer func() {
		if !keepPrepared {
			_ = prepared.close()
		}
	}()

	manifest := source.Manifest()
	if validatedTarget == "" || !sameResolvedRestoreTarget(manifest.TargetPath, validatedTarget) {
		return errorResultWithCode(ErrCodeConflict, "restore is restricted to the manifest's original target"), BackupStoreOutput{}, nil
	}
	objectLimit := h.backupRestoreReader.RestoreObjectLimit()
	if objectLimit <= 0 || manifest.ObjectBytes < 0 || manifest.ObjectBytes > objectLimit {
		return errorResultFromError(operation.New(operation.KindLimit, "restore object exceeds the configured object limit")), BackupStoreOutput{}, nil
	}

	prepared.backupID = manifest.BackupID
	prepared.requestedPath = manifest.TargetPath
	prepared.resolvedPath = validatedTarget
	prepared.resultFingerprint = manifest.ContentFingerprint
	prepared.objectBytes = manifest.ObjectBytes
	prepared.restoreMode = manifest.OriginalMode
	prepared.restoreModTime, err = time.Parse(time.RFC3339Nano, manifest.OriginalModTime)
	if err != nil {
		return errorResultFromError(operation.New(operation.KindFilesystem, "backup manifest restore timestamp is invalid")), BackupStoreOutput{}, nil
	}

	lstatInfo, lstatErr := os.Lstat(validatedTarget)
	switch {
	case os.IsNotExist(lstatErr):
		prepared.targetSnapshot, err = filesystem.CaptureSnapshot(validatedTarget)
		if err != nil {
			return errorResultFromError(err), BackupStoreOutput{}, nil
		}
	case lstatErr != nil:
		return errorResultFromError(operation.WrapFilesystem("inspect_restore_target", validatedTarget, lstatErr)), BackupStoreOutput{}, nil
	case lstatInfo.Mode()&os.ModeSymlink != 0 || !lstatInfo.Mode().IsRegular():
		return errorResultFromError(operation.New(operation.KindConflict, "restore target is not a real regular file")), BackupStoreOutput{}, nil
	default:
		prepared.targetExisted = true
		prepared.targetIdentity, err = filesystem.OpenFileIdentity(validatedTarget)
		if err != nil {
			return errorResultFromError(err), BackupStoreOutput{}, nil
		}
		prepared.targetSnapshot, err = filesystem.CaptureRegularFileSnapshotBounded(ctx, validatedTarget, objectLimit)
		if err != nil {
			return errorResultFromError(err), BackupStoreOutput{}, nil
		}
		matches, matchErr := prepared.targetIdentity.Matches(validatedTarget)
		if matchErr != nil || !matches {
			return errorResultWithCode(ErrCodeConflict, "restore target identity changed during preview"), BackupStoreOutput{}, nil
		}
		prepared.targetFingerprint, err = filesystem.FingerprintRegularFileSnapshot(prepared.targetSnapshot)
		if err != nil {
			return errorResultFromError(err), BackupStoreOutput{}, nil
		}
		if err := h.backupCapturePreflight.PreflightCaptureBatch(ctx, []backupstore.CaptureRequest{{
			TargetPath:      validatedTarget,
			SourceOperation: backupstore.SourceOperationRestore,
		}}); err != nil {
			return errorResultFromError(err), BackupStoreOutput{}, nil
		}
	}
	if err := source.Verify(ctx); err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	prepared.diff = h.restorePreviewDiff(ctx, &prepared)

	preview, err := h.restorePreviews.put(prepared)
	if err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	keepPrepared = true
	output := BackupStoreOutput{
		Action:  BackupStoreActionRestorePreview,
		Enabled: true,
		State:   BackupStoreStateReady,
		Restore: restoreOutputFromPreview(preview),
	}
	text := restorePreviewText(output.Restore)
	if err := h.checkBackupStoreOutputLimit(output, text); err != nil {
		h.restorePreviews.discard(preview.id)
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) handleBackupStoreRestoreApply(ctx context.Context, previewID string) (*mcp.CallToolResult, BackupStoreOutput, error) {
	if h.backupRestoreStager == nil || h.backupCapture == nil {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store does not provide restore apply authority")), BackupStoreOutput{}, nil
	}
	preview, err := h.restorePreviews.claim(previewID)
	if err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	prepared := &preview.prepared
	defer prepared.close()
	output := BackupStoreOutput{
		Action:  BackupStoreActionRestoreApply,
		Enabled: true,
		State:   BackupStoreStateReady,
		Restore: restoreOutputFromPreview(preview),
	}
	output.Restore.PreviewID = ""
	output.Restore.CreatedAt = ""
	output.Restore.ExpiresAt = ""

	worst := output
	worst.Restore = cloneRestoreOutput(output.Restore)
	worst.Restore.State = BackupStoreRestoreStateUnknown
	worst.Restore.ActualFingerprint = strings.Repeat("f", restorePreviewTokenBytes*2)
	worst.Restore.Applied = true
	worst.Restore.ReadOnlyCleared = true
	if prepared.targetExisted {
		worst.Restore.SafetyBackupID = strings.Repeat("f", restorePreviewTokenBytes*2)
	}
	if err := h.checkBackupStoreOutputLimit(worst, restoreFailureText(worst.Restore, strings.Repeat("\x00", maxRestoreFailureMessageBytes))); err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	if err := ctx.Err(); err != nil {
		cancelled := operation.Wrap(operation.KindCancelled, "apply_restore_preview", prepared.resolvedPath, err)
		return h.restoreApplyFailure(ctx, prepared, output, cancelled, nil, false, 0)
	}
	if err := h.revalidateRestoreAuthorization(prepared); err != nil {
		return h.restoreApplyFailure(ctx, prepared, output, err, nil, false, 0)
	}
	if err := prepared.source.Verify(ctx); err != nil {
		return h.restoreApplyFailure(ctx, prepared, output, err, nil, false, 0)
	}
	current, err := h.revalidatePreparedRestoreTarget(ctx, prepared)
	if err != nil {
		return h.restoreApplyFailure(ctx, prepared, output, err, nil, false, 0)
	}

	mode := os.FileMode(prepared.restoreMode).Perm()
	modTime := prepared.restoreModTime
	var staged *filesystem.StagedReplacement
	if prepared.targetExisted {
		captured, captureErr := h.backupCapture.Capture(ctx, backupstore.CaptureRequest{
			TargetPath:      prepared.resolvedPath,
			SourceOperation: backupstore.SourceOperationRestore,
		})
		if validRestorePreviewID(captured.Manifest.BackupID) {
			output.Restore.SafetyBackupID = captured.Manifest.BackupID
		}
		if captured.Manifest.BackupID == "" {
			if captureErr == nil {
				captureErr = operation.New(operation.KindFilesystem, "restore safety backup did not commit a manifest")
			}
			return h.restoreApplyFailure(ctx, prepared, output, captureErr, staged, false, 0)
		}
		if !validRestorePreviewID(captured.Manifest.BackupID) || captured.Manifest.TargetPath != prepared.resolvedPath ||
			captured.Manifest.SourceOperation != backupstore.SourceOperationRestore ||
			captured.Manifest.ContentFingerprint != prepared.targetFingerprint {
			return h.restoreApplyFailure(ctx, prepared, output, operation.New(operation.KindConflict, "restore safety backup does not match the approved current state"), staged, false, 0)
		}
		if captureErr != nil {
			slog.Warn("restore safety manifest committed; continuing after a post-manifest store error")
		}
		if err := prepared.source.Verify(ctx); err != nil {
			return h.restoreApplyFailure(ctx, prepared, output, err, staged, false, 0)
		}
		current, err = h.revalidatePreparedRestoreTarget(ctx, prepared)
		if err != nil {
			return h.restoreApplyFailure(ctx, prepared, output, err, staged, false, 0)
		}
	}

	staged, err = h.restoreStageReplacement(ctx, prepared.source, prepared.resolvedPath, mode, &modTime)
	if err != nil {
		return h.restoreApplyFailure(ctx, prepared, output, err, nil, false, 0)
	}
	if err := prepared.source.Verify(ctx); err != nil {
		return h.restoreApplyFailure(ctx, prepared, output, err, staged, false, 0)
	}
	current, err = h.revalidatePreparedRestoreTarget(ctx, prepared)
	if err != nil {
		return h.restoreApplyFailure(ctx, prepared, output, err, staged, false, 0)
	}

	originalMode := current.Mode.Perm()
	readOnlyCleared := false
	if prepared.targetExisted && isReadOnly(originalMode) {
		if err := clearReadOnly(prepared.resolvedPath, originalMode); err != nil {
			return h.restoreApplyFailure(ctx, prepared, output, operation.WrapFilesystem("clear_restore_read_only", prepared.resolvedPath, err), staged, false, originalMode)
		}
		readOnlyCleared = true
		refreshed, refreshErr := current.RefreshMetadata(prepared.resolvedPath)
		if refreshErr != nil {
			_ = os.Chmod(prepared.resolvedPath, originalMode)
			return h.restoreApplyFailure(ctx, prepared, output, refreshErr, staged, false, originalMode)
		}
		current = refreshed
	}
	if prepared.targetIdentity != nil {
		if err := prepared.targetIdentity.Close(); err != nil {
			return h.restoreApplyFailure(ctx, prepared, output, operation.WrapFilesystem("close_restore_target_identity", prepared.resolvedPath, err), staged, readOnlyCleared, originalMode)
		}
		prepared.targetIdentity = nil
	}

	_, commitErr := h.restoreCommitReplacement(staged, filesystem.ReplaceOptions{Expected: &current})
	if commitErr != nil {
		return h.restoreApplyFailure(ctx, prepared, output, commitErr, nil, readOnlyCleared, originalMode)
	}
	post, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, prepared.resolvedPath, h.backupRestoreReader.RestoreObjectLimit())
	if err != nil {
		return h.restoreApplyFailure(ctx, prepared, output, err, nil, false, 0)
	}
	actual, err := filesystem.FingerprintRegularFileSnapshot(post)
	if err != nil {
		return h.restoreApplyFailure(ctx, prepared, output, err, nil, false, 0)
	}
	if actual != prepared.resultFingerprint {
		return h.restoreApplyFailure(ctx, prepared, output, operation.New(operation.KindConflict, "restored target does not match the backup fingerprint"), nil, false, 0)
	}
	output.Restore.ActualFingerprint = actual
	output.Restore.State = BackupStoreRestoreStateRestored
	output.Restore.Applied = true
	output.Restore.ReadOnlyCleared = readOnlyCleared
	text := restoreApplyText(output.Restore)
	if err := h.checkBackupStoreOutputLimit(output, text); err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) revalidateRestoreAuthorization(prepared *preparedRestore) error {
	visibility := h.backupVisibilitySnapshot()
	validated, err := visibility.validate(prepared.requestedPath)
	if err != nil {
		return err
	}
	if !sameResolvedRestoreTarget(validated, prepared.resolvedPath) {
		return operation.New(operation.KindConflict, "restore target authorization or resolution changed after preview")
	}
	return nil
}

func sameResolvedRestoreTarget(first, second string) bool {
	if security.PathsEqual(first, second) {
		return true
	}
	firstResolved := security.ResolveAllowedDirs([]string{first})
	secondResolved := security.ResolveAllowedDirs([]string{second})
	return len(firstResolved) == 1 && len(secondResolved) == 1 &&
		security.PathsEqual(firstResolved[0], secondResolved[0])
}

func (h *Handler) revalidatePreparedRestoreTarget(ctx context.Context, prepared *preparedRestore) (filesystem.FileSnapshot, error) {
	if err := h.revalidateRestoreAuthorization(prepared); err != nil {
		return filesystem.FileSnapshot{}, err
	}
	if !prepared.targetExisted {
		current, err := filesystem.CaptureSnapshot(prepared.resolvedPath)
		if err != nil {
			return filesystem.FileSnapshot{}, err
		}
		if current.Exists {
			return filesystem.FileSnapshot{}, operation.New(operation.KindConflict, "restore target appeared after preview")
		}
		return current, nil
	}
	if prepared.targetIdentity == nil {
		return filesystem.FileSnapshot{}, operation.New(operation.KindConflict, "restore target identity is unavailable")
	}
	matches, err := prepared.targetIdentity.Matches(prepared.resolvedPath)
	if err != nil || !matches {
		return filesystem.FileSnapshot{}, operation.New(operation.KindConflict, "restore target identity changed after preview")
	}
	current, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, prepared.resolvedPath, h.backupRestoreReader.RestoreObjectLimit())
	if err != nil {
		return filesystem.FileSnapshot{}, operation.New(operation.KindConflict, "restore target is unavailable or changed after preview")
	}
	fingerprint, err := filesystem.FingerprintRegularFileSnapshot(current)
	if err != nil {
		return filesystem.FileSnapshot{}, err
	}
	if fingerprint != prepared.targetFingerprint {
		return filesystem.FileSnapshot{}, operation.New(operation.KindConflict, "restore target fingerprint changed after preview")
	}
	return current, nil
}

func (h *Handler) restoreApplyFailure(ctx context.Context, prepared *preparedRestore, output BackupStoreOutput, cause error, staged *filesystem.StagedReplacement, readOnlyCleared bool, originalMode os.FileMode) (*mcp.CallToolResult, BackupStoreOutput, error) {
	if staged != nil {
		cause = errors.Join(cause, h.restoreCleanupReplacement(staged))
	}
	if readOnlyCleared && prepared.targetExisted {
		if restoreErr := h.restoreReadOnlyIfUnchanged(prepared, originalMode); restoreErr != nil {
			cause = errors.Join(cause, restoreErr)
		}
	}
	state, actual, applied := h.classifyRestoreTarget(prepared)
	output.Restore.State = state
	output.Restore.ActualFingerprint = actual
	output.Restore.Applied = applied
	output.Restore.ReadOnlyCleared = readOnlyCleared && applied
	mapping := mapOperationError(cause, "")
	text := restoreFailureText(output.Restore, boundedRestoreFailureMessage(mapping.Message))
	if err := h.checkBackupStoreOutputLimit(output, text); err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	return errorResultWithCode(mapping.BatchCode, text), output, nil
}

func (h *Handler) restoreReadOnlyIfUnchanged(prepared *preparedRestore, originalMode os.FileMode) error {
	if originalMode.Perm() == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), patchPackageClassificationTimeout)
	defer cancel()
	current, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, prepared.resolvedPath, h.backupRestoreReader.RestoreObjectLimit())
	if err != nil {
		return nil
	}
	fingerprint, err := filesystem.FingerprintRegularFileSnapshot(current)
	if err != nil || fingerprint != prepared.targetFingerprint {
		return nil
	}
	if err := os.Chmod(prepared.resolvedPath, originalMode); err != nil {
		return operation.WrapFilesystem("restore_target_mode_after_failure", prepared.resolvedPath, err)
	}
	return nil
}

func (h *Handler) classifyRestoreTarget(prepared *preparedRestore) (string, string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), patchPackageClassificationTimeout)
	defer cancel()
	validation := h.ValidatePath(prepared.requestedPath)
	if !validation.Ok() || validation.Path != prepared.resolvedPath {
		return BackupStoreRestoreStateUnknown, "", false
	}
	info, err := os.Lstat(prepared.resolvedPath)
	if os.IsNotExist(err) {
		if prepared.targetExisted {
			return BackupStoreRestoreStateUnknown, "", false
		}
		return BackupStoreRestoreStateMissing, "", false
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return BackupStoreRestoreStateUnknown, "", false
	}
	snapshot, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, prepared.resolvedPath, h.backupRestoreReader.RestoreObjectLimit())
	if err != nil {
		return BackupStoreRestoreStateUnknown, "", false
	}
	fingerprint, err := filesystem.FingerprintRegularFileSnapshot(snapshot)
	if err != nil {
		return BackupStoreRestoreStateUnknown, "", false
	}
	switch fingerprint {
	case prepared.resultFingerprint:
		return BackupStoreRestoreStateRestored, fingerprint, true
	case prepared.targetFingerprint:
		return BackupStoreRestoreStateUnchanged, fingerprint, false
	default:
		return BackupStoreRestoreStateUnknown, fingerprint, false
	}
}

func (h *Handler) restorePreviewDiff(ctx context.Context, prepared *preparedRestore) string {
	if prepared == nil || prepared.source == nil || prepared.objectBytes > restoreDiffInputMaxBytes {
		return ""
	}
	var currentBytes []byte
	if prepared.targetExisted {
		if prepared.targetSnapshot.Size > restoreDiffInputMaxBytes {
			return ""
		}
		var err error
		currentBytes, err = readRestoreDiffBytes(prepared.resolvedPath, restoreDiffInputMaxBytes)
		if err != nil || int64(len(currentBytes)) != prepared.targetSnapshot.Size || prepared.targetSnapshot.Verify(prepared.resolvedPath) != nil {
			return ""
		}
	}
	resultBytes, err := prepared.source.ReadAll(ctx, restoreDiffInputMaxBytes)
	if err != nil {
		return ""
	}
	currentText, currentOK := decodeRestoreText(currentBytes)
	resultText, resultOK := decodeRestoreText(resultBytes)
	if !currentOK || !resultOK {
		return ""
	}
	diff := createUnifiedDiff(ConvertLineEndings(currentText, LineEndingLF), ConvertLineEndings(resultText, LineEndingLF), prepared.requestedPath)
	if int64(len(diff)) > restoreDiffInputMaxBytes {
		return ""
	}
	return diff
}

func readRestoreDiffBytes(path string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, operation.New(operation.KindInvalidInput, "restore diff read limit must be positive")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, operation.New(operation.KindLimit, "restore diff input exceeds the configured limit")
	}
	return data, nil
}

func decodeRestoreText(data []byte) (string, bool) {
	detected := fileEncoding.Detect(data)
	if detected.Charset == "" || (!detected.HasBOM && detected.Confidence < fileEncoding.MinConfidenceThreshold) {
		return "", false
	}
	payload := data
	if detected.HasBOM {
		bomSize := fileEncoding.BOMSize(detected.Charset)
		if bomSize <= 0 || bomSize > len(payload) {
			return "", false
		}
		payload = payload[bomSize:]
	}
	reader, err := fileEncoding.NewDecoderReader(bytes.NewReader(payload), detected.Charset)
	if err != nil {
		return "", false
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, restoreDiffInputMaxBytes*4+1))
	if err != nil || int64(len(decoded)) > restoreDiffInputMaxBytes*4 || !utf8.Valid(decoded) || bytes.IndexByte(decoded, 0) >= 0 {
		return "", false
	}
	return string(decoded), true
}

func restoreOutputFromPreview(preview *restorePreview) *BackupStoreRestoreOutput {
	prepared := preview.prepared
	return &BackupStoreRestoreOutput{
		BackupID:           prepared.backupID,
		PreviewID:          preview.id,
		CreatedAt:          preview.createdAt.Format(timeRFC3339Nano),
		ExpiresAt:          preview.expiresAt.Format(timeRFC3339Nano),
		TargetPath:         prepared.requestedPath,
		TargetExisted:      prepared.targetExisted,
		CurrentFingerprint: prepared.targetFingerprint,
		ResultFingerprint:  prepared.resultFingerprint,
		ObjectBytes:        prepared.objectBytes,
		ObjectVerified:     true,
		Diff:               prepared.diff,
		State:              BackupStoreRestoreStatePrepared,
	}
}

func cloneRestoreOutput(output *BackupStoreRestoreOutput) *BackupStoreRestoreOutput {
	if output == nil {
		return nil
	}
	copy := *output
	return &copy
}

func restorePreviewText(output *BackupStoreRestoreOutput) string {
	return fmt.Sprintf("Restore preview prepared for the original target.\nPreview ID: %s\nExpires: %s\nBackup ID: %s\nResult fingerprint: %s",
		output.PreviewID, output.ExpiresAt, output.BackupID, output.ResultFingerprint)
}

func restoreApplyText(output *BackupStoreRestoreOutput) string {
	text := fmt.Sprintf("Restore applied to the original target.\nBackup ID: %s\nResult fingerprint: %s", output.BackupID, output.ActualFingerprint)
	if output.SafetyBackupID != "" {
		text += "\nSafety backup ID: " + output.SafetyBackupID
	}
	return text
}

func boundedRestoreFailureMessage(message string) string {
	message = strings.ToValidUTF8(message, "\uFFFD")
	if len(message) <= maxRestoreFailureMessageBytes {
		return message
	}
	end := maxRestoreFailureMessageBytes
	for end > 0 && !utf8.ValidString(message[:end]) {
		end--
	}
	return message[:end]
}

func restoreFailureText(output *BackupStoreRestoreOutput, message string) string {
	text := fmt.Sprintf("Restore apply failed or completed with an operational error.\nState: %s\nReason: %s", output.State, message)
	if output.SafetyBackupID != "" {
		text += "\nSafety backup ID: " + output.SafetyBackupID
	}
	return text
}
