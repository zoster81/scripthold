//go:build linux

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
	return expectedData.Ctim.Sec == currentData.Ctim.Sec && expectedData.Ctim.Nsec == currentData.Ctim.Nsec
}
