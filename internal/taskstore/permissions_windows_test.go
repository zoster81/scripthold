//go:build windows

package taskstore

import (
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

func TestOpenRejectsAndDoesNotRepairPermissiveWindowsACL(t *testing.T) {
	store := newTestStore(t)
	if err := setPermissiveTaskWindowsACL(store.root, true); err != nil {
		t.Fatal(err)
	}
	if err := validateSecurePath(store.root, true); err == nil {
		t.Fatal("permissive task-store ACL unexpectedly passed validation")
	}
	opened, err := Open(store.root, nil, store.limits)
	if opened != nil || err == nil {
		t.Fatal("Open accepted a permissive task-store ACL")
	}
	if err := validateSecurePath(store.root, true); err == nil {
		t.Fatal("Open silently repaired the permissive task-store ACL")
	}
}

func setPermissiveTaskWindowsACL(path string, directory bool) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	world, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		return err
	}
	inheritance := uint32(0)
	if directory {
		inheritance = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{
		{AccessPermissions: windows.GENERIC_ALL, AccessMode: windows.SET_ACCESS, Inheritance: inheritance, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_USER, TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid)}},
		{AccessPermissions: windows.GENERIC_READ, AccessMode: windows.GRANT_ACCESS, Inheritance: inheritance, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_WELL_KNOWN_GROUP, TrusteeValue: windows.TrusteeValueFromSID(world)}},
	}, nil)
	if err != nil {
		return err
	}
	err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
	runtime.KeepAlive(user)
	runtime.KeepAlive(world)
	runtime.KeepAlive(acl)
	return err
}
