//go:build windows

package taskstore

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sys/windows"
)

type controlLock struct{ handle windows.Handle }

func acquireControlLock(path string) (*controlLock, error) {
	return acquireWindowsControlLock(context.Background(), path, true)
}

func acquireControlLockContext(ctx context.Context, path string) (*controlLock, error) {
	return acquireWindowsControlLock(ctx, path, true)
}

func tryAcquireControlLock(path string) (*controlLock, error) {
	return acquireWindowsControlLock(context.Background(), path, false)
}

func acquireWindowsControlLock(ctx context.Context, path string, wait bool) (*controlLock, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(30 * time.Second)
	var handle windows.Handle
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		handle, err = windows.CreateFile(ptr, windows.GENERIC_READ|windows.GENERIC_WRITE,
			0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err == nil {
			break
		}
		if (!errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_LOCK_VIOLATION)) || !wait || time.Now().After(deadline) {
			return nil, err
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
