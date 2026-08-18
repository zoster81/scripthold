package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"github.com/zoster81/scripthold/internal/textstream"
)

const (
	patchPackageActionApply  = "apply"
	patchPackageActionVerify = "verify"

	patchPackageStatePrepared  = "prepared"
	patchPackageStateCommitted = "committed"
	patchPackageStateUnchanged = "unchanged"
	patchPackageStateVerified  = "verified"
	patchPackageStateMismatch  = "mismatch"
	patchPackageStateUnknown   = "unknown"

	maxPatchPackageFailureMessageBytes = 1024
	patchPackageClassificationTimeout  = 30 * time.Second
)

type patchPackageApplyPreflight struct {
	mode os.FileMode
}

func validatePatchPackageActionInput(input PatchPackageInput) (string, error) {
	action := strings.TrimSpace(input.Action)
	switch action {
	case patchPackageActionInspect, patchPackageActionDryRun, patchPackageActionVerify:
		if input.PreviewID != "" {
			return "", operation.New(operation.KindInvalidInput, "previewId is accepted only with action=apply")
		}
	case patchPackageActionApply:
		if !validPatchPackagePreviewID(input.PreviewID) {
			return "", operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")
		}
		if !patchPackageManifestEmpty(input.Manifest) {
			return "", operation.New(operation.KindInvalidInput, "action=apply accepts only previewId")
		}
	default:
		return "", operation.New(operation.KindInvalidInput, "action must be inspect, dryRun, apply, or verify")
	}
	return action, nil
}

func patchPackageManifestEmpty(manifest PatchPackageManifest) bool {
	return manifest.FormatVersion == "" && manifest.Label == "" && manifest.FingerprintAlgorithm == "" &&
		manifest.FingerprintMode == "" && manifest.BackupPolicy == "" && len(manifest.Targets) == 0
}

func stagePatchPackageReplacement(ctx context.Context, path string, data []byte, mode os.FileMode) (*filesystem.StagedReplacement, error) {
	return filesystem.StageReplacement(path, textstream.WithContext(ctx, bytes.NewReader(data)), mode, nil)
}

func commitPatchPackageReplacement(_ int, staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
	return staged.Commit(options)
}

func (h *Handler) handlePatchPackageApply(ctx context.Context, previewID string) (*mcp.CallToolResult, PatchPackageOutput, error) {
	preview, err := h.patchPackagePreviews.claim(previewID)
	if err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	prepared := &preview.prepared
	defer prepared.close()
	if err := ctx.Err(); err != nil {
		cancelled := operation.Wrap(operation.KindCancelled, "apply_patch_package", "", err)
		return errorResultFromError(cancelled), PatchPackageOutput{}, nil
	}

	output := patchPackageOutputFromPreview(preview, patchPackageActionApply)
	output.PreviewID = ""
	output.CreatedAt = ""
	output.ExpiresAt = ""
	output.CommittedCount = 0
	output.UnchangedCount = 0
	output.UnknownCount = 0
	if err := h.checkPatchPackageApplyWorstCaseOutput(output); err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}

	preflight := make([]patchPackageApplyPreflight, len(prepared.targets))
	for index := range prepared.targets {
		target := &prepared.targets[index]
		if filesystem.FingerprintRegularFileData(target.prepared.data) != target.prepared.resultFingerprint {
			return errorResultWithCode(ErrCodeConflict, fmt.Sprintf("patch package target %d prepared result no longer matches its fingerprint", index)), PatchPackageOutput{}, nil
		}
		current, failure := h.revalidatePreparedPatchPackageTarget(ctx, target, "before staging")
		if failure != nil {
			return failure, PatchPackageOutput{}, nil
		}
		preflight[index] = patchPackageApplyPreflight{mode: current.Mode.Perm()}
	}

	staged := make([]*filesystem.StagedReplacement, len(prepared.targets))
	if prepared.backupPolicy == editBackupPolicyRequired {
		if h.backupBatchCapture == nil {
			failure := operation.New(operation.KindConflict, "required package backup authority is unavailable")
			return h.patchPackageApplyFailure(prepared, output, -1, failure, staged)
		}
		requests := patchPackageCaptureRequests(prepared.label, prepared.targets)
		if len(requests) > 0 {
			captures, captureErr := h.backupBatchCapture.CaptureBatch(ctx, requests)
			changedIndices := make([]int, 0, len(requests))
			for index := range prepared.targets {
				if prepared.targets[index].prepared.changed {
					changedIndices = append(changedIndices, index)
				}
			}
			invalidBatchResult := len(captures) > len(changedIndices)
			if invalidBatchResult {
				captureErr = errors.Join(operation.New(operation.KindConflict, "backup batch returned unexpected results"), captureErr)
				captures = captures[:len(changedIndices)]
			}
			verifiedCaptures := 0
			for captureIndex, captured := range captures {
				targetIndex := changedIndices[captureIndex]
				manifest := captured.Manifest
				if validPatchPackagePreviewID(manifest.BackupID) {
					output.Results[targetIndex].BackupID = manifest.BackupID
					output.BackupCount++
				}
				if !validPatchPackagePreviewID(manifest.BackupID) || manifest.TargetPath != prepared.targets[targetIndex].resolvedPath ||
					manifest.SourceOperation != backupstore.SourceOperationPatchPackage ||
					manifest.ContentFingerprint != prepared.targets[targetIndex].prepared.targetFingerprint {
					captureErr = errors.Join(captureErr, operation.New(operation.KindConflict, "durable package backup does not match the approved pre-state"))
					break
				}
				verifiedCaptures++
			}
			if invalidBatchResult || verifiedCaptures != len(requests) {
				failedIndex := -1
				if verifiedCaptures < len(changedIndices) {
					failedIndex = changedIndices[verifiedCaptures]
				}
				if captureErr == nil {
					captureErr = operation.New(operation.KindFilesystem, "required package backup batch is incomplete")
				}
				return h.patchPackageApplyFailure(prepared, output, failedIndex, captureErr, staged)
			}
			if captureErr != nil {
				// Every authoritative manifest is durable; only derived projection work failed.
				slog.Warn("package backup manifests committed but derived index refresh reported an error", "backupCount", output.BackupCount)
			}
		}
		for index := range prepared.targets {
			current, failure := h.revalidatePreparedPatchPackageTarget(ctx, &prepared.targets[index], "after package backup")
			if failure != nil {
				if ctx.Err() != nil {
					cancelled := operation.Wrap(operation.KindCancelled, "verify_package_after_backup", prepared.targets[index].resolvedPath, ctx.Err())
					return h.patchPackageApplyFailure(prepared, output, index, cancelled, staged)
				}
				conflict := operation.New(operation.KindConflict, extractPatchPackageFailureMessage(failure))
				return h.patchPackageApplyFailure(prepared, output, index, conflict, staged)
			}
			preflight[index].mode = current.Mode.Perm()
		}
	}

	for index := range prepared.targets {
		target := &prepared.targets[index]
		if !target.prepared.changed {
			continue
		}
		mode := preflight[index].mode
		if isReadOnly(mode) {
			mode |= 0200
		}
		replacement, stageErr := h.patchPackageStageReplacement(ctx, target.prepared.resolvedPath, target.prepared.data, mode)
		if stageErr != nil {
			if ctx.Err() != nil {
				stageErr = operation.Wrap(operation.KindCancelled, "stage_patch_package", target.prepared.resolvedPath, ctx.Err())
			}
			stageErr = h.joinPatchPackageStagingCleanup(stageErr, staged)
			return errorResultFromError(stageErr), PatchPackageOutput{}, nil
		}
		staged[index] = replacement
	}
	actualFingerprints := make([]string, len(prepared.targets))
	for index := range prepared.targets {
		target := &prepared.targets[index]
		if !target.prepared.changed {
			output.Results[index].State = patchPackageStateUnchanged
			output.Results[index].ActualFingerprint = target.prepared.targetFingerprint
			output.UnchangedCount++
			actualFingerprints[index] = target.prepared.targetFingerprint
			continue
		}
		if err := ctx.Err(); err != nil {
			cancelled := operation.Wrap(operation.KindCancelled, "commit_patch_package", target.prepared.resolvedPath, err)
			return h.patchPackageApplyFailure(prepared, output, index, cancelled, staged)
		}
		current, failure := h.revalidatePreparedPatchPackageTarget(ctx, target, "before commit")
		if failure != nil {
			return h.patchPackageApplyFailure(prepared, output, index, operation.New(operation.KindConflict, extractPatchPackageFailureMessage(failure)), staged)
		}
		if target.prepared.identityFile == nil {
			return h.patchPackageApplyFailure(prepared, output, index, operation.New(operation.KindConflict, "patch package target identity is unavailable"), staged)
		}
		if err := target.prepared.identityFile.Close(); err != nil {
			return h.patchPackageApplyFailure(prepared, output, index, operation.WrapFilesystem("close_patch_package_identity", target.prepared.resolvedPath, err), staged)
		}
		target.prepared.identityFile = nil

		currentMode := current.Mode.Perm()
		readOnlyCleared := false
		if isReadOnly(currentMode) {
			if !target.prepared.forceWritable {
				return h.patchPackageApplyFailure(prepared, output, index, operation.New(operation.KindPermission, "target became read-only after patch package dryRun"), staged)
			}
			if err := clearReadOnly(target.prepared.resolvedPath, currentMode); err != nil {
				return h.patchPackageApplyFailure(prepared, output, index, operation.WrapFilesystem("clear_patch_package_read_only", target.prepared.resolvedPath, err), staged)
			}
			readOnlyCleared = true
			refreshed, refreshErr := current.RefreshMetadata(target.prepared.resolvedPath)
			if refreshErr != nil {
				if restoreErr := os.Chmod(target.prepared.resolvedPath, currentMode); restoreErr != nil {
					refreshErr = errors.Join(refreshErr, operation.WrapFilesystem("restore_patch_package_read_only", target.prepared.resolvedPath, restoreErr))
				}
				return h.patchPackageApplyFailure(prepared, output, index, refreshErr, staged)
			}
			current = refreshed
		}

		_, commitErr := h.patchPackageCommitReplacement(index, staged[index], filesystem.ReplaceOptions{Expected: &current})
		if commitErr != nil {
			if readOnlyCleared {
				commitErr = errors.Join(commitErr, h.restorePatchPackageReadOnlyIfUnchanged(target, currentMode))
			}
			return h.patchPackageApplyFailure(prepared, output, index, commitErr, staged)
		}
		staged[index] = nil
		post, postErr := filesystem.CaptureRegularFileSnapshotBounded(ctx, target.prepared.resolvedPath, h.maxFileBytes())
		if postErr != nil {
			return h.patchPackageApplyFailure(prepared, output, index, postErr, staged)
		}
		actual, fingerprintErr := filesystem.FingerprintRegularFileSnapshot(post)
		if fingerprintErr != nil {
			return h.patchPackageApplyFailure(prepared, output, index, fingerprintErr, staged)
		}
		if actual != target.prepared.resultFingerprint {
			return h.patchPackageApplyFailure(prepared, output, index, operation.New(operation.KindConflict, "committed target does not match the prepared result fingerprint"), staged)
		}
		output.Results[index].State = patchPackageStateCommitted
		output.Results[index].ActualFingerprint = actual
		output.Results[index].Applied = true
		output.Results[index].ReadOnlyCleared = readOnlyCleared
		output.CommittedCount++
		actualFingerprints[index] = actual
	}

	if cleanupErr := h.cleanupPatchPackageStaging(staged); cleanupErr != nil {
		return h.patchPackageApplyFailure(prepared, output, max(0, len(prepared.targets)-1), cleanupErr, staged)
	}
	finalTargets := make([]validatedPatchPackageTarget, len(prepared.targets))
	for index := range prepared.targets {
		finalTargets[index].resolvedPath = prepared.targets[index].resolvedPath
	}
	finalFingerprints, finalErr := h.capturePatchPackageFingerprints(ctx, finalTargets)
	if finalErr != nil {
		return h.patchPackageApplyFailure(prepared, output, -1, finalErr, staged)
	}
	for index := range prepared.targets {
		if finalFingerprints[index] != prepared.targets[index].prepared.resultFingerprint {
			conflict := operation.New(operation.KindConflict, fmt.Sprintf("patch package target %d changed during final package verification", index))
			return h.patchPackageApplyFailure(prepared, output, index, conflict, staged)
		}
		output.Results[index].ActualFingerprint = finalFingerprints[index]
		actualFingerprints[index] = finalFingerprints[index]
	}
	output.Applied = true
	output.ActualAggregateFingerprint = patchPackageAggregatePrepared(prepared.targets, actualFingerprints)
	text := patchPackageApplyText(output)
	if err := h.checkPatchPackageResponseLimit(output, text); err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) cleanupPatchPackageStaging(staged []*filesystem.StagedReplacement) error {
	var cleanupErrors []error
	for index, replacement := range staged {
		if replacement == nil {
			continue
		}
		if err := h.patchPackageCleanupReplacement(replacement); err != nil {
			cleanupErrors = append(cleanupErrors, operation.WrapFilesystem("cleanup_patch_package_stage", "", err))
		}
		staged[index] = nil
	}
	return errors.Join(cleanupErrors...)
}

func (h *Handler) joinPatchPackageStagingCleanup(cause error, staged []*filesystem.StagedReplacement) error {
	cleanupErr := h.cleanupPatchPackageStaging(staged)
	if cleanupErr == nil {
		return cause
	}
	if cause == nil {
		return cleanupErr
	}
	return errors.Join(cause, cleanupErr)
}

func (h *Handler) restorePatchPackageReadOnlyIfUnchanged(target *preparedPatchPackageTarget, originalMode os.FileMode) error {
	ctx, cancel := context.WithTimeout(context.Background(), patchPackageClassificationTimeout)
	defer cancel()
	current, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, target.prepared.resolvedPath, h.maxFileBytes())
	if err != nil {
		return operation.Wrap(operation.KindConflict, "inspect_patch_package_read_only_restore", target.prepared.resolvedPath, err)
	}
	fingerprint, err := filesystem.FingerprintRegularFileSnapshot(current)
	if err != nil {
		return err
	}
	if fingerprint != target.prepared.targetFingerprint {
		return nil
	}
	if err := os.Chmod(target.prepared.resolvedPath, originalMode); err != nil {
		return operation.WrapFilesystem("restore_patch_package_read_only", target.prepared.resolvedPath, err)
	}
	return nil
}

func (h *Handler) revalidatePreparedPatchPackageTarget(ctx context.Context, target *preparedPatchPackageTarget, phase string) (filesystem.FileSnapshot, *mcp.CallToolResult) {
	validation := h.ValidatePath(target.requestedPath)
	if !validation.Ok() {
		return filesystem.FileSnapshot{}, validation.Result
	}
	if validation.Path != target.resolvedPath {
		return filesystem.FileSnapshot{}, errorResultWithCode(ErrCodeConflict, fmt.Sprintf("patch package target path changed %s", phase))
	}
	if target.prepared.identityFile == nil {
		return filesystem.FileSnapshot{}, errorResultWithCode(ErrCodeConflict, "patch package target identity is unavailable")
	}
	matches, err := target.prepared.identityFile.Matches(validation.Path)
	if err != nil || !matches {
		return filesystem.FileSnapshot{}, errorResultWithCode(ErrCodeConflict, fmt.Sprintf("patch package target identity changed %s", phase))
	}
	current, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, validation.Path, h.maxFileBytes())
	if err != nil {
		if operation.KindOf(err) == operation.KindCancelled {
			return filesystem.FileSnapshot{}, errorResultFromError(err)
		}
		return filesystem.FileSnapshot{}, errorResultWithCode(ErrCodeConflict, fmt.Sprintf("patch package target is unavailable or changed %s", phase))
	}
	fingerprint, err := filesystem.FingerprintRegularFileSnapshot(current)
	if err != nil {
		return filesystem.FileSnapshot{}, errorResultFromError(err)
	}
	if fingerprint != target.prepared.targetFingerprint {
		return filesystem.FileSnapshot{}, errorResultWithCode(ErrCodeConflict, fmt.Sprintf("patch package target fingerprint changed %s", phase))
	}
	if isReadOnly(current.Mode.Perm()) && !target.prepared.forceWritable {
		return filesystem.FileSnapshot{}, errorResultWithCode(ErrCodePermission, "file is read-only and forceWritable was not approved")
	}
	return current, nil
}

func (h *Handler) patchPackageApplyFailure(prepared *preparedPatchPackage, output PatchPackageOutput, failedIndex int, cause error, staged []*filesystem.StagedReplacement) (*mcp.CallToolResult, PatchPackageOutput, error) {
	cause = h.joinPatchPackageStagingCleanup(cause, staged)
	classificationCtx, cancel := context.WithTimeout(context.Background(), patchPackageClassificationTimeout)
	defer cancel()
	mapping := mapOperationError(cause, "")
	output.Applied = false
	output.FailedIndex = nil
	output.FailedPath = ""
	if failedIndex >= 0 && failedIndex < len(prepared.targets) {
		output.FailedIndex = intPointer(failedIndex)
		output.FailedPath = prepared.targets[failedIndex].requestedPath
	}
	output.FailureCode = mapping.BatchCode
	output.FailureMessage = boundedPatchPackageFailureMessage(mapping.Message)
	output.CommittedCount = 0
	output.UnchangedCount = 0
	output.UnknownCount = 0
	actualFingerprints := make([]string, len(prepared.targets))
	completeAggregate := true

	for index := range prepared.targets {
		target := &prepared.targets[index]
		result := &output.Results[index]
		result.State = ""
		result.ActualFingerprint = ""
		result.Applied = false
		result.Verified = false
		result.ErrorCode = ""
		result.Error = ""
		validation := h.ValidatePath(target.requestedPath)
		if !validation.Ok() || validation.Path != target.resolvedPath || classificationCtx.Err() != nil {
			result.State = patchPackageStateUnknown
			output.UnknownCount++
			completeAggregate = false
			continue
		}
		fingerprints, err := h.capturePatchPackageFingerprints(classificationCtx, []validatedPatchPackageTarget{{resolvedPath: validation.Path}})
		if err != nil || len(fingerprints) != 1 {
			result.State = patchPackageStateUnknown
			output.UnknownCount++
			completeAggregate = false
			continue
		}
		actual := fingerprints[0]
		if actual == "" {
			result.State = patchPackageStateUnknown
			output.UnknownCount++
			completeAggregate = false
			continue
		}
		result.ActualFingerprint = actual
		actualFingerprints[index] = actual
		switch {
		case actual == target.prepared.targetFingerprint:
			result.State = patchPackageStateUnchanged
			output.UnchangedCount++
		case target.prepared.changed && actual == target.prepared.resultFingerprint:
			result.State = patchPackageStateCommitted
			result.Applied = true
			output.CommittedCount++
		default:
			result.State = patchPackageStateUnknown
			output.UnknownCount++
		}
	}
	if completeAggregate {
		output.ActualAggregateFingerprint = patchPackageAggregatePrepared(prepared.targets, actualFingerprints)
	}
	output.PartialCommit = output.CommittedCount > 0 || output.UnknownCount > 0
	code := mapping.BatchCode
	if output.PartialCommit {
		code = ErrCodePartialCommit
	}
	if failedIndex >= 0 && failedIndex < len(output.Results) {
		output.Results[failedIndex].ErrorCode = mapping.BatchCode
		output.Results[failedIndex].Error = output.FailureMessage
	}
	text := patchPackageFailureText(output)
	if err := h.checkPatchPackageResponseLimit(output, text); err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	return errorResultWithCode(code, text), output, nil
}

func (h *Handler) handlePatchPackageVerify(ctx context.Context, manifest PatchPackageManifest, targets []validatedPatchPackageTarget) (*mcp.CallToolResult, PatchPackageOutput, error) {
	expected := make([]string, len(targets))
	for index := range targets {
		if targets[index].expectedResultFingerprint == "" {
			err := operation.New(operation.KindInvalidInput, fmt.Sprintf("patch package target %d expectedResultFingerprint is required for verify", index))
			return errorResultFromError(err), PatchPackageOutput{}, nil
		}
		expected[index] = targets[index].expectedResultFingerprint
	}
	actual, err := h.capturePatchPackageFingerprints(ctx, targets)
	if err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	output := patchPackageBaseOutput(patchPackageActionVerify, manifest, targets)
	output.AggregateMode = patchPackageAggregateModeV1
	output.AggregateAfterFingerprint = patchPackageAggregate(targets, expected)
	output.ActualAggregateFingerprint = patchPackageAggregate(targets, actual)
	output.Verified = true
	for index := range targets {
		result := &output.Results[index]
		result.ActualFingerprint = actual[index]
		result.ResultFingerprint = expected[index]
		if actual[index] == expected[index] {
			result.State = patchPackageStateVerified
			result.Verified = true
		} else {
			result.State = patchPackageStateMismatch
			output.Verified = false
			output.MismatchCount++
		}
	}
	text := patchPackageVerifyText(output)
	if err := h.checkPatchPackageResponseLimit(output, text); err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	if !output.Verified {
		return errorResultWithCode(ErrCodeConflict, text), output, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func patchPackageOutputFromPreview(preview *patchPackagePreview, action string) PatchPackageOutput {
	prepared := preview.prepared
	output := PatchPackageOutput{
		Action:                     action,
		FormatVersion:              prepared.formatVersion,
		Label:                      prepared.label,
		FingerprintAlgorithm:       prepared.fingerprintAlgorithm,
		FingerprintMode:            prepared.fingerprintMode,
		BackupPolicy:               prepared.backupPolicy,
		AggregateMode:              prepared.aggregateMode,
		AggregateBeforeFingerprint: prepared.aggregateBeforeFingerprint,
		AggregateAfterFingerprint:  prepared.aggregateAfterFingerprint,
		PreviewID:                  preview.id,
		CreatedAt:                  preview.createdAt.Format(timeRFC3339Nano),
		ExpiresAt:                  preview.expiresAt.Format(timeRFC3339Nano),
		TargetCount:                len(prepared.targets),
		Results:                    make([]PatchPackageTargetResult, len(prepared.targets)),
	}
	for index, target := range prepared.targets {
		output.Results[index] = PatchPackageTargetResult{
			Index:                     index,
			Path:                      target.requestedPath,
			State:                     patchPackageStatePrepared,
			ExpectedFingerprint:       target.expectedFingerprint,
			ActualFingerprint:         target.prepared.targetFingerprint,
			ExpectedResultFingerprint: target.expectedResultFingerprint,
			ResultFingerprint:         target.prepared.resultFingerprint,
			Diff:                      target.prepared.diff,
			Encoding:                  target.prepared.encoding,
			HasBOM:                    target.prepared.hasBOM,
			BOMType:                   target.prepared.bomType,
			LineEndingStyle:           target.prepared.lineEndingStyle,
			Changed:                   target.prepared.changed,
		}
		if target.prepared.changed {
			output.ChangedCount++
		} else {
			output.UnchangedCount++
		}
	}
	return output
}

func patchPackageAggregatePrepared(targets []preparedPatchPackageTarget, fingerprints []string) string {
	canonical := make([]string, len(targets))
	for index := range targets {
		canonical[index] = targets[index].canonicalManifestPath
	}
	return patchPackageAggregateCanonical(canonical, fingerprints)
}

func (h *Handler) checkPatchPackageApplyWorstCaseOutput(output PatchPackageOutput) error {
	worst := output
	worst.Results = append([]PatchPackageTargetResult(nil), output.Results...)
	worst.PartialCommit = true
	if worst.BackupPolicy == editBackupPolicyRequired {
		worst.BackupCount = worst.ChangedCount
	}
	worst.ActualAggregateFingerprint = strings.Repeat("f", sha256.Size*2)
	worst.CommittedCount = output.TargetCount
	worst.UnchangedCount = output.TargetCount
	worst.UnknownCount = output.TargetCount
	worst.FailedIndex = intPointer(max(0, output.TargetCount-1))
	for _, result := range worst.Results {
		if len(result.Path) > len(worst.FailedPath) {
			worst.FailedPath = result.Path
		}
	}
	worst.FailureCode = ErrCodeEncodingAmbiguous
	worst.FailureMessage = strings.Repeat("\x00", maxPatchPackageFailureMessageBytes)
	for index := range worst.Results {
		worst.Results[index].State = patchPackageStateUnchanged
		worst.Results[index].Applied = true
		worst.Results[index].ReadOnlyCleared = true
		if worst.BackupPolicy == editBackupPolicyRequired && worst.Results[index].Changed {
			worst.Results[index].BackupID = strings.Repeat("f", patchPackagePreviewTokenBytes*2)
		}
	}
	if len(worst.Results) > 0 {
		failed := len(worst.Results) - 1
		worst.Results[failed].ErrorCode = ErrCodeEncodingAmbiguous
		worst.Results[failed].Error = strings.Repeat("\x00", maxPatchPackageFailureMessageBytes)
	}
	return h.checkPatchPackageResponseLimit(worst, patchPackageFailureText(worst))
}

func patchPackageApplyText(output PatchPackageOutput) string {
	text := fmt.Sprintf("Patch package applied in manifest order: %d committed, %d unchanged.\nAggregate result: %s",
		output.CommittedCount, output.UnchangedCount, output.ActualAggregateFingerprint)
	if output.BackupPolicy != "" {
		text += fmt.Sprintf("\nDurable pre-state backups: %d", output.BackupCount)
	}
	return text
}

func patchPackageVerifyText(output PatchPackageOutput) string {
	if output.Verified {
		return fmt.Sprintf("Patch package verification passed for %d targets.\nAggregate fingerprint: %s", output.TargetCount, output.ActualAggregateFingerprint)
	}
	return fmt.Sprintf("Patch package verification failed: %d of %d targets do not match expectedResultFingerprint.\nExpected aggregate: %s\nActual aggregate: %s",
		output.MismatchCount, output.TargetCount, output.AggregateAfterFingerprint, output.ActualAggregateFingerprint)
}

func patchPackageFailureText(output PatchPackageOutput) string {
	status := "Patch package apply failed before a committed or unknown target state was detected."
	if output.PartialCommit {
		status = "Patch package apply stopped with a partial or uncertain commit state; no automatic rollback was attempted."
	}
	location := "Package-level failure"
	if output.FailedIndex != nil {
		location = fmt.Sprintf("Failure at target %d (%s)", *output.FailedIndex, output.FailedPath)
	}
	text := fmt.Sprintf("%s\n%s: %s [%s]\nFinal classification: %d committed, %d unchanged, %d unknown.",
		status, location, output.FailureMessage, output.FailureCode,
		output.CommittedCount, output.UnchangedCount, output.UnknownCount)
	if output.BackupPolicy != "" {
		text += fmt.Sprintf("\nDurable pre-state backups: %d", output.BackupCount)
	}
	return text
}

func extractPatchPackageFailureMessage(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return "patch package target validation failed"
	}
	if text, ok := result.Content[0].(*mcp.TextContent); ok && text.Text != "" {
		return text.Text
	}
	return "patch package target validation failed"
}

func boundedPatchPackageFailureMessage(message string) string {
	message = strings.ToValidUTF8(message, "\uFFFD")
	if len(message) <= maxPatchPackageFailureMessageBytes {
		return message
	}
	end := maxPatchPackageFailureMessageBytes
	for end > 0 && !utf8.ValidString(message[:end]) {
		end--
	}
	return message[:end]
}

func intPointer(value int) *int {
	return &value
}
