package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func preparedHelperOwnershipForLifecycle(t *testing.T) (*DetachedHelperOwnership, *InstallationBoundary, []byte, []byte, reconciliationDeps) {
	t.Helper()
	boundary, _, admission, transactionID, sourceBytes, deps := helperOwnershipFixture(t)
	candidateBytes, err := os.ReadFile(filepath.Join(boundary.Directory, candidateArtifactName))
	if err != nil {
		t.Fatal(err)
	}
	if err := admission.Close(); err != nil {
		t.Fatal(err)
	}
	ownership, err := acquireDetachedHelperOwnershipWith(
		context.Background(),
		filepath.Join(boundary.Directory, helperArtifactName),
		transactionID,
		runtime.GOOS,
		runtime.GOARCH,
		500*time.Millisecond,
		helperOwnershipDeps{reconciliation: deps},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ownership.Close() })
	return ownership, boundary, sourceBytes, candidateBytes, deps
}

func TestRunDetachedHelperOwnershipCommitsAndFinalizes(t *testing.T) {
	ownership, boundary, _, candidateBytes, deps := preparedHelperOwnershipForLifecycle(t)
	if err := runDetachedHelperOwnershipWith(
		context.Background(), ownership, runtime.GOOS, runtime.GOARCH, deps,
	); err != nil {
		t.Fatal(err)
	}
	state, err := readInstallationStateLocked(boundary, ownership.inspection)
	if err != nil {
		t.Fatal(err)
	}
	if state.Pending != nil || state.Current.Version != "3.3.0" ||
		state.Current.SHA256 != bytesSHA256ForTest(candidateBytes) {
		t.Fatalf("unexpected finalized candidate state: %#v", state)
	}
	if state.Terminal == nil || state.Terminal.Outcome != terminalOutcomeCommitted {
		t.Fatalf("unexpected terminal outcome: %#v", state.Terminal)
	}
}

func TestRunDetachedHelperOwnershipRollsBackFailedCandidateVerification(t *testing.T) {
	ownership, boundary, sourceBytes, candidateBytes, deps := preparedHelperOwnershipForLifecycle(t)
	base := deps.observeTarget
	deps.observeTarget = func(ctx context.Context, inspection *StandaloneInspection, goos, goarch string) (installationCurrentState, error) {
		data, err := os.ReadFile(inspection.ExecutablePath)
		if err != nil {
			return installationCurrentState{}, err
		}
		if string(data) == string(candidateBytes) {
			return installationCurrentState{}, errors.New("candidate version smoke failed")
		}
		return base(ctx, inspection, goos, goarch)
	}
	err := runDetachedHelperOwnershipWith(
		context.Background(), ownership, runtime.GOOS, runtime.GOARCH, deps,
	)
	if err == nil {
		t.Fatal("failed candidate verification must remain visible to helper exit")
	}
	installed, readErr := os.ReadFile(ownership.inspection.ExecutablePath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(installed) != string(sourceBytes) {
		t.Fatalf("automatic rollback installed %q, want source %q", installed, sourceBytes)
	}
	state, readErr := readInstallationStateLocked(boundary, ownership.inspection)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if state.Pending != nil || state.Current.Version != "3.2.1" ||
		state.Current.SHA256 != bytesSHA256ForTest(sourceBytes) {
		t.Fatalf("unexpected finalized rollback state: %#v", state)
	}
	if state.Terminal == nil || state.Terminal.Outcome != terminalOutcomeRolledBack {
		t.Fatalf("unexpected rollback terminal outcome: %#v", state.Terminal)
	}
}
