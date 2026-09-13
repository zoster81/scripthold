package backupstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

type preparedCaptureRequest struct {
	request CaptureRequest
	size    int64
}

// PreflightCaptureBatch validates a bounded capture set and verifies that its
// conservative aggregate reservation can be admitted without changing store
// state. Identical future bytes are still charged at their full source sizes.
func (store *Store) PreflightCaptureBatch(ctx context.Context, requests []CaptureRequest) error {
	prepared, err := store.prepareCaptureBatch(ctx, requests)
	if err != nil {
		return err
	}
	if err := store.validateIdentityAndLayout(); err != nil {
		return err
	}
	store.stateMu.Lock()
	defer store.stateMu.Unlock()
	_, err = store.planBatchReservationsLocked(prepared, false)
	return err
}

// CaptureBatch reserves the complete conservative package budget atomically,
// then captures manifests in request order. A returned prefix contains every
// manifest that became durable before an error; those manifests are never
// removed implicitly.
func (store *Store) CaptureBatch(ctx context.Context, requests []CaptureRequest) (results []CaptureResult, err error) {
	defer func() {
		if len(results) == 0 {
			return
		}
		// Objects and manifests are already durable at this point. Persist the
		// latest derived projection once for the complete batch or durable prefix;
		// startup can still rebuild it if this best-effort cache write fails.
		err = errors.Join(err, store.persistCurrentDerivedIndex())
	}()
	prepared, err := store.prepareCaptureBatch(ctx, requests)
	if err != nil {
		return nil, err
	}
	if err := store.validateIdentityAndLayout(); err != nil {
		return nil, err
	}
	store.stateMu.Lock()
	reservations, err := store.planBatchReservationsLocked(prepared, true)
	store.stateMu.Unlock()
	if err != nil {
		return nil, err
	}
	released := make([]bool, len(reservations))
	defer func() {
		for index := range reservations {
			if !released[index] {
				store.release(reservations[index])
			}
		}
	}()

	var derivedErrors []error
	for index := range prepared {
		result, captureErr := store.capture(ctx, prepared[index].request, prepared[index].size, true, true)
		store.release(reservations[index])
		released[index] = true
		if result.Manifest.BackupID != "" {
			results = append(results, result)
			if captureErr != nil {
				derivedErrors = append(derivedErrors, captureErr)
			}
			continue
		}
		if captureErr == nil {
			captureErr = operation.New(operation.KindFilesystem, "batch capture did not commit a manifest")
		}
		return results, errors.Join(captureErr, errors.Join(derivedErrors...))
	}
	return results, errors.Join(derivedErrors...)
}

func (store *Store) prepareCaptureBatch(ctx context.Context, requests []CaptureRequest) ([]preparedCaptureRequest, error) {
	if store == nil {
		return nil, operation.New(operation.KindInvalidInput, "backup store is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if len(requests) == 0 {
		return nil, operation.New(operation.KindInvalidInput, "backup capture batch must not be empty")
	}
	if len(requests) > store.limits.MaxManifests {
		return nil, operation.New(operation.KindLimit, "backup capture batch exceeds the manifest limit")
	}
	prepared := make([]preparedCaptureRequest, len(requests))
	seen := make(map[string]struct{}, len(requests))
	for index, request := range requests {
		if err := ctx.Err(); err != nil {
			return nil, operation.Wrap(operation.KindCancelled, "preflight_backup_batch", "", err)
		}
		normalized, err := validateCaptureRequest(request)
		if err != nil {
			return nil, err
		}
		key := batchTargetKey(normalized.TargetPath)
		if _, duplicate := seen[key]; duplicate {
			return nil, operation.New(operation.KindInvalidInput, "backup capture batch contains duplicate target paths")
		}
		seen[key] = struct{}{}
		info, err := os.Lstat(normalized.TargetPath)
		if err != nil {
			return nil, sanitizedFilesystemError("backup target cannot be inspected", err)
		}
		if isLinkOrReparse(info) || !info.Mode().IsRegular() {
			return nil, operation.New(operation.KindInvalidInput, "backup target must be a real regular file")
		}
		if info.Size() < 0 || info.Size() > store.limits.MaxObjectBytes {
			return nil, operation.New(operation.KindLimit, "backup target exceeds the maximum object size")
		}
		prepared[index] = preparedCaptureRequest{request: normalized, size: info.Size()}
	}
	return prepared, nil
}

func (store *Store) planBatchReservationsLocked(prepared []preparedCaptureRequest, commit bool) ([]reservation, error) {
	if store.closed {
		return nil, operation.New(operation.KindConflict, "backup store is closed")
	}
	if store.gcActive {
		return nil, operation.New(operation.KindConflict, "backup capture is unavailable while GC is active")
	}
	var batchBytes int64
	batchManifests := 0
	batchPinned := 0
	batchTargets := make(map[string]int, len(prepared))
	reservations := make([]reservation, len(prepared))
	for index, item := range prepared {
		if !addNonNegativeInt64(&batchBytes, item.size) {
			return nil, operation.New(operation.KindLimit, "backup batch byte reservation exceeds supported range")
		}
		batchManifests++
		reserved := reservation{bytes: item.size, manifests: 1}
		if item.request.Pinned {
			batchPinned++
			reserved.pinned = 1
		} else {
			batchTargets[item.request.TargetPath]++
			reserved.targetPath = item.request.TargetPath
		}
		reservations[index] = reserved
	}
	if store.index.TotalObjectBytes < 0 || store.reservedBytes < 0 ||
		batchBytes > store.limits.MaxTotalBytes-store.index.TotalObjectBytes-store.reservedBytes {
		return nil, operation.New(operation.KindLimit, "backup total-byte quota is exhausted")
	}
	if batchManifests > store.limits.MaxManifests-store.index.ManifestCount-store.reservedManifests {
		return nil, operation.New(operation.KindLimit, "backup manifest quota is exhausted")
	}
	if batchPinned > store.limits.MaxPinned-store.index.PinnedCount-store.reservedPinned {
		return nil, operation.New(operation.KindLimit, "backup pinned-manifest quota is exhausted")
	}
	// Per-target version pressure is resolved after each durable manifest by
	// deterministic FIFO retention. It must not reject read-only preflight or
	// package admission merely because the target already reached retention.
	if !commit {
		return reservations, nil
	}
	store.reservedBytes += batchBytes
	store.reservedManifests += batchManifests
	store.reservedPinned += batchPinned
	for targetPath, count := range batchTargets {
		store.reservedTargets[targetPath] += count
	}
	return reservations, nil
}

func batchTargetKey(path string) string {
	clean := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}
	return clean
}
