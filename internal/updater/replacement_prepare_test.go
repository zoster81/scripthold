package updater

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestPrepareExecutableReplacementKeepsTargetAndCandidateBytes(t *testing.T) {
	boundary, inspection, admission, transactionID, sourceBytes, deps := helperOwnershipFixture(t)
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

	candidatePath := filepath.Join(boundary.Directory, candidateArtifactName)
	beforeCandidate, err := os.ReadFile(candidatePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ownership.prepareExecutableReplacementWith(context.Background(), runtime.GOOS, runtime.GOARCH, deps); err != nil {
		t.Fatal(err)
	}
	target, err := os.ReadFile(inspection.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(target) != string(sourceBytes) {
		t.Fatalf("replacement preparation modified target: %q", target)
	}
	afterCandidate, err := os.ReadFile(candidatePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterCandidate) != string(beforeCandidate) {
		t.Fatal("replacement preparation changed candidate bytes")
	}
	result, err := reconcileInstallationLocked(
		context.Background(), boundary, inspection, runtime.GOOS, runtime.GOARCH, deps, ownership.controlLock,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReconciliationPrepared {
		t.Fatalf("status = %q, want %q; problem=%q", result.Status, ReconciliationPrepared, result.Problem)
	}
}

func TestPrepareExecutableReplacementRequiresHelperOwnership(t *testing.T) {
	var ownership *DetachedHelperOwnership
	if err := ownership.PrepareExecutableReplacement(context.Background()); err == nil {
		t.Fatal("nil helper ownership must fail closed")
	}
}
