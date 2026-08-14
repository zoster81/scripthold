//go:build windows

package backupstore

import (
	"io/fs"
	"syscall"
)

func recoveryPlatformFileIdentityStable(expected, current fs.FileInfo) bool {
	expectedData, expectedOK := expected.Sys().(*syscall.Win32FileAttributeData)
	currentData, currentOK := current.Sys().(*syscall.Win32FileAttributeData)
	if !expectedOK || !currentOK || expectedData == nil || currentData == nil {
		return false
	}
	return expectedData.CreationTime.HighDateTime == currentData.CreationTime.HighDateTime &&
		expectedData.CreationTime.LowDateTime == currentData.CreationTime.LowDateTime
}
