//go:build windows

package filesystem

import (
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/windows"
)

func replacePath(source, destination string) error {
	return moveFileEx(source, destination, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

func installPathNoReplace(source, destination string) error {
	return moveFileEx(source, destination, windows.MOVEFILE_WRITE_THROUGH)
}

func movePathNoReplace(source, destination string) error {
	return moveFileEx(source, destination, windows.MOVEFILE_WRITE_THROUGH)
}

func moveFileEx(source, destination string, flags uint32) error {
	sourcePtr, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	destinationPtr, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(sourcePtr, destinationPtr, flags); err != nil {
		if errno, ok := err.(syscall.Errno); ok {
			return fmt.Errorf("MoveFileEx failed (Win32 code %d): %w", uint32(errno), err)
		}
		return err
	}
	return nil
}

func syncDirectory(string) error {
	// MoveFileEx with MOVEFILE_WRITE_THROUGH flushes the rename operation before
	// returning. Windows does not expose a portable directory fsync equivalent.
	return nil
}

func isDestinationExistsError(err error) bool {
	return errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS)
}
