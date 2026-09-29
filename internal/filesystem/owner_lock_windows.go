//go:build windows

package filesystem

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsOwnerFileAllAccess windows.ACCESS_MASK = 0x001F01FF

type ownerACLHeader struct {
	revision  byte
	reserved  byte
	size      uint16
	aceCount  uint16
	reserved2 uint16
}

func restrictOwnerOnlyPath(path string, directory bool) error {
	handle, err := openOwnerOnlyMetadataHandle(path, directory, true)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	if err := restrictOwnerOnlyHandleKind(handle, directory); err != nil {
		return err
	}
	return validateOwnerOnlyPath(path, directory)
}

func validateOwnerOnlyPath(path string, directory bool) error {
	handle, err := openOwnerOnlyMetadataHandle(path, directory, false)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	if err := validateOwnerOnlyHandleKind(handle, directory); err != nil {
		return err
	}
	var details windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &details); err != nil {
		return err
	}
	if details.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("owner-only path must not be a reparse point")
	}
	if directory {
		if details.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
			return errors.New("owner-only path is not a directory")
		}
	} else {
		if details.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 || details.NumberOfLinks != 1 {
			return errors.New("owner-only file is not a single-link regular file")
		}
	}
	return nil
}

func openOwnerOnlyMetadataHandle(path string, directory, writableSecurity bool) (windows.Handle, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	access := uint32(windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL)
	if writableSecurity {
		access |= windows.WRITE_DAC
	}
	attributes := uint32(windows.FILE_ATTRIBUTE_NORMAL | windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if directory {
		attributes |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	return windows.CreateFile(
		pathPtr,
		access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		attributes,
		0,
	)
}

func ownerOnlyACL(directory bool) (*windows.ACL, *windows.Tokenuser, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, nil, err
	}
	inheritance := uint32(0)
	if directory {
		inheritance = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.SET_ACCESS,
		Inheritance:       inheritance,
		Trustee:           windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_USER, TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid)},
	}}, nil)
	return acl, user, err
}

func validateOwnerOnlyDescriptor(descriptor *windows.SECURITY_DESCRIPTOR, directory bool) error {
	if descriptor == nil || !descriptor.IsValid() {
		return errors.New("security descriptor is invalid")
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("security descriptor DACL is not protected")
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	if owner == nil || !owner.Equals(user.User.Sid) {
		return errors.New("security descriptor owner does not match the process identity")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return errors.New("security descriptor DACL is unavailable")
	}
	header := (*ownerACLHeader)(unsafe.Pointer(dacl))
	expected := uint16(1)
	if directory {
		expected = 2
	}
	if header.aceCount != expected {
		return fmt.Errorf("security descriptor has %d access entries, want %d", header.aceCount, expected)
	}
	objectAccess, inheritedAccess := false, false
	for index := uint32(0); index < uint32(header.aceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			return err
		}
		if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("security descriptor contains a non-allow entry")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid == nil || !sid.Equals(user.User.Sid) {
			return errors.New("security descriptor grants access to an unexpected identity")
		}
		switch ace.Header.AceFlags {
		case 0:
			if objectAccess || (ace.Mask != windowsOwnerFileAllAccess && ace.Mask != windows.GENERIC_ALL) {
				return errors.New("security descriptor object access is invalid")
			}
			objectAccess = true
		case windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE | windows.INHERIT_ONLY_ACE:
			if !directory || inheritedAccess || (ace.Mask != windowsOwnerFileAllAccess && ace.Mask != windows.GENERIC_ALL) {
				return errors.New("security descriptor inherited access is invalid")
			}
			inheritedAccess = true
		default:
			return errors.New("security descriptor inheritance flags are invalid")
		}
	}
	if !objectAccess || directory != inheritedAccess {
		return errors.New("security descriptor access entries are incomplete")
	}
	runtime.KeepAlive(user)
	return nil
}

func restrictOwnerOnlyHandle(handle windows.Handle) error {
	return restrictOwnerOnlyHandleKind(handle, false)
}

func restrictOwnerOnlyHandleKind(handle windows.Handle, directory bool) error {
	if err := validateCurrentProcessOwner(handle); err != nil {
		return err
	}
	acl, user, err := ownerOnlyACL(directory)
	if err != nil {
		return err
	}
	if err := windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil); err != nil {
		return err
	}
	runtime.KeepAlive(acl)
	runtime.KeepAlive(user)
	return validateOwnerOnlyHandleKind(handle, directory)
}

func validateCurrentProcessOwner(handle windows.Handle) error {
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	defer runtime.KeepAlive(user)
	if owner == nil || !owner.Equals(user.User.Sid) {
		return errors.New("security descriptor owner does not match the process identity")
	}
	return nil
}

func validateOwnerOnlyHandle(handle windows.Handle) error {
	return validateOwnerOnlyHandleKind(handle, false)
}

func validateOwnerOnlyHandleKind(handle windows.Handle, directory bool) error {
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	return validateOwnerOnlyDescriptor(descriptor, directory)
}

type OwnerOnlyFileLock struct {
	handle     windows.Handle
	overlapped windows.Overlapped
	identity   windows.ByHandleFileInformation
}

func TryAcquireOwnerOnlyFileLock(path string, mode FileLockMode, create bool) (*OwnerOnlyFileLock, error) {
	if mode != LockShared && mode != LockExclusive {
		return nil, errors.New("invalid filesystem lock mode")
	}
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	access := uint32(windows.GENERIC_READ | windows.GENERIC_WRITE | windows.READ_CONTROL)
	disposition := uint32(windows.OPEN_EXISTING)
	if create {
		access |= windows.WRITE_DAC
		disposition = windows.CREATE_NEW
	}
	const attrs = windows.FILE_ATTRIBUTE_NORMAL | windows.FILE_FLAG_OPEN_REPARSE_POINT
	handle, err := windows.CreateFile(pathPtr, access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, disposition, attrs, 0)
	created := create && err == nil
	if create && (errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS)) {
		access = windows.GENERIC_READ | windows.GENERIC_WRITE | windows.READ_CONTROL
		handle, err = windows.CreateFile(pathPtr, access,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil, windows.OPEN_EXISTING, attrs, 0)
	}
	if err != nil {
		return nil, err
	}
	cleanup := func(err error) (*OwnerOnlyFileLock, error) {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	var details windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &details); err != nil {
		return cleanup(err)
	}
	if details.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || details.NumberOfLinks != 1 {
		return cleanup(errors.New("lock is not a single-link regular file"))
	}
	if created {
		if err := restrictOwnerOnlyHandle(handle); err != nil {
			return cleanup(err)
		}
	} else if err := validateOwnerOnlyHandle(handle); err != nil {
		return cleanup(err)
	}
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if mode == LockExclusive {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	overlapped := windows.Overlapped{}
	if err := windows.LockFileEx(handle, flags, 0, 1, 0, &overlapped); err != nil {
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return cleanup(ErrFileLockBusy)
		}
		return cleanup(err)
	}
	lock := &OwnerOnlyFileLock{handle: handle, overlapped: overlapped, identity: details}
	if err := lock.Validate(path); err != nil {
		_ = lock.Close()
		return nil, err
	}
	return lock, nil
}

// Validate confirms that path still names the locked owner-only file.
func (lock *OwnerOnlyFileLock) Validate(path string) error {
	if lock == nil || lock.handle == 0 || lock.handle == windows.InvalidHandle {
		return errors.New("lock handle is unavailable")
	}
	var current windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(lock.handle, &current); err != nil {
		return err
	}
	if !sameWindowsFileIdentity(lock.identity, current) ||
		current.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 ||
		current.NumberOfLinks != 1 {
		return errors.New("lock handle identity changed")
	}
	if err := validateOwnerOnlyHandle(lock.handle); err != nil {
		return err
	}
	pathHandle, err := openOwnerOnlyMetadataHandle(path, false, false)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(pathHandle)
	var pathIdentity windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(pathHandle, &pathIdentity); err != nil {
		return err
	}
	if !sameWindowsFileIdentity(current, pathIdentity) ||
		pathIdentity.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 ||
		pathIdentity.NumberOfLinks != 1 {
		return errors.New("lock path identity changed")
	}
	return validateOwnerOnlyHandle(pathHandle)
}

func sameWindowsFileIdentity(first, second windows.ByHandleFileInformation) bool {
	return first.VolumeSerialNumber == second.VolumeSerialNumber &&
		first.FileIndexHigh == second.FileIndexHigh &&
		first.FileIndexLow == second.FileIndexLow
}

func (lock *OwnerOnlyFileLock) Close() error {
	if lock == nil || lock.handle == 0 || lock.handle == windows.InvalidHandle {
		return nil
	}
	unlockErr := windows.UnlockFileEx(lock.handle, 0, 1, 0, &lock.overlapped)
	closeErr := windows.CloseHandle(lock.handle)
	lock.handle = 0
	lock.identity = windows.ByHandleFileInformation{}
	return errors.Join(unlockErr, closeErr)
}

func readOwnerOnlyFileBounded(path string, maxBytes int64) ([]byte, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, os.ErrInvalid
	}
	defer file.Close()
	var identity windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &identity); err != nil {
		return nil, err
	}
	if identity.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || identity.NumberOfLinks != 1 {
		return nil, errors.New("owner-only file is not a single-link regular file")
	}
	if err := validateOwnerOnlyHandle(handle); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("owner-only file exceeds its size limit")
	}
	pathHandle, err := openOwnerOnlyMetadataHandle(path, false, false)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(pathHandle)
	var current windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(pathHandle, &current); err != nil {
		return nil, err
	}
	if !sameWindowsFileIdentity(identity, current) || current.NumberOfLinks != 1 ||
		current.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return nil, errors.New("owner-only file identity changed during read")
	}
	if err := validateOwnerOnlyHandle(pathHandle); err != nil {
		return nil, err
	}
	return data, nil
}

func restrictOwnerOnlyExecutable(path string) error {
	return restrictOwnerOnlyPath(path, false)
}

func validateOwnerOnlyExecutable(path string) error {
	return validateOwnerOnlyPath(path, false)
}
