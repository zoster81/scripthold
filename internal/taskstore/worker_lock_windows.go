//go:build windows

package taskstore

import (
	"errors"

	"golang.org/x/sys/windows"
)

func tryAcquireWorkerLock(path string) (*controlLock, error) {
	lock, err := tryAcquireControlLock(path)
	if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return nil, errors.New("task worker is already running")
	}
	return lock, err
}
