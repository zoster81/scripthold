//go:build linux || darwin

package taskstore

import (
	"errors"
	"os"
	"syscall"
)

func securePath(path string, directory bool) error {
	mode := os.FileMode(0o600)
	if directory {
		mode = 0o700
	}
	return os.Chmod(path, mode)
}

func validateSecurePath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("task store path must not be a symbolic link")
	}
	if directory != info.IsDir() {
		return errors.New("task store path type is invalid")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("task store path owner does not match the process identity")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("task store path is accessible by another identity")
	}
	return nil
}
