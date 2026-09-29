package updater

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestAdmitProcessIfStatePresentLeavesUnadoptedInstallationUntouched(t *testing.T) {
	parent := canonicalTempDir(t)
	path := filepath.Join(parent, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(path, []byte("not-a-real-build"), 0o700); err != nil {
		t.Fatal(err)
	}

	admission, err := admitProcessIfStatePresent(
		context.Background(), path, runtime.GOOS, runtime.GOARCH, installedEvidenceDeps{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if admission != nil {
		t.Fatal("unadopted installation unexpectedly received process admission")
	}
	if _, err := os.Lstat(filepath.Join(parent, installationStateDirectoryName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unadopted probe created self-update state: %v", err)
	}
}

func TestAdmitProcessIfStatePresentHoldsSharedUseForAdoptedStableState(t *testing.T) {
	parent := canonicalTempDir(t)
	path := filepath.Join(parent, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(path, []byte("installed-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	deps := installedEvidenceDeps{
		readBuildInfo: func(io.ReaderAt) (*debug.BuildInfo, error) { return adoptionBuildInfoForTest(), nil },
		smokeVersion:  func(context.Context, string) (string, error) { return "3.2.1", nil },
	}
	boundary, _, _, err := initializeStableAdoptionWith(
		context.Background(), path, runtime.GOOS, runtime.GOARCH, deps,
	)
	if err != nil {
		t.Fatal(err)
	}

	admission, err := admitProcessIfStatePresent(
		context.Background(), path, runtime.GOOS, runtime.GOARCH, deps,
	)
	if err != nil {
		t.Fatal(err)
	}
	if admission == nil {
		t.Fatal("adopted installation did not receive process admission")
	}
	defer admission.Close()

	if _, err := filesystem.TryAcquireOwnerOnlyFileLock(
		boundary.UseLockPath, filesystem.LockExclusive, false,
	); !errors.Is(err, filesystem.ErrFileLockBusy) {
		t.Fatalf("exclusive use lock while normal process admitted = %v, want busy", err)
	}
}

func TestAdmitProcessIfStatePresentFailsClosedForInvalidExistingState(t *testing.T) {
	parent := canonicalTempDir(t)
	path := filepath.Join(parent, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(path, []byte("installed-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	stateRoot := filepath.Join(parent, installationStateDirectoryName)
	if err := os.Mkdir(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	admission, err := admitProcessIfStatePresent(
		context.Background(), path, runtime.GOOS, runtime.GOARCH, installedEvidenceDeps{},
	)
	if admission != nil || err == nil {
		t.Fatalf("invalid existing state was treated as unadopted: admission=%v err=%v", admission, err)
	}
}

func TestAdmitProcessIfStatePresentRejectsPendingState(t *testing.T) {
	boundary, inspection, admission, candidate, installedDeps, _, _ := pendingPreparationFixture(t)
	if _, err := preparePendingTransactionWith(
		context.Background(), boundary, inspection, admission, candidate, runtime.GOOS, runtime.GOARCH,
		pendingPreparationDeps{
			installedEvidence: installedDeps,
			validateCandidate: func(context.Context, *PreparedCandidate, string, string) error { return nil },
			newTransactionID:  func() (string, error) { return strings.Repeat("7", 64), nil },
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := admission.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := admitProcessIfStatePresent(
		context.Background(), inspection.ExecutablePath, runtime.GOOS, runtime.GOARCH, installedDeps,
	)
	if got != nil || err == nil || !strings.Contains(err.Error(), "pending") {
		t.Fatalf("pending startup admission = %v, err=%v", got, err)
	}
}
