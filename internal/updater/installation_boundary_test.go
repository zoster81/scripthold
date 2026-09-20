package updater

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestOpenInstallationBoundaryCreatesSiblingOwnerOnlyState(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(target, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	inspection, err := inspectStandaloneExecutable(target, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := openInstallationBoundary(inspection, true)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(parent, installationStateDirectoryName)
	if boundary.Directory != want {
		t.Fatalf("state directory = %q, want %q", boundary.Directory, want)
	}
	if err := filesystem.ValidateOwnerOnlyPath(boundary.Directory, true); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{controlLockName, useLockName} {
		path := filepath.Join(boundary.Directory, name)
		if err := filesystem.ValidateOwnerOnlyPath(path, false); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestOpenInstallationBoundaryExistingOnlyDoesNotCreate(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(target, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	inspection, err := inspectStandaloneExecutable(target, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openInstallationBoundary(inspection, false); err == nil {
		t.Fatal("existing-only open unexpectedly created state")
	}
	if _, err := os.Stat(filepath.Join(parent, installationStateDirectoryName)); !os.IsNotExist(err) {
		t.Fatalf("state directory unexpectedly exists: %v", err)
	}
}

func TestOpenInstallationBoundaryRejectsReplacedParentEvidence(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(target, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	inspection, err := inspectStandaloneExecutable(target, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	inspection.ParentIdentity = filesystem.ObjectIdentity{}
	if _, err := openInstallationBoundary(inspection, true); err == nil {
		t.Fatal("invalid parent identity must fail closed")
	}
}

func TestOpenInstallationBoundaryRejectsPreexistingInsecureState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission-mode assertion is Unix-specific")
	}
	parent := t.TempDir()
	target := filepath.Join(parent, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(target, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	inspection, err := inspectStandaloneExecutable(target, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(parent, installationStateDirectoryName)
	if err := os.Mkdir(state, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := openInstallationBoundary(inspection, true); err == nil {
		t.Fatal("preexisting insecure state directory must fail closed")
	}
	info, err := os.Stat(state)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("preexisting state permissions were modified to %04o", info.Mode().Perm())
	}
}
