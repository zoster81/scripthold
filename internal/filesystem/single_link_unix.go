//go:build linux || darwin

package filesystem

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func captureSingleLinkFileIdentity(path string) (key, volumeKey string, err error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return "", "", err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return "", "", os.ErrInvalid
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", "", err
	}
	if !info.Mode().IsRegular() {
		return "", "", errors.New("filesystem entry is not a regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return "", "", errors.New("stable filesystem identity is unavailable")
	}
	if stat.Nlink != 1 {
		return "", "", errors.New("filesystem entry has multiple hard links")
	}
	volumeKey = fmt.Sprintf("unix:%v", stat.Dev)
	key = fmt.Sprintf("%s:%v", volumeKey, stat.Ino)
	return key, volumeKey, nil
}
