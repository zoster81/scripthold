package backupstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

type recoveryApplyPaths struct {
	source         string
	plan           string
	destination    string
	report         string
	staging        string
	state          string
	destinationKey string
}

type RecoveryDestination struct {
	paths     recoveryApplyPaths
	store     *Store
	state     RecoveryState
	resumed   bool
	promoted  bool
	completed bool
}

// IsPromoted reports whether the exact plan-bound recovery session has already
// crossed the audited staging rename boundary. It exposes no local path data.
func (destination *RecoveryDestination) IsPromoted() bool {
	return destination != nil && destination.promoted
}

// IsCompleted reports whether an exact durable recovery report has already been
// recognized for the promoted plan-bound session.
func (destination *RecoveryDestination) IsCompleted() bool {
	return destination != nil && destination.completed
}
func (destination *RecoveryDestination) Close() error {
	if destination == nil || destination.store == nil {
		return nil
	}
	err := destination.store.Close()
	destination.store = nil
	return err
}

// AuthorizeRecoveryApplyPaths validates all operator-supplied recovery paths
// before destination state may be created.
func (store *DiagnosticStore) AuthorizeRecoveryApplyPaths(planPath, destination, report string, plan RecoveryPlan) (recoveryApplyPaths, error) {
	if store == nil {
		return recoveryApplyPaths{}, operation.New(operation.KindInvalidInput, "backup diagnostic store is unavailable")
	}
	if _, err := EncodeRecoveryPlan(plan, false); err != nil {
		return recoveryApplyPaths{}, operation.Wrap(operation.KindInvalidInput, "authorize_backup_recovery_paths", "", err)
	}
	if err := store.validateIdentity(); err != nil {
		return recoveryApplyPaths{}, err
	}
	descriptor := inspectRecoveryDescriptor(store.root)
	if !descriptor.valid || descriptor.descriptor.StoreID != plan.SourceStoreID ||
		descriptor.descriptor.FormatVersion != plan.SourceFormatVersion ||
		descriptor.fingerprint != plan.DescriptorFingerprint {
		return recoveryApplyPaths{}, operation.New(operation.KindConflict, "recovery plan source identity does not match the locked source store")
	}

	canonicalPlan, err := validateRecoveryPlanInputPath(planPath, plan)
	if err != nil {
		return recoveryApplyPaths{}, err
	}
	canonicalDestination, destinationParent, err := validateMissingRecoveryPath(destination, "recovery destination")
	if err != nil {
		return recoveryApplyPaths{}, err
	}
	canonicalReport, _, err := validateMissingRecoveryPath(report, "recovery report")
	if err != nil {
		return recoveryApplyPaths{}, err
	}

	destinationKey := digestRecoveryPathKey(canonicalDestination)
	stagingKey := digestRecoveryStageKey(destinationKey, plan.PlanID)
	staging := filepath.Join(destinationParent, ".scripthold-recovery-"+stagingKey+".staging")
	state := filepath.Join(destinationParent, ".scripthold-recovery-"+stagingKey+".state.json")
	if _, _, err := validateRecoverySiblingPath(staging, "recovery staging"); err != nil {
		return recoveryApplyPaths{}, err
	}
	if _, _, err := validateRecoverySiblingPath(state, "recovery state"); err != nil {
		return recoveryApplyPaths{}, err
	}

	paths := recoveryApplyPaths{
		source:         store.root,
		plan:           canonicalPlan,
		destination:    canonicalDestination,
		report:         canonicalReport,
		staging:        staging,
		state:          state,
		destinationKey: destinationKey,
	}
	if err := validateRecoveryPathSeparation(paths); err != nil {
		return recoveryApplyPaths{}, err
	}
	return paths, nil
}

func (store *DiagnosticStore) authorizeRecoveryApplyPathsAllowExisting(planPath, destination, report string, plan RecoveryPlan) (recoveryApplyPaths, error) {
	if store == nil {
		return recoveryApplyPaths{}, operation.New(operation.KindInvalidInput, "backup diagnostic store is unavailable")
	}
	if _, err := EncodeRecoveryPlan(plan, false); err != nil {
		return recoveryApplyPaths{}, operation.Wrap(operation.KindInvalidInput, "authorize_backup_recovery_paths", "", err)
	}
	if err := store.validateIdentity(); err != nil {
		return recoveryApplyPaths{}, err
	}
	descriptor := inspectRecoveryDescriptor(store.root)
	if !descriptor.valid || descriptor.descriptor.StoreID != plan.SourceStoreID ||
		descriptor.descriptor.FormatVersion != plan.SourceFormatVersion || descriptor.fingerprint != plan.DescriptorFingerprint {
		return recoveryApplyPaths{}, operation.New(operation.KindConflict, "recovery plan source identity does not match the locked source store")
	}
	canonicalPlan, err := validateRecoveryPlanInputPath(planPath, plan)
	if err != nil {
		return recoveryApplyPaths{}, err
	}
	canonicalDestination, destinationParent, err := validateRecoverySiblingPath(destination, "recovery destination")
	if err != nil {
		return recoveryApplyPaths{}, err
	}
	canonicalReport, _, err := validateRecoverySiblingPath(report, "recovery report")
	if err != nil {
		return recoveryApplyPaths{}, err
	}
	destinationKey := digestRecoveryPathKey(canonicalDestination)
	stagingKey := digestRecoveryStageKey(destinationKey, plan.PlanID)
	staging := filepath.Join(destinationParent, ".scripthold-recovery-"+stagingKey+".staging")
	state := filepath.Join(destinationParent, ".scripthold-recovery-"+stagingKey+".state.json")
	if _, _, err := validateRecoverySiblingPath(staging, "recovery staging"); err != nil {
		return recoveryApplyPaths{}, err
	}
	if _, _, err := validateRecoverySiblingPath(state, "recovery state"); err != nil {
		return recoveryApplyPaths{}, err
	}
	paths := recoveryApplyPaths{
		source: store.root, plan: canonicalPlan, destination: canonicalDestination, report: canonicalReport,
		staging: staging, state: state, destinationKey: destinationKey,
	}
	if err := validateRecoveryPathSeparation(paths); err != nil {
		return recoveryApplyPaths{}, err
	}
	return paths, nil
}

// PrepareRecoveryDestination creates a new plan-bound staging store or resumes
// only the exact owner-only state/staging pair previously created for that plan.
func (store *DiagnosticStore) PrepareRecoveryDestination(ctx context.Context, planPath, destination, report string, plan RecoveryPlan) (*RecoveryDestination, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, operation.Wrap(operation.KindCancelled, "prepare_backup_recovery_destination", "", err)
	}

	paths, err := store.authorizeRecoveryApplyPathsAllowExisting(planPath, destination, report, plan)
	if err != nil {
		return nil, err
	}
	if err := store.validateIdentity(); err != nil {
		return nil, err
	}

	stagingInfo, stagingErr := os.Lstat(paths.staging)
	stateInfo, stateErr := os.Lstat(paths.state)
	destinationInfo, destinationErr := os.Lstat(paths.destination)
	reportInfo, reportErr := os.Lstat(paths.report)
	stagingExists := stagingErr == nil
	stateExists := stateErr == nil
	destinationExists := destinationErr == nil
	reportExists := reportErr == nil
	if stagingErr != nil && !os.IsNotExist(stagingErr) {
		return nil, sanitizedFilesystemError("recovery staging cannot be inspected", stagingErr)
	}
	if stateErr != nil && !os.IsNotExist(stateErr) {
		return nil, sanitizedFilesystemError("recovery state cannot be inspected", stateErr)
	}
	if destinationErr != nil && !os.IsNotExist(destinationErr) {
		return nil, sanitizedFilesystemError("recovery destination cannot be inspected", destinationErr)
	}
	if reportErr != nil && !os.IsNotExist(reportErr) {
		return nil, sanitizedFilesystemError("recovery report cannot be inspected", reportErr)
	}
	if stagingExists {
		if !stateExists || destinationExists || reportExists {
			return nil, operation.New(operation.KindConflict, "recovery staging is not an exact resumable pre-promotion state")
		}
		return store.resumeRecoveryDestination(ctx, paths, plan, stagingInfo, stateInfo)
	}
	if stateExists {
		if !destinationExists {
			return nil, operation.New(operation.KindConflict, "recovery state exists without recognized staging or promoted destination")
		}
		return store.resumePromotedRecoveryDestination(ctx, paths, plan, destinationInfo, stateInfo, reportInfo, reportExists)
	}
	if destinationExists {
		return nil, operation.New(operation.KindConflict, "pre-existing recovery destination has no recognized plan-bound recovery state")
	}
	if reportExists {
		return nil, operation.New(operation.KindConflict, "recovery report exists without a recognized completed destination")
	}
	return store.createRecoveryDestination(ctx, paths, plan)
}

func (store *DiagnosticStore) createRecoveryDestination(ctx context.Context, paths recoveryApplyPaths, plan RecoveryPlan) (*RecoveryDestination, error) {
	if _, err := os.Lstat(paths.destination); err == nil {
		return nil, operation.New(operation.KindConflict, "recovery destination already exists")
	} else if !os.IsNotExist(err) {
		return nil, sanitizedFilesystemError("recovery destination cannot be inspected", err)
	}
	if _, err := os.Lstat(paths.staging); err == nil {
		return nil, operation.New(operation.KindConflict, "recovery staging already exists")
	} else if !os.IsNotExist(err) {
		return nil, sanitizedFilesystemError("recovery staging cannot be inspected", err)
	}
	if _, err := os.Lstat(paths.state); err == nil {
		return nil, operation.New(operation.KindConflict, "recovery state already exists")
	} else if !os.IsNotExist(err) {
		return nil, sanitizedFilesystemError("recovery state cannot be inspected", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, operation.Wrap(operation.KindCancelled, "prepare_backup_recovery_destination", "", err)
	}

	destinationStore, err := Open(Options{
		Directory: paths.staging,
		Limits:    recoveryDestinationLimits(plan),
	})
	if err != nil {
		return nil, err
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = destinationStore.Close()
		}
	}()

	if destinationStore.descriptor.StoreID == plan.SourceStoreID {
		return nil, operation.New(operation.KindConflict, "recovery destination StoreID must be fresh")
	}
	state := RecoveryState{
		FormatVersion:      RecoveryStateFormatVersion,
		PlanID:             plan.PlanID,
		DestinationKey:     paths.destinationKey,
		DestinationStoreID: destinationStore.descriptor.StoreID,
		Phase:              RecoveryPhaseBuilding,
	}
	if err := writeRecoveryStateNoReplace(ctx, paths.state, state); err != nil {
		return nil, err
	}
	if err := store.validateIdentity(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, operation.Wrap(operation.KindCancelled, "prepare_backup_recovery_destination", "", err)
	}

	closeOnError = false
	return &RecoveryDestination{paths: paths, store: destinationStore, state: state}, nil
}

func (store *DiagnosticStore) resumeRecoveryDestination(
	ctx context.Context,
	paths recoveryApplyPaths,
	plan RecoveryPlan,
	stagingInfo os.FileInfo,
	stateInfo os.FileInfo,
) (*RecoveryDestination, error) {
	if stagingInfo == nil || isLinkOrReparse(stagingInfo) || !stagingInfo.IsDir() {
		return nil, operation.New(operation.KindConflict, "recovery staging identity is invalid")
	}
	if err := validatePathPermissions(paths.staging, true); err != nil {
		return nil, operation.New(operation.KindConflict, "recovery staging permissions are invalid")
	}
	if stateInfo == nil || isLinkOrReparse(stateInfo) || !stateInfo.Mode().IsRegular() {
		return nil, operation.New(operation.KindConflict, "recovery state identity is invalid")
	}
	if err := validateSingleLink(paths.state, stateInfo); err != nil {
		return nil, operation.New(operation.KindConflict, "recovery state hard-link state is invalid")
	}
	if err := validatePathPermissions(paths.state, false); err != nil {
		return nil, operation.New(operation.KindConflict, "recovery state permissions are invalid")
	}

	state, err := readRecoveryStateFile(paths.state, stateInfo)
	if err != nil {
		return nil, err
	}
	if state.PlanID != plan.PlanID || state.DestinationKey != paths.destinationKey ||
		(state.Phase != RecoveryPhaseBuilding && state.Phase != RecoveryPhaseAudited) {
		return nil, operation.New(operation.KindConflict, "recovery state does not match the requested plan and destination")
	}
	descriptor := inspectRecoveryDescriptor(paths.staging)
	if !descriptor.valid || descriptor.descriptor.StoreID != state.DestinationStoreID ||
		descriptor.descriptor.StoreID == plan.SourceStoreID {
		return nil, operation.New(operation.KindConflict, "recovery staging descriptor does not match the resumable state")
	}
	if err := ctx.Err(); err != nil {
		return nil, operation.Wrap(operation.KindCancelled, "resume_backup_recovery_destination", "", err)
	}

	destinationStore, err := Open(Options{
		Directory: paths.staging,
		Limits:    recoveryDestinationLimits(plan),
	})
	if err != nil {
		return nil, err
	}
	if destinationStore.descriptor.StoreID != state.DestinationStoreID {
		_ = destinationStore.Close()
		return nil, operation.New(operation.KindConflict, "opened recovery staging identity changed")
	}
	if err := store.validateIdentity(); err != nil {
		_ = destinationStore.Close()
		return nil, err
	}
	return &RecoveryDestination{
		paths:   paths,
		store:   destinationStore,
		state:   state,
		resumed: true,
	}, nil
}

func validateRecoveryPlanInputPath(path string, plan RecoveryPlan) (string, error) {
	clean, _, err := validateRecoverySiblingPath(path, "recovery plan")
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return "", sanitizedFilesystemError("recovery plan cannot be inspected", err)
	}
	if info == nil || isLinkOrReparse(info) || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxRecoveryPlanBytes {
		return "", operation.New(operation.KindInvalidPath, "recovery plan must be one bounded regular file")
	}
	if err := validateSingleLink(clean, info); err != nil {
		return "", operation.New(operation.KindInvalidPath, "recovery plan hard-link state is invalid")
	}
	if err := validatePathPermissions(clean, false); err != nil {
		return "", operation.New(operation.KindAccessDenied, "recovery plan permissions are not owner-only")
	}
	data, ok := readStableRecoveryRegularFile(clean, info, maxRecoveryPlanBytes)
	if !ok {
		return "", operation.New(operation.KindConflict, "recovery plan changed while it was being validated")
	}
	decoded, err := DecodeRecoveryPlan(data)
	if err != nil {
		return "", operation.Wrap(operation.KindInvalidInput, "authorize_backup_recovery_paths", "", err)
	}
	if decoded.PlanID != plan.PlanID {
		return "", operation.New(operation.KindConflict, "persisted recovery plan does not match the requested plan")
	}
	return clean, nil
}

func validateMissingRecoveryPath(path, label string) (string, string, error) {
	clean, parent, err := validateRecoverySiblingPath(path, label)
	if err != nil {
		return "", "", err
	}
	if _, err := os.Lstat(clean); err == nil {
		return "", "", operation.New(operation.KindConflict, label+" already exists")
	} else if !os.IsNotExist(err) {
		return "", "", sanitizedFilesystemError(label+" cannot be inspected", err)
	}
	return clean, parent, nil
}

func validateRecoverySiblingPath(path, label string) (string, string, error) {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, '\x00') {
		return "", "", operation.New(operation.KindInvalidPath, label+" must be a non-empty absolute path")
	}
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) || filepath.Dir(clean) == clean {
		return "", "", operation.New(operation.KindInvalidPath, label+" must be an absolute non-root path")
	}
	parent := filepath.Dir(clean)
	set, err := security.NormalizeAllowedDirectorySet([]string{parent})
	if err != nil || len(set.Requested) != 1 || len(set.Resolved) != 1 ||
		!security.PathsEqual(set.Requested[0], set.Resolved[0]) {
		return "", "", operation.New(operation.KindInvalidPath, label+" parent must be one real non-aliased directory")
	}
	parent = set.Resolved[0]
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return "", "", sanitizedFilesystemError(label+" parent cannot be inspected", err)
	}
	if parentInfo == nil || isLinkOrReparse(parentInfo) || !parentInfo.IsDir() {
		return "", "", operation.New(operation.KindInvalidPath, label+" parent must be one real directory")
	}
	clean = filepath.Join(parent, filepath.Base(clean))
	return clean, parent, nil
}

func validateRecoveryPathSeparation(paths recoveryApplyPaths) error {
	all := []struct {
		name string
		path string
	}{
		{"source", paths.source},
		{"plan", paths.plan},
		{"destination", paths.destination},
		{"report", paths.report},
		{"staging", paths.staging},
		{"state", paths.state},
	}
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if security.PathsOverlap(all[i].path, all[j].path) {
				return operation.New(operation.KindAccessDenied, "recovery "+all[i].name+" and "+all[j].name+" paths must not overlap")
			}
		}
	}
	return nil
}

func recoveryDestinationLimits(plan RecoveryPlan) Limits {
	return Limits{
		MaxTotalBytes:        plan.Bounds.MaxBytes,
		MaxObjectBytes:       hardMaxObjectBytes,
		MaxManifests:         plan.Bounds.MaxManifests,
		MaxVersionsPerTarget: hardMaxVersionsPerTarget,
		MaxPinned:            hardMaxPinned,
		RetentionDays:        hardMaxRetentionDays,
		PlanTTLSeconds:       hardMaxPlanTTLSeconds,
	}
}

func digestRecoveryPathKey(path string) string {
	key := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}

func digestRecoveryStageKey(destinationKey, planID string) string {
	digest := sha256.Sum256([]byte(destinationKey + "|" + planID))
	return hex.EncodeToString(digest[:])
}

func writeRecoveryStateNoReplace(ctx context.Context, path string, state RecoveryState) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return operation.Wrap(operation.KindCancelled, "write_backup_recovery_state", "", err)
	}
	data, err := EncodeRecoveryState(state)
	if err != nil {
		return operation.Wrap(operation.KindInvalidInput, "write_backup_recovery_state", "", err)
	}
	clean, parent, err := validateRecoverySiblingPath(path, "recovery state")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return operation.New(operation.KindConflict, "recovery state already exists")
		}
		return sanitizedFilesystemError("recovery state could not be created", err)
	}
	info, statErr := file.Stat()
	if statErr != nil || info == nil || !info.Mode().IsRegular() || isLinkOrReparse(info) {
		_ = file.Close()
		return operation.New(operation.KindFilesystem, "recovery state identity is invalid")
	}
	if err := restrictPathPermissions(clean, false); err != nil {
		_ = file.Close()
		return sanitizedFilesystemError("recovery state permissions could not be restricted", err)
	}
	if err := validateSingleLink(clean, info); err != nil {
		_ = file.Close()
		return operation.New(operation.KindFilesystem, "recovery state hard-link state is invalid")
	}
	if err := writeAndSync(file, data); err != nil {
		_ = file.Close()
		return sanitizedFilesystemError("recovery state could not be persisted", err)
	}
	if err := file.Close(); err != nil {
		return sanitizedFilesystemError("recovery state could not be closed", err)
	}
	if err := syncDirectory(parent); err != nil {
		return sanitizedFilesystemError("recovery state parent could not be synchronized", err)
	}
	finalInfo, err := os.Lstat(clean)
	if err != nil || finalInfo == nil || isLinkOrReparse(finalInfo) || !finalInfo.Mode().IsRegular() ||
		!os.SameFile(info, finalInfo) || finalInfo.Size() != int64(len(data)) {
		return operation.New(operation.KindConflict, "recovery state identity changed during persistence")
	}
	if err := validateSingleLink(clean, finalInfo); err != nil {
		return operation.New(operation.KindFilesystem, "recovery state hard-link state changed")
	}
	if err := validatePathPermissions(clean, false); err != nil {
		return operation.New(operation.KindAccessDenied, "recovery state permissions are not owner-only")
	}
	readBack, readErr := readRecoveryStateFile(clean, finalInfo)
	if readErr != nil || readBack != state {
		return operation.New(operation.KindFilesystem, "recovery state could not be revalidated")
	}
	return nil
}

func readRecoveryStateFile(path string, expected os.FileInfo) (RecoveryState, error) {
	if expected == nil || isLinkOrReparse(expected) || !expected.Mode().IsRegular() ||
		expected.Size() < 0 || expected.Size() > maxRecoveryStateBytes {
		return RecoveryState{}, operation.New(operation.KindConflict, "recovery state identity is invalid")
	}
	if err := validateSingleLink(path, expected); err != nil {
		return RecoveryState{}, operation.New(operation.KindConflict, "recovery state hard-link state is invalid")
	}
	if err := validatePathPermissions(path, false); err != nil {
		return RecoveryState{}, operation.New(operation.KindConflict, "recovery state permissions are invalid")
	}
	data, ok := readStableRecoveryRegularFile(path, expected, maxRecoveryStateBytes)
	if !ok {
		return RecoveryState{}, operation.New(operation.KindConflict, "recovery state changed while it was being read")
	}
	state, err := DecodeRecoveryState(data)
	if err != nil {
		return RecoveryState{}, operation.Wrap(operation.KindConflict, "read_backup_recovery_state", "", err)
	}
	return state, nil
}
