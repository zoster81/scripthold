//go:build linux || darwin

package releasearchive

import (
	"errors"
	"os"
	"syscall"
)

func verifyPrivateStagingRoot(info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return errors.New("directory identity unavailable")
	}
	if stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0o077 != 0 {
		return errors.New("staging root is not owned by the active user with private permissions")
	}
	return nil
}

func verifySingleLinkStagingFile(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil || stat.Nlink != 1 {
		return errors.New("staging file must have exactly one hard link")
	}
	return nil
}
