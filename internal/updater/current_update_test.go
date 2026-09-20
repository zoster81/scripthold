package updater

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/zoster81/scripthold/internal/filesystem"
)

func currentUpdateFixture(t *testing.T) (string, installedEvidenceDeps) {
	t.Helper()
	parent := t.TempDir()
	path := filepath.Join(parent, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(path, []byte("installed-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	installed := installedEvidenceDeps{
		readBuildInfo: func(io.ReaderAt) (*debug.BuildInfo, error) { return adoptionBuildInfoForTest(), nil },
		smokeVersion:  func(context.Context, string) (string, error) { return "3.2.1", nil },
	}
	if _, _, _, err := initializeStableAdoptionWith(context.Background(), path, runtime.GOOS, runtime.GOARCH, installed); err != nil {
		t.Fatal(err)
	}
	return path, installed
}

func TestLaunchCurrentUpdateReportsCurrentWithoutDispatch(t *testing.T) {
	path, installed := currentUpdateFixture(t)
	launchCalls := 0
	var stagingDirectory string

	result, err := launchCurrentUpdateWith(context.Background(), path, runtime.GOOS, runtime.GOARCH, currentUpdateDeps{
		installedEvidence: installed,
		prepareCandidate: func(_ context.Context, currentVersion, directory string) (*PreparedCandidate, error) {
			if currentVersion != "3.2.1" {
				t.Fatalf("current version = %q", currentVersion)
			}
			if err := filesystem.ValidateOwnerOnlyPath(directory, true); err != nil {
				t.Fatalf("staging directory is not owner-only: %v", err)
			}
			stagingDirectory = directory
			return nil, errNoUpdateAvailable
		},
		launch: func(context.Context, *InstallationBoundary, *StandaloneInspection, *ProcessAdmission, *PreparedCandidate) (bool, error) {
			launchCalls++
			return false, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != UpdateLaunchCurrent || result.CurrentVersion != "3.2.1" || result.CandidateVersion != "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if launchCalls != 0 {
		t.Fatalf("launch calls = %d, want 0", launchCalls)
	}
	if stagingDirectory == "" {
		t.Fatal("candidate preparation did not receive staging directory")
	}
	if _, err := os.Lstat(stagingDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging directory still exists: %v", err)
	}
	inspection, err := inspectStandaloneExecutable(path, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := openInstallationBoundary(inspection, false)
	if err != nil {
		t.Fatal(err)
	}
	exclusive, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.UseLockPath, filesystem.LockExclusive, false)
	if err != nil {
		t.Fatalf("no-update path retained process admission: %v", err)
	}
	_ = exclusive.Close()
}

func TestLaunchCurrentUpdateReportsDispatchedAndCleansCandidate(t *testing.T) {
	path, installed := currentUpdateFixture(t)
	var stagingDirectory string
	var candidatePath string

	result, err := launchCurrentUpdateWith(context.Background(), path, runtime.GOOS, runtime.GOARCH, currentUpdateDeps{
		installedEvidence: installed,
		prepareCandidate: func(_ context.Context, currentVersion, directory string) (*PreparedCandidate, error) {
			if currentVersion != "3.2.1" {
				t.Fatalf("current version = %q", currentVersion)
			}
			stagingDirectory = directory
			candidatePath = filepath.Join(directory, "candidate")
			if err := os.WriteFile(candidatePath, []byte("candidate"), 0o700); err != nil {
				return nil, err
			}
			return &PreparedCandidate{Version: "3.3.0", path: candidatePath}, nil
		},
		launch: func(_ context.Context, boundary *InstallationBoundary, _ *StandaloneInspection, admission *ProcessAdmission, candidate *PreparedCandidate) (bool, error) {
			if err := admission.validateFor(boundary); err != nil {
				t.Fatalf("invalid admission during launch: %v", err)
			}
			if candidate.Path() != candidatePath {
				t.Fatalf("candidate path = %q, want %q", candidate.Path(), candidatePath)
			}
			return true, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != UpdateLaunchDispatched || result.CurrentVersion != "3.2.1" || result.CandidateVersion != "3.3.0" {
		t.Fatalf("unexpected result: %#v", result)
	}
	for _, target := range []string{candidatePath, stagingDirectory} {
		if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("transient path still exists %q: %v", target, err)
		}
	}
}

func TestLaunchCurrentUpdatePreservesDispatchedResultOnPostStartError(t *testing.T) {
	path, installed := currentUpdateFixture(t)
	injected := errors.New("post-start cleanup failed")

	result, err := launchCurrentUpdateWith(context.Background(), path, runtime.GOOS, runtime.GOARCH, currentUpdateDeps{
		installedEvidence: installed,
		prepareCandidate: func(_ context.Context, _ string, directory string) (*PreparedCandidate, error) {
			candidatePath := filepath.Join(directory, "candidate")
			if err := os.WriteFile(candidatePath, []byte("candidate"), 0o700); err != nil {
				return nil, err
			}
			return &PreparedCandidate{Version: "3.3.0", path: candidatePath}, nil
		},
		launch: func(context.Context, *InstallationBoundary, *StandaloneInspection, *ProcessAdmission, *PreparedCandidate) (bool, error) {
			return true, injected
		},
	})
	if !errors.Is(err, injected) {
		t.Fatalf("error = %v, want injected error", err)
	}
	if result.Status != UpdateLaunchDispatched || result.CandidateVersion != "3.3.0" {
		t.Fatalf("unexpected result after post-start error: %#v", result)
	}
}
