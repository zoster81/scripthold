package backupstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScanRecoveryEvidenceDoesNotTraverseObjectAlgorithmAlias(t *testing.T) {
	root, manifests := createRecoveryScanStore(t, []byte("must remain outside recovery authority"))
	algorithmRoot := filepath.Join(root, "objects", ObjectAlgorithm)
	external := filepath.Join(filepath.Dir(root), "external-object-tree")
	if err := os.Rename(algorithmRoot, external); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, algorithmRoot); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.CoverageComplete {
		t.Fatalf("aliased object tree produced complete recovery evidence: %#v", evidence)
	}
	if len(evidence.TrustedRecords) != 0 || evidence.RejectedRecordCount != len(manifests) {
		t.Fatalf("aliased object tree became recovery authority: %#v", evidence)
	}
	if !containsRecoveryWarning(evidence.WarningCodes, RecoveryWarningLayoutInvalid) {
		t.Fatalf("aliased object tree did not report a layout issue: %#v", evidence.WarningCodes)
	}
	if containsRecoveryWarning(evidence.WarningCodes, RecoveryWarningScanLimited) {
		t.Fatalf("structural alias was mislabeled as a scan limit: %#v", evidence.WarningCodes)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func captureRecoveryOwnedRegularFileForTest(t *testing.T, path string) recoveryOwnedRegularFile {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	identity, captureErr := captureRecoveryOwnedRegularFile(file)
	closeErr := file.Close()
	if captureErr != nil {
		t.Fatal(captureErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	t.Cleanup(func() { closeRecoveryOwnedRegularFile(&identity) })
	return identity
}

func TestRemoveRecoveryRegularFileIfOwnedNeverDeletesReplacement(t *testing.T) {
	base := canonicalTempDir(t)
	path := filepath.Join(base, "owned-recovery-temp")
	if err := os.WriteFile(path, []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	owned := captureRecoveryOwnedRegularFileForTest(t, path)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	removeRecoveryRegularFileIfOwned(path, owned)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("replacement was deleted: %v", err)
	}
	if string(got) != "replacement" {
		t.Fatalf("replacement changed: %q", got)
	}
}

func TestRemoveRecoveryRegularFileIfOwnedNeverDeletesSameMetadataReplacement(t *testing.T) {
	base := canonicalTempDir(t)
	path := filepath.Join(base, "owned-recovery-temp-same-metadata")
	if err := os.WriteFile(path, []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	owned := captureRecoveryOwnedRegularFileForTest(t, path)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, owned.info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, owned.info.ModTime(), owned.info.ModTime()); err != nil {
		t.Fatal(err)
	}
	removeRecoveryRegularFileIfOwned(path, owned)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("same-metadata replacement was deleted: %v", err)
	}
	if string(got) != "other" {
		t.Fatalf("same-metadata replacement changed: %q", got)
	}
}

func TestRemoveRecoveryRegularFileIfOwnedRemovesSameOwnedFileAfterLegitimateWrite(t *testing.T) {
	base := canonicalTempDir(t)
	path := filepath.Join(base, "owned-recovery-temp-after-write")
	if err := os.WriteFile(path, []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	owned := captureRecoveryOwnedRegularFileForTest(t, path)
	if err := os.WriteFile(path, []byte("legitimate staged bytes changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !removeRecoveryRegularFileIfOwned(path, owned) {
		t.Fatal("same owned file was not recognized after legitimate content mutation")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("same owned file was not removed after legitimate write: %v", err)
	}
}

func TestRemoveRecoveryRegularFileIfOwnedRemovesExactOwnedFile(t *testing.T) {
	base := canonicalTempDir(t)
	path := filepath.Join(base, "owned-recovery-temp")
	if err := os.WriteFile(path, []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	owned := captureRecoveryOwnedRegularFileForTest(t, path)
	removeRecoveryRegularFileIfOwned(path, owned)
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("exact owned file was not removed: %v", err)
	}
}
