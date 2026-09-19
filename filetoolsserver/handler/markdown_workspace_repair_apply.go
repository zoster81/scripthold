package handler

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

type markdownWorkspaceRepairApplyTarget struct {
	document    textDocument
	replacement preparedExistingFileReplacement
}

func (h *Handler) prepareMarkdownWorkspaceRepairApplyTargets(ctx context.Context, prepared *preparedMarkdownWorkspaceRepair) ([]markdownWorkspaceRepairApplyTarget, *mcp.CallToolResult) {
	plans, _, failure := h.prepareMarkdownWorkspaceRepairApplyTargetsIndexed(ctx, prepared)
	return plans, failure
}

func (h *Handler) prepareMarkdownWorkspaceRepairApplyTargetsIndexed(ctx context.Context, prepared *preparedMarkdownWorkspaceRepair) ([]markdownWorkspaceRepairApplyTarget, int, *mcp.CallToolResult) {
	if prepared == nil || len(prepared.targets) == 0 {
		return nil, -1, errorResultWithCode(ErrCodeConflict, "Markdown workspace repair preview is unavailable")
	}
	rootValidation := h.ValidatePath(prepared.root)
	if !rootValidation.Ok() {
		return nil, -1, rootValidation.Result
	}
	if rootValidation.Path != prepared.root {
		return nil, -1, errorResultWithCode(ErrCodeConflict, "Markdown workspace root changed after preview")
	}

	plans := make([]markdownWorkspaceRepairApplyTarget, len(prepared.targets))
	for index := range prepared.targets {
		if err := ctx.Err(); err != nil {
			return nil, index, errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_apply", prepared.root, err))
		}
		plan, failure := h.prepareMarkdownWorkspaceRepairApplyTarget(ctx, prepared.root, &prepared.targets[index])
		if failure != nil {
			return nil, index, failure
		}
		plans[index] = plan
	}
	return plans, -1, nil
}

func (h *Handler) prepareMarkdownWorkspaceRepairApplyTarget(ctx context.Context, root string, target *preparedMarkdownWorkspaceRepairTarget) (markdownWorkspaceRepairApplyTarget, *mcp.CallToolResult) {
	path, failure := h.revalidateMarkdownWorkspaceRepairApplyBinding(root, target)
	if failure != nil {
		return markdownWorkspaceRepairApplyTarget{}, failure
	}
	document, sourceData, failure := h.readMarkdownWorkspaceRepairApplySource(ctx, target, path)
	if failure != nil {
		return markdownWorkspaceRepairApplyTarget{}, failure
	}
	if failure := materializeMarkdownWorkspaceRepairApplyResult(target, document, sourceData); failure != nil {
		return markdownWorkspaceRepairApplyTarget{}, failure
	}
	return markdownWorkspaceRepairApplyTarget{
		document:    document,
		replacement: preparedMarkdownWorkspaceRepairReplacement(target),
	}, nil
}

func (h *Handler) revalidateMarkdownWorkspaceRepairApplyBinding(root string, target *preparedMarkdownWorkspaceRepairTarget) (string, *mcp.CallToolResult) {
	if target == nil || target.identityFile == nil {
		return "", errorResultWithCode(ErrCodeConflict, "Markdown workspace repair target identity is unavailable")
	}
	validation := h.ValidatePath(target.requestedPath)
	if !validation.Ok() {
		return "", validation.Result
	}
	if validation.Path != target.resolvedPath || !security.IsPathWithinAllowedDirectories(validation.Path, []string{root}) {
		return "", errorResultWithCode(ErrCodeConflict, "Markdown workspace repair target binding changed after preview")
	}
	matches, err := target.identityFile.Matches(validation.Path)
	if err != nil || !matches {
		return "", errorResultWithCode(ErrCodeConflict, "Markdown workspace repair target identity changed after preview")
	}
	return validation.Path, nil
}

func (h *Handler) readMarkdownWorkspaceRepairApplySource(ctx context.Context, target *preparedMarkdownWorkspaceRepairTarget, path string) (textDocument, []byte, *mcp.CallToolResult) {
	document, sourceData, err := h.readTextDocumentWithData(ctx, path, target.encoding)
	if err != nil {
		return textDocument{}, nil, errorResultFromError(err)
	}
	matches, err := target.identityFile.Matches(path)
	if err != nil || !matches {
		return textDocument{}, nil, errorResultWithCode(ErrCodeConflict, "Markdown workspace repair target identity changed while revalidating")
	}
	if document.Charset != target.encoding || document.BOM.HasBOM != target.hasBOM || document.BOM.Type != target.bomType {
		return textDocument{}, nil, errorResultWithCode(ErrCodeConflict, "Markdown workspace repair encoding or BOM changed after preview")
	}
	if document.LineEndings.Style != target.lineEndingStyle {
		return textDocument{}, nil, errorResultWithCode(ErrCodeConflict, "Markdown workspace repair line endings changed after preview")
	}
	currentFingerprint, err := filesystem.FingerprintRegularFileSnapshot(document.Snapshot)
	if err != nil {
		return textDocument{}, nil, errorResultFromError(err)
	}
	if currentFingerprint != target.targetFingerprint {
		return textDocument{}, nil, errorResultWithCode(ErrCodeConflict, "Markdown workspace repair target fingerprint changed after preview")
	}
	if target.changed && isReadOnly(document.Mode) {
		return textDocument{}, nil, errorResultWithCode(ErrCodePermission, "Markdown workspace repair target became read-only after preview")
	}
	if !utf8.ValidString(document.Text) {
		err := operation.Wrap(operation.KindEncoding, "markdown_apply", path, fmt.Errorf("decoded Markdown is not valid UTF-8"))
		return textDocument{}, nil, errorResultFromError(err)
	}
	return document, sourceData, nil
}

func materializeMarkdownWorkspaceRepairApplyResult(target *preparedMarkdownWorkspaceRepairTarget, document textDocument, sourceData []byte) *mcp.CallToolResult {
	sourceUTF8 := []byte(document.Text)
	resultUTF8, err := target.change.Apply(sourceUTF8)
	if err != nil {
		return markdownEditErrorResult(err)
	}
	resultData, err := markdownPhysicalResult(document, sourceData, sourceUTF8, resultUTF8)
	if err != nil {
		return errorResultFromError(err)
	}
	if !bytes.Equal(resultData, target.resultData) || filesystem.FingerprintRegularFileData(resultData) != target.resultFingerprint {
		return errorResultWithCode(ErrCodeConflict, "prepared Markdown workspace repair result changed after preview")
	}
	if changed := !bytes.Equal(sourceData, resultData); changed != target.changed {
		return errorResultWithCode(ErrCodeConflict, "prepared Markdown workspace repair change state changed after preview")
	}
	return nil
}

const (
	markdownWorkspaceRepairApplyStatePrepared     = "prepared"
	maxMarkdownWorkspaceRepairFailureMessageBytes = 1024
	markdownWorkspaceRepairClassificationTimeout  = 30 * time.Second
	markdownWorkspaceRepairBackupIDHexLength      = 64
)

type markdownWorkspaceRepairApplyDocumentResult struct {
	Document          string `json:"document"`
	Path              string `json:"path"`
	TargetFingerprint string `json:"targetFingerprint"`
	ResultFingerprint string `json:"resultFingerprint"`
	ActualFingerprint string `json:"actualFingerprint,omitempty"`
	Encoding          string `json:"encoding"`
	HasBOM            bool   `json:"hasBOM"`
	BOMType           string `json:"bomType,omitempty"`
	LineEndingStyle   string `json:"lineEndingStyle"`
	BackupID          string `json:"backupId,omitempty"`
	State             string `json:"state"`
	Changed           bool   `json:"changed"`
	Applied           bool   `json:"applied"`
	ErrorCode         string `json:"errorCode,omitempty"`
	Error             string `json:"error,omitempty"`
}

type markdownWorkspaceRepairApplyResult struct {
	BackupPolicy   string                                       `json:"backupPolicy,omitempty"`
	TotalTargets   int                                          `json:"totalTargets"`
	CommittedCount int                                          `json:"committedCount"`
	UnchangedCount int                                          `json:"unchangedCount"`
	UnknownCount   int                                          `json:"unknownCount"`
	BackupCount    int                                          `json:"backupCount"`
	PartialCommit  bool                                         `json:"partialCommit"`
	FailedIndex    *int                                         `json:"failedIndex,omitempty"`
	FailedDocument string                                       `json:"failedDocument,omitempty"`
	FailureCode    string                                       `json:"failureCode,omitempty"`
	FailureMessage string                                       `json:"failureMessage,omitempty"`
	Results        []markdownWorkspaceRepairApplyDocumentResult `json:"results"`
}

func (h *Handler) applyPreparedMarkdownWorkspaceRepair(ctx context.Context, prepared *preparedMarkdownWorkspaceRepair) (markdownWorkspaceRepairApplyResult, *mcp.CallToolResult) {
	output := newMarkdownWorkspaceRepairApplyResult(prepared)
	if err := ctx.Err(); err != nil {
		return output, errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_apply", "", err))
	}
	plans, failedIndex, failure := h.prepareMarkdownWorkspaceRepairApplyTargetsIndexed(ctx, prepared)
	if failure != nil {
		return output, failure
	}
	if err := h.checkMarkdownWorkspaceRepairApplyResponse(output); err != nil {
		return output, errorResultFromError(err)
	}
	if err := h.captureMarkdownWorkspaceRepairApplyBackups(ctx, prepared, &output); err != nil {
		return h.classifyMarkdownWorkspaceRepairApplyFailure(prepared, output, -1, errorResultFromError(err), nil)
	}
	if persistentBackupRequired(prepared.backupPolicy) {
		plans, failedIndex, failure = h.prepareMarkdownWorkspaceRepairApplyTargetsIndexed(ctx, prepared)
		if failure != nil {
			return h.classifyMarkdownWorkspaceRepairApplyFailure(prepared, output, failedIndex, failure, nil)
		}
	}
	batch, err := h.stageMarkdownWorkspaceRepairApply(ctx, prepared, plans)
	if err != nil {
		return h.classifyMarkdownWorkspaceRepairApplyFailure(prepared, output, -1, errorResultFromError(err), nil)
	}
	return h.commitMarkdownWorkspaceRepairApply(ctx, prepared, output, batch)
}

func newMarkdownWorkspaceRepairApplyResult(prepared *preparedMarkdownWorkspaceRepair) markdownWorkspaceRepairApplyResult {
	output := markdownWorkspaceRepairApplyResult{}
	if prepared == nil {
		return output
	}
	output.BackupPolicy = prepared.backupPolicy
	output.TotalTargets = len(prepared.targets)
	output.Results = make([]markdownWorkspaceRepairApplyDocumentResult, len(prepared.targets))
	for index := range prepared.targets {
		target := &prepared.targets[index]
		output.Results[index] = markdownWorkspaceRepairApplyDocumentResult{
			Document:          string(target.documentKey),
			Path:              target.requestedPath,
			TargetFingerprint: target.targetFingerprint,
			ResultFingerprint: target.resultFingerprint,
			Encoding:          target.encoding,
			HasBOM:            target.hasBOM,
			BOMType:           target.bomType,
			LineEndingStyle:   target.lineEndingStyle,
			State:             markdownWorkspaceRepairApplyStatePrepared,
			Changed:           target.changed,
		}
	}
	return output
}

func (h *Handler) checkMarkdownWorkspaceRepairApplyResponse(output markdownWorkspaceRepairApplyResult) error {
	worst := output
	worst.Results = append([]markdownWorkspaceRepairApplyDocumentResult(nil), output.Results...)
	for index := range worst.Results {
		result := &worst.Results[index]
		if result.Changed {
			result.State = string(existingFileReplacementStateCommitted)
			result.ActualFingerprint = result.ResultFingerprint
			result.Applied = true
			worst.CommittedCount++
			if persistentBackupRequired(worst.BackupPolicy) {
				result.BackupID = strings.Repeat("f", markdownWorkspaceRepairBackupIDHexLength)
				worst.BackupCount++
			}
		} else {
			result.State = string(existingFileReplacementStateUnchanged)
			result.ActualFingerprint = result.TargetFingerprint
			worst.UnchangedCount++
		}
	}
	if len(worst.Results) > 0 {
		failed := len(worst.Results) - 1
		worst.FailedIndex = workspaceRepairIntPointer(failed)
		worst.FailedDocument = worst.Results[failed].Document
		worst.FailureCode = ErrCodeEncodingAmbiguous
		worst.FailureMessage = strings.Repeat("\x00", maxMarkdownWorkspaceRepairFailureMessageBytes)
		worst.Results[failed].ErrorCode = worst.FailureCode
		worst.Results[failed].Error = worst.FailureMessage
		worst.PartialCommit = true
	}
	return h.checkMarkdownMutationResponseLimit(worst, markdownWorkspaceRepairApplyFailureText(worst))
}

func (h *Handler) captureMarkdownWorkspaceRepairApplyBackups(ctx context.Context, prepared *preparedMarkdownWorkspaceRepair, output *markdownWorkspaceRepairApplyResult) error {
	if prepared == nil || !persistentBackupRequired(prepared.backupPolicy) {
		return nil
	}
	changed := markdownWorkspaceRepairChangedIndices(prepared.targets)
	if len(changed) == 0 {
		return nil
	}
	if h.backupBatchCapture == nil {
		return operation.New(operation.KindConflict, "required backup batch authority is unavailable")
	}
	requests := make([]backupstore.CaptureRequest, len(changed))
	for captureIndex, targetIndex := range changed {
		requests[captureIndex] = backupstore.CaptureRequest{
			TargetPath:      prepared.targets[targetIndex].resolvedPath,
			SourceOperation: backupstore.SourceOperationEdit,
			Pinned:          persistentBackupPinned(prepared.backupPolicy),
		}
	}
	captures, captureErr := h.backupBatchCapture.CaptureBatch(ctx, requests)
	return validateMarkdownWorkspaceRepairBackupCaptures(prepared, output, changed, captures, captureErr)
}

func validateMarkdownWorkspaceRepairBackupCaptures(prepared *preparedMarkdownWorkspaceRepair, output *markdownWorkspaceRepairApplyResult, changed []int, captures []backupstore.CaptureResult, captureErr error) error {
	invalidBatch := len(captures) > len(changed)
	if invalidBatch {
		captureErr = errors.Join(captureErr, operation.New(operation.KindConflict, "backup batch returned unexpected results"))
		captures = captures[:len(changed)]
	}
	verified := 0
	for captureIndex, captured := range captures {
		targetIndex := changed[captureIndex]
		manifest := captured.Manifest
		if validMarkdownWorkspaceRepairBackupID(manifest.BackupID) {
			output.Results[targetIndex].BackupID = manifest.BackupID
			output.BackupCount++
		}
		if !markdownWorkspaceRepairBackupMatches(manifest, prepared.targets[targetIndex]) {
			captureErr = errors.Join(captureErr, operation.New(operation.KindConflict, "durable Markdown workspace repair backup does not match the approved pre-state"))
			break
		}
		verified++
	}
	if !invalidBatch && verified == len(changed) {
		if captureErr != nil {
			slog.Warn("Markdown workspace repair backup manifests committed but derived index refresh reported an error", "backupCount", verified)
		}
		return nil
	}
	if captureErr == nil {
		captureErr = operation.New(operation.KindFilesystem, "required Markdown workspace repair backup batch is incomplete")
	}
	return captureErr
}

func markdownWorkspaceRepairChangedIndices(targets []preparedMarkdownWorkspaceRepairTarget) []int {
	indices := make([]int, 0, len(targets))
	for index := range targets {
		if targets[index].changed {
			indices = append(indices, index)
		}
	}
	return indices
}

func markdownWorkspaceRepairBackupMatches(manifest backupstore.Manifest, target preparedMarkdownWorkspaceRepairTarget) bool {
	return validMarkdownWorkspaceRepairBackupID(manifest.BackupID) &&
		manifest.TargetPath == target.resolvedPath &&
		manifest.SourceOperation == backupstore.SourceOperationEdit &&
		manifest.ContentFingerprint == target.targetFingerprint
}

func validMarkdownWorkspaceRepairBackupID(id string) bool {
	if len(id) != markdownWorkspaceRepairBackupIDHexLength {
		return false
	}
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == markdownWorkspaceRepairBackupIDHexLength/2
}

func (h *Handler) stageMarkdownWorkspaceRepairApply(ctx context.Context, prepared *preparedMarkdownWorkspaceRepair, plans []markdownWorkspaceRepairApplyTarget) (*existingFileReplacementBatch, error) {
	if prepared == nil || len(plans) != len(prepared.targets) {
		return nil, operation.New(operation.KindInvalidInput, "Markdown workspace repair apply plans do not match targets")
	}
	replacements := make([]preparedExistingFileReplacement, len(plans))
	modes := make([]os.FileMode, len(plans))
	for index := range plans {
		replacements[index] = plans[index].replacement
		modes[index] = plans[index].document.Mode.Perm()
	}
	return h.stageExistingFileReplacementBatch(
		ctx,
		replacements,
		modes,
		h.existingFileReplacementOps,
		"stage_markdown_workspace_repair",
		"cleanup_markdown_workspace_repair_stage",
	)
}

func (h *Handler) commitMarkdownWorkspaceRepairApply(ctx context.Context, prepared *preparedMarkdownWorkspaceRepair, output markdownWorkspaceRepairApplyResult, batch *existingFileReplacementBatch) (markdownWorkspaceRepairApplyResult, *mcp.CallToolResult) {
	for index := range prepared.targets {
		target := &prepared.targets[index]
		if !target.changed {
			markMarkdownWorkspaceRepairUnchanged(&output, index, target.targetFingerprint)
			continue
		}
		if err := ctx.Err(); err != nil {
			failure := errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_apply", target.resolvedPath, err))
			return h.classifyMarkdownWorkspaceRepairApplyFailure(prepared, output, index, failure, batch)
		}
		current, failure := h.prepareMarkdownWorkspaceRepairApplyTarget(ctx, prepared.root, target)
		if failure != nil {
			return h.classifyMarkdownWorkspaceRepairApplyFailure(prepared, output, index, failure, batch)
		}
		replacement := current.replacement
		if replacement.identity() == nil {
			failure = errorResultWithCode(ErrCodeConflict, "Markdown workspace repair target identity is unavailable at commit")
			return h.classifyMarkdownWorkspaceRepairApplyFailure(prepared, output, index, failure, batch)
		}
		if err := replacement.closeIdentity(); err != nil {
			failure = errorResultFromError(operation.WrapFilesystem("close_markdown_workspace_repair_identity", replacement.resolvedPath, err))
			return h.classifyMarkdownWorkspaceRepairApplyFailure(prepared, output, index, failure, batch)
		}
		actual, err := h.commitExistingFileReplacementBatchTarget(ctx, batch, index, current.document.Snapshot, "committed Markdown workspace repair does not match the prepared result fingerprint")
		if err != nil {
			return h.classifyMarkdownWorkspaceRepairApplyFailure(prepared, output, index, errorResultFromError(err), batch)
		}
		markMarkdownWorkspaceRepairCommitted(&output, index, actual)
	}
	if err := batch.cleanup("cleanup_markdown_workspace_repair_stage"); err != nil {
		return h.classifyMarkdownWorkspaceRepairApplyFailure(prepared, output, max(0, len(prepared.targets)-1), errorResultFromError(err), batch)
	}
	if index, err := h.verifyMarkdownWorkspaceRepairApplyFinal(ctx, prepared, &output); err != nil {
		return h.classifyMarkdownWorkspaceRepairApplyFailure(prepared, output, index, errorResultFromError(err), nil)
	}
	return output, nil
}

func markMarkdownWorkspaceRepairUnchanged(output *markdownWorkspaceRepairApplyResult, index int, actual string) {
	result := &output.Results[index]
	result.State = string(existingFileReplacementStateUnchanged)
	result.ActualFingerprint = actual
	result.Applied = false
	output.UnchangedCount++
}

func markMarkdownWorkspaceRepairCommitted(output *markdownWorkspaceRepairApplyResult, index int, actual string) {
	result := &output.Results[index]
	result.State = string(existingFileReplacementStateCommitted)
	result.ActualFingerprint = actual
	result.Applied = true
	output.CommittedCount++
}

func (h *Handler) verifyMarkdownWorkspaceRepairApplyFinal(ctx context.Context, prepared *preparedMarkdownWorkspaceRepair, output *markdownWorkspaceRepairApplyResult) (int, error) {
	for index := range prepared.targets {
		target := &prepared.targets[index]
		snapshot, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, target.resolvedPath, h.maxFileBytes())
		if err != nil {
			return index, err
		}
		actual, err := filesystem.FingerprintRegularFileSnapshot(snapshot)
		if err != nil {
			return index, err
		}
		if actual != target.resultFingerprint {
			return index, operation.New(operation.KindConflict, fmt.Sprintf("Markdown workspace repair target %d changed during final verification", index))
		}
		output.Results[index].ActualFingerprint = actual
	}
	return -1, nil
}

func (h *Handler) classifyMarkdownWorkspaceRepairApplyFailure(prepared *preparedMarkdownWorkspaceRepair, output markdownWorkspaceRepairApplyResult, failedIndex int, failure *mcp.CallToolResult, batch *existingFileReplacementBatch) (markdownWorkspaceRepairApplyResult, *mcp.CallToolResult) {
	code, message := markdownWorkspaceRepairFailureInfo(failure)
	if cleanupErr := batch.cleanup("cleanup_markdown_workspace_repair_stage"); cleanupErr != nil {
		code = ErrCodeIO
		message = boundedMarkdownWorkspaceRepairFailureMessage(message + "; " + cleanupErr.Error())
	}
	classificationCtx, cancel := context.WithTimeout(context.Background(), markdownWorkspaceRepairClassificationTimeout)
	defer cancel()

	output.CommittedCount = 0
	output.UnchangedCount = 0
	output.UnknownCount = 0
	for index := range prepared.targets {
		state, actual, applied := h.classifyExistingFileReplacement(classificationCtx, preparedMarkdownWorkspaceRepairReplacement(&prepared.targets[index]))
		result := &output.Results[index]
		result.State = string(state)
		result.ActualFingerprint = actual
		result.Applied = applied
		result.ErrorCode = ""
		result.Error = ""
		switch state {
		case existingFileReplacementStateCommitted:
			output.CommittedCount++
		case existingFileReplacementStateUnchanged:
			output.UnchangedCount++
		default:
			output.UnknownCount++
		}
	}
	output.PartialCommit = output.CommittedCount > 0 || output.UnknownCount > 0
	output.FailureCode = code
	output.FailureMessage = boundedMarkdownWorkspaceRepairFailureMessage(message)
	if failedIndex >= 0 && failedIndex < len(output.Results) {
		output.FailedIndex = workspaceRepairIntPointer(failedIndex)
		output.FailedDocument = output.Results[failedIndex].Document
		output.Results[failedIndex].ErrorCode = code
		output.Results[failedIndex].Error = output.FailureMessage
	}
	if output.PartialCommit {
		code = ErrCodePartialCommit
	}
	return output, errorResultWithCode(code, markdownWorkspaceRepairApplyFailureText(output))
}

func markdownWorkspaceRepairFailureInfo(failure *mcp.CallToolResult) (string, string) {
	code := ErrCodeOperationFailed
	message := "Markdown workspace repair apply failed"
	if failure == nil {
		return code, message
	}
	if value, ok := failure.Meta[ErrorCodeMetaKey].(string); ok && value != "" {
		code = value
	}
	if len(failure.Content) > 0 {
		if text, ok := failure.Content[0].(*mcp.TextContent); ok && text.Text != "" {
			message = text.Text
		}
	}
	return code, boundedMarkdownWorkspaceRepairFailureMessage(message)
}

func boundedMarkdownWorkspaceRepairFailureMessage(message string) string {
	message = strings.ToValidUTF8(message, "\uFFFD")
	if len(message) <= maxMarkdownWorkspaceRepairFailureMessageBytes {
		return message
	}
	end := maxMarkdownWorkspaceRepairFailureMessageBytes
	for end > 0 && !utf8.ValidString(message[:end]) {
		end--
	}
	return message[:end]
}

func markdownWorkspaceRepairApplyText(output markdownWorkspaceRepairApplyResult) string {
	return fmt.Sprintf("Markdown workspace repair apply: %d committed, %d unchanged, %d unknown; %d durable backups.", output.CommittedCount, output.UnchangedCount, output.UnknownCount, output.BackupCount)
}

func markdownWorkspaceRepairApplyFailureText(output markdownWorkspaceRepairApplyResult) string {
	text := markdownWorkspaceRepairApplyText(output)
	if output.FailureMessage != "" {
		text += "\nReason: " + output.FailureMessage
	}
	return text
}

func workspaceRepairIntPointer(value int) *int {
	return &value
}
