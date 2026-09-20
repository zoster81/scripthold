package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"

	"github.com/zoster81/scripthold/internal/filesystem"
)

type rollbackDeps struct {
	beforeCommit func() error
}

// RollbackCommittedReplacement restores the verified source bytes from the
// immutable known-good artifact through the same durable replacement path.
// Known-good is never consumed; the fixed candidate slot is reused as the
// rollback staging file.
func (ownership *DetachedHelperOwnership) RollbackCommittedReplacement(ctx context.Context) error {
	return ownership.rollbackCommittedReplacementWith(
		ctx,
		runtime.GOOS,
		runtime.GOARCH,
		reconciliationDeps{},
		rollbackDeps{},
	)
}

func (ownership *DetachedHelperOwnership) rollbackCommittedReplacementWith(
	ctx context.Context,
	goos, goarch string,
	reconcileDeps reconciliationDeps,
	deps rollbackDeps,
) error {
	if ownership == nil || ownership.controlLock == nil || ownership.useLock == nil ||
		ownership.boundary == nil || ownership.inspection == nil {
		return errors.New("detached helper ownership is required")
	}
	if err := ownership.controlLock.Validate(ownership.boundary.ControlLockPath); err != nil {
		return fmt.Errorf("validate rollback control lock: %w", err)
	}
	if err := ownership.useLock.Validate(ownership.boundary.UseLockPath); err != nil {
		return fmt.Errorf("validate rollback use lock: %w", err)
	}
	if err := ownership.refreshTargetInspection(goos); err != nil {
		return fmt.Errorf("refresh rollback target inspection: %w", err)
	}

	result, err := reconcileInstallationLocked(
		ctx,
		ownership.boundary,
		ownership.inspection,
		goos,
		goarch,
		reconcileDeps,
		ownership.controlLock,
	)
	if err != nil {
		return err
	}
	switch result.Status {
	case ReconciliationRolledBack:
		return nil
	case ReconciliationCommitted:
	case ReconciliationRecoveryRequired:
		if !result.candidateBytesInstalled {
			return fmt.Errorf("rollback requires installed candidate bytes, observed %s", result.Status)
		}
	default:
		return fmt.Errorf("rollback requires committed transaction, observed %s", result.Status)
	}

	state, err := readInstallationStateForReconciliationLocked(ownership.boundary, ownership.inspection)
	if err != nil {
		return err
	}
	if state.Pending == nil {
		return errors.New("committed rollback transaction disappeared")
	}
	if state.Pending.SourceSHA256 != state.Current.SHA256 ||
		state.Pending.SourceVersion != state.Current.Version {
		return errors.New("rollback source evidence no longer matches current state")
	}
	if observeFixedArtifact(ownership.boundary, knownGoodArtifactName, state.Pending.SourceSHA256) != artifactValid {
		return errors.New("known-good rollback source is missing or invalid")
	}

	candidatePath := filepath.Join(ownership.boundary.Directory, candidateArtifactName)
	if !result.rollbackPrepared {
		if err := stageRollbackSourceCandidate(ctx, ownership.boundary, state.Pending.SourceSHA256); err != nil {
			return err
		}
	}
	if observeCandidateArtifact(ownership.boundary, candidateArtifactName, state.Pending.SourceSHA256) != artifactValid {
		return errors.New("rollback source candidate is missing or invalid")
	}

	candidateIdentity, err := filesystem.CaptureSingleLinkFileIdentity(candidatePath)
	if err != nil {
		return fmt.Errorf("capture rollback candidate identity: %w", err)
	}
	if err := filesystem.PrepareExecutableReplacementCandidate(
		ownership.inspection.ExecutablePath,
		ownership.inspection.ExecutableIdentity,
		candidatePath,
		candidateIdentity,
	); err != nil {
		return fmt.Errorf("prepare rollback executable replacement: %w", err)
	}
	if observeCandidateArtifact(ownership.boundary, candidateArtifactName, state.Pending.SourceSHA256) != artifactValid {
		return errors.New("prepared rollback candidate no longer matches source bytes")
	}

	result, err = reconcileInstallationLocked(
		ctx,
		ownership.boundary,
		ownership.inspection,
		goos,
		goarch,
		reconcileDeps,
		ownership.controlLock,
	)
	if err != nil {
		return err
	}
	rollbackReady := result.rollbackPrepared &&
		(result.Status == ReconciliationCommitted ||
			(result.Status == ReconciliationRecoveryRequired && result.candidateBytesInstalled))
	if !rollbackReady {
		return fmt.Errorf(
			"rollback staging changed transaction state to %s (rollbackPrepared=%v candidateBytesInstalled=%v)",
			result.Status,
			result.rollbackPrepared,
			result.candidateBytesInstalled,
		)
	}
	if deps.beforeCommit != nil {
		if err := deps.beforeCommit(); err != nil {
			return err
		}
	}
	if err := ownership.controlLock.Validate(ownership.boundary.ControlLockPath); err != nil {
		return fmt.Errorf("revalidate rollback control lock before commit: %w", err)
	}
	if err := ownership.useLock.Validate(ownership.boundary.UseLockPath); err != nil {
		return fmt.Errorf("revalidate rollback use lock before commit: %w", err)
	}

	installedIdentity, err := filesystem.CommitExecutableReplacementCandidate(
		ownership.inspection.ExecutablePath,
		ownership.inspection.ExecutableIdentity,
		candidatePath,
		candidateIdentity,
	)
	if err != nil {
		refreshErr := ownership.refreshTargetInspection(goos)
		return errors.Join(
			fmt.Errorf("commit rollback executable replacement: %w", err),
			refreshErr,
		)
	}
	installedInspection, err := inspectStandaloneExecutable(ownership.inspection.ExecutablePath, goos)
	if err != nil {
		return fmt.Errorf("inspect rolled-back executable: %w", err)
	}
	if installedInspection.ExecutableIdentity.StableKey() != installedIdentity.StableKey() ||
		installedInspection.ExecutableIdentity.VolumeKey() != installedIdentity.VolumeKey() {
		return errors.New("rolled-back executable identity changed before verification")
	}
	ownership.inspection = installedInspection

	result, err = reconcileInstallationLocked(
		ctx,
		ownership.boundary,
		ownership.inspection,
		goos,
		goarch,
		reconcileDeps,
		ownership.controlLock,
	)
	if err != nil {
		return err
	}
	if result.Status != ReconciliationRolledBack {
		if result.Problem != "" {
			return fmt.Errorf("rolled-back executable verification failed: %s", result.Problem)
		}
		return fmt.Errorf("rolled-back executable verification observed %s", result.Status)
	}
	if err := ownership.useLock.Validate(ownership.boundary.UseLockPath); err != nil {
		return fmt.Errorf("revalidate rollback use lock: %w", err)
	}
	if err := ownership.controlLock.Validate(ownership.boundary.ControlLockPath); err != nil {
		return fmt.Errorf("revalidate rollback control lock: %w", err)
	}
	if observeFixedArtifact(ownership.boundary, knownGoodArtifactName, state.Pending.SourceSHA256) != artifactValid {
		return errors.New("known-good rollback source changed during rollback")
	}
	return nil
}

func stageRollbackSourceCandidate(
	ctx context.Context,
	boundary *InstallationBoundary,
	expectedSHA256 string,
) error {
	knownGoodPath := filepath.Join(boundary.Directory, knownGoodArtifactName)
	candidatePath := filepath.Join(boundary.Directory, candidateArtifactName)

	knownGoodIdentity, err := filesystem.CaptureSingleLinkFileIdentity(knownGoodPath)
	if err != nil {
		return fmt.Errorf("capture known-good rollback identity: %w", err)
	}
	source, err := filesystem.OpenVerifiedSingleLinkFile(knownGoodPath, knownGoodIdentity)
	if err != nil {
		return fmt.Errorf("open known-good rollback source: %w", err)
	}
	size, digest, err := hashInstalledFile(source)
	if err != nil {
		_ = source.Close()
		return fmt.Errorf("hash known-good rollback source: %w", err)
	}
	if digest != expectedSHA256 {
		_ = source.Close()
		return errors.New("known-good rollback source digest does not match pending source")
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		_ = source.Close()
		return fmt.Errorf("rewind known-good rollback source: %w", err)
	}

	_, writeErr := filesystem.WriteOwnerOnlyExecutableNoReplace(ctx, candidatePath, source, size)
	closeErr := source.Close()
	if writeErr != nil || closeErr != nil {
		return errors.Join(writeErr, closeErr)
	}
	matches, err := knownGoodIdentity.Matches(knownGoodPath)
	if err != nil {
		return err
	}
	if !matches {
		return errors.New("known-good rollback source identity changed during staging")
	}
	if observeCandidateArtifact(boundary, candidateArtifactName, expectedSHA256) != artifactValid {
		return errors.New("durable rollback source candidate failed verification")
	}
	return nil
}

func (ownership *DetachedHelperOwnership) refreshTargetInspection(goos string) error {
	if ownership == nil || ownership.boundary == nil || ownership.inspection == nil {
		return errors.New("detached helper ownership is required")
	}
	refreshed, err := inspectStandaloneExecutable(ownership.inspection.ExecutablePath, goos)
	if err != nil {
		return err
	}
	if err := validateInstallationBoundaryBinding(ownership.boundary, refreshed); err != nil {
		return err
	}
	ownership.inspection = refreshed
	return nil
}
