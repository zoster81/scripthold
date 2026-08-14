//go:build !windows && !linux && !darwin

package backupstore

import "io/fs"

func recoveryPlatformFileIdentityStable(expected, current fs.FileInfo) bool {
	return expected != nil && current != nil
}
