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

func helperOwnershipFixture(t *testing.T) (*InstallationBoundary, *StandaloneInspection, *ProcessAdmission, string, []byte, reconciliationDeps) {
	t.Helper()
	boundary, inspection, admission, candidate, installedDeps, sourceBytes, candidateBytes := pendingPreparationFixture(t)
	transactionID := strings.Repeat("5", 64)
	if _, err := preparePendingTransactionWith(
		context.Background(), boundary, inspection, admission, candidate, runtime.GOOS, runtime.GOARCH,
		pendingPreparationDeps{
			installedEvidence: installedDeps,
			validateCandidate: func(context.Context, *PreparedCandidate, string, string) error { return nil },
			newTransactionID:  func() (string, error) { return transactionID, nil },
		},
	); err != nil {
		t.Fatal(err)
	}
	return boundary, inspection, admission, transactionID, sourceBytes, reconciliationDeps{
		observeTarget: fixtureTargetObserver(sourceBytes, candidateBytes),
	}
}

func TestDetachedHelperInvocationContract(t *testing.T) {
	transactionID := strings.Repeat("a", 64)
	args, err := DetachedHelperArguments(transactionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != detachedHelperCommand || args[1] != transactionID {
		t.Fatalf("unexpected helper args: %#v", args)
	}
	got, matched, err := ParseDetachedHelperInvocation(args)
	if err != nil || !matched || got != transactionID {
		t.Fatalf("parse = %q, %v, %v", got, matched, err)
	}
	if _, matched, err := ParseDetachedHelperInvocation([]string{"server"}); err != nil || matched {
		t.Fatalf("non-helper args matched: matched=%v err=%v", matched, err)
	}
	for _, invalid := range [][]string{
		{detachedHelperCommand},
		{detachedHelperCommand, "bad"},
		{detachedHelperCommand, transactionID, "extra"},
	} {
		if _, matched, err := ParseDetachedHelperInvocation(invalid); !matched || err == nil {
			t.Fatalf("invalid helper args accepted: %#v matched=%v err=%v", invalid, matched, err)
		}
	}
}

func TestAcquireDetachedHelperOwnershipWaitsForExistingProcessAndThenOwnsBothLocks(t *testing.T) {
	boundary, _, admission, transactionID, sourceBytes, deps := helperOwnershipFixture(t)
	helperPath := filepath.Join(boundary.Directory, helperArtifactName)

	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := make(chan struct {
		ownership *DetachedHelperOwnership
		err       error
	}, 1)
	go func() {
		ownership, err := acquireDetachedHelperOwnershipWith(
			waitCtx, helperPath, transactionID, runtime.GOOS, runtime.GOARCH,
			500*time.Millisecond, helperOwnershipDeps{reconciliation: deps},
		)
		result <- struct {
			ownership *DetachedHelperOwnership
			err       error
		}{ownership: ownership, err: err}
	}()

	time.Sleep(50 * time.Millisecond)
	select {
	case outcome := <-result:
		t.Fatalf("helper ownership returned before shared use lock released: %v", outcome.err)
	default:
	}
	if _, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false); !errors.Is(err, filesystem.ErrFileLockBusy) {
		t.Fatalf("control lock while helper waits = %v, want busy", err)
	}
	if err := admission.Close(); err != nil {
		t.Fatal(err)
	}
	outcome := <-result
	if outcome.err != nil {
		t.Fatal(outcome.err)
	}
	defer outcome.ownership.Close()
	if outcome.ownership.reconciliation.Status != ReconciliationPrepared {
		t.Fatalf("status = %q", outcome.ownership.reconciliation.Status)
	}
	if _, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false); !errors.Is(err, filesystem.ErrFileLockBusy) {
		t.Fatalf("control lock while helper owns = %v, want busy", err)
	}
	if _, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.UseLockPath, filesystem.LockShared, false); !errors.Is(err, filesystem.ErrFileLockBusy) {
		t.Fatalf("shared use lock while helper owns exclusive = %v, want busy", err)
	}
	got, err := os.ReadFile(outcome.ownership.inspection.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(sourceBytes) {
		t.Fatalf("helper ownership modified target: %q", got)
	}
}

func TestAcquireDetachedHelperOwnershipTimeoutLeavesPreparedStateUntouched(t *testing.T) {
	boundary, inspection, admission, transactionID, sourceBytes, deps := helperOwnershipFixture(t)
	defer admission.Close()
	helperPath := filepath.Join(boundary.Directory, helperArtifactName)
	beforeState, err := os.ReadFile(filepath.Join(boundary.Directory, installationStateFileName))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ownership, err := acquireDetachedHelperOwnershipWith(
		ctx, helperPath, transactionID, runtime.GOOS, runtime.GOARCH,
		60*time.Millisecond, helperOwnershipDeps{reconciliation: deps},
	)
	if ownership != nil || !errors.Is(err, ErrDetachedHelperUseTimeout) {
		t.Fatalf("ownership=%v err=%v, want timeout", ownership, err)
	}
	afterState, err := os.ReadFile(filepath.Join(boundary.Directory, installationStateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(afterState) != string(beforeState) {
		t.Fatal("timeout modified state")
	}
	got, err := os.ReadFile(inspection.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(sourceBytes) {
		t.Fatal("timeout modified target")
	}
	if _, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false); err != nil {
		t.Fatalf("control lock not released after timeout: %v", err)
	}
}

func TestAcquireDetachedHelperOwnershipRejectsTransactionMismatch(t *testing.T) {
	boundary, _, admission, _, _, deps := helperOwnershipFixture(t)
	defer admission.Close()
	helperPath := filepath.Join(boundary.Directory, helperArtifactName)
	ownership, err := acquireDetachedHelperOwnershipWith(
		context.Background(), helperPath, strings.Repeat("6", 64), runtime.GOOS, runtime.GOARCH,
		50*time.Millisecond, helperOwnershipDeps{reconciliation: deps},
	)
	if ownership != nil || err == nil {
		t.Fatalf("transaction mismatch accepted: ownership=%v err=%v", ownership, err)
	}
}

func TestAcquireDetachedHelperOwnershipRequiresPreparedReconciliation(t *testing.T) {
	boundary, _, admission, transactionID, _, deps := helperOwnershipFixture(t)
	if err := admission.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(boundary.Directory, candidateArtifactName)); err != nil {
		t.Fatal(err)
	}
	ownership, err := acquireDetachedHelperOwnershipWith(
		context.Background(), filepath.Join(boundary.Directory, helperArtifactName), transactionID,
		runtime.GOOS, runtime.GOARCH, 100*time.Millisecond, helperOwnershipDeps{reconciliation: deps},
	)
	if ownership != nil || err == nil {
		t.Fatalf("non-prepared transaction accepted: ownership=%v err=%v", ownership, err)
	}
}

func TestAcquireDetachedHelperOwnershipRejectsHelperPathOutsideStateRoot(t *testing.T) {
	boundary, _, admission, transactionID, _, deps := helperOwnershipFixture(t)
	defer admission.Close()
	outside := filepath.Join(t.TempDir(), helperArtifactName)
	data, err := os.ReadFile(filepath.Join(boundary.Directory, helperArtifactName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, data, 0o700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := filesystem.RestrictOwnerOnlyExecutable(outside); err != nil {
			t.Fatal(err)
		}
	}
	ownership, err := acquireDetachedHelperOwnershipWith(
		context.Background(), outside, transactionID, runtime.GOOS, runtime.GOARCH,
		50*time.Millisecond, helperOwnershipDeps{reconciliation: deps},
	)
	if ownership != nil || err == nil {
		t.Fatalf("outside helper path accepted: ownership=%v err=%v", ownership, err)
	}
}

func TestAcquireDetachedHelperOwnershipWaitsForControlLaunchRace(t *testing.T) {
	boundary, _, admission, transactionID, _, deps := helperOwnershipFixture(t)
	if err := admission.Close(); err != nil {
		t.Fatal(err)
	}
	control, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false)
	if err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		ownership, acquireErr := acquireDetachedHelperOwnershipWith(
			context.Background(),
			filepath.Join(boundary.Directory, helperArtifactName),
			transactionID,
			runtime.GOOS,
			runtime.GOARCH,
			500*time.Millisecond,
			helperOwnershipDeps{
				reconciliation:     deps,
				controlWaitTimeout: 500 * time.Millisecond,
			},
		)
		if ownership != nil {
			acquireErr = errors.Join(acquireErr, ownership.Close())
		}
		result <- acquireErr
	}()

	time.Sleep(30 * time.Millisecond)
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatalf("helper did not survive control launch race: %v", err)
	}
}

func TestAcquireDetachedHelperOwnershipControlTimeoutLeavesTargetUntouched(t *testing.T) {
	boundary, inspection, admission, transactionID, sourceBytes, deps := helperOwnershipFixture(t)
	if err := admission.Close(); err != nil {
		t.Fatal(err)
	}
	control, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()

	ownership, err := acquireDetachedHelperOwnershipWith(
		context.Background(),
		filepath.Join(boundary.Directory, helperArtifactName),
		transactionID,
		runtime.GOOS,
		runtime.GOARCH,
		500*time.Millisecond,
		helperOwnershipDeps{
			reconciliation:     deps,
			controlWaitTimeout: 60 * time.Millisecond,
		},
	)
	if ownership != nil || !errors.Is(err, ErrDetachedHelperControlTimeout) {
		t.Fatalf("ownership=%v err=%v, want control timeout", ownership, err)
	}
	got, err := os.ReadFile(inspection.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(sourceBytes) {
		t.Fatal("control timeout modified target")
	}
}

func TestAcquireDetachedHelperOwnershipCancellationReleasesControl(t *testing.T) {
	boundary, inspection, admission, transactionID, sourceBytes, deps := helperOwnershipFixture(t)
	defer admission.Close()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		ownership, acquireErr := acquireDetachedHelperOwnershipWith(
			ctx,
			filepath.Join(boundary.Directory, helperArtifactName),
			transactionID,
			runtime.GOOS,
			runtime.GOARCH,
			time.Second,
			helperOwnershipDeps{reconciliation: deps},
		)
		if ownership != nil {
			acquireErr = errors.Join(acquireErr, ownership.Close())
		}
		result <- acquireErr
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		lock, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false)
		if errors.Is(err, filesystem.ErrFileLockBusy) {
			break
		}
		if err == nil {
			_ = lock.Close()
		} else {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not acquire control lock before cancellation")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel result = %v, want context.Canceled", err)
	}
	if _, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false); err != nil {
		t.Fatalf("control lock not released after cancellation: %v", err)
	}
	got, err := os.ReadFile(inspection.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(sourceBytes) {
		t.Fatal("cancellation modified target")
	}
}
