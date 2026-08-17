//go:build windows

package filesystem

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func replacePath(source, destination string) error {
	return moveFileEx(source, destination, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

// windowsFileRenameInfoPrefix mirrors FILE_RENAME_INFO through the first UTF-16
// code unit. FileName is variable-length, so callers allocate a byte buffer
// starting at unsafe.Offsetof(FileName) rather than unsafe.Sizeof this struct.
type windowsFileRenameInfoPrefix struct {
	Flags          uint32
	RootDirectory  windows.Handle
	FileNameLength uint32
	FileName       uint16
}

func replacePathWithOpenDestination(source, destination string) error {
	sourcePtr, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	destinationUTF16, err := windows.UTF16FromString(destination)
	if err != nil {
		return err
	}
	if len(destinationUTF16) <= 1 {
		return windows.ERROR_INVALID_NAME
	}

	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	handle, err := windows.CreateFile(
		sourcePtr,
		windowsDeleteAccess,
		share,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)

	var layout windowsFileRenameInfoPrefix
	nameOffset := unsafe.Offsetof(layout.FileName)
	nameUnits := destinationUTF16[:len(destinationUTF16)-1]
	buffer := make([]byte, int(nameOffset)+len(nameUnits)*2)
	info := (*windowsFileRenameInfoPrefix)(unsafe.Pointer(&buffer[0]))
	info.Flags = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	info.FileNameLength = uint32(len(nameUnits) * 2)
	encodedName := unsafe.Slice((*uint16)(unsafe.Pointer(&buffer[int(nameOffset)])), len(nameUnits))
	copy(encodedName, nameUnits)

	if err := windows.SetFileInformationByHandle(handle, windows.FileRenameInfoEx, &buffer[0], uint32(len(buffer))); err != nil {
		if errno, ok := err.(syscall.Errno); ok {
			return fmt.Errorf("SetFileInformationByHandle(FileRenameInfoEx) failed (Win32 code %d): %w", uint32(errno), err)
		}
		return err
	}
	return nil
}

func isUnsupportedOpenDestinationReplaceError(err error) bool {
	return errors.Is(err, windows.ERROR_INVALID_PARAMETER) ||
		errors.Is(err, windows.ERROR_NOT_SUPPORTED) ||
		errors.Is(err, windows.ERROR_INVALID_FUNCTION) ||
		errors.Is(err, windows.ERROR_CALL_NOT_IMPLEMENTED) ||
		errors.Is(err, windows.ERROR_PROC_NOT_FOUND)
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
