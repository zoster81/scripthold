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

// LaunchCurrentRecoveryHelper dispatches explicit recovery through the fixed
// helper copy. It performs no recovery mutation in the caller process.
// A true result means the detached helper process was started, even if
// launcher cleanup subsequently reports an error.
func LaunchCurrentRecoveryHelper(ctx context.Context) (bool, error) {
	inspection, err := InspectCurrentStandaloneExecutable()
	if err != nil {
		return false, err
	}
	boundary, err := openInstallationBoundary(inspection, false)
	if err != nil {
		return false, err
	}
	return launchRecoveryHelperWith(
		ctx,
		boundary,
		inspection,
		runtime.GOOS,
		runtime.GOARCH,
		helperLaunchDeps{},
	)
}

func launchRecoveryHelperWith(
	ctx context.Context,
	boundary *InstallationBoundary,
	inspection *StandaloneInspection,
	goos, goarch string,
	deps helperLaunchDeps,
) (started bool, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := validateInstallationBoundaryBinding(boundary, inspection); err != nil {
		return false, err
	}
	if deps.start == nil {
		deps.start = startDetachedSelfUpdateHelper
	}

	control, err := filesystem.TryAcquireOwnerOnlyFileLock(
		boundary.ControlLockPath,
		filesystem.LockExclusive,
		false,
	)
	if err != nil {
		return false, fmt.Errorf("acquire recovery launch control lock: %w", err)
	}
	releaseControl := true
	defer func() {
		if releaseControl {
			err = errors.Join(err, control.Close())
		}
	}()
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return false, fmt.Errorf("validate recovery launch control lock: %w", err)
	}

	state, err := readInstallationStateForReconciliationLocked(boundary, inspection)
	if err != nil {
		return false, err
	}
	if state.Pending == nil {
		return false, errors.New("explicit recovery requires a pending self-update transaction")
	}
	transactionID := state.Pending.TransactionID

	result, err := reconcileInstallationLocked(
		ctx,
		boundary,
		inspection,
		goos,
		goarch,
		deps.reconciliation,
		control,
	)
	if err != nil {
		return false, err
	}
	if err := validateHelperOwnershipState(helperOwnershipRecovery, result); err != nil {
		return false, err
	}
	if observeFixedArtifact(boundary, helperArtifactName, state.Pending.SourceSHA256) != artifactValid {
		return false, errors.New("recovery helper artifact no longer matches source evidence")
	}

	useLock, err := filesystem.TryAcquireOwnerOnlyFileLock(
		boundary.UseLockPath,
		filesystem.LockShared,
		false,
	)
	if err != nil {
		return false, fmt.Errorf("acquire recovery launcher use lock: %w", err)
	}
	releaseUse := true
	defer func() {
		if releaseUse {
			err = errors.Join(err, useLock.Close())
		}
	}()

	if err := useLock.Validate(boundary.UseLockPath); err != nil {
		return false, fmt.Errorf("validate recovery launcher use lock: %w", err)
	}
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return false, fmt.Errorf("revalidate recovery launch control lock: %w", err)
	}
	if err := validateHelperTransactionLocked(boundary, inspection, transactionID); err != nil {
		return false, err
	}
	result, err = reconcileInstallationLocked(
		ctx,
		boundary,
		inspection,
		goos,
		goarch,
		deps.reconciliation,
		control,
	)
	if err != nil {
		return false, err
	}
	if err := validateHelperOwnershipState(helperOwnershipRecovery, result); err != nil {
		return false, fmt.Errorf("recovery state changed before helper dispatch: %w", err)
	}

	args, err := RecoveryHelperArguments(transactionID)
	if err != nil {
		return false, err
	}
	helperPath := filepath.Join(boundary.Directory, helperArtifactName)
	if err := filesystem.ValidateOwnerOnlyExecutable(helperPath); err != nil {
		return false, fmt.Errorf("validate recovery helper executable: %w", err)
	}
	releaseProcess, err := deps.start(
		helperPath,
		args,
		minimalSelfUpdateHelperEnvironment(os.Environ()),
	)
	if err != nil {
		return false, fmt.Errorf("start detached self-update recovery helper: %w", err)
	}
	if releaseProcess == nil {
		return true, errors.New("detached recovery helper started without a releasable process handle")
	}

	started = true
	releaseControl = false
	controlErr := control.Close()
	releaseUse = false
	useErr := useLock.Close()
	processErr := releaseProcess()
	return true, errors.Join(controlErr, useErr, processErr)
}
