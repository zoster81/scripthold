//go:build linux || darwin

package deferredoperation

import (
	"context"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

type storeLock struct{ file *os.File }

func acquireStoreLock(ctx context.Context, path string, wait bool) (*storeLock, error) {
	ctx = nonNilContext(ctx)
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, os.ErrInvalid
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, err
	}
	deadline := time.Now().Add(storeLockWaitMaximum)
	mode := unix.LOCK_EX | unix.LOCK_NB
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		err = unix.Flock(fd, mode)
		if err == nil {
			return &storeLock{file: file}, nil
		}
		if !wait || (!errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN)) || time.Now().After(deadline) {
			_ = file.Close()
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
			_ = file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (lock *storeLock) close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	fd := int(lock.file.Fd())
	err := errors.Join(unix.Flock(fd, unix.LOCK_UN), lock.file.Close())
	lock.file = nil
	return err
}
