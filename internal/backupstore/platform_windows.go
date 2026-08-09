//go:build windows

package backupstore

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsFileAllAccess windows.ACCESS_MASK = 0x001F01FF

type aclHeader struct {
	revision  byte
	reserved  byte
	size      uint16
	aceCount  uint16
	reserved2 uint16
}

func isLinkOrReparse(info os.FileInfo) bool {
	if info == nil || info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

func restrictPathPermissions(path string, directory bool) error {
	acl, user, err := ownerOnlyACL(directory)
	if err != nil {
		return err
	}
	err = windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	)
	runtime.KeepAlive(user)
	runtime.KeepAlive(acl)
	if err != nil {
		return err
	}
	err = windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION,
		user.User.Sid,
		nil,
		nil,
		nil,
	)
	runtime.KeepAlive(user)
	if err != nil {
		return err
	}
	return validatePathPermissions(path, directory)
}

func restrictHandlePermissions(handle windows.Handle, directory bool) error {
	acl, user, err := ownerOnlyACL(directory)
	if err != nil {
		return err
	}
	err = windows.SetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	)
	runtime.KeepAlive(user)
	runtime.KeepAlive(acl)
	if err != nil {
		return err
	}
	err = windows.SetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION,
		user.User.Sid,
		nil,
		nil,
		nil,
	)
	runtime.KeepAlive(user)
	if err != nil {
		return err
	}
	return validateHandlePermissions(handle, directory)
}

func validatePathPermissions(path string, directory bool) error {
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return err
	}
	return validateOwnerOnlyDescriptor(descriptor, directory)
}

func validateHandlePermissions(handle windows.Handle, directory bool) error {
	descriptor, err := windows.GetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return err
	}
	return validateOwnerOnlyDescriptor(descriptor, directory)
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
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
		},
	}}, nil)
	if err != nil {
		return nil, nil, err
	}
	return acl, user, nil
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
	if err != nil {
		return err
	}
	if dacl == nil {
		return errors.New("security descriptor DACL is missing")
	}
	header := (*aclHeader)(unsafe.Pointer(dacl))
	expectedEntries := uint16(1)
	if directory {
		expectedEntries = 2
	}
	if header.aceCount != expectedEntries {
		return fmt.Errorf("security descriptor has %d access entries, want %d", header.aceCount, expectedEntries)
	}

	objectAccess := false
	inheritedAccess := false
	for index := uint32(0); index < uint32(header.aceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			return err
		}
		if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("security descriptor access entry is not an allow entry")
		}
		entrySID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if entrySID == nil || !entrySID.Equals(user.User.Sid) {
			return errors.New("security descriptor grants access to an unexpected identity")
		}

		switch ace.Header.AceFlags {
		case 0:
			if objectAccess || (ace.Mask != windowsFileAllAccess && ace.Mask != windows.GENERIC_ALL) {
				return errors.New("security descriptor object access is invalid")
			}
			objectAccess = true
		case windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE | windows.INHERIT_ONLY_ACE:
			if !directory || inheritedAccess || (ace.Mask != windows.GENERIC_ALL && ace.Mask != windowsFileAllAccess) {
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

func syncDirectory(string) error {
	// Windows has no portable directory fsync. File data is explicitly synced,
	// and the lifetime lock prevents concurrent store initialization.
	return nil
}
