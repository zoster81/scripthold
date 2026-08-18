package backupstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

// Capture durably installs or verifies an immutable object, then commits one
// immutable manifest. The derived index is refreshed only after the manifest is
// authoritative. A non-zero result may accompany an index-persistence error.
func (store *Store) Capture(ctx context.Context, request CaptureRequest) (CaptureResult, error) {
	return store.capture(ctx, request, -1, false)
}

func (store *Store) capture(ctx context.Context, request CaptureRequest, expectedSize int64, preReserved bool) (result CaptureResult, err error) {
	if store == nil {
		return CaptureResult{}, operation.New(operation.KindInvalidInput, "backup store is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	request, err = validateCaptureRequest(request)
	if err != nil {
		return CaptureResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return CaptureResult{}, operation.Wrap(operation.KindCancelled, "capture_backup", "", err)
	}

	lstatInfo, err := os.Lstat(request.TargetPath)
	if err != nil {
		return CaptureResult{}, sanitizedFilesystemError("backup target cannot be inspected", err)
	}
	if isLinkOrReparse(lstatInfo) || !lstatInfo.Mode().IsRegular() {
		return CaptureResult{}, operation.New(operation.KindInvalidInput, "backup target must be a real regular file")
	}
	if lstatInfo.Size() > store.limits.MaxObjectBytes {
		return CaptureResult{}, operation.New(operation.KindLimit, "backup target exceeds the maximum object size")
	}
	if expectedSize >= 0 && lstatInfo.Size() != expectedSize {
		return CaptureResult{}, operation.New(operation.KindConflict, "backup target size changed after batch preflight")
	}

	identity, err := filesystem.OpenFileIdentity(request.TargetPath)
	if err != nil {
		return CaptureResult{}, sanitizedFilesystemError("backup target identity cannot be retained", err)
	}
	defer func() {
		if closeErr := identity.Close(); closeErr != nil {
			err = errors.Join(err, sanitizedFilesystemError("backup target identity could not be released", closeErr))
		}
	}()
	currentInfo, err := os.Stat(request.TargetPath)
	if err != nil || !currentInfo.Mode().IsRegular() || !os.SameFile(lstatInfo, currentInfo) {
		return CaptureResult{}, operation.New(operation.KindConflict, "backup target identity changed before capture")
	}
	matches, err := identity.Matches(request.TargetPath)
	if err != nil || !matches {
		return CaptureResult{}, operation.New(operation.KindConflict, "backup target identity changed before capture")
	}

	if !preReserved {
		reservation, reserveErr := store.reserve(lstatInfo.Size(), request)
		if reserveErr != nil {
			return CaptureResult{}, reserveErr
		}
		defer store.release(reservation)
	}
	if err := store.validateIdentityAndLayout(); err != nil {
		return CaptureResult{}, err
	}

	stagedPath, stagedDigest, stagedSize, err := store.ops.stageTarget(store, ctx, request.TargetPath, lstatInfo.Size())
	if err != nil {
		return CaptureResult{}, err
	}
	defer func() {
		if stagedPath != "" {
			if removeErr := os.Remove(stagedPath); removeErr != nil && !os.IsNotExist(removeErr) {
				err = errors.Join(err, sanitizedFilesystemError("backup staging file could not be removed", removeErr))
			}
		}
	}()

	if err := ctx.Err(); err != nil {
		return CaptureResult{}, operation.Wrap(operation.KindCancelled, "capture_backup", "", err)
	}

	verified, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, request.TargetPath, store.limits.MaxObjectBytes)
	if err != nil {
		if operation.KindOf(err) == operation.KindConflict {
			return CaptureResult{}, operation.New(operation.KindConflict, "backup target changed during capture")
		}
		return CaptureResult{}, err
	}
	verifiedDigest, ok := verified.ContentDigest()
	if !ok || verified.Size != stagedSize || verifiedDigest != stagedDigest ||
		verified.Mode != lstatInfo.Mode() || !verified.ModTime.Equal(lstatInfo.ModTime()) {
		return CaptureResult{}, operation.New(operation.KindConflict, "backup target changed during capture")
	}
	matches, err = identity.Matches(request.TargetPath)
	if err != nil || !matches {
		return CaptureResult{}, operation.New(operation.KindConflict, "backup target identity changed during capture")
	}

	digestText := hex.EncodeToString(stagedDigest[:])
	fingerprint, err := filesystem.FingerprintRegularFileContentDigest(stagedSize, digestText)
	if err != nil {
		return CaptureResult{}, err
	}

	store.transactionMu.Lock()
	defer store.transactionMu.Unlock()
	durableStateChanged := false
	defer func() {
		if err != nil && durableStateChanged {
			err = errors.Join(err, store.refreshDerivedIndex(context.Background()))
		}
	}()
	if store.isClosed() {
		return CaptureResult{}, operation.New(operation.KindConflict, "backup store is closed")
	}
	if err := store.validateIdentityAndLayout(); err != nil {
		return CaptureResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return CaptureResult{}, operation.Wrap(operation.KindCancelled, "capture_backup", "", err)
	}
	objectCreated, installErr := store.installOrVerifyObject(ctx, stagedPath, digestText, stagedSize)
	if objectCreated {
		durableStateChanged = true
		stagedPath = ""
	}
	if installErr != nil {
		return CaptureResult{}, installErr
	}
	manifest, manifestInstalled, manifestErr := store.ops.commitManifest(store, Manifest{
		FormatVersion:      ManifestVersion,
		StoreFormatVersion: FormatVersion,
		StoreID:            store.descriptor.StoreID,
		CreatedAt:          utcTimestamp(time.Now()),
		TargetPath:         request.TargetPath,
		SourceOperation:    request.SourceOperation,
		ObjectAlgorithm:    ObjectAlgorithm,
		ObjectDigest:       digestText,
		ObjectBytes:        stagedSize,
		ContentFingerprint: fingerprint,
		OriginalMode:       uint32(verified.Mode.Perm()),
		OriginalModTime:    utcTimestamp(verified.ModTime),
		Label:              request.Label,
		Pinned:             request.Pinned,
	})
	if manifestInstalled {
		durableStateChanged = true
	}
	if manifestErr != nil {
		return CaptureResult{}, manifestErr
	}
	result = CaptureResult{Manifest: manifest, ObjectCreated: objectCreated}
	refreshErr := store.refreshDerivedIndex(ctx)
	if refreshErr != nil {
		return result, refreshErr
	}
	durableStateChanged = false
	return result, nil
}

func (store *Store) stageTarget(ctx context.Context, target string, expectedSize int64) (path string, digest [sha256.Size]byte, size int64, err error) {
	file, err := os.Open(target)
	if err != nil {
		return "", digest, 0, sanitizedFilesystemError("backup target cannot be opened", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != expectedSize {
		return "", digest, 0, operation.New(operation.KindConflict, "backup target metadata changed before capture")
	}

	staging, err := os.CreateTemp(filepath.Join(store.root, "staging"), ".capture-object-*.tmp")
	if err != nil {
		return "", digest, 0, sanitizedFilesystemError("backup staging file could not be created", err)
	}
	path = staging.Name()
	defer func() {
		if closeErr := staging.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			err = errors.Join(err, sanitizedFilesystemError("backup staging file could not be closed", closeErr))
		}
		if err != nil {
			_ = os.Remove(path)
			path = ""
			digest = [sha256.Size]byte{}
			size = 0
		}
	}()
	if err := restrictPathPermissions(path, false); err != nil {
		return "", digest, 0, sanitizedFilesystemError("backup staging permissions could not be restricted", err)
	}
	stagingInfo, err := staging.Stat()
	if err != nil {
		return "", digest, 0, sanitizedFilesystemError("backup staging file cannot be inspected", err)
	}
	if err := validateSingleLink(path, stagingInfo); err != nil {
		return "", digest, 0, operation.New(operation.KindFilesystem, "backup staging hard-link state is invalid")
	}

	hasher := sha256.New()
	buffer := make([]byte, 128*1024)
	remaining := expectedSize
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return "", digest, 0, operation.Wrap(operation.KindCancelled, "stage_backup_object", "", err)
		}
		readSize := int64(len(buffer))
		if remaining < readSize {
			readSize = remaining
		}
		read, readErr := io.ReadFull(file, buffer[:readSize])
		if read > 0 {
			if _, writeErr := staging.Write(buffer[:read]); writeErr != nil {
				return "", digest, 0, sanitizedFilesystemError("backup staging file could not be written", writeErr)
			}
			_, _ = hasher.Write(buffer[:read])
			size += int64(read)
			remaining -= int64(read)
		}
		if readErr != nil {
			return "", digest, 0, operation.New(operation.KindConflict, "backup target was truncated during capture")
		}
	}
	var extra [1]byte
	if read, readErr := file.Read(extra[:]); read != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
		return "", digest, 0, operation.New(operation.KindConflict, "backup target size changed during capture")
	}
	if err := staging.Sync(); err != nil {
		return "", digest, 0, sanitizedFilesystemError("backup staging file could not be synchronized", err)
	}
	if err := staging.Close(); err != nil {
		return "", digest, 0, sanitizedFilesystemError("backup staging file could not be closed", err)
	}
	copy(digest[:], hasher.Sum(nil))
	return path, digest, size, nil
}

func (store *Store) installOrVerifyObject(ctx context.Context, stagedPath, digest string, size int64) (bool, error) {
	destination := objectPath(store.root, digest)
	if info, err := os.Lstat(destination); err == nil {
		if verifyErr := verifyExistingObject(ctx, destination, info, digest, size); verifyErr != nil {
			return false, verifyErr
		}
		if removeErr := os.Remove(stagedPath); removeErr != nil {
			return false, sanitizedFilesystemError("deduplicated backup staging file could not be removed", removeErr)
		}
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, sanitizedFilesystemError("backup object cannot be inspected", err)
	}

	shard := filepath.Dir(destination)
	if err := ensureDirectory(shard); err != nil {
		return false, sanitizedFilesystemError("backup object shard could not be created", err)
	}
	if err := filesystem.MoveNoReplace(stagedPath, destination); err != nil {
		if operation.KindOf(err) == operation.KindConflict {
			info, statErr := os.Lstat(destination)
			if statErr != nil {
				return false, sanitizedFilesystemError("concurrent backup object cannot be inspected", statErr)
			}
			if verifyErr := verifyExistingObject(ctx, destination, info, digest, size); verifyErr != nil {
				return false, verifyErr
			}
			if removeErr := os.Remove(stagedPath); removeErr != nil {
				return false, sanitizedFilesystemError("deduplicated backup staging file could not be removed", removeErr)
			}
			return false, nil
		}
		return false, sanitizedFilesystemError("backup object could not be installed", err)
	}
	if err := restrictPathPermissions(destination, false); err != nil {
		return true, sanitizedFilesystemError("backup object permissions could not be restricted", err)
	}
	info, err := os.Lstat(destination)
	if err != nil {
		return true, sanitizedFilesystemError("backup object cannot be inspected after installation", err)
	}
	if err := verifyExistingObject(ctx, destination, info, digest, size); err != nil {
		return true, err
	}
	return true, nil
}

func verifyExistingObject(ctx context.Context, path string, info os.FileInfo, digest string, size int64) error {
	if info == nil || isLinkOrReparse(info) || !info.Mode().IsRegular() || info.Size() != size {
		return operation.New(operation.KindFilesystem, "backup object metadata is invalid")
	}
	if err := validateSingleLink(path, info); err != nil {
		return operation.New(operation.KindFilesystem, "backup object hard-link state is invalid")
	}
	if err := validatePathPermissions(path, false); err != nil {
		return sanitizedFilesystemError("backup object permissions are not owner-only", err)
	}
	actual, err := hashRegularFile(ctx, path, size)
	if err != nil {
		return err
	}
	if actual != digest {
		return operation.New(operation.KindFilesystem, "backup object digest does not match its identifier")
	}
	return nil
}

func (store *Store) commitManifest(template Manifest) (Manifest, bool, error) {
	for attempt := 0; attempt < 4; attempt++ {
		backupID, err := newBackupID()
		if err != nil {
			return Manifest{}, false, err
		}
		manifest := template
		manifest.BackupID = backupID
		manifest, err = finalizeManifestChecksum(manifest)
		if err != nil {
			return Manifest{}, false, err
		}
		if err := validateManifest(manifest, store.descriptor); err != nil {
			return Manifest{}, false, err
		}
		data, err := encodeManifest(manifest)
		if err != nil {
			return Manifest{}, false, err
		}
		staged, err := store.writeStagingData(".capture-manifest-*.tmp", data)
		if err != nil {
			return Manifest{}, false, err
		}
		destination := manifestPath(store.root, backupID)
		moveErr := filesystem.MoveNoReplace(staged, destination)
		if moveErr != nil {
			_ = os.Remove(staged)
			if operation.KindOf(moveErr) == operation.KindConflict {
				continue
			}
			return Manifest{}, false, sanitizedFilesystemError("backup manifest could not be installed", moveErr)
		}
		if err := restrictPathPermissions(destination, false); err != nil {
			return manifest, true, sanitizedFilesystemError("backup manifest permissions could not be restricted", err)
		}
		manifestInfo, err := os.Lstat(destination)
		if err != nil {
			return manifest, true, sanitizedFilesystemError("backup manifest cannot be inspected after installation", err)
		}
		if err := validateSingleLink(destination, manifestInfo); err != nil {
			return manifest, true, operation.New(operation.KindFilesystem, "backup manifest hard-link state is invalid")
		}
		return manifest, true, nil
	}
	return Manifest{}, false, operation.New(operation.KindConflict, "backup manifest identifier collision limit reached")
}

func (store *Store) writeStagingData(pattern string, data []byte) (path string, err error) {
	file, err := os.CreateTemp(filepath.Join(store.root, "staging"), pattern)
	if err != nil {
		return "", sanitizedFilesystemError("backup staging file could not be created", err)
	}
	path = file.Name()
	defer func() {
		if closeErr := file.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			err = errors.Join(err, sanitizedFilesystemError("backup staging file could not be closed", closeErr))
		}
		if err != nil {
			_ = os.Remove(path)
			path = ""
		}
	}()
	if err := restrictPathPermissions(path, false); err != nil {
		return "", sanitizedFilesystemError("backup staging permissions could not be restricted", err)
	}
	stagingInfo, err := file.Stat()
	if err != nil {
		return "", sanitizedFilesystemError("backup staging file cannot be inspected", err)
	}
	if err := validateSingleLink(path, stagingInfo); err != nil {
		return "", operation.New(operation.KindFilesystem, "backup staging hard-link state is invalid")
	}
	if err := writeAndSync(file, data); err != nil {
		return "", sanitizedFilesystemError("backup staging file could not be persisted", err)
	}
	if err := file.Close(); err != nil {
		return "", sanitizedFilesystemError("backup staging file could not be closed", err)
	}
	return path, nil
}

func (store *Store) reserve(bytes int64, request CaptureRequest) (reservation, error) {
	store.stateMu.Lock()
	defer store.stateMu.Unlock()
	if store.closed {
		return reservation{}, operation.New(operation.KindConflict, "backup store is closed")
	}
	if store.gcActive {
		return reservation{}, operation.New(operation.KindConflict, "backup capture is unavailable while GC is active")
	}
	if bytes < 0 || bytes > store.limits.MaxObjectBytes {
		return reservation{}, operation.New(operation.KindLimit, "backup object size exceeds the configured limit")
	}
	if store.index.TotalObjectBytes > store.limits.MaxTotalBytes-bytes-store.reservedBytes {
		return reservation{}, operation.New(operation.KindLimit, "backup total-byte quota is exhausted")
	}
	if store.index.ManifestCount+store.reservedManifests >= store.limits.MaxManifests {
		return reservation{}, operation.New(operation.KindLimit, "backup manifest quota is exhausted")
	}
	if request.Pinned && store.index.PinnedCount+store.reservedPinned >= store.limits.MaxPinned {
		return reservation{}, operation.New(operation.KindLimit, "backup pinned-manifest quota is exhausted")
	}
	reserved := reservation{bytes: bytes, manifests: 1}
	if request.Pinned {
		reserved.pinned = 1
	} else {
		targetCount := store.reservedTargets[request.TargetPath]
		for _, target := range store.index.Targets {
			if target.TargetPath == request.TargetPath {
				targetCount += target.ManifestCount - target.PinnedCount
				break
			}
		}
		if targetCount >= store.limits.MaxVersionsPerTarget {
			return reservation{}, operation.New(operation.KindLimit, "backup target-version quota is exhausted")
		}
		reserved.targetPath = request.TargetPath
	}
	store.reservedBytes += reserved.bytes
	store.reservedManifests += reserved.manifests
	store.reservedPinned += reserved.pinned
	if reserved.targetPath != "" {
		store.reservedTargets[reserved.targetPath]++
	}
	return reserved, nil
}

func (store *Store) release(reserved reservation) {
	store.stateMu.Lock()
	defer store.stateMu.Unlock()
	store.reservedBytes -= reserved.bytes
	store.reservedManifests -= reserved.manifests
	store.reservedPinned -= reserved.pinned
	if reserved.targetPath != "" {
		store.reservedTargets[reserved.targetPath]--
		if store.reservedTargets[reserved.targetPath] <= 0 {
			delete(store.reservedTargets, reserved.targetPath)
		}
	}
}

func (store *Store) refreshDerivedIndex(ctx context.Context) error {
	if err := store.validateIdentityAndLayout(); err != nil {
		return err
	}
	scan, err := scanStore(ctx, store.root, store.descriptor, scanOptions{
		mode:       AuditQuick,
		maxObjects: store.limits.MaxManifests,
		maxBytes:   store.limits.MaxTotalBytes,
		checkIndex: false,
	})
	if err != nil {
		return err
	}
	if structuralErr := firstStructuralIssue(scan.report); structuralErr != nil {
		return structuralErr
	}
	index := buildIndex(store.descriptor, scan.manifests, scan.objects)
	store.stateMu.Lock()
	store.index = index
	store.stateMu.Unlock()
	if err := store.ops.persistIndex(store.root, index); err != nil {
		return err
	}
	return nil
}
