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

func TestCommitExecutableReplacementConsumesCandidateAndReconcilesCommitted(t *testing.T) {
	boundary, _, admission, transactionID, _, deps := helperOwnershipFixture(t)
	if err := admission.Close(); err != nil {
		t.Fatal(err)
	}
	candidatePath := filepath.Join(boundary.Directory, candidateArtifactName)
	candidateBytes, err := os.ReadFile(candidatePath)
	if err != nil {
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
	defer ownership.Close()

	if err := ownership.commitExecutableReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(candidatePath); !os.IsNotExist(err) {
		t.Fatalf("candidate artifact remains after commit: %v", err)
	}
	installed, err := os.ReadFile(ownership.inspection.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(candidateBytes) {
		t.Fatalf("installed bytes = %q, want candidate %q", installed, candidateBytes)
	}
	result, err := reconcileInstallationLocked(
		context.Background(), boundary, ownership.inspection, runtime.GOOS, runtime.GOARCH, deps, ownership.controlLock,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReconciliationCommitted {
		t.Fatalf("status = %q, want %q; problem=%q", result.Status, ReconciliationCommitted, result.Problem)
	}
}

func TestCommitExecutableReplacementRequiresHelperOwnership(t *testing.T) {
	var ownership *DetachedHelperOwnership
	if err := ownership.CommitExecutableReplacement(context.Background()); err == nil {
		t.Fatal("nil helper ownership must fail closed")
	}
}

func TestCommitExecutableReplacementVerificationFailureLeavesCommittedBytesForRecovery(t *testing.T) {
	boundary, _, admission, transactionID, _, deps := helperOwnershipFixture(t)
	if err := admission.Close(); err != nil {
		t.Fatal(err)
	}
	candidatePath := filepath.Join(boundary.Directory, candidateArtifactName)
	candidateBytes, err := os.ReadFile(candidatePath)
	if err != nil {
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
	defer ownership.Close()

	baseObserver := deps.observeTarget
	verifyFailure := errors.New("injected committed verification failure")
	failingDeps := deps
	failingDeps.observeTarget = func(ctx context.Context, inspection *StandaloneInspection, goos, goarch string) (installationCurrentState, error) {
		observed, observeErr := baseObserver(ctx, inspection, goos, goarch)
		if observeErr != nil {
			return installationCurrentState{}, observeErr
		}
		if observed.Version == "3.3.0" {
			return installationCurrentState{}, verifyFailure
		}
		return observed, nil
	}
	if err := ownership.commitExecutableReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, failingDeps); err == nil {
		t.Fatal("post-commit verification failure must be reported")
	}
	if _, err := os.Lstat(candidatePath); !os.IsNotExist(err) {
		t.Fatalf("candidate was not consumed despite committed replacement: %v", err)
	}
	installed, err := os.ReadFile(ownership.inspection.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(candidateBytes) {
		t.Fatalf("installed bytes changed after verification failure: %q", installed)
	}
	result, err := reconcileInstallationLocked(
		context.Background(), boundary, ownership.inspection, runtime.GOOS, runtime.GOARCH, deps, ownership.controlLock,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReconciliationCommitted {
		t.Fatalf("recoverable observed status = %q, want %q; problem=%q", result.Status, ReconciliationCommitted, result.Problem)
	}
}
