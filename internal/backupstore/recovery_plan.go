package backupstore

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

// BuildRecoveryPlan converts one recovery evidence snapshot into a deterministic,
// path-free persisted plan. The resulting plan records evidence; it grants no
// authority to apply without a fresh matching source scan.
func BuildRecoveryPlan(evidence RecoveryEvidence) (RecoveryPlan, error) {
	actions := make([]RecoveryAction, 0, len(evidence.TrustedRecords))
	destinationPinnedCount := 0
	for _, record := range evidence.TrustedRecords {
		manifest := record.Manifest
		if manifest.Pinned {
			destinationPinnedCount++
		}
		actions = append(actions, RecoveryAction{
			BackupID:         manifest.BackupID,
			ManifestChecksum: manifest.ManifestChecksum,
			ObjectDigest:     manifest.ObjectDigest,
			ObjectBytes:      manifest.ObjectBytes,
		})
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].BackupID < actions[j].BackupID })

	rejected := append([]RecoveryRejectedRecord(nil), evidence.RejectedRecords...)
	sort.Slice(rejected, func(i, j int) bool { return rejected[i].BackupID < rejected[j].BackupID })
	reasonCounts := append([]RecoveryReasonCount(nil), evidence.RejectedReasonCounts...)
	sort.Slice(reasonCounts, func(i, j int) bool { return reasonCounts[i].Reason < reasonCounts[j].Reason })
	warnings := append([]string(nil), evidence.WarningCodes...)
	sort.Strings(warnings)

	plan := RecoveryPlan{
		FormatVersion:    RecoveryPlanFormatVersion,
		EvidenceDigest:   evidence.EvidenceDigest,
		Bounds:           evidence.Bounds,
		CoverageComplete: evidence.CoverageComplete,

		TrustedRecordCount: len(actions),
		TrustedObjectCount: evidence.TrustedObjectCount,
		TrustedBytes:       evidence.TrustedBytes,

		DestinationManifestCount: len(actions),
		DestinationObjectCount:   evidence.TrustedObjectCount,
		DestinationBytes:         evidence.TrustedBytes,
		DestinationPinnedCount:   destinationPinnedCount,

		Actions:              actions,
		RejectedRecords:      rejected,
		RejectedReasonCounts: reasonCounts,

		RejectedRecordCount:    evidence.RejectedRecordCount,
		OrphanObjectCount:      evidence.OrphanObjectCount,
		OrphanObjectBytes:      evidence.OrphanObjectBytes,
		UnknownEntryCount:      evidence.UnknownEntryCount,
		ResidueEntryCount:      evidence.ResidueEntryCount,
		ResidueEntryBytes:      evidence.ResidueEntryBytes,
		DerivedStateIssueCount: evidence.DerivedStateIssueCount,
		WarningCodes:           warnings,

		Applicable:   evidence.DescriptorValid && evidence.CoverageComplete,
		HasOmissions: evidence.RejectedRecordCount > 0,
	}
	if evidence.DescriptorValid {
		plan.SourceStoreID = evidence.SourceDescriptor.StoreID
		plan.SourceFormatVersion = evidence.SourceDescriptor.FormatVersion
		plan.DescriptorFingerprint = evidence.DescriptorFingerprint
	}
	return FinalizeRecoveryPlan(plan)
}

// CreateRecoveryPlan scans the retained existing source and builds the matching
// deterministic review plan while the existing-source lock remains held.
func (store *DiagnosticStore) CreateRecoveryPlan(ctx context.Context, bounds RecoveryBounds) (RecoveryPlan, error) {
	evidence, err := store.ScanRecoveryEvidence(ctx, bounds)
	if err != nil {
		return RecoveryPlan{}, err
	}
	return BuildRecoveryPlan(evidence)
}

// WriteRecoveryPlan installs a completed plan at a separate absolute path using
// an owner-only same-parent staging file followed by a native no-replace move.
func (store *DiagnosticStore) WriteRecoveryPlan(ctx context.Context, output string, plan RecoveryPlan, pretty bool) (err error) {
	if store == nil {
		return operation.New(operation.KindInvalidInput, "backup diagnostic store is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return operation.Wrap(operation.KindCancelled, "write_backup_recovery_plan", "", err)
	}

	data, err := EncodeRecoveryPlan(plan, pretty)
	if err != nil {
		return operation.Wrap(operation.KindInvalidInput, "write_backup_recovery_plan", "", err)
	}
	output, parent, parentInfo, err := store.validateRecoveryPlanOutput(output)
	if err != nil {
		return err
	}

	store.transactionMu.Lock()
	defer store.transactionMu.Unlock()
	if err := store.validateIdentity(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return operation.Wrap(operation.KindCancelled, "write_backup_recovery_plan", "", err)
	}
	if _, statErr := os.Lstat(output); statErr == nil {
		return operation.New(operation.KindConflict, "recovery plan output already exists")
	} else if !os.IsNotExist(statErr) {
		return sanitizedFilesystemError("recovery plan output cannot be inspected", statErr)
	}

	staged, createErr := os.CreateTemp(parent, ".scripthold-recovery-plan-*.tmp")
	if createErr != nil {
		return sanitizedFilesystemError("recovery plan staging file could not be created", createErr)
	}
	stagedPath := staged.Name()
	stagedIdentity, identityErr := captureRecoveryOwnedRegularFile(staged)
	if identityErr != nil {
		_ = staged.Close()
		return sanitizedFilesystemError("recovery plan staging identity could not be captured", identityErr)
	}
	defer closeRecoveryOwnedRegularFile(&stagedIdentity)
	defer func() {
		if staged != nil {
			_ = staged.Close()
		}
		removeRecoveryPlanStageIfOwned(stagedPath, stagedIdentity)
	}()
	if err := restrictPathPermissions(stagedPath, false); err != nil {
		return sanitizedFilesystemError("recovery plan staging permissions could not be restricted", err)
	}
	if err := validateSingleLink(stagedPath, stagedIdentity.info); err != nil {
		return operation.New(operation.KindFilesystem, "recovery plan staging hard-link state is invalid")
	}
	if err := writeAndSync(staged, data); err != nil {
		return sanitizedFilesystemError("recovery plan staging data could not be persisted", err)
	}
	if closeErr := staged.Close(); closeErr != nil {
		staged = nil
		return sanitizedFilesystemError("recovery plan staging file could not be closed", closeErr)
	}
	staged = nil
	postWriteInfo, statErr := os.Lstat(stagedPath)
	if statErr != nil || postWriteInfo == nil || isLinkOrReparse(postWriteInfo) || !postWriteInfo.Mode().IsRegular() ||
		!recoveryOwnedRegularFileStable(stagedPath, stagedIdentity) || postWriteInfo.Size() != int64(len(data)) {
		return operation.New(operation.KindConflict, "recovery plan staging identity changed during persistence")
	}
	stagedIdentity.info = postWriteInfo

	if err := ctx.Err(); err != nil {
		return operation.Wrap(operation.KindCancelled, "write_backup_recovery_plan", "", err)
	}
	if !recoveryDirectoryIdentityStable(parent, parentInfo) {
		return operation.New(operation.KindConflict, "recovery plan parent identity changed")
	}
	if !recoveryOwnedRegularFileStable(stagedPath, stagedIdentity) {
		return operation.New(operation.KindConflict, "recovery plan staging identity changed")
	}
	if err := validateSingleLink(stagedPath, stagedIdentity.info); err != nil {
		return operation.New(operation.KindFilesystem, "recovery plan staging hard-link state changed")
	}
	if err := validatePathPermissions(stagedPath, false); err != nil {
		return sanitizedFilesystemError("recovery plan staging permissions are not owner-only", err)
	}
	if _, statErr := os.Lstat(output); statErr == nil {
		return operation.New(operation.KindConflict, "recovery plan output already exists")
	} else if !os.IsNotExist(statErr) {
		return sanitizedFilesystemError("recovery plan output cannot be inspected", statErr)
	}

	if moveErr := filesystem.MoveNoReplace(stagedPath, output); moveErr != nil {
		return sanitizedFilesystemError("recovery plan output could not be installed", moveErr)
	}
	stagedPath = ""

	finalInfo, statErr := os.Lstat(output)
	if statErr != nil || finalInfo == nil || isLinkOrReparse(finalInfo) || !finalInfo.Mode().IsRegular() ||
		!recoveryOwnedRegularFileStable(output, stagedIdentity) || finalInfo.Size() != int64(len(data)) {
		return operation.New(operation.KindFilesystem, "recovery plan output identity is invalid")
	}
	if err := validateSingleLink(output, finalInfo); err != nil {
		return operation.New(operation.KindFilesystem, "recovery plan output hard-link state is invalid")
	}
	if err := validatePathPermissions(output, false); err != nil {
		return sanitizedFilesystemError("recovery plan output permissions are not owner-only", err)
	}
	readBack, ok := readStableRecoveryRegularFile(output, finalInfo, maxRecoveryPlanBytes)
	if !ok {
		return operation.New(operation.KindFilesystem, "recovery plan output could not be revalidated")
	}
	decoded, decodeErr := DecodeRecoveryPlan(readBack)
	if decodeErr != nil || decoded.PlanID != plan.PlanID {
		return operation.New(operation.KindFilesystem, "recovery plan output identity does not match the planned evidence")
	}
	if !recoveryDirectoryIdentityStable(parent, parentInfo) {
		return operation.New(operation.KindConflict, "recovery plan parent identity changed")
	}
	if err := store.validateIdentity(); err != nil {
		return err
	}
	return nil
}

func (store *DiagnosticStore) validateRecoveryPlanOutput(output string) (string, string, os.FileInfo, error) {
	if strings.TrimSpace(output) == "" || strings.ContainsRune(output, '\x00') {
		return "", "", nil, operation.New(operation.KindInvalidPath, "recovery plan output must be a non-empty absolute path")
	}
	clean := filepath.Clean(output)
	if !filepath.IsAbs(clean) || filepath.Dir(clean) == clean {
		return "", "", nil, operation.New(operation.KindInvalidPath, "recovery plan output must be an absolute file path")
	}
	parent := filepath.Dir(clean)
	if err := validateExistingComponents(parent); err != nil {
		return "", "", nil, sanitizedFilesystemError("recovery plan output parent is unsafe", err)
	}
	set, err := security.NormalizeAllowedDirectorySet([]string{parent})
	if err != nil || len(set.Requested) != 1 || len(set.Resolved) != 1 || !security.PathsEqual(set.Requested[0], set.Resolved[0]) {
		return "", "", nil, operation.New(operation.KindInvalidPath, "recovery plan output parent must not use a path alias")
	}
	parent = set.Resolved[0]
	clean = filepath.Join(parent, filepath.Base(clean))
	if security.PathsOverlap(clean, store.root) {
		return "", "", nil, operation.New(operation.KindAccessDenied, "recovery plan output must not overlap the source store")
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return "", "", nil, sanitizedFilesystemError("recovery plan output parent cannot be inspected", err)
	}
	if isLinkOrReparse(parentInfo) || !parentInfo.IsDir() {
		return "", "", nil, operation.New(operation.KindInvalidPath, "recovery plan output parent is not a real directory")
	}
	return clean, parent, parentInfo, nil
}

func recoveryDirectoryIdentityStable(path string, expected os.FileInfo) bool {
	if expected == nil {
		return false
	}
	current, err := os.Lstat(path)
	return err == nil && current != nil && current.IsDir() && !isLinkOrReparse(current) && os.SameFile(expected, current)
}

func removeRecoveryPlanStageIfOwned(path string, expected recoveryOwnedRegularFile) {
	removeRecoveryRegularFileIfOwned(path, expected)
}
