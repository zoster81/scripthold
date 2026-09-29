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

func TestAdmitStableProcessHoldsSharedUseLock(t *testing.T) {
	parent := canonicalTempDir(t)
	path := filepath.Join(parent, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(path, []byte("installed-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	deps := installedEvidenceDeps{
		readBuildInfo: func(io.ReaderAt) (*debug.BuildInfo, error) { return adoptionBuildInfoForTest(), nil },
		smokeVersion:  func(context.Context, string) (string, error) { return "3.2.1", nil },
	}
	boundary, inspection, _, err := initializeStableAdoptionWith(context.Background(), path, runtime.GOOS, runtime.GOARCH, deps)
	if err != nil {
		t.Fatal(err)
	}
	admission, err := admitStableProcessWith(context.Background(), boundary, inspection, runtime.GOOS, runtime.GOARCH, deps)
	if err != nil {
		t.Fatal(err)
	}
	defer admission.Close()
	if _, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.UseLockPath, filesystem.LockExclusive, false); !errors.Is(err, filesystem.ErrFileLockBusy) {
		t.Fatalf("exclusive use lock while admitted = %v, want ErrFileLockBusy", err)
	}
}

func TestAdmitStableProcessRejectsObservedByteDrift(t *testing.T) {
	parent := canonicalTempDir(t)
	path := filepath.Join(parent, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(path, []byte("installed-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	deps := installedEvidenceDeps{
		readBuildInfo: func(io.ReaderAt) (*debug.BuildInfo, error) { return adoptionBuildInfoForTest(), nil },
		smokeVersion:  func(context.Context, string) (string, error) { return "3.2.1", nil },
	}
	boundary, inspection, _, err := initializeStableAdoptionWith(context.Background(), path, runtime.GOOS, runtime.GOARCH, deps)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := admitStableProcessWith(context.Background(), boundary, inspection, runtime.GOOS, runtime.GOARCH, deps); err == nil {
		t.Fatal("changed installed bytes must fail closed")
	}
}

func TestAdmitStableProcessRequiresUseLockBeforeControlRelease(t *testing.T) {
	parent := canonicalTempDir(t)
	path := filepath.Join(parent, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(path, []byte("installed-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	deps := installedEvidenceDeps{
		readBuildInfo: func(io.ReaderAt) (*debug.BuildInfo, error) { return adoptionBuildInfoForTest(), nil },
		smokeVersion:  func(context.Context, string) (string, error) { return "3.2.1", nil },
	}
	boundary, inspection, _, err := initializeStableAdoptionWith(context.Background(), path, runtime.GOOS, runtime.GOARCH, deps)
	if err != nil {
		t.Fatal(err)
	}
	exclusiveUse, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.UseLockPath, filesystem.LockExclusive, false)
	if err != nil {
		t.Fatal(err)
	}
	defer exclusiveUse.Close()
	if _, err := admitStableProcessWith(context.Background(), boundary, inspection, runtime.GOOS, runtime.GOARCH, deps); !errors.Is(err, filesystem.ErrFileLockBusy) {
		t.Fatalf("admission while use locked = %v, want ErrFileLockBusy", err)
	}
	control, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false)
	if err != nil {
		t.Fatalf("control lock remained held after failed admission: %v", err)
	}
	_ = control.Close()
}

func TestAdmitStableProcessRejectsPendingTransaction(t *testing.T) {
	boundary, inspection, admission, candidate, installedDeps, _, _ := pendingPreparationFixture(t)
	if _, err := preparePendingTransactionWith(
		context.Background(), boundary, inspection, admission, candidate, runtime.GOOS, runtime.GOARCH,
		pendingPreparationDeps{
			installedEvidence: installedDeps,
			validateCandidate: func(context.Context, *PreparedCandidate, string, string) error { return nil },
			newTransactionID:  func() (string, error) { return strings.Repeat("4", 64), nil },
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := admission.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := admitStableProcessWith(context.Background(), boundary, inspection, runtime.GOOS, runtime.GOARCH, installedDeps); err == nil {
		t.Fatal("pending transaction must block new normal-process admission")
	}
}
