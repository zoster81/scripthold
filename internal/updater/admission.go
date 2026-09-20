package updater

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/zoster81/scripthold/internal/filesystem"
)

// ProcessAdmission holds shared installation-use authority for one normal process.
type ProcessAdmission struct {
	useLock     *filesystem.OwnerOnlyFileLock
	useLockPath string
}

// Close releases shared installation-use authority.
func (admission *ProcessAdmission) Close() error {
	if admission == nil || admission.useLock == nil {
		return nil
	}
	err := admission.useLock.Close()
	admission.useLock = nil
	admission.useLockPath = ""
	return err
}

func (admission *ProcessAdmission) validateFor(boundary *InstallationBoundary) error {
	if admission == nil || admission.useLock == nil || boundary == nil ||
		admission.useLockPath != boundary.UseLockPath {
		return errors.New("valid process admission is required")
	}
	return admission.useLock.Validate(boundary.UseLockPath)
}

func admitCurrentProcess(ctx context.Context, boundary *InstallationBoundary, inspection *StandaloneInspection) (*ProcessAdmission, error) {
	return admitStableProcessWith(ctx, boundary, inspection, runtime.GOOS, runtime.GOARCH, installedEvidenceDeps{})
}

func admitStableProcessWith(
	ctx context.Context,
	boundary *InstallationBoundary,
	inspection *StandaloneInspection,
	goos, goarch string,
	deps installedEvidenceDeps,
) (_ *ProcessAdmission, err error) {
	if err := validateInstallationBoundary(boundary, inspection); err != nil {
		return nil, err
	}
	control, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false)
	if err != nil {
		return nil, fmt.Errorf("acquire installation control lock: %w", err)
	}
	defer func() {
		if closeErr := control.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return nil, fmt.Errorf("validate installation control lock: %w", err)
	}

	state, err := readInstallationStateLocked(boundary, inspection)
	if err != nil {
		return nil, err
	}
	if state.Pending != nil {
		return nil, errors.New("pending update transaction blocks normal process admission")
	}
	observed, err := observeInstalledBinary(ctx, inspection, goos, goarch, deps)
	if err != nil {
		return nil, err
	}
	if observed != state.Current {
		return nil, errors.New("installed executable does not match verified current state")
	}
	if err := cleanupFinalizedTransactionLocked(boundary, inspection, state, control); err != nil {
		return nil, fmt.Errorf("cleanup finalized update transaction: %w", err)
	}
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return nil, fmt.Errorf("revalidate installation control lock: %w", err)
	}
	if err := validateInstallationBoundary(boundary, inspection); err != nil {
		return nil, err
	}

	useLock, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.UseLockPath, filesystem.LockShared, false)
	if err != nil {
		return nil, fmt.Errorf("acquire shared installation use lock: %w", err)
	}
	if err := useLock.Validate(boundary.UseLockPath); err != nil {
		_ = useLock.Close()
		return nil, fmt.Errorf("validate shared installation use lock: %w", err)
	}
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		_ = useLock.Close()
		return nil, fmt.Errorf("revalidate control lock after use admission: %w", err)
	}
	return &ProcessAdmission{useLock: useLock, useLockPath: boundary.UseLockPath}, nil
}
