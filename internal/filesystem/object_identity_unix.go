//go:build linux || darwin

package filesystem

import (
	"fmt"
	"os"
	"syscall"
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
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return "", "", false, fmt.Errorf("stable stat identity is unavailable for %s", path)
	}
	volumeKey = fmt.Sprintf("unix:%v", stat.Dev)
	key = fmt.Sprintf("%s:%v", volumeKey, stat.Ino)
	return key, volumeKey, isDir, nil
}
