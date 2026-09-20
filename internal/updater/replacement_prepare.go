package updater

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/zoster81/scripthold/internal/filesystem"
)

// PrepareExecutableReplacement qualifies target metadata and prepares the fixed
// candidate artifact for a later atomic replacement. It never changes target.
func (ownership *DetachedHelperOwnership) PrepareExecutableReplacement(ctx context.Context) error {
	return ownership.prepareExecutableReplacementWith(ctx, runtime.GOOS, runtime.GOARCH, reconciliationDeps{})
}

func (ownership *DetachedHelperOwnership) prepareExecutableReplacementWith(
	ctx context.Context,
	goos, goarch string,
	deps reconciliationDeps,
) error {
	if ownership == nil || ownership.controlLock == nil || ownership.useLock == nil ||
		ownership.boundary == nil || ownership.inspection == nil {
		return errors.New("detached helper ownership is required")
	}
	if err := ownership.controlLock.Validate(ownership.boundary.ControlLockPath); err != nil {
		return fmt.Errorf("validate executable replacement control lock: %w", err)
	}
	if err := ownership.useLock.Validate(ownership.boundary.UseLockPath); err != nil {
		return fmt.Errorf("validate executable replacement use lock: %w", err)
	}

	before, err := reconcileInstallationLocked(
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
	if before.Status != ReconciliationPrepared {
		return fmt.Errorf("executable replacement requires prepared transaction, observed %s", before.Status)
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
		return fmt.Errorf("capture executable replacement candidate identity: %w", err)
	}
	if err := filesystem.PrepareExecutableReplacementCandidate(
		ownership.inspection.ExecutablePath,
		ownership.inspection.ExecutableIdentity,
		candidatePath,
		candidateIdentity,
	); err != nil {
		return err
	}
	if observeCandidateArtifact(ownership.boundary, state.Pending.CandidateSHA256) != artifactValid {
		return errors.New("prepared executable replacement candidate no longer matches pending bytes")
	}

	after, err := reconcileInstallationLocked(
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
	if after.Status != ReconciliationPrepared {
		return fmt.Errorf("replacement preparation changed transaction state to %s", after.Status)
	}
	if err := ownership.useLock.Validate(ownership.boundary.UseLockPath); err != nil {
		return fmt.Errorf("revalidate executable replacement use lock: %w", err)
	}
	if err := ownership.controlLock.Validate(ownership.boundary.ControlLockPath); err != nil {
		return fmt.Errorf("revalidate executable replacement control lock: %w", err)
	}
	return nil
}
