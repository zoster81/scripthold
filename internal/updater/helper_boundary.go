package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/security"
)

const (
	detachedHelperCommand            = "_self-update-helper"
	detachedHelperControlWaitTimeout = 5 * time.Second
	detachedHelperUseWaitTimeout     = 30 * time.Second
	detachedHelperLockRetryDelay     = 25 * time.Millisecond
)

// ErrDetachedHelperUseTimeout reports that existing normal processes did not
// release shared installation-use authority within the helper's bounded wait.
var (
	ErrDetachedHelperControlTimeout = errors.New("detached self-update helper timed out waiting for installation control")
	ErrDetachedHelperUseTimeout     = errors.New("detached self-update helper timed out waiting for installation use")
)

type helperOwnershipDeps struct {
	reconciliation     reconciliationDeps
	controlWaitTimeout time.Duration
}

// DetachedHelperOwnership holds exclusive local authority for one prepared
// update transaction. Acquiring it never changes the installed executable.
type DetachedHelperOwnership struct {
	boundary       *InstallationBoundary
	inspection     *StandaloneInspection
	reconciliation ReconciliationResult
	controlLock    *filesystem.OwnerOnlyFileLock
	useLock        *filesystem.OwnerOnlyFileLock
}

// Close releases helper use authority before its control authority.
func (ownership *DetachedHelperOwnership) Close() error {
	if ownership == nil {
		return nil
	}
	var err error
	if ownership.useLock != nil {
		err = errors.Join(err, ownership.useLock.Close())
		ownership.useLock = nil
	}
	if ownership.controlLock != nil {
		err = errors.Join(err, ownership.controlLock.Close())
		ownership.controlLock = nil
	}
	return err
}

// DetachedHelperArguments returns the private invocation contract used when a
// copied helper process is launched by a later self-update increment.
func DetachedHelperArguments(transactionID string) ([]string, error) {
	if err := validateTransactionID(transactionID); err != nil {
		return nil, err
	}
	return []string{detachedHelperCommand, transactionID}, nil
}

// ParseDetachedHelperInvocation recognizes and strictly validates the private
// helper invocation contract without accepting paths or other authority inputs.
func ParseDetachedHelperInvocation(args []string) (transactionID string, matched bool, err error) {
	if len(args) == 0 || args[0] != detachedHelperCommand {
		return "", false, nil
	}
	if len(args) != 2 {
		return "", true, errors.New("invalid detached self-update helper invocation")
	}
	if err := validateTransactionID(args[1]); err != nil {
		return "", true, err
	}
	return args[1], true, nil
}

// AcquireCurrentDetachedHelperOwnership reopens the installation from the
// currently running helper copy, verifies its pending transaction, and waits
// for exclusive use authority. It never replaces or rolls back the target.
func AcquireCurrentDetachedHelperOwnership(ctx context.Context, transactionID string) (*DetachedHelperOwnership, error) {
	path, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve helper executable: %w", err)
	}
	return acquireDetachedHelperOwnershipWith(
		ctx,
		path,
		transactionID,
		runtime.GOOS,
		runtime.GOARCH,
		detachedHelperUseWaitTimeout,
		helperOwnershipDeps{},
	)
}

func acquireDetachedHelperOwnershipWith(
	ctx context.Context,
	helperExecutablePath, transactionID, goos, goarch string,
	useWaitTimeout time.Duration,
	deps helperOwnershipDeps,
) (_ *DetachedHelperOwnership, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateTransactionID(transactionID); err != nil {
		return nil, err
	}
	if useWaitTimeout <= 0 {
		return nil, errors.New("detached helper use-lock timeout must be positive")
	}

	boundary, inspection, err := reopenDetachedHelperInstallation(helperExecutablePath, transactionID, goos)
	if err != nil {
		return nil, err
	}

	controlWaitTimeout := deps.controlWaitTimeout
	if controlWaitTimeout <= 0 {
		controlWaitTimeout = detachedHelperControlWaitTimeout
	}
	control, err := waitForExclusiveFileLock(
		ctx,
		boundary.ControlLockPath,
		controlWaitTimeout,
		ErrDetachedHelperControlTimeout,
		"installation control lock",
	)
	if err != nil {
		return nil, err
	}
	releaseControl := true
	defer func() {
		if releaseControl {
			err = errors.Join(err, control.Close())
		}
	}()
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return nil, fmt.Errorf("validate installation control lock: %w", err)
	}
	if err := validateHelperTransactionLocked(boundary, inspection, transactionID); err != nil {
		return nil, err
	}

	reconciliation, err := reconcileInstallationLocked(
		ctx, boundary, inspection, goos, goarch, deps.reconciliation, control,
	)
	if err != nil {
		return nil, err
	}
	if reconciliation.Status != ReconciliationPrepared {
		return nil, fmt.Errorf("detached helper requires prepared transaction, observed %s", reconciliation.Status)
	}

	useLock, err := waitForExclusiveFileLock(
		ctx,
		boundary.UseLockPath,
		useWaitTimeout,
		ErrDetachedHelperUseTimeout,
		"installation use lock",
	)
	if err != nil {
		return nil, err
	}
	releaseUse := true
	defer func() {
		if releaseUse {
			err = errors.Join(err, useLock.Close())
		}
	}()

	if err := useLock.Validate(boundary.UseLockPath); err != nil {
		return nil, fmt.Errorf("validate exclusive installation use lock: %w", err)
	}
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return nil, fmt.Errorf("revalidate installation control lock: %w", err)
	}
	if err := validateHelperTransactionLocked(boundary, inspection, transactionID); err != nil {
		return nil, err
	}
	reconciliation, err = reconcileInstallationLocked(
		ctx, boundary, inspection, goos, goarch, deps.reconciliation, control,
	)
	if err != nil {
		return nil, err
	}
	if reconciliation.Status != ReconciliationPrepared {
		return nil, fmt.Errorf("transaction changed while helper waited for use authority: %s", reconciliation.Status)
	}
	if err := useLock.Validate(boundary.UseLockPath); err != nil {
		return nil, fmt.Errorf("revalidate exclusive installation use lock: %w", err)
	}
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return nil, fmt.Errorf("revalidate installation control lock after use acquisition: %w", err)
	}

	releaseUse = false
	releaseControl = false
	return &DetachedHelperOwnership{
		boundary:       boundary,
		inspection:     inspection,
		reconciliation: reconciliation,
		controlLock:    control,
		useLock:        useLock,
	}, nil
}

func reopenDetachedHelperInstallation(
	helperExecutablePath, transactionID, goos string,
) (*InstallationBoundary, *StandaloneInspection, error) {
	if helperExecutablePath == "" {
		return nil, nil, errors.New("helper executable path is empty")
	}
	absolute, err := filepath.Abs(helperExecutablePath)
	if err != nil {
		return nil, nil, fmt.Errorf("make helper path absolute: %w", err)
	}
	absolute = filepath.Clean(absolute)
	if !platformPathNameEqual(filepath.Base(absolute), helperArtifactName, goos) {
		return nil, nil, errors.New("detached helper is not running from the fixed helper artifact")
	}
	stateDirectory := filepath.Dir(absolute)
	if !platformPathNameEqual(filepath.Base(stateDirectory), installationStateDirectoryName, goos) {
		return nil, nil, errors.New("detached helper is outside the installation state directory")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve helper path: %w", err)
	}
	if !security.PathsEqual(absolute, filepath.Clean(resolved)) {
		return nil, nil, errors.New("detached helper path is linked or aliased")
	}
	if err := filesystem.ValidateOwnerOnlyPath(stateDirectory, true); err != nil {
		return nil, nil, fmt.Errorf("validate helper state directory: %w", err)
	}
	if err := filesystem.ValidateOwnerOnlyExecutable(absolute); err != nil {
		return nil, nil, fmt.Errorf("validate helper executable: %w", err)
	}

	statePayload, err := filesystem.ReadOwnerOnlyFileBounded(
		filepath.Join(stateDirectory, installationStateFileName),
		maxInstallationStateBytes,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("read helper installation state: %w", err)
	}
	state, err := decodeInstallationState(statePayload)
	if err != nil {
		return nil, nil, err
	}
	if state.Pending == nil || state.Pending.TransactionID != transactionID {
		return nil, nil, errors.New("detached helper transaction does not match pending state")
	}

	expectedParent := filepath.Dir(stateDirectory)
	if !security.PathsEqual(filepath.Dir(state.Target.Path), expectedParent) {
		return nil, nil, errors.New("installation target is not a sibling of helper state")
	}
	inspection, err := inspectStandaloneExecutable(state.Target.Path, goos)
	if err != nil {
		return nil, nil, err
	}
	if !security.PathsEqual(inspection.ParentPath, expectedParent) {
		return nil, nil, errors.New("inspected target parent does not match helper state parent")
	}
	boundary, err := openInstallationBoundary(inspection, false)
	if err != nil {
		return nil, nil, err
	}
	if !security.PathsEqual(boundary.Directory, stateDirectory) {
		return nil, nil, errors.New("helper state directory does not match installation boundary")
	}
	if err := validateInstallationState(state, inspection); err != nil {
		return nil, nil, fmt.Errorf("validate prepared helper state: %w", err)
	}
	if observeFixedArtifact(boundary, helperArtifactName, state.Current.SHA256) != artifactValid {
		return nil, nil, errors.New("helper artifact no longer matches source evidence")
	}
	return boundary, inspection, nil
}

func validateHelperTransactionLocked(
	boundary *InstallationBoundary,
	inspection *StandaloneInspection,
	transactionID string,
) error {
	state, err := readInstallationStateForReconciliationLocked(boundary, inspection)
	if err != nil {
		return err
	}
	if state.Pending == nil || state.Pending.TransactionID != transactionID {
		return errors.New("detached helper transaction changed or disappeared")
	}
	return nil
}

func waitForExclusiveFileLock(
	ctx context.Context,
	path string,
	timeout time.Duration,
	timeoutErr error,
	label string,
) (*filesystem.OwnerOnlyFileLock, error) {
	if timeout <= 0 {
		return nil, errors.New("exclusive lock wait timeout must be positive")
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lock, err := filesystem.TryAcquireOwnerOnlyFileLock(path, filesystem.LockExclusive, false)
		switch {
		case err == nil:
			return lock, nil
		case !errors.Is(err, filesystem.ErrFileLockBusy):
			return nil, fmt.Errorf("acquire exclusive %s: %w", label, err)
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, timeoutErr
		}
		delay := detachedHelperLockRetryDelay
		if remaining < delay {
			delay = remaining
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
