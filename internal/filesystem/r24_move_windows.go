//go:build windows

package filesystem

import "golang.org/x/sys/windows"

func nativeMovePathNoReplace(source, destination string) error {
	return moveFileEx(source, destination, windows.MOVEFILE_WRITE_THROUGH)
}
