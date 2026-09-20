//go:build linux || darwin

package filesystem

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func openVerifiedSingleLinkFile(path string, expected ObjectIdentity) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, os.ErrInvalid
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("filesystem entry is not a regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		_ = file.Close()
		return nil, errors.New("stable filesystem identity is unavailable")
	}
	if stat.Nlink != 1 {
		_ = file.Close()
		return nil, errors.New("filesystem entry has multiple hard links")
	}
	volumeKey := fmt.Sprintf("unix:%v", stat.Dev)
	key := fmt.Sprintf("%s:%v", volumeKey, stat.Ino)
	if expected.key != key || expected.volumeKey != volumeKey {
		_ = file.Close()
		return nil, errors.New("opened filesystem entry does not match expected identity")
	}
	return file, nil
}
