//go:build windows

package deferredoperation

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sys/windows"
)

type storeLock struct{ handle windows.Handle }

func acquireStoreLock(ctx context.Context, path string, wait bool) (*storeLock, error) {
	ctx = nonNilContext(ctx)
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(storeLockWaitMaximum)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		handle, openErr := windows.CreateFile(ptr, windows.GENERIC_READ|windows.GENERIC_WRITE,
			0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if openErr == nil {
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
			return &storeLock{handle: handle}, nil
		}
		if (!errors.Is(openErr, windows.ERROR_SHARING_VIOLATION) && !errors.Is(openErr, windows.ERROR_LOCK_VIOLATION)) || !wait || time.Now().After(deadline) {
			return nil, openErr
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (lock *storeLock) close() error {
	if lock == nil || lock.handle == 0 || lock.handle == windows.InvalidHandle {
		return nil
	}
	err := windows.CloseHandle(lock.handle)
	lock.handle = 0
	return err
}
