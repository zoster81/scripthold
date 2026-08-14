package backupstore

import (
	"os"

	"github.com/zoster81/scripthold/internal/operation"
)

// LoadRecoveryPlan loads one explicit persisted recovery plan through the same
// bounded, owner-only, single-link and strict-codec rules used by apply path
// authorization. The returned document is evidence only; callers must still
// re-scan the locked source and require the same deterministic plan identity.
func LoadRecoveryPlan(path string) (RecoveryPlan, error) {
	clean, _, err := validateRecoverySiblingPath(path, "recovery plan")
	if err != nil {
		return RecoveryPlan{}, err
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return RecoveryPlan{}, sanitizedFilesystemError("recovery plan cannot be inspected", err)
	}
	if info == nil || isLinkOrReparse(info) || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxRecoveryPlanBytes {
		return RecoveryPlan{}, operation.New(operation.KindInvalidPath, "recovery plan must be one bounded regular file")
	}
	if err := validateSingleLink(clean, info); err != nil {
		return RecoveryPlan{}, operation.New(operation.KindInvalidPath, "recovery plan hard-link state is invalid")
	}
	if err := validatePathPermissions(clean, false); err != nil {
		return RecoveryPlan{}, operation.New(operation.KindAccessDenied, "recovery plan permissions are not owner-only")
	}
	data, ok := readStableRecoveryRegularFile(clean, info, maxRecoveryPlanBytes)
	if !ok || !recoveryFileIdentityStable(clean, info) {
		return RecoveryPlan{}, operation.New(operation.KindConflict, "recovery plan changed while it was being loaded")
	}
	plan, err := DecodeRecoveryPlan(data)
	if err != nil {
		return RecoveryPlan{}, operation.Wrap(operation.KindInvalidInput, "load_backup_recovery_plan", "", err)
	}
	return plan, nil
}
