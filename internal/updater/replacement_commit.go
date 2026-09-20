package updater

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/zoster81/scripthold/internal/filesystem"
)

// CommitExecutableReplacement atomically consumes the prepared candidate and
// verifies the installed candidate through the normal observed evidence path.
// It leaves the pending transaction intact for later finalization or rollback.
func (ownership *DetachedHelperOwnership) CommitExecutableReplacement(ctx context.Context) error {
	return ownership.commitExecutableReplacementWith(ctx, runtime.GOOS, runtime.GOARCH, reconciliationDeps{})
}

func (ownership *DetachedHelperOwnership) commitExecutableReplacementWith(
	ctx context.Context,
	goos, goarch string,
	deps reconciliationDeps,
) error {
	if ownership == nil || ownership.controlLock == nil || ownership.useLock == nil ||
		ownership.boundary == nil || ownership.inspection == nil {
		return errors.New("detached helper ownership is required")
	}
	if err := ownership.prepareExecutableReplacementWith(ctx, goos, goarch, deps); err != nil {
		return err
	}
	if err := ownership.controlLock.Validate(ownership.boundary.ControlLockPath); err != nil {
		return fmt.Errorf("validate executable commit control lock: %w", err)
	}
	if err := ownership.useLock.Validate(ownership.boundary.UseLockPath); err != nil {
		return fmt.Errorf("validate executable commit use lock: %w", err)
	}

	state, err := readInstallationStateForReconciliationLocked(ownership.boundary, ownership.inspection)
	if err != nil {
		return err
	}
	if state.Pending == nil {
		return errors.New("prepared replacement transaction disappeared")
	}
	candidatePath := filepath.Join(ownership.boundary.Directory, candidateArtifactName)
	candidateIdentity, err := filesystem.CaptureSingleLinkFileIdentity(candidatePath)
	if err != nil {
		return fmt.Errorf("capture prepared candidate identity: %w", err)
	}
	installedIdentity, err := filesystem.CommitExecutableReplacementCandidate(
		ownership.inspection.ExecutablePath,
		ownership.inspection.ExecutableIdentity,
		candidatePath,
		candidateIdentity,
	)
	if err != nil {
		return err
	}

	installedInspection, err := inspectStandaloneExecutable(ownership.inspection.ExecutablePath, goos)
	if err != nil {
		return fmt.Errorf("inspect committed executable: %w", err)
	}
	if installedInspection.ExecutableIdentity.StableKey() != installedIdentity.StableKey() ||
		installedInspection.ExecutableIdentity.VolumeKey() != installedIdentity.VolumeKey() {
		return errors.New("committed executable identity changed before verification")
	}
	ownership.inspection = installedInspection

	result, err := reconcileInstallationLocked(
		ctx,
		ownership.boundary,
		ownership.inspection,
		goos,
		goarch,
		deps,
		ownership.controlLock,
	)
	if err != nil {
		return err
	}
	if result.Status != ReconciliationCommitted {
		if result.Problem != "" {
			return fmt.Errorf("committed executable verification failed: %s", result.Problem)
		}
		return fmt.Errorf("committed executable verification observed %s", result.Status)
	}
	if err := ownership.useLock.Validate(ownership.boundary.UseLockPath); err != nil {
		return fmt.Errorf("revalidate executable commit use lock: %w", err)
	}
	if err := ownership.controlLock.Validate(ownership.boundary.ControlLockPath); err != nil {
		return fmt.Errorf("revalidate executable commit control lock: %w", err)
	}
	return nil
}
