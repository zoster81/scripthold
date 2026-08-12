package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

func (h *Handler) readExactMutationTarget(ctx context.Context, requestedPath string) (preparedByteMutationTarget, error) {
	validation := h.ValidatePath(requestedPath)
	if !validation.Ok() {
		return preparedByteMutationTarget{}, validation.Err
	}
	identity, err := filesystem.OpenFileIdentity(validation.Path)
	if err != nil {
		return preparedByteMutationTarget{}, err
	}
	keepIdentity := false
	defer func() {
		if !keepIdentity {
			_ = identity.Close()
		}
	}()

	session, err := filesystem.OpenReadSession(validation.Path)
	if err != nil {
		return preparedByteMutationTarget{}, err
	}
	defer session.Close()
	if session.Size() > h.maxFileBytes() {
		return preparedByteMutationTarget{}, operation.New(operation.KindLimit, fmt.Sprintf("file size %d exceeds configured limit %d", session.Size(), h.maxFileBytes()))
	}
	if err := session.Start(0); err != nil {
		return preparedByteMutationTarget{}, err
	}
	readLimit := h.maxFileBytes()
	if readLimit < math.MaxInt64 {
		readLimit++
	}
	data, err := io.ReadAll(io.LimitReader(session, readLimit))
	if err != nil {
		return preparedByteMutationTarget{}, err
	}
	if int64(len(data)) > h.maxFileBytes() {
		return preparedByteMutationTarget{}, operation.New(operation.KindLimit, fmt.Sprintf("file exceeds configured limit %d", h.maxFileBytes()))
	}
	snapshot, err := session.Finish()
	if err != nil {
		return preparedByteMutationTarget{}, err
	}
	if err := session.Close(); err != nil {
		return preparedByteMutationTarget{}, operation.WrapFilesystem("close_mutation_preview_source", validation.Path, err)
	}
	matches, matchErr := identity.Matches(validation.Path)
	if matchErr != nil || !matches {
		return preparedByteMutationTarget{}, operation.New(operation.KindConflict, "target file identity changed during preview")
	}
	fingerprint, err := filesystem.FingerprintRegularFileSnapshot(snapshot)
	if err != nil {
		return preparedByteMutationTarget{}, err
	}
	keepIdentity = true
	return preparedByteMutationTarget{
		requestedPath:     requestedPath,
		resolvedPath:      validation.Path,
		data:              data,
		targetFingerprint: fingerprint,
		resultFingerprint: fingerprint,
		sourceSnapshot:    snapshot,
		sourceMode:        session.Mode(),
		identityFile:      identity,
	}, nil
}

func (h *Handler) revalidateExactMutationTarget(ctx context.Context, target *preparedByteMutationTarget) (filesystem.FileSnapshot, error) {
	if target == nil {
		return filesystem.FileSnapshot{}, operation.New(operation.KindConflict, "prepared mutation target is unavailable")
	}
	validation := h.ValidatePath(target.requestedPath)
	if !validation.Ok() {
		return filesystem.FileSnapshot{}, validation.Err
	}
	if validation.Path != target.resolvedPath {
		return filesystem.FileSnapshot{}, operation.New(operation.KindConflict, "target path changed after preview")
	}
	if target.identityFile == nil {
		return filesystem.FileSnapshot{}, operation.New(operation.KindConflict, "target identity is unavailable")
	}
	matches, err := target.identityFile.Matches(validation.Path)
	if err != nil || !matches {
		return filesystem.FileSnapshot{}, operation.New(operation.KindConflict, "target file identity changed after preview")
	}
	current, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, validation.Path, h.maxFileBytes())
	if err != nil {
		return filesystem.FileSnapshot{}, operation.New(operation.KindConflict, "target is unavailable or changed after preview")
	}
	fingerprint, err := filesystem.FingerprintRegularFileSnapshot(current)
	if err != nil {
		return filesystem.FileSnapshot{}, err
	}
	if fingerprint != target.targetFingerprint {
		return filesystem.FileSnapshot{}, operation.New(operation.KindConflict, "target fingerprint changed after preview")
	}
	return current, nil
}

func (h *Handler) preflightRequiredMutationBackups(ctx context.Context, prepared *preparedByteMutation, sourceOperation backupstore.SourceOperation) error {
	if prepared == nil || prepared.backupPolicy != editBackupPolicyRequired {
		return nil
	}
	requests := make([]backupstore.CaptureRequest, 0, len(prepared.targets))
	for index := range prepared.targets {
		if !prepared.targets[index].changed {
			continue
		}
		requests = append(requests, backupstore.CaptureRequest{
			TargetPath:      prepared.targets[index].resolvedPath,
			SourceOperation: sourceOperation,
		})
	}
	if len(requests) == 0 {
		return nil
	}
	if h.backupCapturePreflight == nil {
		return operation.New(operation.KindInvalidInput, "required backup preflight authority is unavailable")
	}
	return h.backupCapturePreflight.PreflightCaptureBatch(ctx, requests)
}

func validateCapturedMutationBackup(result backupstore.CaptureResult, target *preparedByteMutationTarget, sourceOperation backupstore.SourceOperation) error {
	manifest := result.Manifest
	if target == nil || !validByteMutationPreviewID(manifest.BackupID) || manifest.TargetPath != target.resolvedPath ||
		manifest.SourceOperation != sourceOperation || manifest.ContentFingerprint != target.targetFingerprint {
		return operation.New(operation.KindConflict, "durable backup does not match the approved mutation pre-state")
	}
	return nil
}

func (h *Handler) captureRequiredMutationBackup(ctx context.Context, target *preparedByteMutationTarget, sourceOperation backupstore.SourceOperation) (string, error) {
	if target == nil || !target.changed {
		return "", nil
	}
	if h.backupCapture == nil {
		return "", operation.New(operation.KindConflict, "required backup store is unavailable")
	}
	captured, captureErr := h.backupCapture.Capture(ctx, backupstore.CaptureRequest{
		TargetPath:      target.resolvedPath,
		SourceOperation: sourceOperation,
	})
	if captured.Manifest.BackupID == "" {
		if captureErr == nil {
			captureErr = operation.New(operation.KindFilesystem, "required backup did not commit a manifest")
		}
		return "", captureErr
	}
	if err := validateCapturedMutationBackup(captured, target, sourceOperation); err != nil {
		return captured.Manifest.BackupID, err
	}
	if captureErr != nil {
		slog.Warn("backup manifest committed; continuing after a post-manifest store error", "sourceOperation", sourceOperation)
	}
	return captured.Manifest.BackupID, nil
}

func (h *Handler) captureRequiredMutationBackups(ctx context.Context, prepared *preparedByteMutation, sourceOperation backupstore.SourceOperation) ([]string, error) {
	backupIDs := make([]string, len(prepared.targets))
	changedIndices := make([]int, 0, len(prepared.targets))
	requests := make([]backupstore.CaptureRequest, 0, len(prepared.targets))
	for index := range prepared.targets {
		if !prepared.targets[index].changed {
			continue
		}
		changedIndices = append(changedIndices, index)
		requests = append(requests, backupstore.CaptureRequest{
			TargetPath:      prepared.targets[index].resolvedPath,
			SourceOperation: sourceOperation,
		})
	}
	if len(requests) == 0 {
		return backupIDs, nil
	}
	if len(requests) == 1 {
		id, err := h.captureRequiredMutationBackup(ctx, &prepared.targets[changedIndices[0]], sourceOperation)
		backupIDs[changedIndices[0]] = id
		return backupIDs, err
	}
	if h.backupBatchCapture == nil {
		return backupIDs, operation.New(operation.KindConflict, "required backup batch authority is unavailable")
	}
	captures, captureErr := h.backupBatchCapture.CaptureBatch(ctx, requests)
	if len(captures) > len(requests) {
		captureErr = errors.Join(captureErr, operation.New(operation.KindConflict, "backup batch returned unexpected results"))
		captures = captures[:len(requests)]
	}
	verified := 0
	for captureIndex, captured := range captures {
		targetIndex := changedIndices[captureIndex]
		backupIDs[targetIndex] = captured.Manifest.BackupID
		if err := validateCapturedMutationBackup(captured, &prepared.targets[targetIndex], sourceOperation); err != nil {
			captureErr = errors.Join(captureErr, err)
			break
		}
		verified++
	}
	if verified != len(requests) {
		if captureErr == nil {
			captureErr = operation.New(operation.KindFilesystem, "required backup batch is incomplete")
		}
		return backupIDs, captureErr
	}
	if captureErr != nil {
		slog.Warn("backup manifests committed; continuing after a post-manifest batch-store error", "sourceOperation", sourceOperation, "backupCount", verified)
	}
	return backupIDs, nil
}
