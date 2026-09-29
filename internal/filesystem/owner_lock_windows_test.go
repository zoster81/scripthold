//go:build windows

package filesystem

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

func TestOwnerNormalizationRequired(t *testing.T) {
	user, err := windows.StringToSid("S-1-5-18")
	if err != nil {
		t.Fatal(err)
	}
	defaultOwner, err := windows.StringToSid("S-1-5-32-544")
	if err != nil {
		t.Fatal(err)
	}
	other, err := windows.StringToSid("S-1-5-32-545")
	if err != nil {
		t.Fatal(err)
	}

	if normalize, err := ownerNormalizationRequired(user, user, defaultOwner); err != nil || normalize {
		t.Fatalf("current user owner = (%v, %v), want (false, nil)", normalize, err)
	}
	if normalize, err := ownerNormalizationRequired(defaultOwner, user, defaultOwner); err != nil || !normalize {
		t.Fatalf("default token owner = (%v, %v), want (true, nil)", normalize, err)
	}
	if normalize, err := ownerNormalizationRequired(other, user, defaultOwner); err == nil || normalize {
		t.Fatalf("unrelated owner = (%v, %v), want (false, error)", normalize, err)
	}
	if normalize, err := ownerNormalizationRequired(nil, user, defaultOwner); err == nil || normalize {
		t.Fatalf("nil owner = (%v, %v), want (false, error)", normalize, err)
	}
}

func TestRestrictCurrentUserOwnedPathDoesNotRequireWriteOwner(t *testing.T) {
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
			handle, err := openOwnerOnlyMetadataHandle(path, tc.directory, true)
			if err != nil {
				t.Fatalf("open path for owner normalization: %v", err)
			}
			if err := normalizeCurrentProcessOwner(handle); err != nil {
				_ = windows.CloseHandle(handle)
				t.Fatalf("normalize current process owner: %v", err)
			}
			if err := windows.CloseHandle(handle); err != nil {
				t.Fatalf("close owner-normalization handle: %v", err)
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
