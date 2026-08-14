//go:build darwin

package backupstore

import (
	"io/fs"
	"syscall"
)

func recoveryPlatformFileIdentityStable(expected, current fs.FileInfo) bool {
	expectedData, expectedOK := expected.Sys().(*syscall.Stat_t)
	currentData, currentOK := current.Sys().(*syscall.Stat_t)
	if !expectedOK || !currentOK || expectedData == nil || currentData == nil {
		return false
	}
	return expectedData.Ctimespec.Sec == currentData.Ctimespec.Sec &&
		expectedData.Ctimespec.Nsec == currentData.Ctimespec.Nsec &&
		expectedData.Birthtimespec.Sec == currentData.Birthtimespec.Sec &&
		expectedData.Birthtimespec.Nsec == currentData.Birthtimespec.Nsec
}
