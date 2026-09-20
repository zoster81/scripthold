package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/filesystem"
)

func committedOwnershipFixture(t *testing.T) (*DetachedHelperOwnership, *InstallationBoundary, []byte, []byte, reconciliationDeps) {
	t.Helper()
	boundary, _, admission, transactionID, sourceBytes, deps := helperOwnershipFixture(t)
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
	t.Cleanup(func() { _ = ownership.Close() })
	if err := ownership.commitExecutableReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	return ownership, boundary, sourceBytes, candidateBytes, deps
}

func TestRollbackCommittedReplacementRestoresSourceWithoutConsumingKnownGood(t *testing.T) {
	ownership, boundary, sourceBytes, candidateBytes, deps := committedOwnershipFixture(t)
	knownGoodPath := filepath.Join(boundary.Directory, knownGoodArtifactName)
	knownGoodID, err := filesystem.CaptureSingleLinkFileIdentity(knownGoodPath)
	if err != nil {
		t.Fatal(err)
	}
	knownGoodBefore, err := os.ReadFile(knownGoodPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(knownGoodBefore) != string(sourceBytes) {
		t.Fatalf("known-good bytes = %q, want source %q", knownGoodBefore, sourceBytes)
	}

	if err := ownership.rollbackCommittedReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps, rollbackDeps{}); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(ownership.inspection.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(sourceBytes) {
		t.Fatalf("rolled-back target = %q, want source %q (candidate was %q)", installed, sourceBytes, candidateBytes)
	}
	if _, err := os.Lstat(filepath.Join(boundary.Directory, candidateArtifactName)); !os.IsNotExist(err) {
		t.Fatalf("rollback candidate remains after commit: %v", err)
	}
	knownGoodAfter, err := os.ReadFile(knownGoodPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(knownGoodAfter) != string(knownGoodBefore) {
		t.Fatal("rollback modified immutable known-good bytes")
	}
	matches, err := knownGoodID.Matches(knownGoodPath)
	if err != nil || !matches {
		t.Fatalf("known-good identity changed: matches=%v err=%v", matches, err)
	}
	result, err := reconcileInstallationLocked(
		context.Background(), boundary, ownership.inspection, runtime.GOOS, runtime.GOARCH, deps, ownership.controlLock,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReconciliationRolledBack || result.rollbackPrepared {
		t.Fatalf("status=%q rollbackPrepared=%v problem=%q", result.Status, result.rollbackPrepared, result.Problem)
	}
}

func TestRollbackCommittedReplacementResumesDurableSourceCandidate(t *testing.T) {
	ownership, boundary, sourceBytes, _, deps := committedOwnershipFixture(t)
	injected := errors.New("stop after durable rollback staging")
	err := ownership.rollbackCommittedReplacementWith(
		context.Background(),
		runtime.GOOS,
		runtime.GOARCH,
		deps,
		rollbackDeps{beforeCommit: func() error { return injected }},
	)
	if !errors.Is(err, injected) {
		t.Fatalf("error = %v, want injected staging stop", err)
	}
	candidatePath := filepath.Join(boundary.Directory, candidateArtifactName)
	staged, err := os.ReadFile(candidatePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(staged) != string(sourceBytes) {
		t.Fatalf("rollback staging bytes = %q, want source %q", staged, sourceBytes)
	}
	result, err := reconcileInstallationLocked(
		context.Background(), boundary, ownership.inspection, runtime.GOOS, runtime.GOARCH, deps, ownership.controlLock,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReconciliationCommitted || !result.rollbackPrepared {
		t.Fatalf("staged rollback status=%q rollbackPrepared=%v problem=%q", result.Status, result.rollbackPrepared, result.Problem)
	}

	if err := ownership.rollbackCommittedReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps, rollbackDeps{}); err != nil {
		t.Fatal(err)
	}
	result, err = reconcileInstallationLocked(
		context.Background(), boundary, ownership.inspection, runtime.GOOS, runtime.GOARCH, deps, ownership.controlLock,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReconciliationRolledBack {
		t.Fatalf("resumed rollback status=%q problem=%q", result.Status, result.Problem)
	}
}

func TestRollbackCommittedReplacementPostCommitVerificationFailureLeavesRolledBackBytes(t *testing.T) {
	ownership, boundary, sourceBytes, _, deps := committedOwnershipFixture(t)
	baseObserver := deps.observeTarget
	verifyFailure := errors.New("injected rollback verification failure")
	failing := deps
	failing.observeTarget = func(ctx context.Context, inspection *StandaloneInspection, goos, goarch string) (installationCurrentState, error) {
		observed, err := baseObserver(ctx, inspection, goos, goarch)
		if err != nil {
			return installationCurrentState{}, err
		}
		if observed.Version == "3.2.1" {
			return installationCurrentState{}, verifyFailure
		}
		return observed, nil
	}
	if err := ownership.rollbackCommittedReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, failing, rollbackDeps{}); err == nil {
		t.Fatal("post-rollback verification failure must be reported")
	}
	installed, err := os.ReadFile(ownership.inspection.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(sourceBytes) {
		t.Fatalf("rolled-back bytes changed after verification failure: %q", installed)
	}
	if _, err := os.Lstat(filepath.Join(boundary.Directory, candidateArtifactName)); !os.IsNotExist(err) {
		t.Fatalf("rollback candidate was not consumed: %v", err)
	}
	result, err := reconcileInstallationLocked(
		context.Background(), boundary, ownership.inspection, runtime.GOOS, runtime.GOARCH, deps, ownership.controlLock,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReconciliationRolledBack {
		t.Fatalf("recoverable status=%q want=%q problem=%q", result.Status, ReconciliationRolledBack, result.Problem)
	}
}

func TestRollbackCommittedReplacementRequiresHelperOwnership(t *testing.T) {
	var ownership *DetachedHelperOwnership
	if err := ownership.RollbackCommittedReplacement(context.Background()); err == nil {
		t.Fatal("nil helper ownership must fail closed")
	}
}

func TestRollbackCommittedReplacementRefreshesStaleTargetIdentity(t *testing.T) {
	boundary, _, admission, transactionID, sourceBytes, deps := helperOwnershipFixture(t)
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
	defer ownership.Close()
	staleInspection := *ownership.inspection

	if err := ownership.commitExecutableReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	ownership.inspection = &staleInspection
	if err := ownership.rollbackCommittedReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps, rollbackDeps{}); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(ownership.inspection.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(sourceBytes) {
		t.Fatalf("rollback from stale inspection installed %q, want source %q", installed, sourceBytes)
	}
}

func TestRollbackCommittedReplacementRejectsUnexpectedCandidateSlotBytes(t *testing.T) {
	ownership, boundary, _, candidateBytes, deps := committedOwnershipFixture(t)
	candidatePath := filepath.Join(boundary.Directory, candidateArtifactName)
	if err := os.WriteFile(candidatePath, []byte("unexpected-rollback-bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := filesystem.RestrictOwnerOnlyExecutable(candidatePath); err != nil {
			t.Fatal(err)
		}
	}
	result, err := reconcileInstallationLocked(
		context.Background(), boundary, ownership.inspection, runtime.GOOS, runtime.GOARCH, deps, ownership.controlLock,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReconciliationRecoveryRequired {
		t.Fatalf("unexpected candidate slot status=%q, want %q", result.Status, ReconciliationRecoveryRequired)
	}
	if err := ownership.rollbackCommittedReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps, rollbackDeps{}); err == nil {
		t.Fatal("rollback accepted unexpected candidate slot bytes")
	}
	installed, err := os.ReadFile(ownership.inspection.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(candidateBytes) {
		t.Fatalf("rejected rollback changed installed candidate bytes: %q", installed)
	}
}

func TestPendingStateRejectsIndistinguishableSourceAndCandidateDigests(t *testing.T) {
	boundary, inspection, _, _, _ := prepareReconciliationFixture(t)
	state, err := readInstallationState(boundary, inspection)
	if err != nil {
		t.Fatal(err)
	}
	if state.Pending == nil {
		t.Fatal("pending state missing")
	}
	state.Pending.CandidateSHA256 = state.Pending.SourceSHA256
	if err := validateInstallationState(state, inspection); err == nil {
		t.Fatal("equal source/candidate digests must fail closed")
	}
}

func TestRollbackCommittedReplacementIsIdempotentAfterSuccess(t *testing.T) {
	ownership, boundary, sourceBytes, _, deps := committedOwnershipFixture(t)
	if err := ownership.rollbackCommittedReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps, rollbackDeps{}); err != nil {
		t.Fatal(err)
	}
	if err := ownership.rollbackCommittedReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps, rollbackDeps{}); err != nil {
		t.Fatalf("second rollback = %v, want idempotent success", err)
	}
	installed, err := os.ReadFile(ownership.inspection.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(sourceBytes) {
		t.Fatalf("idempotent rollback changed target: %q", installed)
	}
	if _, err := os.Lstat(filepath.Join(boundary.Directory, candidateArtifactName)); !os.IsNotExist(err) {
		t.Fatalf("idempotent rollback recreated candidate: %v", err)
	}
}
