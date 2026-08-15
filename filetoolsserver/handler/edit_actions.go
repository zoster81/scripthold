package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

const (
	editActionDirect         = "direct"
	editActionPreview        = "preview"
	editActionApply          = "apply"
	editBackupPolicyRequired = "required"
	editApplyStateUnchanged  = "unchanged"
	editApplyStateCommitted  = "committed"
	editApplyStateUnknown    = "unknown"

	editApplyClassificationTimeout = 30 * time.Second
)

func normalizeEditBackupPolicy(value string) (string, error) {
	if value == "" || value == editBackupPolicyRequired {
		return value, nil
	}
	return "", operation.New(operation.KindInvalidInput, "backupPolicy must be exactly required when provided")
}

func (h *Handler) effectivePersistentBackupPolicy(requested string) (string, error) {
	requested, err := normalizeEditBackupPolicy(requested)
	if err != nil {
		return "", err
	}
	if requested == editBackupPolicyRequired {
		return editBackupPolicyRequired, nil
	}
	if h != nil && h.config != nil && h.config.Backup.DefaultPolicy == "required" {
		return editBackupPolicyRequired, nil
	}
	return "", nil
}

func validateEditActionInput(input EditFileInput) (string, error) {
	backupPolicy, err := normalizeEditBackupPolicy(input.BackupPolicy)
	if err != nil {
		return "", err
	}
	action := strings.ToLower(strings.TrimSpace(input.Action))
	if action == "" {
		action = editActionDirect
	}
	switch action {
	case editActionDirect:
		if input.PreviewID != "" {
			return "", operation.New(operation.KindInvalidInput, "previewId is valid only with action=apply")
		}
		if backupPolicy != "" {
			return "", operation.New(operation.KindInvalidInput, "backupPolicy is valid only with action=preview")
		}
	case editActionPreview:
		if input.PreviewID != "" {
			return "", operation.New(operation.KindInvalidInput, "previewId is not accepted with action=preview")
		}
		if input.DryRun {
			return "", operation.New(operation.KindInvalidInput, "dryRun is not accepted with action=preview")
		}
	case editActionApply:
		if !validEditPreviewID(input.PreviewID) {
			return "", operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")
		}
		if input.Path != "" || len(input.Edits) != 0 || strings.TrimSpace(input.Patch) != "" ||
			input.DryRun || input.Encoding != "" || input.ForceWritable != nil || input.BackupPolicy != "" {
			return "", operation.New(operation.KindInvalidInput, "action=apply accepts only previewId")
		}
	default:
		return "", operation.New(operation.KindInvalidInput, "action must be direct, preview, or apply")
	}
	return action, nil
}

func (h *Handler) prepareEdit(ctx context.Context, input EditFileInput) (preparedEdit, *mcp.CallToolResult) {
	backupPolicy, err := h.effectivePersistentBackupPolicy(input.BackupPolicy)
	if err != nil {
		return preparedEdit{}, errorResultFromError(err)
	}
	if len(input.Edits) == 0 && strings.TrimSpace(input.Patch) == "" {
		return preparedEdit{}, errorResult("edits or patch is required")
	}
	if len(input.Edits) > 0 && strings.TrimSpace(input.Patch) != "" {
		return preparedEdit{}, errorResult("edits and patch are mutually exclusive")
	}

	validation := h.ValidatePath(input.Path)
	if !validation.Ok() {
		return preparedEdit{}, validation.Result
	}

	var identityFile *filesystem.FileIdentity
	keepIdentityFile := false
	if strings.EqualFold(strings.TrimSpace(input.Action), editActionPreview) {
		var err error
		identityFile, err = filesystem.OpenFileIdentity(validation.Path)
		if err != nil {
			return preparedEdit{}, errorResultFromError(err)
		}
		defer func() {
			if !keepIdentityFile {
				_ = identityFile.Close()
			}
		}()
	}

	document, sourceData, err := h.readTextDocumentWithData(ctx, validation.Path, input.Encoding)
	if err != nil {
		return preparedEdit{}, errorResultFromError(err)
	}
	if identityFile != nil {
		matches, identityErr := identityFile.Matches(validation.Path)
		if identityErr != nil {
			return preparedEdit{}, errorResultFromError(identityErr)
		}
		if !matches {
			return preparedEdit{}, errorResultWithCode(ErrCodeConflict, "target file identity changed while preparing edit preview")
		}
	}

	readOnly := isReadOnly(document.Mode)
	forceWritable := input.ForceWritable != nil && *input.ForceWritable
	if readOnly && !forceWritable {
		return preparedEdit{}, errorResultWithCode(ErrCodePermission, "file is read-only — STOP, do NOT retry and do NOT attempt to change file attributes. Ask the user whether to proceed with forceWritable: true, or skip this file")
	}
	if document.LineEndings.Style == LineEndingMixed {
		slog.Warn("file has mixed line endings", "path", input.Path, "crlf", document.LineEndings.CRLFCount, "lf", document.LineEndings.LFCount)
	}

	content := ConvertLineEndings(document.Text, LineEndingLF)
	var modifiedContent string
	if strings.TrimSpace(input.Patch) != "" {
		modifiedContent, err = applyUnifiedPatch(content, input.Patch, input.Path, h.maxFileBytes())
	} else {
		modifiedContent, err = applyEdits(content, input.Edits)
	}
	if err != nil {
		return preparedEdit{}, errorResultFromError(err)
	}

	changed := modifiedContent != content
	dataToWrite := append([]byte(nil), sourceData...)
	if changed {
		contentToWrite := restoreDocumentLineEndings(modifiedContent, document.LineEndings.Style)
		dataToWrite, err = encodeTextDocument(document, contentToWrite, bomPreserve)
		if err != nil {
			return preparedEdit{}, errorResult(fmt.Sprintf("failed to encode file: %v", err))
		}
	}
	if int64(len(dataToWrite)) > h.maxFileBytes() {
		limitErr := operation.New(operation.KindLimit, fmt.Sprintf("prepared file size %d exceeds the %d-byte edit limit", len(dataToWrite), h.maxFileBytes()))
		return preparedEdit{}, errorResultFromError(limitErr)
	}

	targetFingerprint, err := filesystem.FingerprintRegularFileSnapshot(document.Snapshot)
	if err != nil {
		return preparedEdit{}, errorResultFromError(err)
	}
	prepared := preparedEdit{
		requestedPath:     input.Path,
		resolvedPath:      validation.Path,
		data:              dataToWrite,
		diff:              createUnifiedDiff(content, modifiedContent, input.Path),
		targetFingerprint: targetFingerprint,
		resultFingerprint: filesystem.FingerprintRegularFileData(dataToWrite),
		encoding:          document.Charset,
		hasBOM:            document.BOM.HasBOM,
		bomType:           document.BOM.Type,
		lineEndingStyle:   document.LineEndings.Style,
		backupPolicy:      backupPolicy,
		changed:           changed,
		forceWritable:     forceWritable,
		sourceMode:        document.Mode,
		sourceSnapshot:    document.Snapshot,
		identityFile:      identityFile,
	}
	keepIdentityFile = identityFile != nil
	return prepared, nil
}

func (h *Handler) handleDirectEdit(ctx context.Context, input EditFileInput) (*mcp.CallToolResult, EditFileOutput, error) {
	prepared, failure := h.prepareEdit(ctx, input)
	if failure != nil {
		return failure, EditFileOutput{}, nil
	}
	output := editOutputFromPrepared(editActionDirect, prepared)
	worstCase := output
	worstCase.ReadOnlyCleared = true
	if err := h.checkEditResponseLimit(worstCase, prepared.diff+"\nRead-only flag was cleared."); err != nil {
		return errorResultFromError(err), EditFileOutput{}, nil
	}

	readOnlyCleared := false
	if !input.DryRun && prepared.changed {
		var commitFailure *mcp.CallToolResult
		readOnlyCleared, commitFailure = h.commitPreparedEdit(ctx, prepared, prepared.sourceSnapshot, prepared.sourceMode)
		if commitFailure != nil {
			return commitFailure, EditFileOutput{}, nil
		}
	}
	output.ReadOnlyCleared = readOnlyCleared
	output.Applied = !input.DryRun && prepared.changed
	text := prepared.diff
	if readOnlyCleared {
		text += "\nRead-only flag was cleared."
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) handleEditPreview(ctx context.Context, input EditFileInput) (*mcp.CallToolResult, EditFileOutput, error) {
	backupPolicy, err := h.effectivePersistentBackupPolicy(input.BackupPolicy)
	if err != nil {
		return errorResultFromError(err), EditFileOutput{}, nil
	}
	prepared, failure := h.prepareEdit(ctx, input)
	if failure != nil {
		return failure, EditFileOutput{}, nil
	}
	if prepared.changed && backupPolicy == editBackupPolicyRequired {
		if h.backupCapturePreflight == nil {
			if prepared.identityFile != nil {
				_ = prepared.identityFile.Close()
				prepared.identityFile = nil
			}
			return errorResultFromError(operation.New(operation.KindInvalidInput, "required backup preflight authority is unavailable")), EditFileOutput{}, nil
		}
		if err := h.backupCapturePreflight.PreflightCaptureBatch(ctx, []backupstore.CaptureRequest{{
			TargetPath:      prepared.resolvedPath,
			SourceOperation: backupstore.SourceOperationEdit,
		}}); err != nil {
			if prepared.identityFile != nil {
				_ = prepared.identityFile.Close()
				prepared.identityFile = nil
			}
			return errorResultFromError(err), EditFileOutput{}, nil
		}
	}
	preview, err := h.editPreviews.put(prepared)
	if err != nil {
		if prepared.identityFile != nil {
			_ = prepared.identityFile.Close()
		}
		return errorResultFromError(err), EditFileOutput{}, nil
	}
	output := editOutputFromPreview(preview)
	text := editPreviewText(output)
	if err := h.checkEditResponseLimit(output, text); err != nil {
		h.editPreviews.discard(preview.id)
		return errorResultFromError(err), EditFileOutput{}, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) handleEditApply(ctx context.Context, previewID string) (*mcp.CallToolResult, EditFileOutput, error) {
	preview, err := h.editPreviews.claim(previewID)
	if err != nil {
		return errorResultFromError(err), EditFileOutput{}, nil
	}
	prepared := preview.prepared
	if prepared.identityFile != nil {
		defer prepared.identityFile.Close()
	}
	if err := ctx.Err(); err != nil {
		cancelled := operation.Wrap(operation.KindCancelled, "apply_edit_preview", prepared.resolvedPath, err)
		return errorResultFromError(cancelled), EditFileOutput{}, nil
	}
	if filesystem.FingerprintRegularFileData(prepared.data) != prepared.resultFingerprint {
		return errorResultWithCode(ErrCodeConflict, "prepared edit result no longer matches its fingerprint"), EditFileOutput{}, nil
	}

	validation := h.ValidatePath(prepared.requestedPath)
	if !validation.Ok() {
		return validation.Result, EditFileOutput{}, nil
	}
	if validation.Path != prepared.resolvedPath {
		return errorResultWithCode(ErrCodeConflict, "path changed after edit preview"), EditFileOutput{}, nil
	}
	if prepared.identityFile == nil {
		return errorResultWithCode(ErrCodeConflict, "edit preview identity is unavailable"), EditFileOutput{}, nil
	}
	matches, err := prepared.identityFile.Matches(validation.Path)
	if err != nil || !matches {
		return errorResultWithCode(ErrCodeConflict, "target file identity changed after edit preview"), EditFileOutput{}, nil
	}
	current, err := filesystem.CaptureSnapshotWithDigest(validation.Path)
	if err != nil {
		return errorResultWithCode(ErrCodeConflict, "target is unavailable or changed after edit preview"), EditFileOutput{}, nil
	}
	currentFingerprint, err := filesystem.FingerprintRegularFileSnapshot(current)
	if err != nil {
		return errorResultFromError(err), EditFileOutput{}, nil
	}
	if currentFingerprint != prepared.targetFingerprint {
		return errorResultWithCode(ErrCodeConflict, "target fingerprint changed after edit preview"), EditFileOutput{}, nil
	}

	output := editOutputFromPreview(preview)
	output.Action = editActionApply
	output.PreviewID = ""
	output.Applied = false
	output.Changed = false
	output.State = editApplyStateUnchanged
	output.ActualFingerprint = currentFingerprint
	worstCase := output
	worstCase.Applied = true
	worstCase.ReadOnlyCleared = true
	if prepared.changed && prepared.backupPolicy == editBackupPolicyRequired {
		worstCase.BackupID = strings.Repeat("f", editPreviewTokenBytes*2)
	}
	if err := h.checkEditResponseLimit(worstCase, editApplyText(worstCase)+"\nRead-only flag was cleared."); err != nil {
		return errorResultFromError(err), EditFileOutput{}, nil
	}
	matches, err = prepared.identityFile.Matches(validation.Path)
	if err != nil || !matches {
		return errorResultWithCode(ErrCodeConflict, "target file identity changed before edit commit"), EditFileOutput{}, nil
	}

	if prepared.changed && prepared.backupPolicy == editBackupPolicyRequired {
		if h.backupCapture == nil {
			err := operation.New(operation.KindConflict, "required backup store is unavailable")
			return errorResultFromError(err), output, nil
		}
		captured, captureErr := h.backupCapture.Capture(ctx, backupstore.CaptureRequest{
			TargetPath:      validation.Path,
			SourceOperation: backupstore.SourceOperationEdit,
		})
		if captured.Manifest.BackupID == "" {
			if captureErr == nil {
				captureErr = operation.New(operation.KindFilesystem, "required backup did not commit a manifest")
			}
			return errorResultFromError(captureErr), output, nil
		}
		output.BackupID = captured.Manifest.BackupID
		if !validEditPreviewID(output.BackupID) || captured.Manifest.TargetPath != validation.Path ||
			captured.Manifest.SourceOperation != backupstore.SourceOperationEdit ||
			captured.Manifest.ContentFingerprint != prepared.targetFingerprint {
			return errorResultWithCode(ErrCodeConflict, "durable backup does not match the approved edit pre-state"), output, nil
		}
		if captureErr != nil {
			// The manifest is authoritative and durable; the derived index is disposable.
			slog.Warn("backup manifest committed but derived index refresh reported an error")
		}
		matches, err = prepared.identityFile.Matches(validation.Path)
		if err != nil || !matches {
			return errorResultWithCode(ErrCodeConflict, "target file identity changed after durable backup"), output, nil
		}
	}

	if err := prepared.identityFile.Close(); err != nil {
		failure := errorResultFromError(operation.WrapFilesystem("close_edit_preview_identity", validation.Path, err))
		return failure, output, nil
	}
	prepared.identityFile = nil

	readOnlyCleared := false
	if prepared.changed {
		var commitFailure *mcp.CallToolResult
		readOnlyCleared, commitFailure = h.commitPreparedEdit(ctx, prepared, current, current.Mode.Perm())
		if commitFailure != nil {
			return h.classifyEditApplyFailure(prepared, output, commitFailure)
		}
		output.ReadOnlyCleared = readOnlyCleared
	}
	post, err := filesystem.CaptureSnapshotWithDigest(validation.Path)
	if err != nil {
		return h.classifyEditApplyFailure(prepared, output, errorResultFromError(err))
	}
	actualFingerprint, err := filesystem.FingerprintRegularFileSnapshot(post)
	if err != nil {
		return h.classifyEditApplyFailure(prepared, output, errorResultFromError(err))
	}
	if actualFingerprint != prepared.resultFingerprint {
		return h.classifyEditApplyFailure(prepared, output, errorResultWithCode(ErrCodeConflict, "applied file does not match the prepared result fingerprint"))
	}
	output.ReadOnlyCleared = readOnlyCleared
	output.ActualFingerprint = actualFingerprint
	output.State = editApplyStateUnchanged
	output.Changed = false
	if prepared.changed {
		output.State = editApplyStateCommitted
		output.Changed = true
	}
	output.Applied = prepared.changed
	text := editApplyText(output)
	if readOnlyCleared {
		text += "\nRead-only flag was cleared."
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) classifyEditApplyFailure(prepared preparedEdit, output EditFileOutput, failure *mcp.CallToolResult) (*mcp.CallToolResult, EditFileOutput, error) {
	output.Applied = false
	output.State = editApplyStateUnknown
	output.ActualFingerprint = ""
	output.Changed = false

	classificationCtx, cancel := context.WithTimeout(context.Background(), editApplyClassificationTimeout)
	defer cancel()
	validation := h.ValidatePath(prepared.requestedPath)
	if validation.Ok() && validation.Path == prepared.resolvedPath && classificationCtx.Err() == nil {
		post, err := filesystem.CaptureRegularFileSnapshotBounded(classificationCtx, validation.Path, h.maxFileBytes())
		if err == nil {
			if actual, fingerprintErr := filesystem.FingerprintRegularFileSnapshot(post); fingerprintErr == nil {
				output.ActualFingerprint = actual
				switch actual {
				case prepared.targetFingerprint:
					output.State = editApplyStateUnchanged
					output.Changed = false
				case prepared.resultFingerprint:
					output.State = editApplyStateCommitted
					output.Changed = prepared.changed
				default:
					output.State = editApplyStateUnknown
					output.Changed = actual != prepared.targetFingerprint
				}
			}
		}
	}

	if output.State == editApplyStateUnchanged {
		return failure, output, nil
	}
	message := "edit apply failed after the target may have changed"
	if failure != nil && len(failure.Content) > 0 {
		if text, ok := failure.Content[0].(*mcp.TextContent); ok && text.Text != "" {
			message = text.Text
		}
	}
	return errorResultWithCode(ErrCodePartialCommit, message), output, nil
}

func (h *Handler) commitPreparedEdit(ctx context.Context, prepared preparedEdit, expected filesystem.FileSnapshot, currentMode os.FileMode) (bool, *mcp.CallToolResult) {
	if err := ctx.Err(); err != nil {
		return false, errorResultFromError(operation.Wrap(operation.KindCancelled, "commit_prepared_edit", prepared.resolvedPath, err))
	}
	validation := h.ValidatePath(prepared.requestedPath)
	if !validation.Ok() {
		return false, validation.Result
	}
	if validation.Path != prepared.resolvedPath {
		return false, errorResultWithCode(ErrCodeConflict, "path changed while preparing edit")
	}

	readOnly := isReadOnly(currentMode)
	if readOnly && !prepared.forceWritable {
		return false, errorResultWithCode(ErrCodePermission, "file is read-only — STOP, do NOT retry and do NOT attempt to change file attributes. Ask the user whether to proceed with forceWritable: true, or skip this file")
	}
	writeMode := currentMode
	readOnlyCleared := false
	if readOnly {
		if err := clearReadOnly(validation.Path, currentMode); err != nil {
			return false, errorResultFromError(fmt.Errorf("failed to clear read-only flag: %w", err))
		}
		readOnlyCleared = true
		writeMode = currentMode | 0200
		slog.Info("cleared read-only flag", "path", prepared.requestedPath)
		refreshed, err := expected.RefreshMetadata(validation.Path)
		if err != nil {
			if restoreErr := os.Chmod(validation.Path, currentMode); restoreErr != nil {
				slog.Error("failed to restore read-only mode after snapshot failure", "path", prepared.requestedPath, "error", restoreErr)
			}
			return false, errorResultFromError(fmt.Errorf("failed to refresh file snapshot: %w", err))
		}
		expected = refreshed
	}

	if err := h.replaceFile(validation.Path, prepared.data, filesystem.ReplaceOptions{Mode: writeMode, Expected: &expected}); err != nil {
		if readOnlyCleared {
			if restoreErr := os.Chmod(validation.Path, currentMode); restoreErr != nil {
				slog.Error("failed to restore read-only mode after edit failure", "path", prepared.requestedPath, "error", restoreErr)
			}
		}
		return false, errorResultFromError(fmt.Errorf("failed to write file: %w", err))
	}
	return readOnlyCleared, nil
}

func editOutputFromPrepared(action string, prepared preparedEdit) EditFileOutput {
	return EditFileOutput{
		Action:            action,
		Diff:              prepared.diff,
		TargetPath:        prepared.requestedPath,
		TargetFingerprint: prepared.targetFingerprint,
		ResultFingerprint: prepared.resultFingerprint,
		Encoding:          prepared.encoding,
		HasBOM:            prepared.hasBOM,
		BOMType:           prepared.bomType,
		LineEndingStyle:   prepared.lineEndingStyle,
		BackupPolicy:      prepared.backupPolicy,
		Changed:           prepared.changed,
	}
}

func editOutputFromPreview(preview *editPreview) EditFileOutput {
	output := editOutputFromPrepared(editActionPreview, preview.prepared)
	output.PreviewID = preview.id
	output.CreatedAt = preview.createdAt.Format(timeRFC3339Nano)
	output.ExpiresAt = preview.expiresAt.Format(timeRFC3339Nano)
	return output
}

const timeRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"

func editPreviewText(output EditFileOutput) string {
	policy := ""
	if output.BackupPolicy != "" {
		policy = "\nBackup policy: " + output.BackupPolicy
	}
	return fmt.Sprintf("Edit preview prepared.\nPreview ID: %s\nExpires: %s\nTarget fingerprint: %s\nResult fingerprint: %s%s\n\n%s",
		output.PreviewID, output.ExpiresAt, output.TargetFingerprint, output.ResultFingerprint, policy, output.Diff)
}

func editApplyText(output EditFileOutput) string {
	backup := ""
	if output.BackupID != "" {
		backup = "\nBackup ID: " + output.BackupID
	}
	return fmt.Sprintf("Edit preview applied.\nTarget fingerprint: %s\nResult fingerprint: %s%s\n\n%s",
		output.TargetFingerprint, output.ResultFingerprint, backup, output.Diff)
}

func (h *Handler) checkEditResponseLimit(output EditFileOutput, text string) error {
	encoded, err := json.Marshal(output)
	if err != nil {
		return operation.Wrap(operation.KindFilesystem, "encode_edit_output", "", err)
	}
	if int64(len(encoded))+int64(len(text)) > h.maxOutputBytes() {
		return operation.New(operation.KindLimit, fmt.Sprintf("edit output exceeds limit %d bytes", h.maxOutputBytes()))
	}
	return nil
}
