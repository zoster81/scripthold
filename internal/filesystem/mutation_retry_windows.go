//go:build windows

package filesystem

import (
	"errors"

	"golang.org/x/sys/windows"
)

func isRetryableAtomicReplaceError(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}

func tryAlternativeAtomicReplace(stagedPath, targetPath string, previousErr error) (bool, error) {
	if !isRetryableAtomicReplaceError(previousErr) {
		return false, nil
	}

	// Do not use POSIX replacement to bypass a target that denies DELETE access
	// through ACLs or incompatible sharing. The fallback is only for the narrower
	// MoveFileEx limitation where the target is otherwise deletable but an open
	// handle still prevents classic replacement semantics.
	probe := probeWindowsAtomicReplacePath(targetPath, true)
	if !probe.Exists || !probe.DeleteAccessGranted {
		return false, nil
	}

	err := replacePathWithOpenDestination(stagedPath, targetPath)
	if err == nil {
		return true, nil
	}
	if isUnsupportedOpenDestinationReplaceError(err) {
		return false, nil
	}
	return true, err
}
