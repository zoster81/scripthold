package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/zoster81/scripthold/internal/filesystem"
)

// FinalizeTransaction durably publishes one verified committed or rolled-back
// transaction as stable state. Artifact cleanup is intentionally deferred to a
// later normal-process admission so the detached helper never has to delete
// its own executable.
func (ownership *DetachedHelperOwnership) FinalizeTransaction(ctx context.Context) error {
	return ownership.finalizeTransactionWith(ctx, runtime.GOOS, runtime.GOARCH, reconciliationDeps{})
}

func (ownership *DetachedHelperOwnership) finalizeTransactionWith(
	ctx context.Context,
	goos, goarch string,
	deps reconciliationDeps,
) error {
	if ownership == nil || ownership.controlLock == nil || ownership.useLock == nil ||
		ownership.boundary == nil || ownership.inspection == nil {
		return errors.New("detached helper ownership is required")
	}
	if err := ownership.controlLock.Validate(ownership.boundary.ControlLockPath); err != nil {
		return fmt.Errorf("validate finalization control lock: %w", err)
	}
	if err := ownership.useLock.Validate(ownership.boundary.UseLockPath); err != nil {
		return fmt.Errorf("validate finalization use lock: %w", err)
	}
	if err := ownership.refreshTargetInspection(goos); err != nil {
		return fmt.Errorf("refresh finalization target inspection: %w", err)
	}

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
	if result.Status == ReconciliationStable {
		state, readErr := readInstallationStateLocked(ownership.boundary, ownership.inspection)
		if readErr != nil {
			return readErr
		}
		if state.Terminal != nil {
			return nil
		}
		return errors.New("stable installation has no finalized transaction evidence")
	}
	if result.Status != ReconciliationCommitted && result.Status != ReconciliationRolledBack {
		return fmt.Errorf("transaction finalization requires committed or rolled-back state, observed %s", result.Status)
	}
	if result.Status == ReconciliationCommitted && result.rollbackPrepared {
		return errors.New("cannot finalize committed success while rollback source is staged")
	}

	existing, err := readInstallationStateForReconciliationLocked(ownership.boundary, ownership.inspection)
	if err != nil {
		return err
	}
	if existing.Pending == nil {
		return errors.New("transaction finalization requires pending evidence")
	}
	pending := *existing.Pending

	next := existing
	next.Target = targetStateFromInspection(ownership.inspection)
	next.Pending = nil
	switch result.Status {
	case ReconciliationCommitted:
		next.Current = installationCurrentState{
			Version: pending.CandidateVersion,
			SHA256:  pending.CandidateSHA256,
			Build:   pending.CandidateBuild,
		}
		next.Terminal = &installationTerminalState{
			TransactionID:  pending.TransactionID,
			Outcome:        terminalOutcomeCommitted,
			SourceSHA256:   pending.SourceSHA256,
			CleanupPending: true,
		}
	case ReconciliationRolledBack:
		next.Current = existing.Current
		next.Terminal = &installationTerminalState{
			TransactionID:  pending.TransactionID,
			Outcome:        terminalOutcomeRolledBack,
			SourceSHA256:   pending.SourceSHA256,
			CleanupPending: true,
		}
	}
	if err := persistReconciledStableStateLocked(
		ownership.boundary,
		ownership.inspection,
		next,
		existing,
	); err != nil {
		return fmt.Errorf("publish finalized stable installation state: %w", err)
	}

	result, err = reconcileInstallationLocked(
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
	if result.Status != ReconciliationStable {
		return fmt.Errorf("finalized transaction did not reconcile stable: %s", result.Status)
	}
	ownership.reconciliation = result
	return nil
}

func targetStateFromInspection(inspection *StandaloneInspection) installationTargetState {
	return installationTargetState{
		Path:           inspection.ExecutablePath,
		Identity:       inspection.ExecutableIdentity.StableKey(),
		Volume:         inspection.ExecutableIdentity.VolumeKey(),
		ParentIdentity: inspection.ParentIdentity.StableKey(),
		ParentVolume:   inspection.ParentIdentity.VolumeKey(),
	}
}

func cleanupFinalizedTransactionLocked(
	boundary *InstallationBoundary,
	inspection *StandaloneInspection,
	state installationState,
	control *filesystem.OwnerOnlyFileLock,
) error {
	if state.Pending != nil || state.Terminal == nil || !state.Terminal.CleanupPending {
		return nil
	}
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return fmt.Errorf("validate finalization cleanup control lock: %w", err)
	}
	if err := validateInstallationState(state, inspection); err != nil {
		return err
	}

	candidatePath := filepath.Join(boundary.Directory, candidateArtifactName)
	if _, err := os.Lstat(candidatePath); err == nil {
		return errors.New("finalized transaction candidate artifact unexpectedly exists")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect finalized candidate artifact: %w", err)
	}

	for _, name := range []string{knownGoodArtifactName, helperArtifactName} {
		if err := removeFinalizedArtifact(
			boundary,
			name,
			state.Terminal.SourceSHA256,
		); err != nil {
			return err
		}
	}
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return fmt.Errorf("revalidate finalization cleanup control lock: %w", err)
	}

	next := state
	terminal := *state.Terminal
	terminal.CleanupPending = false
	next.Terminal = &terminal
	if err := persistInstallationStateLocked(boundary, inspection, next, false, &state); err != nil {
		return fmt.Errorf("persist completed finalization cleanup: %w", err)
	}
	return nil
}

func removeFinalizedArtifact(
	boundary *InstallationBoundary,
	name, expectedSHA256 string,
) error {
	path := filepath.Join(boundary.Directory, name)
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect finalized artifact %s: %w", name, err)
	}
	if err := filesystem.ValidateOwnerOnlyExecutable(path); err != nil {
		return fmt.Errorf("validate finalized artifact %s: %w", name, err)
	}
	snapshot, err := filesystem.CaptureRegularFileSnapshotBounded(
		context.Background(),
		path,
		maxSelfUpdateAssetBytes,
	)
	if err != nil {
		return fmt.Errorf("snapshot finalized artifact %s: %w", name, err)
	}
	digest, ok := snapshot.ContentDigest()
	if !ok || fmt.Sprintf("%x", digest) != expectedSHA256 {
		return fmt.Errorf("finalized artifact %s does not match cleanup evidence", name)
	}
	if err := filesystem.RemoveFile(path, &snapshot); err != nil {
		return fmt.Errorf("remove finalized artifact %s: %w", name, err)
	}
	return nil
}
