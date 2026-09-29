//go:build windows

package filesystem

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

func TestRestrictOwnerOnlyPathDoesNotRequireWriteOwner(t *testing.T) {
	for _, tc := range []struct {
		name      string
		directory bool
	}{
		{name: "file"},
		{name: "directory", directory: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "target")
			if tc.directory {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := setCurrentUserModifyOnlyACL(path, tc.directory); err != nil {
				t.Fatalf("prepare modify-only ACL: %v", err)
			}
			if err := RestrictOwnerOnlyPath(path, tc.directory); err != nil {
				t.Fatalf("restrict owner-only path: %v", err)
			}
			if err := ValidateOwnerOnlyPath(path, tc.directory); err != nil {
				t.Fatalf("validate owner-only path: %v", err)
			}
		})
	}
}

func setCurrentUserModifyOnlyACL(path string, directory bool) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	inheritance := uint32(0)
	if directory {
		inheritance = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.FILE_GENERIC_EXECUTE | windows.DELETE,
		AccessMode:        windows.SET_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
		},
	}}, nil)
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
	return err
}
