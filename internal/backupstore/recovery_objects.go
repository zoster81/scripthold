package backupstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

// RecoveryObjectResult reports path-free verified object reconstruction progress.
type RecoveryObjectResult struct {
	VerifiedObjectCount int   `json:"verifiedObjectCount"`
	CreatedObjectCount  int   `json:"createdObjectCount"`
	ReusedObjectCount   int   `json:"reusedObjectCount"`
	VerifiedBytes       int64 `json:"verifiedBytes"`
}

type recoveryObjectCopyItem struct {
	digest string
	bytes  int64
}

type recoveryObjectCopyOps struct {
	write         func(*os.File, []byte) (int, error)
	sync          func(*os.File) error
	beforeInstall func(string) error
}

// ReconstructRecoveryObjects streams every unique trusted object from the locked
// source through a complete digest/size verification before immutable no-replace
// installation or exact reuse in the staged destination.
func (store *DiagnosticStore) ReconstructRecoveryObjects(
	ctx context.Context,
	destination *RecoveryDestination,
	plan RecoveryPlan,
	evidence RecoveryEvidence,
) (RecoveryObjectResult, error) {
	return store.reconstructRecoveryObjectsWithOps(ctx, destination, plan, evidence, recoveryObjectCopyOps{})
}

func (store *DiagnosticStore) reconstructRecoveryObjectsWithOps(
	ctx context.Context,
	destination *RecoveryDestination,
	plan RecoveryPlan,
	evidence RecoveryEvidence,
	ops recoveryObjectCopyOps,
) (RecoveryObjectResult, error) {
	if store == nil || destination == nil || destination.store == nil {
		return RecoveryObjectResult{}, operation.New(operation.KindInvalidInput, "recovery object reconstruction state is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return RecoveryObjectResult{}, operation.Wrap(operation.KindCancelled, "reconstruct_backup_recovery_objects", "", err)
	}
	if _, err := EncodeRecoveryPlan(plan, false); err != nil {
		return RecoveryObjectResult{}, operation.Wrap(operation.KindInvalidInput, "reconstruct_backup_recovery_objects", "", err)
	}
	if !plan.Applicable || !plan.CoverageComplete {
		return RecoveryObjectResult{}, operation.New(operation.KindInvalidInput, "recovery plan is not applicable")
	}
	freshPlan, err := BuildRecoveryPlan(evidence)
	if err != nil {
		return RecoveryObjectResult{}, operation.Wrap(operation.KindInvalidInput, "reconstruct_backup_recovery_objects", "", err)
	}
	if freshPlan.PlanID != plan.PlanID {
		return RecoveryObjectResult{}, operation.New(operation.KindConflict, "recovery source evidence no longer matches the reviewed plan")
	}
	if !security.PathsEqual(destination.paths.source, store.root) || destination.state.PlanID != plan.PlanID ||
		destination.state.DestinationKey != destination.paths.destinationKey ||
		destination.state.DestinationStoreID != destination.store.descriptor.StoreID ||
		destination.store.descriptor.StoreID == plan.SourceStoreID {
		return RecoveryObjectResult{}, operation.New(operation.KindConflict, "recovery destination state does not match the reviewed plan")
	}
	if destination.state.Phase != RecoveryPhaseBuilding && destination.state.Phase != RecoveryPhaseAudited {
		return RecoveryObjectResult{}, operation.New(operation.KindConflict, "recovery destination phase cannot reconstruct objects")
	}

	items, err := recoveryPlanObjectItems(plan)
	if err != nil {
		return RecoveryObjectResult{}, operation.Wrap(operation.KindInvalidInput, "reconstruct_backup_recovery_objects", "", err)
	}
	ops = normalizeRecoveryObjectCopyOps(ops)

	store.transactionMu.Lock()
	defer store.transactionMu.Unlock()
	destination.store.transactionMu.Lock()
	defer destination.store.transactionMu.Unlock()

	if err := store.validateIdentity(); err != nil {
		return RecoveryObjectResult{}, err
	}
	if destination.store.isClosed() {
		return RecoveryObjectResult{}, operation.New(operation.KindConflict, "recovery destination store is closed")
	}
	if err := destination.store.validateIdentityAndLayout(); err != nil {
		return RecoveryObjectResult{}, err
	}

	result := RecoveryObjectResult{}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return result, operation.Wrap(operation.KindCancelled, "reconstruct_backup_recovery_objects", "", err)
		}
		if item.bytes < 0 || result.VerifiedBytes < 0 || item.bytes > plan.DestinationBytes-result.VerifiedBytes ||
			item.bytes > plan.Bounds.MaxBytes-result.VerifiedBytes {
			return result, operation.New(operation.KindLimit, "recovery object copy byte bound would be exceeded")
		}
		created, copyErr := store.copyRecoveryObject(ctx, destination.store, item, ops)
		if copyErr != nil {
			return result, copyErr
		}
		result.VerifiedObjectCount++
		if created {
			result.CreatedObjectCount++
		} else {
			result.ReusedObjectCount++
		}
		result.VerifiedBytes += item.bytes
	}
	if result.VerifiedObjectCount != plan.DestinationObjectCount || result.VerifiedBytes != plan.DestinationBytes ||
		result.CreatedObjectCount+result.ReusedObjectCount != result.VerifiedObjectCount {
		return result, operation.New(operation.KindConflict, "recovery object reconstruction did not match the reviewed plan")
	}
	if err := store.validateIdentity(); err != nil {
		return result, err
	}
	if err := destination.store.validateIdentityAndLayout(); err != nil {
		return result, err
	}
	return result, nil
}

func recoveryPlanObjectItems(plan RecoveryPlan) ([]recoveryObjectCopyItem, error) {
	byDigest := make(map[string]int64, plan.DestinationObjectCount)
	for _, action := range plan.Actions {
		if existing, ok := byDigest[action.ObjectDigest]; ok {
			if existing != action.ObjectBytes {
				return nil, errors.New("recovery plan object sizes are inconsistent")
			}
			continue
		}
		byDigest[action.ObjectDigest] = action.ObjectBytes
	}
	if len(byDigest) != plan.DestinationObjectCount {
		return nil, errors.New("recovery plan object count is inconsistent")
	}
	items := make([]recoveryObjectCopyItem, 0, len(byDigest))
	for digest, bytes := range byDigest {
		items = append(items, recoveryObjectCopyItem{digest: digest, bytes: bytes})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].digest < items[j].digest })
	return items, nil
}

func normalizeRecoveryObjectCopyOps(ops recoveryObjectCopyOps) recoveryObjectCopyOps {
	if ops.write == nil {
		ops.write = func(file *os.File, data []byte) (int, error) { return file.Write(data) }
	}
	if ops.sync == nil {
		ops.sync = func(file *os.File) error { return file.Sync() }
	}
	return ops
}

func (store *DiagnosticStore) copyRecoveryObject(
	ctx context.Context,
	destination *Store,
	item recoveryObjectCopyItem,
	ops recoveryObjectCopyOps,
) (created bool, err error) {
	if err := recoveryContextError(ctx, "copy_backup_recovery_object"); err != nil {
		return false, err
	}
	if !validHexIdentifier(item.digest) || item.bytes < 0 || item.bytes > hardMaxObjectBytes {
		return false, operation.New(operation.KindInvalidInput, "recovery object evidence is invalid")
	}
	if err := store.validateIdentity(); err != nil {
		return false, err
	}
	if err := destination.validateIdentityAndLayout(); err != nil {
		return false, err
	}

	sourcePath := objectPath(store.root, item.digest)
	sourceInfo, statErr := os.Lstat(sourcePath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return false, operation.New(operation.KindConflict, "recovery source object is missing")
		}
		return false, sanitizedFilesystemError("recovery source object cannot be inspected", statErr)
	}
	if sourceInfo == nil || isLinkOrReparse(sourceInfo) || !sourceInfo.Mode().IsRegular() || sourceInfo.Size() != item.bytes {
		return false, operation.New(operation.KindConflict, "recovery source object metadata changed")
	}
	if err := validateSingleLink(sourcePath, sourceInfo); err != nil {
		return false, operation.New(operation.KindConflict, "recovery source object hard-link state changed")
	}
	if err := validatePathPermissions(sourcePath, false); err != nil {
		return false, operation.New(operation.KindConflict, "recovery source object permissions changed")
	}

	source, openErr := os.Open(sourcePath)
	if openErr != nil {
		return false, sanitizedFilesystemError("recovery source object cannot be opened", openErr)
	}
	defer source.Close()
	openedInfo, statErr := source.Stat()
	if statErr != nil || openedInfo == nil || !openedInfo.Mode().IsRegular() || openedInfo.Size() != item.bytes ||
		!os.SameFile(sourceInfo, openedInfo) || !recoveryFileIdentityStable(sourcePath, sourceInfo) {
		return false, operation.New(operation.KindConflict, "recovery source object identity changed before copy")
	}

	stagingRoot := filepath.Join(destination.root, "staging")
	staged, createErr := os.CreateTemp(stagingRoot, ".recover-object-*.tmp")
	if createErr != nil {
		return false, sanitizedFilesystemError("recovery object staging file could not be created", createErr)
	}
	stagedPath := staged.Name()
	stagedIdentity, identityErr := captureRecoveryOwnedRegularFile(staged)
	if identityErr != nil {
		_ = staged.Close()
		return false, sanitizedFilesystemError("recovery object staging identity could not be captured", identityErr)
	}
	defer closeRecoveryOwnedRegularFile(&stagedIdentity)
	defer func() {
		if staged != nil {
			_ = staged.Close()
		}
		if err != nil {
			removeRecoveryObjectStageIfOwned(stagedPath, stagedIdentity)
		}
	}()
	if err := restrictPathPermissions(stagedPath, false); err != nil {
		return false, sanitizedFilesystemError("recovery object staging permissions could not be restricted", err)
	}
	if err := validateSingleLink(stagedPath, stagedIdentity.info); err != nil {
		return false, operation.New(operation.KindFilesystem, "recovery object staging hard-link state is invalid")
	}

	hasher := sha256.New()
	buffer := make([]byte, 128*1024)
	remaining := item.bytes
	var copied int64
	for remaining > 0 {
		if err := recoveryContextError(ctx, "copy_backup_recovery_object"); err != nil {
			return false, err
		}
		readSize := int64(len(buffer))
		if remaining < readSize {
			readSize = remaining
		}
		read, readErr := io.ReadFull(source, buffer[:readSize])
		if read > 0 {
			written, writeErr := ops.write(staged, buffer[:read])
			if writeErr != nil {
				return false, sanitizedFilesystemError("recovery object staging file could not be written", writeErr)
			}
			if written != read {
				return false, sanitizedFilesystemError("recovery object staging file write was incomplete", io.ErrShortWrite)
			}
			_, _ = hasher.Write(buffer[:read])
			copied += int64(read)
			remaining -= int64(read)
		}
		if readErr != nil {
			return false, operation.New(operation.KindConflict, "recovery source object was truncated during copy")
		}
	}
	var extra [1]byte
	if read, readErr := source.Read(extra[:]); read != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
		return false, operation.New(operation.KindConflict, "recovery source object size changed during copy")
	}
	if copied != item.bytes {
		return false, operation.New(operation.KindConflict, "recovery source object copy size is inconsistent")
	}
	actualDigest := hex.EncodeToString(hasher.Sum(nil))
	if actualDigest != item.digest {
		return false, operation.New(operation.KindConflict, "recovery source object digest changed during copy")
	}
	if !recoveryFileIdentityStable(sourcePath, sourceInfo) || validateSingleLink(sourcePath, sourceInfo) != nil ||
		validatePathPermissions(sourcePath, false) != nil {
		return false, operation.New(operation.KindConflict, "recovery source object identity changed during copy")
	}
	if err := ops.sync(staged); err != nil {
		return false, sanitizedFilesystemError("recovery object staging file could not be synchronized", err)
	}
	if err := staged.Close(); err != nil {
		staged = nil
		return false, sanitizedFilesystemError("recovery object staging file could not be closed", err)
	}
	staged = nil

	postWriteInfo, statErr := os.Lstat(stagedPath)
	if statErr != nil || postWriteInfo == nil || isLinkOrReparse(postWriteInfo) || !postWriteInfo.Mode().IsRegular() ||
		!recoveryOwnedRegularFileStable(stagedPath, stagedIdentity) || postWriteInfo.Size() != item.bytes {
		return false, operation.New(operation.KindConflict, "recovery object staging identity changed during copy")
	}
	stagedIdentity.info = postWriteInfo
	if err := validateSingleLink(stagedPath, stagedIdentity.info); err != nil {
		return false, operation.New(operation.KindFilesystem, "recovery object staging hard-link state changed")
	}
	if err := validatePathPermissions(stagedPath, false); err != nil {
		return false, sanitizedFilesystemError("recovery object staging permissions are not owner-only", err)
	}
	if err := store.validateIdentity(); err != nil {
		return false, err
	}
	if err := destination.validateIdentityAndLayout(); err != nil {
		return false, err
	}
	if ops.beforeInstall != nil {
		if err := ops.beforeInstall(item.digest); err != nil {
			return false, err
		}
	}

	created, installErr := installOrVerifyRecoveryObject(ctx, destination, stagedPath, stagedIdentity, item.digest, item.bytes)
	if installErr != nil {
		return created, installErr
	}
	stagedPath = ""
	if !recoveryFileIdentityStable(sourcePath, sourceInfo) || validateSingleLink(sourcePath, sourceInfo) != nil ||
		validatePathPermissions(sourcePath, false) != nil {
		return created, operation.New(operation.KindConflict, "recovery source object identity changed before object reconstruction completed")
	}
	if err := store.validateIdentity(); err != nil {
		return created, err
	}
	if err := destination.validateIdentityAndLayout(); err != nil {
		return created, err
	}
	return created, nil
}

func removeRecoveryObjectStageIfOwned(path string, expected recoveryOwnedRegularFile) {
	removeRecoveryRegularFileIfOwned(path, expected)
}

func installOrVerifyRecoveryObject(
	ctx context.Context,
	destination *Store,
	stagedPath string,
	stagedIdentity recoveryOwnedRegularFile,
	digest string,
	size int64,
) (bool, error) {
	if err := recoveryContextError(ctx, "install_backup_recovery_object"); err != nil {
		return false, err
	}
	if destination == nil || stagedPath == "" || stagedIdentity.info == nil {
		return false, operation.New(operation.KindInvalidInput, "recovery object staging identity is unavailable")
	}
	objectDestination := objectPath(destination.root, digest)
	if info, err := os.Lstat(objectDestination); err == nil {
		if verifyErr := verifyExistingObject(ctx, objectDestination, info, digest, size); verifyErr != nil {
			return false, verifyErr
		}
		if !removeRecoveryRegularFileIfOwned(stagedPath, stagedIdentity) {
			return false, operation.New(operation.KindConflict, "recovery object staging identity changed before deduplication cleanup")
		}
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, sanitizedFilesystemError("recovery destination object cannot be inspected", err)
	}

	if !recoveryOwnedRegularFileStable(stagedPath, stagedIdentity) || validateSingleLink(stagedPath, stagedIdentity.info) != nil ||
		validatePathPermissions(stagedPath, false) != nil {
		return false, operation.New(operation.KindConflict, "recovery object staging identity changed before installation")
	}
	shard := filepath.Dir(objectDestination)
	if err := ensureDirectory(shard); err != nil {
		return false, sanitizedFilesystemError("recovery destination object shard could not be created", err)
	}
	if err := filesystem.MoveNoReplace(stagedPath, objectDestination); err != nil {
		if operation.KindOf(err) != operation.KindConflict {
			return false, sanitizedFilesystemError("recovery destination object could not be installed", err)
		}
		info, statErr := os.Lstat(objectDestination)
		if statErr != nil {
			return false, sanitizedFilesystemError("concurrent recovery destination object cannot be inspected", statErr)
		}
		if verifyErr := verifyExistingObject(ctx, objectDestination, info, digest, size); verifyErr != nil {
			return false, verifyErr
		}
		if !removeRecoveryRegularFileIfOwned(stagedPath, stagedIdentity) {
			return false, operation.New(operation.KindConflict, "recovery object staging identity changed after concurrent deduplication")
		}
		return false, nil
	}
	info, err := os.Lstat(objectDestination)
	if err != nil {
		return true, sanitizedFilesystemError("recovery destination object cannot be inspected after installation", err)
	}
	if info == nil || !recoveryOwnedRegularFileStable(objectDestination, stagedIdentity) {
		return true, operation.New(operation.KindConflict, "recovery destination object is not the staged object that was verified")
	}
	if err := restrictPathPermissions(objectDestination, false); err != nil {
		return true, sanitizedFilesystemError("recovery destination object permissions could not be restricted", err)
	}
	info, err = os.Lstat(objectDestination)
	if err != nil {
		return true, sanitizedFilesystemError("recovery destination object cannot be re-inspected after installation", err)
	}
	if err := verifyExistingObject(ctx, objectDestination, info, digest, size); err != nil {
		return true, err
	}
	return true, nil
}
