//go:build windows

package diagnostics

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

func validateLogDirectoryPlatform(info os.FileInfo) error {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return errors.New("diagnostics directory metadata is unavailable")
	}
	if data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("diagnostics directory must not be a reparse point")
	}
	return nil
}
