//go:build windows

package filesystem

import (
	"fmt"
	"os"

	"github.com/zoster81/scripthold/internal/operation"
	"golang.org/x/sys/windows"
)

func captureObjectIdentity(path string) (key, volumeKey string, isDir bool, err error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", "", false, err
	}
	isDir, err = validateIdentityKind(path, info)
	if err != nil {
		return "", "", false, err
	}

	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", "", false, err
	}
	handle, err := windows.CreateFile(
		pathPtr,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return "", "", false, err
	}
	defer windows.CloseHandle(handle)

	var details windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &details); err != nil {
		return "", "", false, err
	}
	if details.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return "", "", false, operation.New(operation.KindSymlinkEscape, fmt.Sprintf("reparse-point filesystem object is not allowed: %s", path))
	}
	volumeKey = fmt.Sprintf("win:%08x", details.VolumeSerialNumber)
	key = fmt.Sprintf("%s:%08x%08x", volumeKey, details.FileIndexHigh, details.FileIndexLow)
	return key, volumeKey, isDir, nil
}
