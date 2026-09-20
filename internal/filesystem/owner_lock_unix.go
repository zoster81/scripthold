//go:build linux || darwin

package filesystem

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func restrictOwnerOnlyPath(path string, directory bool) error {
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	if directory {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Open(path, flags, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return os.ErrInvalid
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}
	if directory != info.IsDir() || (!directory && !info.Mode().IsRegular()) {
		return errors.New("owner-only path has the wrong filesystem kind")
	}
	mode := uint32(0o600)
	if directory {
		mode = 0o700
	}
	if err := unix.Fchmod(fd, mode); err != nil {
		return err
	}
	handleInfo, err := file.Stat()
	if err != nil {
		return err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(handleInfo, pathInfo) {
		return errors.New("owner-only path identity changed during restriction")
	}
	return validateOwnerOnlyPath(path, directory)
}

func validateOwnerOnlyPath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("owner-only path must not be a symlink")
	}
	if directory != info.IsDir() || (!directory && !info.Mode().IsRegular()) {
		return errors.New("owner-only path has the wrong filesystem kind")
	}
	expected := os.FileMode(0o600)
	if directory {
		expected = 0o700
	}
	if info.Mode().Perm() != expected {
		return fmt.Errorf("permissions are %04o, want %04o", info.Mode().Perm(), expected)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return errors.New("filesystem owner metadata is unavailable")
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return errors.New("filesystem owner does not match the process identity")
	}
	if !directory && stat.Nlink != 1 {
		return errors.New("owner-only regular file has multiple hard links")
	}
	return nil
}

type OwnerOnlyFileLock struct {
	file *os.File
}

func TryAcquireOwnerOnlyFileLock(path string, mode FileLockMode, create bool) (*OwnerOnlyFileLock, error) {
	if mode != LockShared && mode != LockExclusive {
		return nil, errors.New("invalid filesystem lock mode")
	}
	flags := unix.O_RDWR | unix.O_CLOEXEC | unix.O_NOFOLLOW
	fd, created, err := openUnixLockFile(path, flags, create)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, os.ErrInvalid
	}
	cleanup := func(err error) (*OwnerOnlyFileLock, error) {
		_ = file.Close()
		return nil, err
	}
	if created {
		if err := file.Chmod(0o600); err != nil {
			return cleanup(err)
		}
	}
	if err := validateUnixOwnerOnlyLockFile(file, path); err != nil {
		return cleanup(err)
	}
	flag := unix.LOCK_SH | unix.LOCK_NB
	if mode == LockExclusive {
		flag = unix.LOCK_EX | unix.LOCK_NB
	}
	if err := unix.Flock(fd, flag); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return cleanup(ErrFileLockBusy)
		}
		return cleanup(err)
	}
	lock := &OwnerOnlyFileLock{file: file}
	if err := lock.Validate(path); err != nil {
		_ = lock.Close()
		return nil, err
	}
	return lock, nil
}

func openUnixLockFile(path string, flags int, create bool) (fd int, created bool, err error) {
	if !create {
		fd, err = unix.Open(path, flags, 0)
		return fd, false, err
	}
	fd, err = unix.Open(path, flags|unix.O_CREAT|unix.O_EXCL, 0o600)
	if err == nil {
		return fd, true, nil
	}
	if !errors.Is(err, unix.EEXIST) {
		return -1, false, err
	}
	fd, err = unix.Open(path, flags, 0)
	return fd, false, err
}

func validateUnixOwnerOnlyLockFile(file *os.File, path string) error {
	if file == nil {
		return errors.New("lock handle is unavailable")
	}
	handleInfo, err := file.Stat()
	if err != nil {
		return err
	}
	if !handleInfo.Mode().IsRegular() {
		return errors.New("lock is not a regular file")
	}
	stat, ok := handleInfo.Sys().(*syscall.Stat_t)
	if !ok || stat == nil || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) || handleInfo.Mode().Perm() != 0o600 {
		return errors.New("lock metadata is not owner-only single-link state")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(handleInfo, pathInfo) {
		return errors.New("lock path identity changed")
	}
	return nil
}

// Validate confirms that path still names the locked owner-only file.
func (lock *OwnerOnlyFileLock) Validate(path string) error {
	if lock == nil || lock.file == nil {
		return errors.New("lock handle is unavailable")
	}
	return validateUnixOwnerOnlyLockFile(lock.file, path)
}

func (lock *OwnerOnlyFileLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	fd := int(lock.file.Fd())
	unlockErr := unix.Flock(fd, unix.LOCK_UN)
	closeErr := lock.file.Close()
	lock.file = nil
	return errors.Join(unlockErr, closeErr)
}

func readOwnerOnlyFileBounded(path string, maxBytes int64) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, os.ErrInvalid
	}
	defer file.Close()
	if err := validateUnixOwnerOnlyLockFile(file, path); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("owner-only file exceeds its size limit")
	}
	if err := validateUnixOwnerOnlyLockFile(file, path); err != nil {
		return nil, err
	}
	return data, nil
}
