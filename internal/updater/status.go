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

// InstallationStatus is a read-only observation of local self-update adoption
// and transaction state. It performs no network access or durable mutation.
type InstallationStatus struct {
	Adopted           bool
	State             ReconciliationStatus
	CurrentVersion    string
	CandidateVersion  string
	RecoveryAvailable bool
	Problem           string
}

// ObserveCurrentInstallationStatus inspects the running executable and its
// sibling self-update state without creating, repairing, or cleaning anything.
func ObserveCurrentInstallationStatus(ctx context.Context) (InstallationStatus, error) {
	inspection, err := InspectCurrentStandaloneExecutable()
	if err != nil {
		return InstallationStatus{}, err
	}
	return observeInstallationStatusWith(
		ctx,
		inspection,
		runtime.GOOS,
		runtime.GOARCH,
		reconciliationDeps{},
	)
}

func observeInstallationStatusWith(
	ctx context.Context,
	inspection *StandaloneInspection,
	goos, goarch string,
	deps reconciliationDeps,
) (_ InstallationStatus, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return InstallationStatus{}, err
	}
	if inspection == nil {
		return InstallationStatus{}, errors.New("standalone inspection evidence is required")
	}

	stateDirectory := filepath.Join(inspection.ParentPath, installationStateDirectoryName)
	if _, statErr := os.Lstat(stateDirectory); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return InstallationStatus{Adopted: false}, nil
		}
		return InstallationStatus{}, fmt.Errorf("inspect installation state directory: %w", statErr)
	}

	boundary, boundaryErr := openInstallationBoundary(inspection, false)
	if boundaryErr != nil {
		return InstallationStatus{
			Adopted: true,
			State:   ReconciliationRecoveryRequired,
			Problem: "installation state boundary cannot be safely validated",
		}, nil
	}
	control, err := filesystem.TryAcquireOwnerOnlyFileLock(
		boundary.ControlLockPath,
		filesystem.LockExclusive,
		false,
	)
	if err != nil {
		return InstallationStatus{}, fmt.Errorf("acquire installation status control lock: %w", err)
	}
	defer func() {
		if closeErr := control.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return InstallationStatus{}, fmt.Errorf("validate installation status control lock: %w", err)
	}

	state, stateErr := readInstallationStateForReconciliationLocked(boundary, inspection)
	if stateErr != nil {
		return InstallationStatus{
			Adopted: true,
			State:   ReconciliationRecoveryRequired,
			Problem: "installation state cannot be safely validated",
		}, nil
	}
	result, err := reconcileInstallationLocked(
		ctx,
		boundary,
		inspection,
		goos,
		goarch,
		deps,
		control,
	)
	if err != nil {
		return InstallationStatus{}, err
	}
	after, afterErr := readInstallationStateForReconciliationLocked(boundary, inspection)
	if afterErr != nil || !installationStatesEqual(state, after) {
		return InstallationStatus{
			Adopted:        true,
			State:          ReconciliationRecoveryRequired,
			CurrentVersion: state.Current.Version,
			Problem:        "installation state changed during status observation",
		}, nil
	}
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return InstallationStatus{}, fmt.Errorf("revalidate installation status control lock: %w", err)
	}

	status := InstallationStatus{
		Adopted:           true,
		State:             result.Status,
		CurrentVersion:    state.Current.Version,
		RecoveryAvailable: reconciliationRecoveryAvailable(result),
		Problem:           result.Problem,
	}
	if state.Pending != nil {
		status.CandidateVersion = state.Pending.CandidateVersion
	}
	return status, nil
}

func reconciliationRecoveryAvailable(result ReconciliationResult) bool {
	switch result.Status {
	case ReconciliationPrepared, ReconciliationCommitted, ReconciliationRolledBack:
		return true
	case ReconciliationRecoveryRequired:
		return result.candidateBytesInstalled
	default:
		return false
	}
}
