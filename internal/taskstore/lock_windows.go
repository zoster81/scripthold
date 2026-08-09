//go:build windows

package taskstore

import (
	"errors"
	"time"

	"golang.org/x/sys/windows"
)

type controlLock struct{ handle windows.Handle }

func acquireControlLock(path string) (*controlLock, error) {
	return acquireWindowsControlLock(path, true)
}

func tryAcquireControlLock(path string) (*controlLock, error) {
	return acquireWindowsControlLock(path, false)
}

func acquireWindowsControlLock(path string, wait bool) (*controlLock, error) {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(30 * time.Second)
	var handle windows.Handle
	for {
		handle, err = windows.CreateFile(ptr, windows.GENERIC_READ|windows.GENERIC_WRITE,
			0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err == nil {
			break
		}
		if (!errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_LOCK_VIOLATION)) || !wait || time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(10 * time.Millisecond)
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || info.NumberOfLinks != 1 {
		_ = windows.CloseHandle(handle)
		return nil, windows.ERROR_INVALID_DATA
	}
	if err := securePath(path, false); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	return &controlLock{handle: handle}, nil
}

func (lock *controlLock) close() error {
	if lock == nil || lock.handle == 0 || lock.handle == windows.InvalidHandle {
		return nil
	}
	err := windows.CloseHandle(lock.handle)
	lock.handle = 0
	return err
}
