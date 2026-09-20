package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestRecoveryHelperInvocationContractIsDistinct(t *testing.T) {
	transactionID := strings.Repeat("c", 64)
	args, err := RecoveryHelperArguments(transactionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != detachedRecoveryHelperCommand || args[1] != transactionID {
		t.Fatalf("unexpected recovery helper args: %#v", args)
	}
	got, matched, err := ParseDetachedRecoveryHelperInvocation(args)
	if err != nil || !matched || got != transactionID {
		t.Fatalf("recovery parse=%q matched=%v err=%v", got, matched, err)
	}
	if _, matched, err := ParseDetachedHelperInvocation(args); err != nil || matched {
		t.Fatalf("recovery args matched automatic helper: matched=%v err=%v", matched, err)
	}
}

func TestRecoveryHelperOwnershipAcceptsChangedCommittedTargetIdentity(t *testing.T) {
	ownership, boundary, _, _, deps := preparedHelperOwnershipForLifecycle(t)
	transactionState, err := readInstallationStateForReconciliationLocked(boundary, ownership.inspection)
	if err != nil {
		t.Fatal(err)
	}
	transactionID := transactionState.Pending.TransactionID
	if err := ownership.commitExecutableReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	helperPath := filepath.Join(boundary.Directory, helperArtifactName)
	if err := ownership.Close(); err != nil {
		t.Fatal(err)
	}

	recovered, err := acquireDetachedHelperOwnershipWithMode(
		context.Background(),
		helperPath,
		transactionID,
		runtime.GOOS,
		runtime.GOARCH,
		500*time.Millisecond,
		helperOwnershipDeps{reconciliation: deps},
		helperOwnershipRecovery,
	)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.reconciliation.Status != ReconciliationCommitted {
		t.Fatalf("status=%q, want committed", recovered.reconciliation.Status)
	}
	if err := recovered.Close(); err != nil {
		t.Fatal(err)
	}

	automatic, err := acquireDetachedHelperOwnershipWith(
		context.Background(),
		helperPath,
		transactionID,
		runtime.GOOS,
		runtime.GOARCH,
		100*time.Millisecond,
		helperOwnershipDeps{reconciliation: deps},
	)
	if automatic != nil || err == nil {
		if automatic != nil {
			_ = automatic.Close()
		}
		t.Fatalf("automatic helper accepted committed recovery state: ownership=%v err=%v", automatic, err)
	}
}

func TestLaunchRecoveryHelperStartsWithSharedUseHandoff(t *testing.T) {
	ownership, boundary, _, _, deps := preparedHelperOwnershipForLifecycle(t)
	if err := ownership.commitExecutableReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	inspection := ownership.inspection
	if err := ownership.Close(); err != nil {
		t.Fatal(err)
	}
	control, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false)
	if err != nil {
		t.Fatal(err)
	}
	state, err := readInstallationStateForReconciliationLocked(boundary, inspection)
	if closeErr := control.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	transactionID := state.Pending.TransactionID

	started := false
	released := false
	startedResult, err := launchRecoveryHelperWith(
		context.Background(),
		boundary,
		inspection,
		runtime.GOOS,
		runtime.GOARCH,
		helperLaunchDeps{
			reconciliation: deps,
			start: func(path string, args []string, env []string) (func() error, error) {
				started = true
				if path != filepath.Join(boundary.Directory, helperArtifactName) {
					t.Fatalf("helper path=%q", path)
				}
				if len(args) != 2 || args[0] != detachedRecoveryHelperCommand || args[1] != transactionID {
					t.Fatalf("recovery args=%#v", args)
				}
				if _, lockErr := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false); !errors.Is(lockErr, filesystem.ErrFileLockBusy) {
					t.Fatalf("control during recovery start=%v, want busy", lockErr)
				}
				if _, lockErr := filesystem.TryAcquireOwnerOnlyFileLock(boundary.UseLockPath, filesystem.LockExclusive, false); !errors.Is(lockErr, filesystem.ErrFileLockBusy) {
					t.Fatalf("exclusive use during recovery start=%v, want busy", lockErr)
				}
				return func() error {
					released = true
					return nil
				}, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !startedResult {
		t.Fatal("successful recovery helper launch was not reported as started")
	}
	if !started || !released {
		t.Fatalf("started=%v released=%v", started, released)
	}
	control, err = filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false)
	if err != nil {
		t.Fatalf("control lock remained held: %v", err)
	}
	_ = control.Close()
	use, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.UseLockPath, filesystem.LockExclusive, false)
	if err != nil {
		t.Fatalf("use lock remained held: %v", err)
	}
	_ = use.Close()
}

func TestLaunchRecoveryHelperReportsStartedWhenProcessReleaseFails(t *testing.T) {
	ownership, boundary, _, _, deps := preparedHelperOwnershipForLifecycle(t)
	inspection := ownership.inspection
	if err := ownership.Close(); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("release failed")
	startedResult, err := launchRecoveryHelperWith(
		context.Background(),
		boundary,
		inspection,
		runtime.GOOS,
		runtime.GOARCH,
		helperLaunchDeps{
			reconciliation: deps,
			start: func(string, []string, []string) (func() error, error) {
				return func() error { return injected }, nil
			},
		},
	)
	if !startedResult || !errors.Is(err, injected) {
		t.Fatalf("started=%v err=%v", startedResult, err)
	}
}

func TestLaunchRecoveryHelperStartFailureLeavesTransactionUntouched(t *testing.T) {
	ownership, boundary, _, candidateBytes, deps := preparedHelperOwnershipForLifecycle(t)
	inspection := ownership.inspection
	if err := ownership.Close(); err != nil {
		t.Fatal(err)
	}
	beforeState, err := os.ReadFile(filepath.Join(boundary.Directory, installationStateFileName))
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("recovery start failed")
	startedResult, err := launchRecoveryHelperWith(
		context.Background(),
		boundary,
		inspection,
		runtime.GOOS,
		runtime.GOARCH,
		helperLaunchDeps{
			reconciliation: deps,
			start: func(string, []string, []string) (func() error, error) {
				return nil, injected
			},
		},
	)
	if startedResult {
		t.Fatal("failed recovery helper start was reported as started")
	}
	if !errors.Is(err, injected) {
		t.Fatalf("launch error=%v, want injected failure", err)
	}
	afterState, err := os.ReadFile(filepath.Join(boundary.Directory, installationStateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeState) != string(afterState) {
		t.Fatal("failed recovery launch modified durable state")
	}
	got, err := os.ReadFile(filepath.Join(boundary.Directory, candidateArtifactName))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(candidateBytes) {
		t.Fatal("failed recovery launch modified candidate")
	}
}
