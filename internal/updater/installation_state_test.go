package updater

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/filesystem"
)

func validStableStateForTest(t *testing.T) (*InstallationBoundary, *StandaloneInspection, installationState) {
	t.Helper()
	parent := canonicalTempDir(t)
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
	state := installationState{
		FormatVersion: installationStateFormatVersion,
		Target: installationTargetState{
			Path:           inspection.ExecutablePath,
			Identity:       inspection.ExecutableIdentity.StableKey(),
			Volume:         inspection.ExecutableIdentity.VolumeKey(),
			ParentIdentity: inspection.ParentIdentity.StableKey(),
			ParentVolume:   inspection.ParentIdentity.VolumeKey(),
		},
		Current: installationCurrentState{
			Version: "3.2.1",
			SHA256:  strings.Repeat("a", 64),
			Build: installationBuildIdentity{
				Module:   officialModulePath,
				GOOS:     runtime.GOOS,
				GOARCH:   runtime.GOARCH,
				VCS:      "git",
				Revision: strings.Repeat("b", 40),
				VCSClean: true,
			},
		},
	}
	return boundary, inspection, state
}

func TestInstallationStateCodecRoundTrip(t *testing.T) {
	_, inspection, state := validStableStateForTest(t)
	payload, err := encodeInstallationState(state)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeInstallationState(payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != state {
		t.Fatalf("decoded state differs: got %#v want %#v", decoded, state)
	}
	if err := validateInstallationState(decoded, inspection); err != nil {
		t.Fatal(err)
	}
}

func TestInstallationStateCodecRejectsMalformedOrUnknownEvidence(t *testing.T) {
	_, _, state := validStableStateForTest(t)
	payload, err := encodeInstallationState(state)
	if err != nil {
		t.Fatal(err)
	}
	unknown := strings.TrimSpace(string(payload))
	unknown = strings.TrimSuffix(unknown, "}") + ",\"unknown\":true}"
	tests := [][]byte{
		nil,
		[]byte("{\"formatVersion\":2}"),
		[]byte(strings.Replace(string(payload), "\"sha256\": \""+strings.Repeat("a", 64)+"\"", "\"sha256\": \"ABC\"", 1)),
		[]byte(strings.Replace(string(payload), "\"vcs\": \"git\"", "\"vcs\": \"hg\"", 1)),
		[]byte(strings.Replace(string(payload), "\"vcsClean\": true", "\"vcsClean\": false", 1)),
		[]byte(unknown),
	}
	for _, candidate := range tests {
		if _, err := decodeInstallationState(candidate); err == nil {
			t.Fatalf("invalid state accepted: %q", candidate)
		}
	}
}

func TestPersistStableInstallationStateCreatesAndReadsOwnerOnlyState(t *testing.T) {
	boundary, inspection, state := validStableStateForTest(t)
	if err := persistStableInstallationState(boundary, inspection, state); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(boundary.Directory, installationStateFileName)
	if err := filesystem.ValidateOwnerOnlyPath(path, false); err != nil {
		t.Fatal(err)
	}
	got, err := readInstallationState(boundary, inspection)
	if err != nil {
		t.Fatal(err)
	}
	if got != state {
		t.Fatalf("state round trip differs: got %#v want %#v", got, state)
	}
}

func TestPersistStableInstallationStateRejectsIdentityDrift(t *testing.T) {
	boundary, inspection, state := validStableStateForTest(t)
	state.Target.Identity = "tampered"
	if err := persistStableInstallationState(boundary, inspection, state); err == nil {
		t.Fatal("identity drift must fail closed")
	}
	if _, err := os.Stat(filepath.Join(boundary.Directory, installationStateFileName)); !os.IsNotExist(err) {
		t.Fatalf("state file unexpectedly exists: %v", err)
	}
}

func TestReadInstallationStateRejectsOversizedFile(t *testing.T) {
	boundary, inspection, _ := validStableStateForTest(t)
	path := filepath.Join(boundary.Directory, installationStateFileName)
	if err := os.WriteFile(path, make([]byte, maxInstallationStateBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := filesystem.RestrictOwnerOnlyPath(path, false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := readInstallationState(boundary, inspection); err == nil {
		t.Fatal("oversized state must fail closed")
	}
}

func TestPersistStableInstallationStateReplacesExistingOwnerOnlyState(t *testing.T) {
	boundary, inspection, state := validStableStateForTest(t)
	if err := persistStableInstallationState(boundary, inspection, state); err != nil {
		t.Fatal(err)
	}
	state.Current.Version = "3.2.2"
	state.Current.SHA256 = strings.Repeat("c", 64)
	if err := persistStableInstallationState(boundary, inspection, state); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(boundary.Directory, installationStateFileName)
	if err := filesystem.ValidateOwnerOnlyPath(path, false); err != nil {
		t.Fatal(err)
	}
	got, err := readInstallationState(boundary, inspection)
	if err != nil {
		t.Fatal(err)
	}
	if got.Current.Version != "3.2.2" || got.Current.SHA256 != strings.Repeat("c", 64) {
		t.Fatalf("replacement state not observed: %#v", got.Current)
	}
}

func TestInstallationStatePersistenceRequiresControlLockOwnership(t *testing.T) {
	boundary, inspection, state := validStableStateForTest(t)
	lock, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := persistStableInstallationState(boundary, inspection, state); !errors.Is(err, filesystem.ErrFileLockBusy) {
		t.Fatalf("persist while control locked = %v, want ErrFileLockBusy", err)
	}
	if _, err := readInstallationState(boundary, inspection); !errors.Is(err, filesystem.ErrFileLockBusy) {
		t.Fatalf("read while control locked = %v, want ErrFileLockBusy", err)
	}
}

func TestPersistStableInstallationStateRejectsCorruptExistingState(t *testing.T) {
	boundary, inspection, state := validStableStateForTest(t)
	path := filepath.Join(boundary.Directory, installationStateFileName)
	corrupt := []byte("{\"formatVersion\":1,\"corrupt\":true}")
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := filesystem.RestrictOwnerOnlyPath(path, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := persistStableInstallationState(boundary, inspection, state); err == nil {
		t.Fatal("corrupt existing state must not be overwritten")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(corrupt) {
		t.Fatalf("corrupt state was modified: %q", got)
	}
}

func TestInstallationBoundaryValidationRejectsForgedPaths(t *testing.T) {
	boundary, inspection, _ := validStableStateForTest(t)
	forged := *boundary
	forged.ControlLockPath = filepath.Join(boundary.Directory, "other.lock")
	if err := validateInstallationBoundary(&forged, inspection); err == nil {
		t.Fatal("forged control-lock path must fail closed")
	}
	forged = *boundary
	forged.Directory = filepath.Join(inspection.ParentPath, "other-state")
	if err := validateInstallationBoundary(&forged, inspection); err == nil {
		t.Fatal("forged state directory must fail closed")
	}
}
