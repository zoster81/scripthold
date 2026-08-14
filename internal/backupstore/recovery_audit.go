package backupstore

import (
	"context"
	"os"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

// RebuildAndAuditRecoveryDestination rebuilds derived index state solely from
// recovered destination manifests/objects, performs a complete full audit, and
// durably marks the plan-bound recovery state audited only after every expected
// count and integrity invariant matches the reviewed plan.
func (store *DiagnosticStore) RebuildAndAuditRecoveryDestination(
	ctx context.Context,
	destination *RecoveryDestination,
	plan RecoveryPlan,
) (AuditReport, error) {
	if store == nil || destination == nil || destination.store == nil {
		return AuditReport{}, operation.New(operation.KindInvalidInput, "recovery audit state is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return AuditReport{}, operation.Wrap(operation.KindCancelled, "audit_backup_recovery_destination", "", err)
	}
	if _, err := EncodeRecoveryPlan(plan, false); err != nil {
		return AuditReport{}, operation.Wrap(operation.KindInvalidInput, "audit_backup_recovery_destination", "", err)
	}
	if !plan.Applicable || !plan.CoverageComplete {
		return AuditReport{}, operation.New(operation.KindInvalidInput, "recovery plan is not applicable")
	}
	if !security.PathsEqual(destination.paths.source, store.root) || destination.state.PlanID != plan.PlanID ||
		destination.state.DestinationKey != destination.paths.destinationKey ||
		destination.state.DestinationStoreID != destination.store.descriptor.StoreID ||
		destination.store.descriptor.StoreID == plan.SourceStoreID {
		return AuditReport{}, operation.New(operation.KindConflict, "recovery destination state does not match the reviewed plan")
	}
	if destination.state.Phase != RecoveryPhaseBuilding && destination.state.Phase != RecoveryPhaseAudited {
		return AuditReport{}, operation.New(operation.KindConflict, "recovery destination phase cannot be audited")
	}
	maxEntries := plan.Bounds.MaxManifests
	if plan.Bounds.MaxObjects > maxEntries {
		maxEntries = plan.Bounds.MaxObjects
	}
	if maxEntries <= 0 || plan.Bounds.MaxBytes <= 0 {
		return AuditReport{}, operation.New(operation.KindInvalidInput, "recovery audit bounds are invalid")
	}

	store.transactionMu.Lock()
	defer store.transactionMu.Unlock()
	destination.store.transactionMu.Lock()
	defer destination.store.transactionMu.Unlock()

	if err := store.validateIdentity(); err != nil {
		return AuditReport{}, err
	}
	if destination.store.isClosed() {
		return AuditReport{}, operation.New(operation.KindConflict, "recovery destination store is closed")
	}
	if err := destination.store.validateIdentityAndLayout(); err != nil {
		return AuditReport{}, err
	}

	quick, err := scanStore(ctx, destination.store.root, destination.store.descriptor, scanOptions{
		mode:       AuditQuick,
		maxObjects: maxEntries,
		maxBytes:   plan.Bounds.MaxBytes,
		checkIndex: false,
	})
	if err != nil {
		return AuditReport{}, err
	}
	if err := validateRecoveryAuditReportAgainstPlan(quick.report, plan, false); err != nil {
		return quick.report, err
	}
	index := buildIndex(destination.store.descriptor, quick.manifests, quick.objects)
	if index.ManifestCount != plan.DestinationManifestCount || index.ObjectCount != plan.DestinationObjectCount ||
		index.PinnedCount != plan.DestinationPinnedCount || index.TotalObjectBytes != plan.DestinationBytes {
		return quick.report, operation.New(operation.KindConflict, "recovery destination index projection does not match the reviewed plan")
	}
	if err := persistIndex(destination.store.root, index); err != nil {
		return quick.report, err
	}
	destination.store.stateMu.Lock()
	destination.store.index = index
	destination.store.stateMu.Unlock()

	full, err := scanStore(ctx, destination.store.root, destination.store.descriptor, scanOptions{
		mode:       AuditFull,
		maxObjects: maxEntries,
		maxBytes:   plan.Bounds.MaxBytes,
		checkIndex: true,
	})
	if err != nil {
		return AuditReport{}, err
	}
	if err := validateRecoveryAuditReportAgainstPlan(full.report, plan, true); err != nil {
		return full.report, err
	}
	if index.Generation != full.report.Generation {
		return full.report, operation.New(operation.KindConflict, "recovery destination generation changed during full audit")
	}
	persisted, loadErr := loadIndex(destination.store.root, destination.store.descriptor)
	if loadErr != nil || !indexesEquivalent(persisted, index) {
		return full.report, operation.New(operation.KindConflict, "recovery destination index changed during full audit")
	}
	if err := store.validateIdentity(); err != nil {
		return full.report, err
	}
	if err := destination.store.validateIdentityAndLayout(); err != nil {
		return full.report, err
	}
	if destination.state.Phase == RecoveryPhaseBuilding {
		if err := transitionRecoveryState(ctx, destination, RecoveryPhaseAudited); err != nil {
			return full.report, err
		}
	} else if err := verifyRecoveryStateFile(destination); err != nil {
		return full.report, err
	}
	return full.report, nil
}

func validateRecoveryAuditReportAgainstPlan(report AuditReport, plan RecoveryPlan, requireFull bool) error {
	if requireFull && report.Mode != AuditFull {
		return operation.New(operation.KindConflict, "recovery destination audit was not full")
	}
	if !requireFull && report.Mode != AuditQuick {
		return operation.New(operation.KindConflict, "recovery destination pre-audit mode is invalid")
	}
	if !report.Healthy || len(report.Issues) != 0 || report.ManifestCount != plan.DestinationManifestCount ||
		report.ObjectCount != plan.DestinationObjectCount || report.ReferencedBytes != plan.DestinationBytes ||
		report.OrphanObjectCount != 0 || report.OrphanObjectBytes != 0 || report.StagingEntryCount != 0 ||
		report.StagingEntryBytes != 0 || report.TrashEntryCount != 0 || report.TrashEntryBytes != 0 {
		return operation.New(operation.KindConflict, "recovery destination authoritative state does not match the reviewed plan")
	}
	if requireFull && !report.IndexConsistent {
		return operation.New(operation.KindConflict, "recovery destination index is not consistent after full audit")
	}
	return nil
}

func transitionRecoveryState(ctx context.Context, destination *RecoveryDestination, next RecoveryPhase) error {
	if destination == nil {
		return operation.New(operation.KindInvalidInput, "recovery state transition is unavailable")
	}
	if err := recoveryContextError(ctx, "transition_backup_recovery_state"); err != nil {
		return err
	}
	current := destination.state
	if current.Phase == next {
		return verifyRecoveryStateFile(destination)
	}
	allowed := (current.Phase == RecoveryPhaseBuilding && next == RecoveryPhaseAudited) ||
		(current.Phase == RecoveryPhaseAudited && next == RecoveryPhasePromoted)
	if !allowed {
		return operation.New(operation.KindConflict, "recovery state transition is invalid")
	}
	path := destination.paths.state
	info, err := os.Lstat(path)
	if err != nil {
		return sanitizedFilesystemError("recovery state cannot be inspected for transition", err)
	}
	if info == nil || isLinkOrReparse(info) || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxRecoveryStateBytes {
		return operation.New(operation.KindConflict, "recovery state identity is invalid before transition")
	}
	if err := validateSingleLink(path, info); err != nil {
		return operation.New(operation.KindConflict, "recovery state hard-link state is invalid before transition")
	}
	if err := validatePathPermissions(path, false); err != nil {
		return operation.New(operation.KindConflict, "recovery state permissions are invalid before transition")
	}
	data, ok := readStableRecoveryRegularFile(path, info, maxRecoveryStateBytes)
	if !ok || !recoveryFileIdentityStable(path, info) {
		return operation.New(operation.KindConflict, "recovery state changed before transition")
	}
	decoded, err := DecodeRecoveryState(data)
	if err != nil || decoded != current {
		return operation.New(operation.KindConflict, "recovery state content does not match the active recovery session")
	}
	snapshot, err := filesystem.CaptureSnapshotWithData(path, data)
	if err != nil {
		return sanitizedFilesystemError("recovery state snapshot could not be captured", err)
	}
	nextState := current
	nextState.Phase = next
	encoded, err := EncodeRecoveryState(nextState)
	if err != nil {
		return operation.Wrap(operation.KindInvalidInput, "transition_backup_recovery_state", "", err)
	}
	if err := filesystem.ReplaceFile(path, encoded, filesystem.ReplaceOptions{Mode: 0o600, Expected: &snapshot}); err != nil {
		return sanitizedFilesystemError("recovery state could not be transitioned", err)
	}
	if err := restrictPathPermissions(path, false); err != nil {
		return sanitizedFilesystemError("recovery state permissions could not be restricted after transition", err)
	}
	finalInfo, err := os.Lstat(path)
	if err != nil {
		return sanitizedFilesystemError("recovery state cannot be inspected after transition", err)
	}
	if isLinkOrReparse(finalInfo) || !finalInfo.Mode().IsRegular() || finalInfo.Size() != int64(len(encoded)) {
		return operation.New(operation.KindConflict, "recovery state identity is invalid after transition")
	}
	if err := validateSingleLink(path, finalInfo); err != nil {
		return operation.New(operation.KindConflict, "recovery state hard-link state is invalid after transition")
	}
	if err := validatePathPermissions(path, false); err != nil {
		return operation.New(operation.KindConflict, "recovery state permissions are invalid after transition")
	}
	readBack, err := readRecoveryStateFile(path, finalInfo)
	if err != nil || readBack != nextState {
		return operation.New(operation.KindFilesystem, "recovery state could not be revalidated after transition")
	}
	destination.state = nextState
	return nil
}

func verifyRecoveryStateFile(destination *RecoveryDestination) error {
	if destination == nil {
		return operation.New(operation.KindInvalidInput, "recovery state is unavailable")
	}
	info, err := os.Lstat(destination.paths.state)
	if err != nil {
		return sanitizedFilesystemError("recovery state cannot be inspected", err)
	}
	state, err := readRecoveryStateFile(destination.paths.state, info)
	if err != nil {
		return err
	}
	if state != destination.state {
		return operation.New(operation.KindConflict, "recovery state does not match the active recovery session")
	}
	return nil
}
