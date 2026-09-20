//go:build windows

package filesystem

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func openVerifiedSingleLinkFile(path string, expected ObjectIdentity) (*os.File, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	var details windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &details); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	if details.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("filesystem entry is linked or not a regular file")
	}
	if details.NumberOfLinks != 1 {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("filesystem entry has multiple hard links")
	}
	volumeKey := fmt.Sprintf("win:%08x", details.VolumeSerialNumber)
	key := fmt.Sprintf("%s:%08x%08x", volumeKey, details.FileIndexHigh, details.FileIndexLow)
	if expected.key != key || expected.volumeKey != volumeKey {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("opened filesystem entry does not match expected identity")
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, os.ErrInvalid
	}
	return file, nil
}
