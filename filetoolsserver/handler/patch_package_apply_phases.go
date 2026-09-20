package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

type patchPackageApplyPhaseFailure struct {
	index int
	err   error
}

func (h *Handler) preflightPatchPackageApply(ctx context.Context, prepared *preparedPatchPackage) ([]patchPackageApplyPreflight, *mcp.CallToolResult) {
	preflight := make([]patchPackageApplyPreflight, len(prepared.plan.targets))
	for index := range prepared.plan.targets {
		target := &prepared.plan.targets[index]
		if !preparedEditPlanReplacement(target).resultMatchesFingerprint() {
			return nil, errorResultWithCode(ErrCodeConflict, fmt.Sprintf("patch package target %d prepared result no longer matches its fingerprint", index))
		}
		current, failure := h.revalidatePreparedPatchPackageTarget(ctx, target, "before staging")
		if failure != nil {
			return nil, failure
		}
		preflight[index].mode = current.Mode.Perm()
	}
	return preflight, nil
}

func (h *Handler) capturePatchPackageApplyBackups(ctx context.Context, prepared *preparedPatchPackage, output *PatchPackageOutput, preflight []patchPackageApplyPreflight) *patchPackageApplyPhaseFailure {
	if !persistentBackupRequired(prepared.backupPolicy) {
		return nil
	}
	if h.backupBatchCapture == nil {
		return &patchPackageApplyPhaseFailure{index: -1, err: operation.New(operation.KindConflict, "required package backup authority is unavailable")}
	}

	requests := prepared.plan.backupCaptureRequests(backupstore.SourceOperationPatchPackage, prepared.label, prepared.backupPolicy)
	if len(requests) > 0 {
		captures, captureErr := h.backupBatchCapture.CaptureBatch(ctx, requests)
		if failure := validatePatchPackageBackupCaptures(prepared, output, requests, captures, captureErr); failure != nil {
			return failure
		}
		if captureErr != nil {
			// Every authoritative manifest is durable; only derived projection work failed.
			slog.Warn("package backup manifests committed but derived index refresh reported an error", "backupCount", output.BackupCount)
		}
	}
	return h.revalidatePatchPackageAfterBackups(ctx, prepared, preflight)
}

func validatePatchPackageBackupCaptures(prepared *preparedPatchPackage, output *PatchPackageOutput, requests []backupstore.CaptureRequest, captures []backupstore.CaptureResult, captureErr error) *patchPackageApplyPhaseFailure {
	changedIndices := patchPackageChangedTargetIndices(prepared.plan.targets)
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
		if !patchPackageBackupMatches(manifest, prepared.plan.targets[targetIndex]) {
			captureErr = errors.Join(captureErr, operation.New(operation.KindConflict, "durable package backup does not match the approved pre-state"))
			break
		}
		verifiedCaptures++
	}
	if !invalidBatchResult && verifiedCaptures == len(requests) {
		return nil
	}
	if captureErr == nil {
		captureErr = operation.New(operation.KindFilesystem, "required package backup batch is incomplete")
	}
	return &patchPackageApplyPhaseFailure{
		index: patchPackageFirstUnverifiedTarget(changedIndices, verifiedCaptures),
		err:   captureErr,
	}
}

func patchPackageChangedTargetIndices(targets []preparedEditPlanTarget) []int {
	indices := make([]int, 0, len(targets))
	for index := range targets {
		if targets[index].prepared.changed {
			indices = append(indices, index)
		}
	}
	return indices
}

func patchPackageBackupMatches(manifest backupstore.Manifest, target preparedEditPlanTarget) bool {
	return validPatchPackagePreviewID(manifest.BackupID) &&
		manifest.TargetPath == target.resolvedPath &&
		manifest.SourceOperation == backupstore.SourceOperationPatchPackage &&
		manifest.ContentFingerprint == target.prepared.targetFingerprint
}

func patchPackageFirstUnverifiedTarget(changedIndices []int, verified int) int {
	if verified < len(changedIndices) {
		return changedIndices[verified]
	}
	return -1
}

func (h *Handler) revalidatePatchPackageAfterBackups(ctx context.Context, prepared *preparedPatchPackage, preflight []patchPackageApplyPreflight) *patchPackageApplyPhaseFailure {
	for index := range prepared.plan.targets {
		current, failure := h.revalidatePreparedPatchPackageTarget(ctx, &prepared.plan.targets[index], "after package backup")
		if failure == nil {
			preflight[index].mode = current.Mode.Perm()
			continue
		}
		if ctx.Err() != nil {
			return &patchPackageApplyPhaseFailure{
				index: index,
				err:   operation.Wrap(operation.KindCancelled, "verify_package_after_backup", prepared.plan.targets[index].resolvedPath, ctx.Err()),
			}
		}
		return &patchPackageApplyPhaseFailure{
			index: index,
			err:   operation.New(operation.KindConflict, extractPatchPackageFailureMessage(failure)),
		}
	}
	return nil
}

func (h *Handler) stagePatchPackageApply(ctx context.Context, prepared *preparedPatchPackage, preflight []patchPackageApplyPreflight) (*existingFileReplacementBatch, error) {
	replacements := make([]preparedExistingFileReplacement, len(prepared.plan.targets))
	modes := make([]os.FileMode, len(prepared.plan.targets))
	for index := range prepared.plan.targets {
		replacements[index] = preparedEditPlanReplacement(&prepared.plan.targets[index])
		modes[index] = preflight[index].mode
	}
	return h.stageExistingFileReplacementBatch(
		ctx,
		replacements,
		modes,
		h.existingFileReplacementOps,
		"stage_patch_package",
		"cleanup_patch_package_stage",
	)
}

func (h *Handler) commitPatchPackageApply(ctx context.Context, prepared *preparedPatchPackage, output *PatchPackageOutput, batch *existingFileReplacementBatch, actualFingerprints []string) *patchPackageApplyPhaseFailure {
	for index := range prepared.plan.targets {
		target := &prepared.plan.targets[index]
		if !target.prepared.changed {
			markPatchPackageTargetUnchanged(output, actualFingerprints, index, target)
			continue
		}
		actual, readOnlyCleared, err := h.commitPatchPackageApplyTarget(ctx, index, target, batch)
		if err != nil {
			return &patchPackageApplyPhaseFailure{index: index, err: err}
		}
		markPatchPackageTargetCommitted(output, actualFingerprints, index, actual, readOnlyCleared)
	}
	return nil
}

func markPatchPackageTargetUnchanged(output *PatchPackageOutput, actualFingerprints []string, index int, target *preparedEditPlanTarget) {
	output.Results[index].State = patchPackageStateUnchanged
	output.Results[index].ActualFingerprint = target.prepared.targetFingerprint
	output.UnchangedCount++
	actualFingerprints[index] = target.prepared.targetFingerprint
}

func markPatchPackageTargetCommitted(output *PatchPackageOutput, actualFingerprints []string, index int, actual string, readOnlyCleared bool) {
	output.Results[index].State = patchPackageStateCommitted
	output.Results[index].ActualFingerprint = actual
	output.Results[index].Applied = true
	output.Results[index].ReadOnlyCleared = readOnlyCleared
	output.CommittedCount++
	actualFingerprints[index] = actual
}

func (h *Handler) commitPatchPackageApplyTarget(ctx context.Context, index int, target *preparedEditPlanTarget, batch *existingFileReplacementBatch) (string, bool, error) {
	replacement := preparedEditPlanReplacement(target)
	current, originalMode, readOnlyCleared, err := h.preparePatchPackageCommitTarget(ctx, target, replacement)
	if err != nil {
		return "", false, err
	}
	actual, commitErr := h.commitExistingFileReplacementBatchTarget(
		ctx,
		batch,
		index,
		current,
		"committed target does not match the prepared result fingerprint",
	)
	if commitErr != nil {
		if readOnlyCleared {
			commitErr = errors.Join(commitErr, h.restorePatchPackageReadOnlyIfUnchanged(target, originalMode))
		}
		return "", false, commitErr
	}
	return actual, readOnlyCleared, nil
}

func (h *Handler) preparePatchPackageCommitTarget(ctx context.Context, target *preparedEditPlanTarget, replacement preparedExistingFileReplacement) (filesystem.FileSnapshot, os.FileMode, bool, error) {
	if err := ctx.Err(); err != nil {
		return filesystem.FileSnapshot{}, 0, false, operation.Wrap(operation.KindCancelled, "commit_patch_package", replacement.resolvedPath, err)
	}
	current, failure := h.revalidatePreparedPatchPackageTarget(ctx, target, "before commit")
	if failure != nil {
		return filesystem.FileSnapshot{}, 0, false, operation.New(operation.KindConflict, extractPatchPackageFailureMessage(failure))
	}
	if replacement.identity() == nil {
		return filesystem.FileSnapshot{}, 0, false, operation.New(operation.KindConflict, "patch package target identity is unavailable")
	}
	if err := replacement.closeIdentity(); err != nil {
		return filesystem.FileSnapshot{}, 0, false, operation.WrapFilesystem("close_patch_package_identity", replacement.resolvedPath, err)
	}
	return h.prepareExistingFileReplacementWritable(replacement, current, "target became read-only after patch package dryRun")
}

func (h *Handler) prepareExistingFileReplacementWritable(replacement preparedExistingFileReplacement, current filesystem.FileSnapshot, readOnlyMessage string) (filesystem.FileSnapshot, os.FileMode, bool, error) {
	currentMode := current.Mode.Perm()
	if !isReadOnly(currentMode) {
		return current, currentMode, false, nil
	}
	if replacement.readOnlyRequiresApproval(currentMode) {
		return filesystem.FileSnapshot{}, currentMode, false, operation.New(operation.KindPermission, readOnlyMessage)
	}
	if err := clearReadOnly(replacement.resolvedPath, currentMode); err != nil {
		return filesystem.FileSnapshot{}, currentMode, false, operation.WrapFilesystem("clear_patch_package_read_only", replacement.resolvedPath, err)
	}
	refreshed, err := current.RefreshMetadata(replacement.resolvedPath)
	if err == nil {
		return refreshed, currentMode, true, nil
	}
	if restoreErr := os.Chmod(replacement.resolvedPath, currentMode); restoreErr != nil {
		err = errors.Join(err, operation.WrapFilesystem("restore_patch_package_read_only", replacement.resolvedPath, restoreErr))
	}
	return filesystem.FileSnapshot{}, currentMode, true, err
}

func (h *Handler) verifyPatchPackageApplyFinal(ctx context.Context, prepared *preparedPatchPackage, output *PatchPackageOutput, batch *existingFileReplacementBatch, actualFingerprints []string) *patchPackageApplyPhaseFailure {
	if err := batch.cleanup("cleanup_patch_package_stage"); err != nil {
		return &patchPackageApplyPhaseFailure{index: max(0, len(prepared.plan.targets)-1), err: err}
	}
	finalTargets := patchPackageFinalTargets(prepared.plan.targets)
	finalFingerprints, err := h.capturePatchPackageFingerprints(ctx, finalTargets)
	if err != nil {
		return &patchPackageApplyPhaseFailure{index: -1, err: err}
	}
	for index := range prepared.plan.targets {
		if finalFingerprints[index] != prepared.plan.targets[index].prepared.resultFingerprint {
			return &patchPackageApplyPhaseFailure{
				index: index,
				err:   operation.New(operation.KindConflict, fmt.Sprintf("patch package target %d changed during final package verification", index)),
			}
		}
		output.Results[index].ActualFingerprint = finalFingerprints[index]
		actualFingerprints[index] = finalFingerprints[index]
	}
	return nil
}

func patchPackageFinalTargets(targets []preparedEditPlanTarget) []validatedPatchPackageTarget {
	finalTargets := make([]validatedPatchPackageTarget, len(targets))
	for index := range targets {
		finalTargets[index].resolvedPath = targets[index].resolvedPath
	}
	return finalTargets
}

func (h *Handler) classifyPatchPackageFailureTargets(ctx context.Context, prepared *preparedPatchPackage, output *PatchPackageOutput) ([]string, bool) {
	actualFingerprints := make([]string, len(prepared.plan.targets))
	completeAggregate := true
	for index := range prepared.plan.targets {
		state, actual, applied := h.classifyPatchPackageFailureTarget(ctx, &prepared.plan.targets[index])
		result := &output.Results[index]
		resetPatchPackageFailureResult(result)
		result.State = state
		result.ActualFingerprint = actual
		result.Applied = applied
		actualFingerprints[index] = actual
		switch state {
		case patchPackageStateCommitted:
			output.CommittedCount++
		case patchPackageStateUnchanged:
			output.UnchangedCount++
		default:
			output.UnknownCount++
			completeAggregate = false
		}
	}
	return actualFingerprints, completeAggregate
}

func resetPatchPackageFailureResult(result *PatchPackageTargetResult) {
	result.State = ""
	result.ActualFingerprint = ""
	result.Applied = false
	result.Verified = false
	result.ErrorCode = ""
	result.Error = ""
}

func (h *Handler) classifyPatchPackageFailureTarget(ctx context.Context, target *preparedEditPlanTarget) (string, string, bool) {
	state, actual, applied := h.classifyExistingFileReplacement(ctx, preparedEditPlanReplacement(target))
	return string(state), actual, applied
}
