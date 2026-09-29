package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestInstallationStatusNotAdoptedDoesNotCreateState(t *testing.T) {
	parent := canonicalTempDir(t)
	target := filepath.Join(parent, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(target, []byte("standalone"), 0o700); err != nil {
		t.Fatal(err)
	}
	inspection, err := inspectStandaloneExecutable(target, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	status, err := observeInstallationStatusWith(
		context.Background(), inspection, runtime.GOOS, runtime.GOARCH, reconciliationDeps{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if status.Adopted || status.State != "" || status.RecoveryAvailable {
		t.Fatalf("unexpected unadopted status: %#v", status)
	}
	if _, err := os.Lstat(filepath.Join(parent, installationStateDirectoryName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("status created installation state directory: %v", err)
	}
}

func TestInstallationStatusPreparedIsReadOnlyAndReportsVersions(t *testing.T) {
	boundary, inspection, _, _, deps := prepareReconciliationFixture(t)
	before := snapshotReconciliationFilesForTest(t, boundary, inspection.ExecutablePath)

	status, err := observeInstallationStatusWith(
		context.Background(), inspection, runtime.GOOS, runtime.GOARCH, deps,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Adopted || status.State != ReconciliationPrepared ||
		status.CurrentVersion != "3.2.1" || status.InstalledVersion != "3.2.1" ||
		status.CandidateVersion != "3.3.0" || !status.RecoveryAvailable || status.Problem != "" {
		t.Fatalf("unexpected prepared status: %#v", status)
	}
	after := snapshotReconciliationFilesForTest(t, boundary, inspection.ExecutablePath)
	if !reconciliationSnapshotsEqualForTest(before, after) {
		t.Fatal("read-only status observation modified target/state/artifacts")
	}
}

func TestInstallationStatusCommittedAndRolledBackRecoveryAvailability(t *testing.T) {
	t.Run("committed", func(t *testing.T) {
		ownership, _, _, _, deps := preparedHelperOwnershipForLifecycle(t)
		if err := ownership.commitExecutableReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
			t.Fatal(err)
		}
		inspection := ownership.inspection
		if err := ownership.Close(); err != nil {
			t.Fatal(err)
		}
		status, err := observeInstallationStatusWith(
			context.Background(), inspection, runtime.GOOS, runtime.GOARCH, deps,
		)
		if err != nil {
			t.Fatal(err)
		}
		if status.State != ReconciliationCommitted || !status.RecoveryAvailable ||
			status.CurrentVersion != "3.2.1" || status.InstalledVersion != "3.3.0" ||
			status.CandidateVersion != "3.3.0" {
			t.Fatalf("unexpected committed status: %#v", status)
		}
	})
	t.Run("rolled_back", func(t *testing.T) {
		ownership, _, _, _, deps := preparedHelperOwnershipForLifecycle(t)
		if err := ownership.commitExecutableReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
			t.Fatal(err)
		}
		if err := ownership.rollbackCommittedReplacementWith(
			context.Background(), runtime.GOOS, runtime.GOARCH, deps, rollbackDeps{},
		); err != nil {
			t.Fatal(err)
		}
		inspection := ownership.inspection
		if err := ownership.Close(); err != nil {
			t.Fatal(err)
		}
		status, err := observeInstallationStatusWith(
			context.Background(), inspection, runtime.GOOS, runtime.GOARCH, deps,
		)
		if err != nil {
			t.Fatal(err)
		}
		if status.State != ReconciliationRolledBack || status.InstalledVersion != "3.2.1" ||
			!status.RecoveryAvailable {
			t.Fatalf("unexpected rolled-back status: %#v", status)
		}
	})
}

func TestInstallationStatusRecoveryRequiredOnlyOffersSafeRecovery(t *testing.T) {
	t.Run("candidate bytes", func(t *testing.T) {
		boundary, inspection, _, candidateBytes, deps := prepareReconciliationFixture(t)
		candidatePath := filepath.Join(boundary.Directory, candidateArtifactName)
		if err := os.Remove(inspection.ExecutablePath); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(candidatePath, inspection.ExecutablePath); err != nil {
			t.Fatal(err)
		}
		currentInspection, err := inspectStandaloneExecutable(inspection.ExecutablePath, runtime.GOOS)
		if err != nil {
			t.Fatal(err)
		}
		deps.observeTarget = func(context.Context, *StandaloneInspection, string, string) (installationCurrentState, error) {
			return installationCurrentState{}, errors.New("candidate smoke failed")
		}
		status, err := observeInstallationStatusWith(
			context.Background(), currentInspection, runtime.GOOS, runtime.GOARCH, deps,
		)
		if err != nil {
			t.Fatal(err)
		}
		if status.State != ReconciliationRecoveryRequired || status.InstalledVersion != "" ||
			!status.RecoveryAvailable || status.CandidateVersion != "3.3.0" {
			t.Fatalf("unexpected candidate recovery status: %#v", status)
		}
		if got, err := os.ReadFile(currentInspection.ExecutablePath); err != nil || string(got) != string(candidateBytes) {
			t.Fatalf("status changed candidate target: %q err=%v", got, err)
		}
	})

	t.Run("unknown bytes", func(t *testing.T) {
		_, inspection, _, _, deps := prepareReconciliationFixture(t)
		if err := os.WriteFile(inspection.ExecutablePath, []byte("unknown"), 0o700); err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS == "windows" {
			if err := filesystem.RestrictOwnerOnlyExecutable(inspection.ExecutablePath); err != nil {
				t.Fatal(err)
			}
		}
		status, err := observeInstallationStatusWith(
			context.Background(), inspection, runtime.GOOS, runtime.GOARCH, deps,
		)
		if err != nil {
			t.Fatal(err)
		}
		if status.State != ReconciliationRecoveryRequired || status.RecoveryAvailable {
			t.Fatalf("unsafe recovery was offered: %#v", status)
		}
	})
}

func TestInstallationStatusStableReportsCurrentWithoutRecovery(t *testing.T) {
	boundary, inspection, admission, _, installedDeps, _, _ := pendingPreparationFixture(t)
	defer admission.Close()

	status, err := observeInstallationStatusWith(
		context.Background(),
		inspection,
		runtime.GOOS,
		runtime.GOARCH,
		reconciliationDeps{
			observeTarget: func(ctx context.Context, inspection *StandaloneInspection, goos, goarch string) (installationCurrentState, error) {
				return observeInstalledBinary(ctx, inspection, goos, goarch, installedDeps)
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Adopted || status.State != ReconciliationStable ||
		status.CurrentVersion != "3.2.1" || status.InstalledVersion != "3.2.1" ||
		status.CandidateVersion != "" || status.RecoveryAvailable || status.Problem != "" {
		t.Fatalf("unexpected stable status: %#v", status)
	}
	if _, err := os.Stat(filepath.Join(boundary.Directory, installationStateFileName)); err != nil {
		t.Fatalf("stable status lost installation state: %v", err)
	}
}

func TestInstallationStatusExistingDamagedBoundaryIsNotUnadopted(t *testing.T) {
	boundary, inspection, admission, _, _, _, _ := pendingPreparationFixture(t)
	defer admission.Close()
	if err := os.Remove(boundary.ControlLockPath); err != nil {
		t.Fatal(err)
	}

	status, err := observeInstallationStatusWith(
		context.Background(), inspection, runtime.GOOS, runtime.GOARCH, reconciliationDeps{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Adopted || status.State != ReconciliationRecoveryRequired ||
		status.RecoveryAvailable || status.Problem == "" {
		t.Fatalf("damaged state boundary was hidden as unadopted: %#v", status)
	}
	if _, err := os.Stat(boundary.Directory); err != nil {
		t.Fatalf("status modified damaged state directory: %v", err)
	}
}
