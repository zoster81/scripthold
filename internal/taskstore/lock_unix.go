//go:build linux || darwin

package taskstore

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

type controlLock struct{ file *os.File }

func acquireControlLock(path string) (*controlLock, error) {
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
	if err := unix.Flock(fd, unix.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &controlLock{file: file}, nil
}

func (lock *controlLock) close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	fd := int(lock.file.Fd())
	err := errors.Join(unix.Flock(fd, unix.LOCK_UN), lock.file.Close())
	lock.file = nil
	return err
}
