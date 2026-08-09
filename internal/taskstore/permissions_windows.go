//go:build windows

package taskstore

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

func securePath(path string, directory bool) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
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
	if err != nil {
		return err
	}
	err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
	runtime.KeepAlive(user)
	runtime.KeepAlive(acl)
	if err != nil {
		return err
	}
	err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION, user.User.Sid, nil, nil, nil)
	runtime.KeepAlive(user)
	if err != nil {
		return err
	}
	return validateSecurePath(path, directory)
}

func validateSecurePath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if directory != info.IsDir() {
		return errors.New("task store path type is invalid")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("task store path must not be a link")
	}
	if data, ok := info.Sys().(*syscall.Win32FileAttributeData); ok && data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("task store path must not be a reparse point")
	}
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	if descriptor == nil || !descriptor.IsValid() {
		return errors.New("task store security descriptor is invalid")
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("task store DACL is not protected")
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
		return errors.New("task store owner does not match the process identity")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	if dacl == nil {
		return errors.New("task store DACL is missing")
	}
	header := (*aclHeader)(unsafe.Pointer(dacl))
	want := uint16(1)
	if directory {
		want = 2
	}
	if header.aceCount != want {
		return fmt.Errorf("task store DACL has %d entries, want %d", header.aceCount, want)
	}
	for index := uint32(0); index < uint32(header.aceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			return err
		}
		if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("task store DACL contains a non-allow entry")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid == nil || !sid.Equals(user.User.Sid) {
			return errors.New("task store DACL grants another identity")
		}
		if ace.Mask != windowsFileAllAccess && ace.Mask != windows.GENERIC_ALL {
			return errors.New("task store DACL access mask is invalid")
		}
	}
	runtime.KeepAlive(user)
	return nil
}
