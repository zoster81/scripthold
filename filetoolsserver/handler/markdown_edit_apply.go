package handler

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

type markdownEditApplyPlan struct {
	path               string
	document           textDocument
	resultData         []byte
	currentFingerprint string
	output             MarkdownApplyOutput
}

func (h *Handler) HandleMarkdownApply(ctx context.Context, _ *mcp.CallToolRequest, input MarkdownApplyInput) (*mcp.CallToolResult, MarkdownApplyOutput, error) {
	preview, err := h.markdownPreviews.claim(input.PreviewID)
	if err != nil {
		return errorResultFromError(err), MarkdownApplyOutput{}, nil
	}
	switch preview.kind {
	case markdownPreviewCreate:
		return h.handleMarkdownCreateApply(ctx, preview)
	case markdownPreviewWorkspaceRepair:
		return h.handleMarkdownWorkspaceRepairApply(ctx, preview)
	default:
		return h.handleMarkdownEditApply(ctx, preview)
	}
}

func (h *Handler) handleMarkdownEditApply(ctx context.Context, preview *markdownPreview) (*mcp.CallToolResult, MarkdownApplyOutput, error) {
	if preview.kind != markdownPreviewEdit || preview.edit == nil {
		return errorResultWithCode(ErrCodeConflict, "Markdown preview is not an edit preview"), MarkdownApplyOutput{}, nil
	}
	prepared := *preview.edit
	defer func() {
		if prepared.identityFile != nil {
			_ = prepared.identityFile.Close()
		}
	}()
	if err := ctx.Err(); err != nil {
		return errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_apply", prepared.resolvedPath, err)), MarkdownApplyOutput{}, nil
	}
	plan, failure := h.prepareMarkdownEditApply(ctx, prepared)
	if failure != nil {
		return failure, MarkdownApplyOutput{}, nil
	}
	if failure = h.checkMarkdownEditApplyResponse(prepared, plan.output); failure != nil {
		return failure, MarkdownApplyOutput{}, nil
	}
	if failure = h.captureMarkdownEditApplyBackup(ctx, &prepared, plan.path, &plan.output); failure != nil {
		return failure, plan.output, nil
	}
	if !prepared.changed {
		return markdownApplySuccess(plan.output), plan.output, nil
	}
	return h.commitMarkdownEditApply(ctx, &prepared, plan)
}

func (h *Handler) prepareMarkdownEditApply(ctx context.Context, prepared preparedMarkdownEdit) (markdownEditApplyPlan, *mcp.CallToolResult) {
	path, failure := h.revalidateMarkdownEditApplyTarget(prepared, "after preview")
	if failure != nil {
		return markdownEditApplyPlan{}, failure
	}
	document, sourceData, currentFingerprint, failure := h.readMarkdownEditApplySource(ctx, prepared, path)
	if failure != nil {
		return markdownEditApplyPlan{}, failure
	}
	resultData, failure := materializeMarkdownEditApplyResult(prepared, document, sourceData, path)
	if failure != nil {
		return markdownEditApplyPlan{}, failure
	}
	return markdownEditApplyPlan{
		path: path, document: document, resultData: resultData,
		currentFingerprint: currentFingerprint,
		output:             markdownApplyOutput(prepared, currentFingerprint),
	}, nil
}

func (h *Handler) revalidateMarkdownEditApplyTarget(prepared preparedMarkdownEdit, phase string) (string, *mcp.CallToolResult) {
	validated := h.ValidatePath(prepared.requestedPath)
	if !validated.Ok() {
		return "", validated.Result
	}
	if validated.Path != prepared.resolvedPath {
		return "", errorResultWithCode(ErrCodeConflict, "Markdown target path changed after preview")
	}
	if prepared.identityFile == nil {
		return "", errorResultWithCode(ErrCodeConflict, "Markdown preview identity is unavailable")
	}
	matches, err := prepared.identityFile.Matches(validated.Path)
	if err != nil || !matches {
		return "", errorResultWithCode(ErrCodeConflict, "Markdown target identity changed "+phase)
	}
	return validated.Path, nil
}

func (h *Handler) readMarkdownEditApplySource(ctx context.Context, prepared preparedMarkdownEdit, path string) (textDocument, []byte, string, *mcp.CallToolResult) {
	document, sourceData, err := h.readTextDocumentWithData(ctx, path, prepared.encoding)
	if err != nil {
		return textDocument{}, nil, "", errorResultFromError(err)
	}
	if document.Charset != prepared.encoding || document.BOM.HasBOM != prepared.hasBOM || document.BOM.Type != prepared.bomType {
		return textDocument{}, nil, "", errorResultWithCode(ErrCodeConflict, "Markdown encoding or BOM changed after preview")
	}
	currentFingerprint, err := filesystem.FingerprintRegularFileSnapshot(document.Snapshot)
	if err != nil {
		return textDocument{}, nil, "", errorResultFromError(err)
	}
	if currentFingerprint != prepared.targetFingerprint {
		return textDocument{}, nil, "", errorResultWithCode(ErrCodeConflict, "Markdown target fingerprint changed after preview")
	}
	if prepared.changed && isReadOnly(document.Mode) {
		return textDocument{}, nil, "", errorResultWithCode(ErrCodePermission, "Markdown target became read-only after preview")
	}
	if !utf8.ValidString(document.Text) {
		err := operation.Wrap(operation.KindEncoding, "markdown_apply", path, fmt.Errorf("decoded Markdown is not valid UTF-8"))
		return textDocument{}, nil, "", errorResultFromError(err)
	}
	return document, sourceData, currentFingerprint, nil
}

func materializeMarkdownEditApplyResult(prepared preparedMarkdownEdit, document textDocument, sourceData []byte, path string) ([]byte, *mcp.CallToolResult) {
	currentUTF8 := []byte(document.Text)
	resultUTF8, err := prepared.semantic.Apply(currentUTF8)
	if err != nil {
		return nil, markdownEditErrorResult(err)
	}
	if !bytes.Equal(resultUTF8, prepared.resultUTF8) {
		return nil, errorResultWithCode(ErrCodeConflict, "Marksplice result changed after preview")
	}
	resultData, err := markdownPhysicalResult(document, sourceData, currentUTF8, resultUTF8)
	if err != nil {
		return nil, errorResultFromError(err)
	}
	if filesystem.FingerprintRegularFileData(resultData) != prepared.resultFingerprint {
		return nil, errorResultWithCode(ErrCodeConflict, "prepared Markdown result no longer matches its fingerprint")
	}
	return resultData, nil
}

func (h *Handler) checkMarkdownEditApplyResponse(prepared preparedMarkdownEdit, output MarkdownApplyOutput) *mcp.CallToolResult {
	worstCase := output
	worstCase.Applied = prepared.changed
	worstCase.Changed = prepared.changed
	if prepared.changed {
		worstCase.State = editApplyStateCommitted
	}
	if prepared.changed && persistentBackupRequired(prepared.backupPolicy) {
		worstCase.BackupID = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	}
	if err := h.checkMarkdownMutationResponseLimit(worstCase, markdownApplyText(worstCase)); err != nil {
		return errorResultFromError(err)
	}
	return nil
}

func (h *Handler) captureMarkdownEditApplyBackup(ctx context.Context, prepared *preparedMarkdownEdit, path string, output *MarkdownApplyOutput) *mcp.CallToolResult {
	if _, failure := h.revalidateMarkdownEditApplyTarget(*prepared, "before apply"); failure != nil {
		return failure
	}
	if !prepared.changed || !persistentBackupRequired(prepared.backupPolicy) {
		return nil
	}
	if h.backupCapture == nil {
		return errorResultFromError(operation.New(operation.KindConflict, "required backup store is unavailable"))
	}
	captured, captureErr := h.backupCapture.Capture(ctx, backupstore.CaptureRequest{
		TargetPath: path, SourceOperation: backupstore.SourceOperationEdit,
		Pinned: persistentBackupPinned(prepared.backupPolicy),
	})
	if captured.Manifest.BackupID == "" {
		if captureErr == nil {
			captureErr = operation.New(operation.KindFilesystem, "required backup did not commit a manifest")
		}
		return errorResultFromError(captureErr)
	}
	output.BackupID = captured.Manifest.BackupID
	if !markdownEditBackupMatches(captured.Manifest, path, prepared.targetFingerprint) {
		return errorResultWithCode(ErrCodeConflict, "durable backup does not match the approved Markdown pre-state")
	}
	if captureErr != nil {
		slog.Warn("Markdown backup manifest committed but derived index refresh reported an error")
	}
	_, failure := h.revalidateMarkdownEditApplyTarget(*prepared, "after durable backup")
	return failure
}

func markdownEditBackupMatches(manifest backupstore.Manifest, path, fingerprint string) bool {
	return validMarkdownEditPreviewID(manifest.BackupID) &&
		manifest.TargetPath == path &&
		manifest.SourceOperation == backupstore.SourceOperationEdit &&
		manifest.ContentFingerprint == fingerprint
}

func (h *Handler) commitMarkdownEditApply(ctx context.Context, prepared *preparedMarkdownEdit, plan markdownEditApplyPlan) (*mcp.CallToolResult, MarkdownApplyOutput, error) {
	if err := ctx.Err(); err != nil {
		return errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_apply", plan.path, err)), plan.output, nil
	}
	if _, failure := h.revalidateMarkdownEditApplyTarget(*prepared, "at commit boundary"); failure != nil {
		return failure, plan.output, nil
	}
	if err := prepared.identityFile.Close(); err != nil {
		return errorResultFromError(operation.WrapFilesystem("close_markdown_preview_identity", plan.path, err)), plan.output, nil
	}
	prepared.identityFile = nil
	if err := h.replaceFile(plan.path, plan.resultData, filesystem.ReplaceOptions{Mode: plan.document.Mode.Perm(), Expected: &plan.document.Snapshot}); err != nil {
		return h.classifyMarkdownApplyFailure(*prepared, plan.output, errorResultFromError(fmt.Errorf("failed to write Markdown file: %w", err)))
	}
	return h.verifyMarkdownEditApplyCommit(*prepared, plan)
}

func (h *Handler) verifyMarkdownEditApplyCommit(prepared preparedMarkdownEdit, plan markdownEditApplyPlan) (*mcp.CallToolResult, MarkdownApplyOutput, error) {
	post, err := filesystem.CaptureSnapshotWithDigest(plan.path)
	if err != nil {
		return h.classifyMarkdownApplyFailure(prepared, plan.output, errorResultFromError(err))
	}
	actualFingerprint, err := filesystem.FingerprintRegularFileSnapshot(post)
	if err != nil {
		return h.classifyMarkdownApplyFailure(prepared, plan.output, errorResultFromError(err))
	}
	if actualFingerprint != prepared.resultFingerprint {
		return h.classifyMarkdownApplyFailure(prepared, plan.output, errorResultWithCode(ErrCodeConflict, "applied Markdown file does not match the prepared result fingerprint"))
	}
	plan.output.ActualFingerprint = actualFingerprint
	plan.output.State = editApplyStateCommitted
	plan.output.Changed = true
	plan.output.Applied = true
	return markdownApplySuccess(plan.output), plan.output, nil
}

func markdownApplySuccess(output MarkdownApplyOutput) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: markdownApplyText(output)}}}
}
