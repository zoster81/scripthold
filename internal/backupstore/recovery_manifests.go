package backupstore

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

// RecoveryManifestResult reports path-free verified manifest reconstruction progress.
type RecoveryManifestResult struct {
	VerifiedManifestCount int `json:"verifiedManifestCount"`
	CreatedManifestCount  int `json:"createdManifestCount"`
	ReusedManifestCount   int `json:"reusedManifestCount"`
}

type recoveryVerifiedObjectIdentity struct {
	path string
	info fs.FileInfo
}

// ReconstructRecoveryManifests re-reads every accepted source manifest, preserves
// its logical backup identity and metadata, substitutes only the fresh destination
// StoreID, recomputes its checksum, and installs it no-replace after verifying the
// referenced destination object.
func (store *DiagnosticStore) ReconstructRecoveryManifests(
	ctx context.Context,
	destination *RecoveryDestination,
	plan RecoveryPlan,
	evidence RecoveryEvidence,
) (RecoveryManifestResult, error) {
	if store == nil || destination == nil || destination.store == nil {
		return RecoveryManifestResult{}, operation.New(operation.KindInvalidInput, "recovery manifest reconstruction state is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return RecoveryManifestResult{}, operation.Wrap(operation.KindCancelled, "reconstruct_backup_recovery_manifests", "", err)
	}
	if _, err := EncodeRecoveryPlan(plan, false); err != nil {
		return RecoveryManifestResult{}, operation.Wrap(operation.KindInvalidInput, "reconstruct_backup_recovery_manifests", "", err)
	}
	if !plan.Applicable || !plan.CoverageComplete {
		return RecoveryManifestResult{}, operation.New(operation.KindInvalidInput, "recovery plan is not applicable")
	}
	freshPlan, err := BuildRecoveryPlan(evidence)
	if err != nil {
		return RecoveryManifestResult{}, operation.Wrap(operation.KindInvalidInput, "reconstruct_backup_recovery_manifests", "", err)
	}
	if freshPlan.PlanID != plan.PlanID {
		return RecoveryManifestResult{}, operation.New(operation.KindConflict, "recovery source evidence no longer matches the reviewed plan")
	}
	if !security.PathsEqual(destination.paths.source, store.root) || destination.state.PlanID != plan.PlanID ||
		destination.state.DestinationKey != destination.paths.destinationKey ||
		destination.state.DestinationStoreID != destination.store.descriptor.StoreID ||
		destination.store.descriptor.StoreID == plan.SourceStoreID {
		return RecoveryManifestResult{}, operation.New(operation.KindConflict, "recovery destination state does not match the reviewed plan")
	}
	if destination.state.Phase != RecoveryPhaseBuilding && destination.state.Phase != RecoveryPhaseAudited {
		return RecoveryManifestResult{}, operation.New(operation.KindConflict, "recovery destination phase cannot reconstruct manifests")
	}

	actions := append([]RecoveryAction(nil), plan.Actions...)
	sort.Slice(actions, func(i, j int) bool { return actions[i].BackupID < actions[j].BackupID })

	store.transactionMu.Lock()
	defer store.transactionMu.Unlock()
	destination.store.transactionMu.Lock()
	defer destination.store.transactionMu.Unlock()

	if err := store.validateIdentity(); err != nil {
		return RecoveryManifestResult{}, err
	}
	if destination.store.isClosed() {
		return RecoveryManifestResult{}, operation.New(operation.KindConflict, "recovery destination store is closed")
	}
	if err := destination.store.validateIdentityAndLayout(); err != nil {
		return RecoveryManifestResult{}, err
	}
	descriptor := inspectRecoveryDescriptor(store.root)
	if !descriptor.valid || descriptor.descriptor.StoreID != plan.SourceStoreID ||
		descriptor.descriptor.FormatVersion != plan.SourceFormatVersion || descriptor.fingerprint != plan.DescriptorFingerprint {
		return RecoveryManifestResult{}, operation.New(operation.KindConflict, "recovery source descriptor no longer matches the reviewed plan")
	}

	verifiedObjects := make(map[string]recoveryVerifiedObjectIdentity, plan.DestinationObjectCount)
	result := RecoveryManifestResult{}
	for _, action := range actions {
		if err := recoveryContextError(ctx, "reconstruct_backup_recovery_manifests"); err != nil {
			return result, err
		}
		objectIdentity, ok := verifiedObjects[action.ObjectDigest]
		if !ok {
			objectPathValue := objectPath(destination.store.root, action.ObjectDigest)
			objectInfo, statErr := os.Lstat(objectPathValue)
			if statErr != nil {
				return result, operation.New(operation.KindConflict, "recovery destination object required by a manifest is missing")
			}
			if verifyErr := verifyExistingObject(ctx, objectPathValue, objectInfo, action.ObjectDigest, action.ObjectBytes); verifyErr != nil {
				return result, operation.Wrap(operation.KindConflict, "reconstruct_backup_recovery_manifests", "", errors.New("recovery destination object is not trustworthy"))
			}
			verifiedObjects[action.ObjectDigest] = recoveryVerifiedObjectIdentity{path: objectPathValue, info: objectInfo}
		} else if !recoveryFileIdentityStable(objectIdentity.path, objectIdentity.info) ||
			validateSingleLink(objectIdentity.path, objectIdentity.info) != nil || validatePathPermissions(objectIdentity.path, false) != nil {
			return result, operation.New(operation.KindConflict, "recovery destination object identity changed during manifest reconstruction")
		}

		sourcePath := manifestPath(store.root, action.BackupID)
		sourceInfo, statErr := os.Lstat(sourcePath)
		if statErr != nil {
			return result, operation.New(operation.KindConflict, "recovery source manifest is missing")
		}
		sourceManifest, readErr := readRecoveryManifestStrict(sourcePath, sourceInfo, descriptor.descriptor)
		if readErr != nil {
			return result, operation.New(operation.KindConflict, "recovery source manifest is no longer trustworthy")
		}
		if sourceManifest.BackupID != action.BackupID || sourceManifest.ManifestChecksum != action.ManifestChecksum ||
			sourceManifest.ObjectDigest != action.ObjectDigest || sourceManifest.ObjectBytes != action.ObjectBytes {
			return result, operation.New(operation.KindConflict, "recovery source manifest no longer matches the reviewed plan")
		}

		recovered := sourceManifest
		recovered.StoreID = destination.store.descriptor.StoreID
		recovered.ManifestChecksum = ""
		recovered, err = finalizeManifestChecksum(recovered)
		if err != nil {
			return result, err
		}
		if err := validateManifest(recovered, destination.store.descriptor); err != nil {
			return result, operation.Wrap(operation.KindConflict, "reconstruct_backup_recovery_manifests", "", errors.New("recovery manifest cannot be represented exactly in the destination store"))
		}

		created, installErr := installOrVerifyRecoveryManifest(ctx, destination.store, recovered)
		if installErr != nil {
			return result, installErr
		}
		if !recoveryFileIdentityStable(sourcePath, sourceInfo) || validateSingleLink(sourcePath, sourceInfo) != nil ||
			validatePathPermissions(sourcePath, false) != nil {
			return result, operation.New(operation.KindConflict, "recovery source manifest identity changed before reconstruction completed")
		}
		result.VerifiedManifestCount++
		if created {
			result.CreatedManifestCount++
		} else {
			result.ReusedManifestCount++
		}
	}

	if result.VerifiedManifestCount != plan.DestinationManifestCount ||
		result.CreatedManifestCount+result.ReusedManifestCount != result.VerifiedManifestCount {
		return result, operation.New(operation.KindConflict, "recovery manifest reconstruction did not match the reviewed plan")
	}
	for _, objectIdentity := range verifiedObjects {
		if !recoveryFileIdentityStable(objectIdentity.path, objectIdentity.info) || validateSingleLink(objectIdentity.path, objectIdentity.info) != nil ||
			validatePathPermissions(objectIdentity.path, false) != nil {
			return result, operation.New(operation.KindConflict, "recovery destination object identity changed before manifest reconstruction completed")
		}
	}
	if err := store.validateIdentity(); err != nil {
		return result, err
	}
	if err := destination.store.validateIdentityAndLayout(); err != nil {
		return result, err
	}
	return result, nil
}

func readRecoveryManifestStrict(path string, expected fs.FileInfo, descriptor Descriptor) (Manifest, error) {
	if expected == nil || isLinkOrReparse(expected) || !expected.Mode().IsRegular() || expected.Size() < 0 || expected.Size() > maxManifestBytes {
		return Manifest{}, operation.New(operation.KindFilesystem, "recovery manifest metadata is invalid")
	}
	if err := validateSingleLink(path, expected); err != nil {
		return Manifest{}, operation.New(operation.KindFilesystem, "recovery manifest hard-link state is invalid")
	}
	if err := validatePathPermissions(path, false); err != nil {
		return Manifest{}, operation.New(operation.KindFilesystem, "recovery manifest permissions are invalid")
	}
	data, ok := readStableRecoveryRegularFile(path, expected, maxManifestBytes)
	if !ok || !recoveryFileIdentityStable(path, expected) {
		return Manifest{}, operation.New(operation.KindConflict, "recovery manifest identity changed while it was read")
	}
	var manifest Manifest
	if err := decodeStrictRecoveryJSON(data, maxManifestBytes, &manifest); err != nil {
		return Manifest{}, err
	}
	if err := validateManifest(manifest, descriptor); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func installOrVerifyRecoveryManifest(ctx context.Context, destination *Store, manifest Manifest) (bool, error) {
	if err := recoveryContextError(ctx, "install_backup_recovery_manifest"); err != nil {
		return false, err
	}
	path := manifestPath(destination.root, manifest.BackupID)
	if info, err := os.Lstat(path); err == nil {
		existing, readErr := readRecoveryManifestStrict(path, info, destination.descriptor)
		if readErr != nil || existing != manifest {
			return false, operation.New(operation.KindConflict, "existing recovery destination manifest is not an exact reusable record")
		}
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, sanitizedFilesystemError("recovery destination manifest cannot be inspected", err)
	}

	data, err := encodeManifest(manifest)
	if err != nil {
		return false, err
	}
	staged, stagedIdentity, err := writeRecoveryStagingData(
		filepath.Join(destination.root, "staging"),
		".recover-manifest-*.tmp",
		data,
	)
	if err != nil {
		return false, sanitizedFilesystemError("recovery manifest staging data could not be persisted", err)
	}
	defer closeRecoveryOwnedRegularFile(&stagedIdentity)
	removeStaged := true
	defer func() {
		if removeStaged {
			removeRecoveryRegularFileIfOwned(staged, stagedIdentity)
		}
	}()
	if err := recoveryContextError(ctx, "install_backup_recovery_manifest"); err != nil {
		return false, err
	}
	if !recoveryOwnedRegularFileStable(staged, stagedIdentity) || validateSingleLink(staged, stagedIdentity.info) != nil ||
		validatePathPermissions(staged, false) != nil {
		return false, operation.New(operation.KindConflict, "recovery manifest staging identity changed before installation")
	}
	moveErr := filesystem.MoveNoReplace(staged, path)
	if moveErr != nil {
		if operation.KindOf(moveErr) != operation.KindConflict {
			return false, sanitizedFilesystemError("recovery destination manifest could not be installed", moveErr)
		}
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return false, sanitizedFilesystemError("concurrent recovery destination manifest cannot be inspected", statErr)
		}
		existing, readErr := readRecoveryManifestStrict(path, info, destination.descriptor)
		if readErr != nil || existing != manifest {
			return false, operation.New(operation.KindConflict, "concurrent recovery destination manifest is not an exact reusable record")
		}
		if !removeRecoveryRegularFileIfOwned(staged, stagedIdentity) {
			return false, operation.New(operation.KindConflict, "recovery manifest staging identity changed after concurrent deduplication")
		}
		removeStaged = false
		return false, nil
	}
	removeStaged = false
	if err := restrictPathPermissions(path, false); err != nil {
		return true, sanitizedFilesystemError("recovery destination manifest permissions could not be restricted", err)
	}
	info, statErr := os.Lstat(path)
	if statErr != nil {
		return true, sanitizedFilesystemError("recovery destination manifest cannot be inspected after installation", statErr)
	}
	if info == nil || !recoveryOwnedRegularFileStable(path, stagedIdentity) {
		return true, operation.New(operation.KindConflict, "recovery destination manifest is not the staged manifest that was verified")
	}
	existing, readErr := readRecoveryManifestStrict(path, info, destination.descriptor)
	if readErr != nil || existing != manifest {
		return true, operation.New(operation.KindFilesystem, "recovery destination manifest could not be revalidated after installation")
	}
	return true, nil
}
