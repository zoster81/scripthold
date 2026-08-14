//go:build !windows

package backupstore

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
)

func TestRecoveryUnixSpecialFileIsEvidenceOnlyAndNeverFollowed(t *testing.T) {
	root, manifests := createRecoveryScanStore(t, []byte("Unix special-file recovery"))
	fifo := filepath.Join(root, "manifests", "attacker-fifo.json")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("FIFO creation unavailable: %v", err)
	}
	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.TrustedRecords) != len(manifests) || evidence.RejectedRecordCount == 0 {
		t.Fatalf("special-file evidence classification=%#v", evidence)
	}
	if recoveryEvidenceReasonCount(evidence, RecoveryRejectManifestInvalid) == 0 {
		t.Fatalf("FIFO manifest evidence did not produce a stable rejection reason: %#v", evidence.RejectedReasonCounts)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}
