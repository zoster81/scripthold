package updater

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zoster81/scripthold/internal/filesystem"
)

const (
	installationStateDirectoryName = ".scripthold-update"
	controlLockName                = "control.lock"
	useLockName                    = "use.lock"
)

// InstallationBoundary identifies the owner-only sibling state root for one
// inspected standalone executable. It contains no update transaction state.
type InstallationBoundary struct {
	Directory       string
	ControlLockPath string
	UseLockPath     string
}

func openInstallationBoundary(inspection *StandaloneInspection, create bool) (*InstallationBoundary, error) {
	if inspection == nil || inspection.ExecutablePath == "" || inspection.ParentPath == "" {
		return nil, errors.New("standalone inspection evidence is required")
	}
	parentMatches, err := inspection.ParentIdentity.Matches(inspection.ParentPath)
	if err != nil {
		return nil, fmt.Errorf("revalidate executable parent identity: %w", err)
	}
	if !parentMatches {
		return nil, errors.New("executable parent identity changed before installation-state access")
	}
	targetMatches, err := inspection.ExecutableIdentity.Matches(inspection.ExecutablePath)
	if err != nil {
		return nil, fmt.Errorf("revalidate executable identity: %w", err)
	}
	if !targetMatches {
		return nil, errors.New("executable identity changed before installation-state access")
	}

	directory := filepath.Join(inspection.ParentPath, installationStateDirectoryName)
	createdDirectory := false
	if create {
		err = os.Mkdir(directory, 0o700)
		createdDirectory = err == nil
		if err != nil && !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("create installation-state directory: %w", err)
		}
	} else {
		if _, err := os.Lstat(directory); err != nil {
			return nil, fmt.Errorf("open installation-state directory: %w", err)
		}
	}
	if createdDirectory {
		if err := filesystem.RestrictOwnerOnlyPath(directory, true); err != nil {
			return nil, fmt.Errorf("restrict installation-state directory: %w", err)
		}
	}
	if err := filesystem.ValidateOwnerOnlyPath(directory, true); err != nil {
		return nil, fmt.Errorf("validate installation-state directory: %w", err)
	}
	directoryIdentity, err := filesystem.CaptureObjectIdentity(directory)
	if err != nil {
		return nil, fmt.Errorf("capture installation-state directory identity: %w", err)
	}
	sameVolume, err := inspection.ParentIdentity.SameVolume(directoryIdentity)
	if err != nil {
		return nil, fmt.Errorf("compare installation-state volume: %w", err)
	}
	if !sameVolume {
		return nil, errors.New("installation-state directory is not on the executable filesystem")
	}

	boundary := &InstallationBoundary{
		Directory:       directory,
		ControlLockPath: filepath.Join(directory, controlLockName),
		UseLockPath:     filepath.Join(directory, useLockName),
	}
	for _, lockPath := range []string{boundary.ControlLockPath, boundary.UseLockPath} {
		if create {
			lock, err := filesystem.TryAcquireOwnerOnlyFileLock(lockPath, filesystem.LockExclusive, true)
			if err != nil {
				return nil, fmt.Errorf("create installation lock %s: %w", filepath.Base(lockPath), err)
			}
			if err := lock.Close(); err != nil {
				return nil, fmt.Errorf("close installation lock %s: %w", filepath.Base(lockPath), err)
			}
		}
		if err := filesystem.ValidateOwnerOnlyPath(lockPath, false); err != nil {
			return nil, fmt.Errorf("validate installation lock %s: %w", filepath.Base(lockPath), err)
		}
	}

	directoryMatches, err := directoryIdentity.Matches(directory)
	if err != nil {
		return nil, fmt.Errorf("revalidate installation-state directory identity: %w", err)
	}
	if !directoryMatches {
		return nil, errors.New("installation-state directory identity changed during validation")
	}
	parentMatches, err = inspection.ParentIdentity.Matches(inspection.ParentPath)
	if err != nil {
		return nil, fmt.Errorf("revalidate executable parent identity: %w", err)
	}
	if !parentMatches {
		return nil, errors.New("executable parent identity changed during installation-state validation")
	}
	targetMatches, err = inspection.ExecutableIdentity.Matches(inspection.ExecutablePath)
	if err != nil {
		return nil, fmt.Errorf("revalidate executable identity: %w", err)
	}
	if !targetMatches {
		return nil, errors.New("executable identity changed during installation-state validation")
	}
	return boundary, nil
}
