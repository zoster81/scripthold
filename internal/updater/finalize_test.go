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

func installedDepsForCurrent(current installationCurrentState) installedEvidenceDeps {
	return installedEvidenceDeps{
		readBuildInfo: func(io.ReaderAt) (*debug.BuildInfo, error) {
			return &debug.BuildInfo{
				Main: debug.Module{Path: officialModulePath},
				Settings: []debug.BuildSetting{
					{Key: "GOOS", Value: current.Build.GOOS},
					{Key: "GOARCH", Value: current.Build.GOARCH},
					{Key: "vcs", Value: current.Build.VCS},
					{Key: "vcs.revision", Value: current.Build.Revision},
					{Key: "vcs.modified", Value: "false"},
				},
			}, nil
		},
		smokeVersion: func(context.Context, string) (string, error) {
			return current.Version, nil
		},
	}
}

func TestFinalizeCommittedPublishesStableBeforeAdmissionCleanup(t *testing.T) {
	ownership, boundary, sourceBytes, _, deps := committedOwnershipFixture(t)
	if err := ownership.finalizeTransactionWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}

	state, err := readInstallationStateLocked(boundary, ownership.inspection)
	if err != nil {
		t.Fatal(err)
	}
	if state.Pending != nil {
		t.Fatal("finalized committed state still has pending transaction")
	}
	if state.Current.Version != "3.3.0" || state.Current.SHA256 == bytesSHA256ForTest(sourceBytes) {
		t.Fatalf("unexpected committed current evidence: %#v", state.Current)
	}
	if state.Target.Identity != ownership.inspection.ExecutableIdentity.StableKey() {
		t.Fatalf("target identity = %q, want %q", state.Target.Identity, ownership.inspection.ExecutableIdentity.StableKey())
	}
	if state.Terminal == nil || state.Terminal.Outcome != terminalOutcomeCommitted ||
		!state.Terminal.CleanupPending || state.Terminal.SourceSHA256 != bytesSHA256ForTest(sourceBytes) {
		t.Fatalf("unexpected committed terminal summary: %#v", state.Terminal)
	}
	for _, name := range []string{knownGoodArtifactName, helperArtifactName} {
		if _, err := os.Stat(filepath.Join(boundary.Directory, name)); err != nil {
			t.Fatalf("artifact %s was cleaned by detached helper: %v", name, err)
		}
	}
	if err := ownership.finalizeTransactionWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatalf("second finalization = %v, want idempotent success", err)
	}

	if err := ownership.Close(); err != nil {
		t.Fatal(err)
	}
	admission, err := admitStableProcessWith(
		context.Background(),
		boundary,
		ownership.inspection,
		runtime.GOOS,
		runtime.GOARCH,
		installedDepsForCurrent(state.Current),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer admission.Close()
	for _, name := range []string{knownGoodArtifactName, candidateArtifactName, helperArtifactName} {
		if _, err := os.Lstat(filepath.Join(boundary.Directory, name)); !os.IsNotExist(err) {
			t.Fatalf("artifact %s remains after normal admission cleanup: %v", name, err)
		}
	}
	cleanState, err := readInstallationState(boundary, ownership.inspection)
	if err != nil {
		t.Fatal(err)
	}
	if cleanState.Terminal == nil || cleanState.Terminal.CleanupPending {
		t.Fatalf("cleanup completion was not persisted: %#v", cleanState.Terminal)
	}
}

func TestFinalizeRolledBackPublishesStableSourceState(t *testing.T) {
	ownership, boundary, sourceBytes, _, deps := committedOwnershipFixture(t)
	if err := ownership.rollbackCommittedReplacementWith(
		context.Background(), runtime.GOOS, runtime.GOARCH, deps, rollbackDeps{},
	); err != nil {
		t.Fatal(err)
	}
	if err := ownership.finalizeTransactionWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	state, err := readInstallationStateLocked(boundary, ownership.inspection)
	if err != nil {
		t.Fatal(err)
	}
	if state.Pending != nil || state.Current.SHA256 != bytesSHA256ForTest(sourceBytes) ||
		state.Current.Version != "3.2.1" {
		t.Fatalf("unexpected rolled-back stable state: %#v", state)
	}
	if state.Target.Identity != ownership.inspection.ExecutableIdentity.StableKey() {
		t.Fatalf("rolled-back target identity not refreshed: %#v", state.Target)
	}
	if state.Terminal == nil || state.Terminal.Outcome != terminalOutcomeRolledBack ||
		!state.Terminal.CleanupPending || state.Terminal.SourceSHA256 != state.Current.SHA256 {
		t.Fatalf("unexpected rolled-back terminal summary: %#v", state.Terminal)
	}
}

func TestAdmissionRefusesChangedFinalizedArtifact(t *testing.T) {
	ownership, boundary, _, _, deps := committedOwnershipFixture(t)
	if err := ownership.finalizeTransactionWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	state, err := readInstallationStateLocked(boundary, ownership.inspection)
	if err != nil {
		t.Fatal(err)
	}
	if err := ownership.Close(); err != nil {
		t.Fatal(err)
	}
	helperPath := filepath.Join(boundary.Directory, helperArtifactName)
	if err := os.WriteFile(helperPath, []byte("changed-helper"), 0o700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := filesystem.RestrictOwnerOnlyExecutable(helperPath); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := admitStableProcessWith(
		context.Background(),
		boundary,
		ownership.inspection,
		runtime.GOOS,
		runtime.GOARCH,
		installedDepsForCurrent(state.Current),
	); err == nil {
		t.Fatal("admission cleaned or ignored changed finalized helper artifact")
	}
	if got, err := os.ReadFile(helperPath); err != nil || string(got) != "changed-helper" {
		t.Fatalf("changed helper was modified: %q err=%v", got, err)
	}
}

func TestFinalizeCommittedRejectsRollbackAlreadyStaged(t *testing.T) {
	ownership, boundary, _, _, deps := committedOwnershipFixture(t)
	stop := errors.New("leave rollback staged")
	err := ownership.rollbackCommittedReplacementWith(
		context.Background(),
		runtime.GOOS,
		runtime.GOARCH,
		deps,
		rollbackDeps{beforeCommit: func() error { return stop }},
	)
	if !errors.Is(err, stop) {
		t.Fatalf("rollback staging error = %v, want injected stop", err)
	}
	if err := ownership.finalizeTransactionWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err == nil {
		t.Fatal("committed finalization accepted an already-staged rollback")
	}
	state, err := readInstallationStateForReconciliationLocked(boundary, ownership.inspection)
	if err != nil {
		t.Fatal(err)
	}
	if state.Pending == nil {
		t.Fatal("rejected finalization removed pending transaction")
	}
}

func TestAdmissionResumesPartialFinalizedCleanup(t *testing.T) {
	ownership, boundary, _, _, deps := committedOwnershipFixture(t)
	if err := ownership.finalizeTransactionWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	state, err := readInstallationStateLocked(boundary, ownership.inspection)
	if err != nil {
		t.Fatal(err)
	}
	if err := ownership.Close(); err != nil {
		t.Fatal(err)
	}

	knownGoodPath := filepath.Join(boundary.Directory, knownGoodArtifactName)
	if err := os.Remove(knownGoodPath); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.SyncDirectory(boundary.Directory); err != nil {
		t.Fatal(err)
	}

	admission, err := admitStableProcessWith(
		context.Background(),
		boundary,
		ownership.inspection,
		runtime.GOOS,
		runtime.GOARCH,
		installedDepsForCurrent(state.Current),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer admission.Close()
	for _, name := range []string{knownGoodArtifactName, candidateArtifactName, helperArtifactName} {
		if _, err := os.Lstat(filepath.Join(boundary.Directory, name)); !os.IsNotExist(err) {
			t.Fatalf("artifact %s remains after resumed cleanup: %v", name, err)
		}
	}
	cleanState, err := readInstallationState(boundary, ownership.inspection)
	if err != nil {
		t.Fatal(err)
	}
	if cleanState.Terminal == nil || cleanState.Terminal.CleanupPending {
		t.Fatalf("resumed cleanup completion was not persisted: %#v", cleanState.Terminal)
	}
}
