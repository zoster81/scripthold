package backupstore

import (
	"context"
	"os"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

type recoveryFinalizeOps struct {
	beforeFinalSourceCheck func() error
	beforePromotionRename  func() error
	afterPromotion         func() error
	afterStatePromoted     func() error
	beforeReportInstall    func() error
	afterReport            func() error
}

// FinalizeRecoveryDestination revalidates the source plan, promotes only an
// already audited staging store, independently full-audits the promoted store,
// and installs or verifies the mandatory no-replace recovery report.
func (store *DiagnosticStore) FinalizeRecoveryDestination(
	ctx context.Context,
	destination *RecoveryDestination,
	plan RecoveryPlan,
	pretty bool,
) (RecoveryReport, error) {
	return store.finalizeRecoveryDestinationWithOps(ctx, destination, plan, pretty, recoveryFinalizeOps{})
}

func (store *DiagnosticStore) finalizeRecoveryDestinationWithOps(
	ctx context.Context,
	destination *RecoveryDestination,
	plan RecoveryPlan,
	pretty bool,
	ops recoveryFinalizeOps,
) (RecoveryReport, error) {
	if store == nil || destination == nil {
		return RecoveryReport{}, operation.New(operation.KindInvalidInput, "recovery finalization state is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := recoveryContextError(ctx, "finalize_backup_recovery"); err != nil {
		return RecoveryReport{}, err
	}
	if _, err := EncodeRecoveryPlan(plan, false); err != nil {
		return RecoveryReport{}, operation.Wrap(operation.KindInvalidInput, "finalize_backup_recovery", "", err)
	}
	if !plan.Applicable || !plan.CoverageComplete || !security.PathsEqual(destination.paths.source, store.root) ||
		destination.state.PlanID != plan.PlanID || destination.state.DestinationKey != destination.paths.destinationKey ||
		destination.state.DestinationStoreID == "" || destination.state.DestinationStoreID == plan.SourceStoreID {
		return RecoveryReport{}, operation.New(operation.KindConflict, "recovery finalization state does not match the reviewed plan")
	}
	if _, err := validateRecoveryPlanInputPath(destination.paths.plan, plan); err != nil {
		return RecoveryReport{}, err
	}
	if err := store.requireFreshRecoveryPlan(ctx, plan); err != nil {
		return RecoveryReport{}, err
	}

	if destination.promoted {
		return store.completePromotedRecoveryDestination(ctx, destination, plan, pretty, ops)
	}
	if destination.store == nil || destination.state.Phase != RecoveryPhaseAudited {
		return RecoveryReport{}, operation.New(operation.KindConflict, "only an audited recovery staging store can be promoted")
	}
	if _, err := store.RebuildAndAuditRecoveryDestination(ctx, destination, plan); err != nil {
		return RecoveryReport{}, err
	}
	if _, err := os.Lstat(destination.paths.report); err == nil {
		return RecoveryReport{}, operation.New(operation.KindConflict, "recovery report already exists before promotion")
	} else if !os.IsNotExist(err) {
		return RecoveryReport{}, sanitizedFilesystemError("recovery report cannot be inspected before promotion", err)
	}
	if ops.beforeFinalSourceCheck != nil {
		if err := ops.beforeFinalSourceCheck(); err != nil {
			return RecoveryReport{}, err
		}
	}
	if err := store.requireFreshRecoveryPlan(ctx, plan); err != nil {
		return RecoveryReport{}, err
	}
	if err := verifyRecoveryStateFile(destination); err != nil {
		return RecoveryReport{}, err
	}
	if err := destination.Close(); err != nil {
		return RecoveryReport{}, sanitizedFilesystemError("recovery staging lock could not be released before promotion", err)
	}
	if ops.beforePromotionRename != nil {
		if err := ops.beforePromotionRename(); err != nil {
			return RecoveryReport{}, err
		}
	}
	if _, err := os.Lstat(destination.paths.destination); err == nil {
		return RecoveryReport{}, operation.New(operation.KindConflict, "recovery destination appeared before promotion")
	} else if !os.IsNotExist(err) {
		return RecoveryReport{}, sanitizedFilesystemError("recovery destination cannot be inspected before promotion", err)
	}
	if err := filesystem.MoveNoReplace(destination.paths.staging, destination.paths.destination); err != nil {
		return RecoveryReport{}, sanitizedFilesystemError("audited recovery staging store could not be promoted", err)
	}
	destination.promoted = true
	if ops.afterPromotion != nil {
		if err := ops.afterPromotion(); err != nil {
			return RecoveryReport{}, err
		}
	}
	if err := transitionRecoveryState(ctx, destination, RecoveryPhasePromoted); err != nil {
		return RecoveryReport{}, err
	}
	if ops.afterStatePromoted != nil {
		if err := ops.afterStatePromoted(); err != nil {
			return RecoveryReport{}, err
		}
	}
	return store.completePromotedRecoveryDestination(ctx, destination, plan, pretty, ops)
}

func (store *DiagnosticStore) requireFreshRecoveryPlan(ctx context.Context, plan RecoveryPlan) error {
	if err := recoveryContextError(ctx, "revalidate_backup_recovery_source"); err != nil {
		return err
	}
	evidence, err := store.ScanRecoveryEvidence(ctx, plan.Bounds)
	if err != nil {
		return err
	}
	fresh, err := BuildRecoveryPlan(evidence)
	if err != nil {
		return operation.Wrap(operation.KindConflict, "revalidate_backup_recovery_source", "", err)
	}
	if fresh.PlanID != plan.PlanID {
		return operation.New(operation.KindConflict, "recovery source evidence changed after plan review")
	}
	return nil
}

func (store *DiagnosticStore) completePromotedRecoveryDestination(
	ctx context.Context,
	destination *RecoveryDestination,
	plan RecoveryPlan,
	pretty bool,
	ops recoveryFinalizeOps,
) (RecoveryReport, error) {
	if !destination.promoted || destination.store != nil {
		return RecoveryReport{}, operation.New(operation.KindConflict, "recovery destination is not in a promoted completion state")
	}
	if destination.state.Phase == RecoveryPhaseAudited {
		if err := transitionRecoveryState(ctx, destination, RecoveryPhasePromoted); err != nil {
			return RecoveryReport{}, err
		}
	} else if destination.state.Phase != RecoveryPhasePromoted {
		return RecoveryReport{}, operation.New(operation.KindConflict, "promoted recovery state phase is invalid")
	} else if err := verifyRecoveryStateFile(destination); err != nil {
		return RecoveryReport{}, err
	}

	audit, index, err := auditPromotedRecoveryDestination(ctx, destination, plan)
	if err != nil {
		return RecoveryReport{}, err
	}
	report := recoveryReportFromAudit(destination, plan, audit, index)
	if err := validateRecoveryReportAgainstPlan(report, plan, destination.state.DestinationStoreID); err != nil {
		return RecoveryReport{}, err
	}
	if ops.beforeReportInstall != nil {
		if err := ops.beforeReportInstall(); err != nil {
			return RecoveryReport{}, err
		}
	}
	installed, err := installOrVerifyRecoveryReport(ctx, destination.paths.report, report, pretty)
	if err != nil {
		return RecoveryReport{}, err
	}
	if installed != report {
		return RecoveryReport{}, operation.New(operation.KindConflict, "persisted recovery report does not match final audit evidence")
	}
	destination.completed = true
	if ops.afterReport != nil {
		if err := ops.afterReport(); err != nil {
			return report, err
		}
	}
	if err := store.validateIdentity(); err != nil {
		return report, err
	}
	if err := verifyRecoveryStateFile(destination); err != nil {
		return report, err
	}
	return report, nil
}

func auditPromotedRecoveryDestination(ctx context.Context, destination *RecoveryDestination, plan RecoveryPlan) (AuditReport, persistedIndex, error) {
	descriptor := inspectRecoveryDescriptor(destination.paths.destination)
	if !descriptor.valid || descriptor.descriptor.StoreID != destination.state.DestinationStoreID ||
		descriptor.descriptor.StoreID == plan.SourceStoreID {
		return AuditReport{}, persistedIndex{}, operation.New(operation.KindConflict, "promoted recovery destination descriptor is invalid")
	}
	diagnostic, err := OpenExistingForDiagnosis(DiagnosticOpenOptions{
		Directory: destination.paths.destination,
		Limits:    recoveryDestinationLimits(plan),
	})
	if err != nil {
		return AuditReport{}, persistedIndex{}, err
	}
	defer diagnostic.Close()

	maxEntries := plan.Bounds.MaxManifests
	if plan.Bounds.MaxObjects > maxEntries {
		maxEntries = plan.Bounds.MaxObjects
	}
	diagnostic.transactionMu.Lock()
	if err := diagnostic.validateIdentity(); err != nil {
		diagnostic.transactionMu.Unlock()
		return AuditReport{}, persistedIndex{}, err
	}
	scan, scanErr := scanStore(ctx, diagnostic.root, descriptor.descriptor, scanOptions{
		mode: AuditFull, maxObjects: maxEntries, maxBytes: plan.Bounds.MaxBytes, checkIndex: true,
	})
	diagnostic.transactionMu.Unlock()
	if scanErr != nil {
		return AuditReport{}, persistedIndex{}, scanErr
	}
	if err := validateRecoveryAuditReportAgainstPlan(scan.report, plan, true); err != nil {
		return scan.report, persistedIndex{}, err
	}
	index, err := loadIndex(destination.paths.destination, descriptor.descriptor)
	if err != nil {
		return scan.report, persistedIndex{}, sanitizedFilesystemError("promoted recovery index cannot be loaded", err)
	}
	if index.Generation != scan.report.Generation || index.ManifestCount != plan.DestinationManifestCount ||
		index.ObjectCount != plan.DestinationObjectCount || index.PinnedCount != plan.DestinationPinnedCount ||
		index.TotalObjectBytes != plan.DestinationBytes {
		return scan.report, persistedIndex{}, operation.New(operation.KindConflict, "promoted recovery index does not match the reviewed plan")
	}
	return scan.report, index, nil
}

func recoveryReportFromAudit(destination *RecoveryDestination, plan RecoveryPlan, audit AuditReport, index persistedIndex) RecoveryReport {
	status := RecoveryStatusRecovered
	if plan.HasOmissions {
		status = RecoveryStatusRecoveredWithOmissions
	}
	return RecoveryReport{
		FormatVersion:      RecoveryReportFormatVersion,
		PlanID:             plan.PlanID,
		DestinationStoreID: destination.state.DestinationStoreID,
		Status:             status,
		Generation:         audit.Generation,
		ManifestCount:      audit.ManifestCount,
		ObjectCount:        audit.ObjectCount,
		PinnedCount:        index.PinnedCount,
		TotalObjectBytes:   audit.ReferencedBytes,
		OmittedRecordCount: plan.RejectedRecordCount,
		FullAudit:          true,
		AuditIssueCount:    len(audit.Issues),
	}
}

func validateRecoveryReportAgainstPlan(report RecoveryReport, plan RecoveryPlan, destinationStoreID string) error {
	if report.PlanID != plan.PlanID || report.DestinationStoreID != destinationStoreID ||
		report.ManifestCount != plan.DestinationManifestCount || report.ObjectCount != plan.DestinationObjectCount ||
		report.PinnedCount != plan.DestinationPinnedCount || report.TotalObjectBytes != plan.DestinationBytes ||
		report.OmittedRecordCount != plan.RejectedRecordCount || !report.FullAudit || report.AuditIssueCount != 0 {
		return operation.New(operation.KindConflict, "recovery report does not match the reviewed plan")
	}
	expectedStatus := RecoveryStatusRecovered
	if plan.HasOmissions {
		expectedStatus = RecoveryStatusRecoveredWithOmissions
	}
	if report.Status != expectedStatus {
		return operation.New(operation.KindConflict, "recovery report omission status does not match the reviewed plan")
	}
	return nil
}

func installOrVerifyRecoveryReport(ctx context.Context, path string, report RecoveryReport, pretty bool) (RecoveryReport, error) {
	if err := recoveryContextError(ctx, "persist_backup_recovery_report"); err != nil {
		return RecoveryReport{}, err
	}
	if info, err := os.Lstat(path); err == nil {
		existing, readErr := readRecoveryReportFile(path, info)
		if readErr != nil || existing != report {
			return RecoveryReport{}, operation.New(operation.KindConflict, "existing recovery report is not exact completion evidence")
		}
		return existing, nil
	} else if !os.IsNotExist(err) {
		return RecoveryReport{}, sanitizedFilesystemError("recovery report cannot be inspected", err)
	}
	data, err := EncodeRecoveryReport(report, pretty)
	if err != nil {
		return RecoveryReport{}, operation.Wrap(operation.KindInvalidInput, "persist_backup_recovery_report", "", err)
	}
	clean, parent, err := validateRecoverySiblingPath(path, "recovery report")
	if err != nil {
		return RecoveryReport{}, err
	}
	stagedPath, stagedIdentity, err := writeRecoveryStagingData(parent, ".scripthold-recovery-report-*.tmp", data)
	if err != nil {
		return RecoveryReport{}, sanitizedFilesystemError("recovery report staging data could not be persisted", err)
	}
	defer closeRecoveryOwnedRegularFile(&stagedIdentity)
	removeStaged := true
	defer func() {
		if removeStaged {
			removeRecoveryRegularFileIfOwned(stagedPath, stagedIdentity)
		}
	}()
	if !recoveryOwnedRegularFileStable(stagedPath, stagedIdentity) || validateSingleLink(stagedPath, stagedIdentity.info) != nil ||
		validatePathPermissions(stagedPath, false) != nil {
		return RecoveryReport{}, operation.New(operation.KindConflict, "recovery report staging identity changed before installation")
	}
	if err := filesystem.MoveNoReplace(stagedPath, clean); err != nil {
		if info, statErr := os.Lstat(clean); statErr == nil {
			existing, readErr := readRecoveryReportFile(clean, info)
			if readErr == nil && existing == report {
				if !removeRecoveryRegularFileIfOwned(stagedPath, stagedIdentity) {
					return RecoveryReport{}, operation.New(operation.KindConflict, "recovery report staging identity changed after concurrent exact report installation")
				}
				removeStaged = false
				return existing, nil
			}
		}
		return RecoveryReport{}, sanitizedFilesystemError("recovery report could not be installed no-replace", err)
	}
	removeStaged = false
	info, err := os.Lstat(clean)
	if err != nil {
		return RecoveryReport{}, sanitizedFilesystemError("recovery report cannot be inspected after installation", err)
	}
	if info == nil || !recoveryOwnedRegularFileStable(clean, stagedIdentity) {
		return RecoveryReport{}, operation.New(operation.KindConflict, "recovery report is not the staged report that was verified")
	}
	existing, err := readRecoveryReportFile(clean, info)
	if err != nil || existing != report {
		return RecoveryReport{}, operation.New(operation.KindFilesystem, "recovery report could not be revalidated after installation")
	}
	return existing, nil
}

func readRecoveryReportFile(path string, expected os.FileInfo) (RecoveryReport, error) {
	if expected == nil || isLinkOrReparse(expected) || !expected.Mode().IsRegular() || expected.Size() < 0 || expected.Size() > maxRecoveryReportBytes {
		return RecoveryReport{}, operation.New(operation.KindConflict, "recovery report identity is invalid")
	}
	if err := validateSingleLink(path, expected); err != nil {
		return RecoveryReport{}, operation.New(operation.KindConflict, "recovery report hard-link state is invalid")
	}
	if err := validatePathPermissions(path, false); err != nil {
		return RecoveryReport{}, operation.New(operation.KindConflict, "recovery report permissions are invalid")
	}
	data, ok := readStableRecoveryRegularFile(path, expected, maxRecoveryReportBytes)
	if !ok || !recoveryFileIdentityStable(path, expected) {
		return RecoveryReport{}, operation.New(operation.KindConflict, "recovery report changed while it was read")
	}
	report, err := DecodeRecoveryReport(data)
	if err != nil {
		return RecoveryReport{}, operation.Wrap(operation.KindConflict, "read_backup_recovery_report", "", err)
	}
	return report, nil
}

func (store *DiagnosticStore) resumePromotedRecoveryDestination(
	ctx context.Context,
	paths recoveryApplyPaths,
	plan RecoveryPlan,
	destinationInfo os.FileInfo,
	stateInfo os.FileInfo,
	reportInfo os.FileInfo,
	reportExists bool,
) (*RecoveryDestination, error) {
	if err := recoveryContextError(ctx, "resume_promoted_backup_recovery"); err != nil {
		return nil, err
	}
	if destinationInfo == nil || isLinkOrReparse(destinationInfo) || !destinationInfo.IsDir() || validatePathPermissions(paths.destination, true) != nil {
		return nil, operation.New(operation.KindConflict, "promoted recovery destination identity is invalid")
	}
	if stateInfo == nil || isLinkOrReparse(stateInfo) || !stateInfo.Mode().IsRegular() ||
		validateSingleLink(paths.state, stateInfo) != nil || validatePathPermissions(paths.state, false) != nil {
		return nil, operation.New(operation.KindConflict, "promoted recovery state identity is invalid")
	}
	state, err := readRecoveryStateFile(paths.state, stateInfo)
	if err != nil {
		return nil, err
	}
	if state.PlanID != plan.PlanID || state.DestinationKey != paths.destinationKey ||
		(state.Phase != RecoveryPhaseAudited && state.Phase != RecoveryPhasePromoted) || state.DestinationStoreID == plan.SourceStoreID {
		return nil, operation.New(operation.KindConflict, "promoted recovery state does not match the requested plan")
	}
	descriptor := inspectRecoveryDescriptor(paths.destination)
	if !descriptor.valid || descriptor.descriptor.StoreID != state.DestinationStoreID {
		return nil, operation.New(operation.KindConflict, "promoted recovery destination descriptor does not match recovery state")
	}
	completed := false
	if reportExists {
		report, readErr := readRecoveryReportFile(paths.report, reportInfo)
		if readErr != nil || validateRecoveryReportAgainstPlan(report, plan, state.DestinationStoreID) != nil {
			return nil, operation.New(operation.KindConflict, "existing recovery report does not match promoted recovery state")
		}
		completed = true
	}
	if err := store.validateIdentity(); err != nil {
		return nil, err
	}
	return &RecoveryDestination{
		paths: paths, state: state, resumed: true, promoted: true, completed: completed,
	}, nil
}
