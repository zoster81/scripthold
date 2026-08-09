//go:build windows

package backupstore

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

type storeLock struct {
	handle   windows.Handle
	identity windows.ByHandleFileInformation
}

func acquireStoreLock(path string) (*storeLock, error) {
	return openStoreLock(path, true)
}

func acquireExistingStoreLock(path string) (*storeLock, error) {
	return openStoreLock(path, false)
}

func openStoreLock(path string, create bool) (*storeLock, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	access := uint32(windows.GENERIC_READ | windows.GENERIC_WRITE | windows.READ_CONTROL)
	const attributes = windows.FILE_ATTRIBUTE_NORMAL | windows.FILE_FLAG_OPEN_REPARSE_POINT

	creationDisposition := uint32(windows.OPEN_EXISTING)
	if create {
		access |= windows.WRITE_DAC | windows.WRITE_OWNER
		creationDisposition = windows.CREATE_NEW
	}
	handle, err := windows.CreateFile(
		pathPtr,
		access,
		0,
		nil,
		creationDisposition,
		attributes,
		0,
	)
	created := create && err == nil
	if create && (errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS)) {
		existingAccess := uint32(windows.GENERIC_READ | windows.GENERIC_WRITE | windows.READ_CONTROL)
		handle, err = windows.CreateFile(
			pathPtr,
			existingAccess,
			0,
			nil,
			windows.OPEN_EXISTING,
			attributes,
			0,
		)
	}
	if err != nil {
		return nil, err
	}

	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || info.NumberOfLinks != 1 {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("backup store lock is not a single-link regular file")
	}
	if created {
		if err := restrictHandlePermissions(handle, false); err != nil {
			_ = windows.CloseHandle(handle)
			return nil, err
		}
	} else if err := validateHandlePermissions(handle, false); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	return &storeLock{handle: handle, identity: info}, nil
}

func isLockConflict(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}

func (lock *storeLock) validateExpected(path string, expected os.FileInfo) error {
	if lock == nil || lock.handle == 0 || lock.handle == windows.InvalidHandle || expected == nil {
		return errors.New("backup store lock acquisition identity is unavailable")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(expected, pathInfo) || expected.Mode() != pathInfo.Mode() || expected.Size() != pathInfo.Size() || !expected.ModTime().Equal(pathInfo.ModTime()) {
		return errors.New("backup store lock changed during acquisition")
	}
	return lock.validate(path)
}

func (lock *storeLock) validate(path string) error {
	if lock == nil || lock.handle == 0 || lock.handle == windows.InvalidHandle {
		return errors.New("backup store lock identity is unavailable")
	}
	var current windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(lock.handle, &current); err != nil {
		return err
	}
	if current.VolumeSerialNumber != lock.identity.VolumeSerialNumber ||
		current.FileIndexHigh != lock.identity.FileIndexHigh || current.FileIndexLow != lock.identity.FileIndexLow ||
		current.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || current.NumberOfLinks != 1 {
		return errors.New("backup store lock handle identity changed")
	}
	if err := validateHandlePermissions(lock.handle, false); err != nil {
		return err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if isLinkOrReparse(pathInfo) || !pathInfo.Mode().IsRegular() {
		return errors.New("backup store lock path identity changed")
	}
	return validatePathPermissions(path, false)
}

func (lock *storeLock) close() error {
	if lock == nil || lock.handle == 0 || lock.handle == windows.InvalidHandle {
		return nil
	}
	err := windows.CloseHandle(lock.handle)
	lock.handle = 0
	lock.identity = windows.ByHandleFileInformation{}
	return err
}
