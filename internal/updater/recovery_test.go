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

func TestRecoverPreparedTransactionCommitsAndFinalizes(t *testing.T) {
	ownership, boundary, _, candidateBytes, deps := preparedHelperOwnershipForLifecycle(t)
	if err := ownership.recoverTransactionWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	state, err := readInstallationStateLocked(boundary, ownership.inspection)
	if err != nil {
		t.Fatal(err)
	}
	if state.Pending != nil || state.Current.SHA256 != bytesSHA256ForTest(candidateBytes) {
		t.Fatalf("unexpected recovered committed state: %#v", state)
	}
	if state.Terminal == nil || state.Terminal.Outcome != terminalOutcomeCommitted {
		t.Fatalf("unexpected terminal outcome: %#v", state.Terminal)
	}
}

func TestRecoverCommittedTransactionFinalizesWithoutReplacement(t *testing.T) {
	ownership, boundary, _, candidateBytes, deps := preparedHelperOwnershipForLifecycle(t)
	if err := ownership.commitExecutableReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	beforeIdentity := ownership.inspection.ExecutableIdentity.StableKey()
	if err := ownership.recoverTransactionWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	if ownership.inspection.ExecutableIdentity.StableKey() != beforeIdentity {
		t.Fatal("verified committed recovery replaced target again")
	}
	state, err := readInstallationStateLocked(boundary, ownership.inspection)
	if err != nil {
		t.Fatal(err)
	}
	if state.Pending != nil || state.Current.SHA256 != bytesSHA256ForTest(candidateBytes) {
		t.Fatalf("unexpected finalized committed recovery state: %#v", state)
	}
}

func TestRecoverRolledBackTransactionFinalizesWithoutAnotherReplacement(t *testing.T) {
	ownership, boundary, sourceBytes, _, deps := preparedHelperOwnershipForLifecycle(t)
	if err := ownership.commitExecutableReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	if err := ownership.rollbackCommittedReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps, rollbackDeps{}); err != nil {
		t.Fatal(err)
	}
	beforeIdentity := ownership.inspection.ExecutableIdentity.StableKey()
	if err := ownership.recoverTransactionWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	if ownership.inspection.ExecutableIdentity.StableKey() != beforeIdentity {
		t.Fatal("rolled-back recovery replaced target again")
	}
	state, err := readInstallationStateLocked(boundary, ownership.inspection)
	if err != nil {
		t.Fatal(err)
	}
	if state.Pending != nil || state.Current.SHA256 != bytesSHA256ForTest(sourceBytes) {
		t.Fatalf("unexpected finalized rollback state: %#v", state)
	}
	if state.Terminal == nil || state.Terminal.Outcome != terminalOutcomeRolledBack {
		t.Fatalf("unexpected terminal outcome: %#v", state.Terminal)
	}
}

func TestRecoverUnverifiedCandidateBytesRollsBackAndFinalizes(t *testing.T) {
	ownership, boundary, sourceBytes, candidateBytes, deps := preparedHelperOwnershipForLifecycle(t)
	base := deps.observeTarget
	deps.observeTarget = func(ctx context.Context, inspection *StandaloneInspection, goos, goarch string) (installationCurrentState, error) {
		data, err := os.ReadFile(inspection.ExecutablePath)
		if err != nil {
			return installationCurrentState{}, err
		}
		if string(data) == string(candidateBytes) {
			return installationCurrentState{}, errors.New("candidate verification failed")
		}
		return base(ctx, inspection, goos, goarch)
	}
	if err := ownership.recoverTransactionWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(ownership.inspection.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(sourceBytes) {
		t.Fatalf("recovery installed %q, want source %q", installed, sourceBytes)
	}
	state, err := readInstallationStateLocked(boundary, ownership.inspection)
	if err != nil {
		t.Fatal(err)
	}
	if state.Pending != nil || state.Terminal == nil || state.Terminal.Outcome != terminalOutcomeRolledBack {
		t.Fatalf("unexpected recovered rollback state: %#v", state)
	}
}

func TestRecoverResumesStagedRollback(t *testing.T) {
	ownership, boundary, sourceBytes, _, deps := preparedHelperOwnershipForLifecycle(t)
	if err := ownership.commitExecutableReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	stop := errors.New("leave rollback staged")
	err := ownership.rollbackCommittedReplacementWith(
		context.Background(),
		runtime.GOOS,
		runtime.GOARCH,
		deps,
		rollbackDeps{beforeCommit: func() error { return stop }},
	)
	if !errors.Is(err, stop) {
		t.Fatalf("staging error=%v, want injected stop", err)
	}
	if err := ownership.recoverTransactionWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(ownership.inspection.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(sourceBytes) {
		t.Fatalf("resumed rollback installed %q, want source %q", installed, sourceBytes)
	}
	state, err := readInstallationStateLocked(boundary, ownership.inspection)
	if err != nil {
		t.Fatal(err)
	}
	if state.Pending != nil || state.Terminal == nil || state.Terminal.Outcome != terminalOutcomeRolledBack {
		t.Fatalf("unexpected staged rollback recovery state: %#v", state)
	}
}

func TestRecoverUnknownTargetFailsClosed(t *testing.T) {
	ownership, boundary, _, _, deps := preparedHelperOwnershipForLifecycle(t)
	target := ownership.inspection.ExecutablePath
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("unknown-target-bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := filesystem.RestrictOwnerOnlyExecutable(target); err != nil {
			t.Fatal(err)
		}
	}
	beforeCandidate, err := os.ReadFile(filepath.Join(boundary.Directory, candidateArtifactName))
	if err != nil {
		t.Fatal(err)
	}
	if err := ownership.recoverTransactionWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err == nil {
		t.Fatal("unknown target bytes were accepted for automatic recovery")
	}
	afterCandidate, err := os.ReadFile(filepath.Join(boundary.Directory, candidateArtifactName))
	if err != nil {
		t.Fatal(err)
	}
	if string(afterCandidate) != string(beforeCandidate) {
		t.Fatal("failed recovery modified prepared candidate")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "unknown-target-bytes" {
		t.Fatalf("failed recovery modified unknown target: %q", got)
	}
}
