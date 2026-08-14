package backupstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestScanRecoveryEvidenceHealthyStoreIsDeterministicAndMutationFree(t *testing.T) {
	root, manifests := createRecoveryScanStore(t, []byte("first recovery object"), []byte("second recovery object"))
	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)

	first, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("recovery evidence is nondeterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if !first.DescriptorValid || !first.CoverageComplete || !validHexIdentifier(first.DescriptorFingerprint) || !validHexIdentifier(first.EvidenceDigest) {
		t.Fatalf("healthy evidence identity=%#v", first)
	}
	if first.SourceDescriptor.StoreID == "" || len(first.TrustedRecords) != len(manifests) || first.TrustedObjectCount != 2 || first.TrustedBytes <= 0 || first.RejectedRecordCount != 0 {
		t.Fatalf("healthy evidence counts=%#v", first)
	}
	if first.OrphanObjectCount != 0 || first.UnknownEntryCount != 0 || first.ResidueEntryCount != 0 || first.DerivedStateIssueCount != 0 {
		t.Fatalf("healthy evidence classified damage: %#v", first)
	}
	wantManifests := append([]Manifest(nil), manifests...)
	sort.Slice(wantManifests, func(i, j int) bool { return wantManifests[i].BackupID < wantManifests[j].BackupID })
	for index, record := range first.TrustedRecords {
		if record.Manifest.BackupID != wantManifests[index].BackupID || record.Manifest.ManifestChecksum != wantManifests[index].ManifestChecksum {
			t.Fatalf("trusted record[%d]=%#v want=%#v", index, record, wantManifests[index])
		}
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func TestScanRecoveryEvidenceClassifiesRecoverableDamage(t *testing.T) {
	firstContent := []byte("first accepted object")
	secondContent := []byte("second corrupt object")
	root, manifests := createRecoveryScanStore(t, firstContent, secondContent)

	corruptPath := objectPath(root, manifests[1].ObjectDigest)
	corrupt := bytes.Repeat([]byte{'X'}, len(secondContent))
	if err := os.WriteFile(corruptPath, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(corruptPath, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "index", "index-v1.json")); err != nil {
		t.Fatal(err)
	}

	orphanContent := []byte("trusted orphan object")
	orphanDigestBytes := sha256.Sum256(orphanContent)
	orphanDigest := hex.EncodeToString(orphanDigestBytes[:])
	orphanPath := objectPath(root, orphanDigest)
	if err := os.MkdirAll(filepath.Dir(orphanPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(filepath.Dir(orphanPath), true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphanPath, orphanContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(orphanPath, false); err != nil {
		t.Fatal(err)
	}

	invalidManifest := filepath.Join(root, "manifests", "operator-corrupt.json")
	if err := os.WriteFile(invalidManifest, []byte("{not-json}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(invalidManifest, false); err != nil {
		t.Fatal(err)
	}
	residue := filepath.Join(root, "staging", "recovery-residue.tmp")
	if err := os.WriteFile(residue, []byte("residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(residue, false); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(root, "operator-note.bin")
	if err := os.WriteFile(unknown, []byte("unknown"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(unknown, false); err != nil {
		t.Fatal(err)
	}

	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.CoverageComplete || !evidence.DescriptorValid {
		t.Fatalf("recoverable damage became incomplete: %#v", evidence)
	}
	if len(evidence.TrustedRecords) != 1 || evidence.TrustedRecords[0].Manifest.BackupID != manifests[0].BackupID {
		t.Fatalf("trusted records=%#v", evidence.TrustedRecords)
	}
	if evidence.RejectedRecordCount != 2 {
		t.Fatalf("rejected records=%d want=2: %#v", evidence.RejectedRecordCount, evidence)
	}
	if len(evidence.RejectedRecords) != 1 || evidence.RejectedRecords[0].BackupID != manifests[1].BackupID || evidence.RejectedRecords[0].Reason != RecoveryRejectObjectDigestMismatch {
		t.Fatalf("identified rejection=%#v", evidence.RejectedRecords)
	}
	if recoveryEvidenceReasonCount(evidence, RecoveryRejectManifestInvalid) == 0 {
		t.Fatalf("missing invalid-manifest aggregate: %#v", evidence.RejectedReasonCounts)
	}
	if evidence.OrphanObjectCount != 1 || evidence.OrphanObjectBytes != int64(len(orphanContent)) || evidence.DerivedStateIssueCount != 1 || evidence.ResidueEntryCount != 1 || evidence.UnknownEntryCount != 1 {
		t.Fatalf("damage classification=%#v", evidence)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func TestScanRecoveryEvidenceLimitsAreVisibleAndNonAuthoritative(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("12345"), []byte("67890"))
	diagnostic := openRecoveryDiagnosticStore(t, root)
	for name, bounds := range map[string]RecoveryBounds{
		"manifests": {MaxManifests: 1, MaxObjects: 2, MaxBytes: 64},
		"objects":   {MaxManifests: 2, MaxObjects: 1, MaxBytes: 64},
		"bytes":     {MaxManifests: 2, MaxObjects: 2, MaxBytes: 4},
	} {
		t.Run(name, func(t *testing.T) {
			evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), bounds)
			if err != nil {
				t.Fatal(err)
			}
			if evidence.CoverageComplete || !containsRecoveryWarning(evidence.WarningCodes, RecoveryWarningScanLimited) {
				t.Fatalf("limited evidence=%#v", evidence)
			}
		})
	}
}

func TestScanRecoveryEvidenceRejectsManifestUnderNonCanonicalFilename(t *testing.T) {
	root, manifests := createRecoveryScanStore(t, []byte("canonical manifest required"))
	canonical := manifestPath(root, manifests[0].BackupID)
	nonCanonical := filepath.Join(root, "manifests", "renamed-valid-manifest.json")
	if err := os.Rename(canonical, nonCanonical); err != nil {
		t.Fatal(err)
	}
	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.TrustedRecords) != 0 || evidence.RejectedRecordCount != 1 {
		t.Fatalf("non-canonical manifest became authority: %#v", evidence)
	}
	if len(evidence.RejectedRecords) != 1 || evidence.RejectedRecords[0].BackupID != manifests[0].BackupID || evidence.RejectedRecords[0].Reason != RecoveryRejectManifestInvalid {
		t.Fatalf("non-canonical manifest rejection=%#v", evidence.RejectedRecords)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}
func TestScanRecoveryEvidenceDoesNotGuessMissingDescriptor(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("descriptor-required"))
	if err := os.Remove(filepath.Join(root, "store.json")); err != nil {
		t.Fatal(err)
	}
	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.DescriptorValid || evidence.SourceDescriptor.StoreID != "" || len(evidence.TrustedRecords) != 0 || evidence.RejectedRecordCount == 0 {
		t.Fatalf("missing descriptor was treated as authority: %#v", evidence)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func TestScanRecoveryEvidenceHonorsCancellationWithoutMutation(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("cancelled"))
	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := diagnostic.ScanRecoveryEvidence(ctx, RecoveryBounds{})
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("cancelled recovery scan error=%v", err)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func createRecoveryScanStore(t *testing.T, contents ...[]byte) (string, []Manifest) {
	t.Helper()
	base := canonicalTempDir(t)
	root := filepath.Join(base, "store")
	store, err := Open(Options{Directory: root, Limits: phase2TestLimits()})
	if err != nil {
		t.Fatal(err)
	}
	manifests := make([]Manifest, 0, len(contents))
	for index, content := range contents {
		target := filepath.Join(base, "target-"+string(rune('a'+index))+".txt")
		if err := os.WriteFile(target, content, 0o600); err != nil {
			_ = store.Close()
			t.Fatal(err)
		}
		result, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
		if err != nil {
			_ = store.Close()
			t.Fatal(err)
		}
		manifests = append(manifests, result.Manifest)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return root, manifests
}

func openRecoveryDiagnosticStore(t *testing.T, root string) *DiagnosticStore {
	t.Helper()
	store, err := OpenExistingForDiagnosis(DiagnosticOpenOptions{Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func recoveryEvidenceReasonCount(evidence RecoveryEvidence, reason RecoveryRejectReason) int {
	for _, count := range evidence.RejectedReasonCounts {
		if count.Reason == reason {
			return count.Count
		}
	}
	return 0
}

func containsRecoveryWarning(codes []string, want string) bool {
	for _, code := range codes {
		if code == want {
			return true
		}
	}
	return false
}

func TestScanRecoveryEvidenceMissingManifestNamespaceIsIncompleteAndNonApplicable(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("authoritative namespace must not disappear silently"))
	if err := os.RemoveAll(filepath.Join(root, "manifests")); err != nil {
		t.Fatal(err)
	}
	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.CoverageComplete {
		t.Fatalf("missing authoritative manifest namespace was reported complete: %#v", evidence)
	}
	if !containsRecoveryWarning(evidence.WarningCodes, RecoveryWarningLayoutInvalid) {
		t.Fatalf("missing manifest namespace did not report layout damage: %#v", evidence.WarningCodes)
	}
	plan, err := BuildRecoveryPlan(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable {
		t.Fatalf("missing authoritative manifest namespace produced applicable plan: %#v", plan)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}
