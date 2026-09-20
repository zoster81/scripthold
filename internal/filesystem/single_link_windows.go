//go:build windows

package filesystem

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

func captureSingleLinkFileIdentity(path string) (key, volumeKey string, err error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", "", err
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return "", "", err
	}
	defer windows.CloseHandle(handle)

	var details windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &details); err != nil {
		return "", "", err
	}
	if details.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return "", "", errors.New("filesystem entry is linked or not a regular file")
	}
	if details.NumberOfLinks != 1 {
		return "", "", errors.New("filesystem entry has multiple hard links")
	}
	volumeKey = fmt.Sprintf("win:%08x", details.VolumeSerialNumber)
	key = fmt.Sprintf("%s:%08x%08x", volumeKey, details.FileIndexHigh, details.FileIndexLow)
	return key, volumeKey, nil
}
