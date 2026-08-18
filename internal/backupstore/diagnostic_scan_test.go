package backupstore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestDiagnosticStoreDiagnoseHealthyStoreWithoutMutation(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "backup-store")
	store := openBackupTestStore(t, root, backupStoreTestLimits())
	target := filepath.Join(base, "private-target.txt")
	if err := os.WriteFile(target, []byte("healthy diagnostic bytes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(context.Background(), CaptureRequest{
		TargetPath:      target,
		SourceOperation: SourceOperationEdit,
		Label:           "private diagnostic label",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	before := snapshotDiagnosticTree(t, root)

	diagnostic, err := OpenExistingForDiagnosis(DiagnosticOpenOptions{Directory: root, Limits: backupStoreTestLimits()})
	if err != nil {
		t.Fatal(err)
	}
	report, err := diagnostic.Diagnose(context.Background(), DiagnosticOptions{Mode: AuditQuick})
	if err != nil {
		_ = diagnostic.Close()
		t.Fatal(err)
	}
	if report.FormatVersion != DiagnosticFormatVersion || report.Mode != AuditQuick || !report.Diagnosable ||
		!report.SafeForNormalOpen || !report.DescriptorValid || !report.LayoutValid || !report.IndexConsistent ||
		report.ManifestCount != 1 || report.ObjectCount != 1 || len(report.Issues) != 0 {
		_ = diagnostic.Close()
		t.Fatalf("healthy diagnostic report=%#v", report)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{root, target, "private diagnostic label", "healthy diagnostic bytes"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("diagnostic JSON exposed private data %q: %s", secret, encoded)
		}
	}
}

func TestDiagnosticStoreDiagnoseReportsMissingIndexWithoutRebuilding(t *testing.T) {
	root := filepath.Join(canonicalTempDir(t), "backup-store")
	store := openBackupTestStore(t, root, backupStoreTestLimits())
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	indexFile := indexPath(root)
	if err := os.Remove(indexFile); err != nil {
		t.Fatal(err)
	}
	before := snapshotDiagnosticTree(t, root)

	diagnostic, err := OpenExistingForDiagnosis(DiagnosticOpenOptions{Directory: root, Limits: backupStoreTestLimits()})
	if err != nil {
		t.Fatal(err)
	}
	report, err := diagnostic.Diagnose(context.Background(), DiagnosticOptions{})
	if err != nil {
		_ = diagnostic.Close()
		t.Fatal(err)
	}
	if !report.SafeForNormalOpen || report.IndexConsistent || !diagnosticHasIssue(report, AuditIssueIndex) {
		_ = diagnostic.Close()
		t.Fatalf("missing-index report=%#v", report)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(indexFile); !os.IsNotExist(err) {
		t.Fatalf("diagnosis rebuilt missing index: %v", err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func TestDiagnosticStoreDiagnoseFailSoftDescriptorAndLayout(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mutate    func(string) error
		issueCode string
	}{
		{
			name: "malformed descriptor",
			mutate: func(root string) error {
				return os.WriteFile(filepath.Join(root, "store.json"), []byte("not-json\n"), 0o600)
			},
			issueCode: DiagnosticIssueDescriptor,
		},
		{
			name: "missing object layout",
			mutate: func(root string) error {
				return os.RemoveAll(filepath.Join(root, "objects"))
			},
			issueCode: DiagnosticIssueLayout,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(canonicalTempDir(t), "backup-store")
			store := openBackupTestStore(t, root, backupStoreTestLimits())
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			if err := tc.mutate(root); err != nil {
				t.Fatal(err)
			}
			if descriptorPath := filepath.Join(root, "store.json"); tc.issueCode == DiagnosticIssueDescriptor {
				if err := restrictPathPermissions(descriptorPath, false); err != nil {
					t.Fatal(err)
				}
			}
			before := snapshotDiagnosticTree(t, root)

			diagnostic, err := OpenExistingForDiagnosis(DiagnosticOpenOptions{Directory: root, Limits: backupStoreTestLimits()})
			if err != nil {
				t.Fatal(err)
			}
			report, err := diagnostic.Diagnose(context.Background(), DiagnosticOptions{Mode: AuditFull})
			if err != nil {
				_ = diagnostic.Close()
				t.Fatal(err)
			}
			if report.SafeForNormalOpen || !diagnosticHasIssue(report, tc.issueCode) {
				_ = diagnostic.Close()
				t.Fatalf("diagnostic report=%#v", report)
			}
			if tc.issueCode == DiagnosticIssueDescriptor && report.Diagnosable {
				_ = diagnostic.Close()
				t.Fatalf("malformed descriptor reported diagnosable: %#v", report)
			}
			if err := diagnostic.Close(); err != nil {
				t.Fatal(err)
			}
			assertDiagnosticTreeUnchanged(t, root, before)
		})
	}
}

func TestDiagnosticStoreDiagnoseMapsManifestAndMissingObjectIssues(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mutate    func(*testing.T, string, CaptureResult)
		issueCode string
	}{
		{
			name: "manifest checksum",
			mutate: func(t *testing.T, root string, captured CaptureResult) {
				t.Helper()
				path := manifestPath(root, captured.Manifest.BackupID)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				marker := []byte(`"manifestChecksum": "`)
				index := strings.Index(string(data), string(marker))
				if index < 0 || index+len(marker) >= len(data) {
					t.Fatal("manifest checksum marker missing")
				}
				position := index + len(marker)
				if data[position] == '0' {
					data[position] = '1'
				} else {
					data[position] = '0'
				}
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := restrictPathPermissions(path, false); err != nil {
					t.Fatal(err)
				}
			},
			issueCode: AuditIssueManifest,
		},
		{
			name: "missing object",
			mutate: func(t *testing.T, root string, captured CaptureResult) {
				t.Helper()
				if err := os.Remove(objectPath(root, captured.Manifest.ObjectDigest)); err != nil {
					t.Fatal(err)
				}
			},
			issueCode: AuditIssueObjectMissing,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := canonicalTempDir(t)
			root := filepath.Join(base, "backup-store")
			store := openBackupTestStore(t, root, backupStoreTestLimits())
			target := filepath.Join(base, "target.txt")
			captured := captureManagementFixture(t, store, target, "diagnostic issue bytes", false)
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			tc.mutate(t, root, captured)
			before := snapshotDiagnosticTree(t, root)

			diagnostic, err := OpenExistingForDiagnosis(DiagnosticOpenOptions{Directory: root, Limits: backupStoreTestLimits()})
			if err != nil {
				t.Fatal(err)
			}
			report, err := diagnostic.Diagnose(context.Background(), DiagnosticOptions{Mode: AuditFull})
			if err != nil {
				_ = diagnostic.Close()
				t.Fatal(err)
			}
			if report.SafeForNormalOpen || !diagnosticHasIssue(report, tc.issueCode) || diagnosticCheckStatus(report, "fullIntegrity") != DiagnosticCheckFailed {
				_ = diagnostic.Close()
				t.Fatalf("diagnostic issue report=%#v", report)
			}
			if err := diagnostic.Close(); err != nil {
				t.Fatal(err)
			}
			assertDiagnosticTreeUnchanged(t, root, before)
		})
	}
}

func TestDiagnosticLayoutSnapshotDetectsDirectoryReplacement(t *testing.T) {
	root := filepath.Join(canonicalTempDir(t), "backup-store")
	store, err := Open(Options{Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := inspectDiagnosticLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	manifests := filepath.Join(root, "manifests")
	displaced := filepath.Join(root, "manifests-displaced")
	if err := os.Rename(manifests, displaced); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(manifests, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(manifests, true); err != nil {
		t.Fatal(err)
	}
	after, err := inspectDiagnosticLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	if diagnosticLayoutSnapshotsEqual(before, after) {
		t.Fatal("layout snapshot accepted a replaced manifests directory")
	}
}

func TestDiagnosticStoreDiagnoseQuickAndFullIntegrity(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "backup-store")
	store := openBackupTestStore(t, root, backupStoreTestLimits())
	target := filepath.Join(base, "target.txt")
	captured := captureManagementFixture(t, store, target, "original object bytes", false)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	objectFile := objectPath(root, captured.Manifest.ObjectDigest)
	if err := os.WriteFile(objectFile, []byte("corrupted object byte"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(objectFile, false); err != nil {
		t.Fatal(err)
	}
	before := snapshotDiagnosticTree(t, root)

	quickStore, err := OpenExistingForDiagnosis(DiagnosticOpenOptions{Directory: root, Limits: backupStoreTestLimits()})
	if err != nil {
		t.Fatal(err)
	}
	quick, err := quickStore.Diagnose(context.Background(), DiagnosticOptions{Mode: AuditQuick})
	if err != nil {
		_ = quickStore.Close()
		t.Fatal(err)
	}
	if !quick.SafeForNormalOpen || diagnosticHasIssue(quick, AuditIssueObjectDigest) {
		_ = quickStore.Close()
		t.Fatalf("quick report unexpectedly hashed object=%#v", quick)
	}
	if err := quickStore.Close(); err != nil {
		t.Fatal(err)
	}

	fullStore, err := OpenExistingForDiagnosis(DiagnosticOpenOptions{Directory: root, Limits: backupStoreTestLimits()})
	if err != nil {
		t.Fatal(err)
	}
	full, err := fullStore.Diagnose(context.Background(), DiagnosticOptions{Mode: AuditFull})
	if err != nil {
		_ = fullStore.Close()
		t.Fatal(err)
	}
	if full.SafeForNormalOpen || !diagnosticHasIssue(full, AuditIssueObjectDigest) {
		_ = fullStore.Close()
		t.Fatalf("full report missed object corruption=%#v", full)
	}
	if err := fullStore.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func TestDiagnosticStoreDiagnoseLimitsCancellationAndDeterminism(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "backup-store")
	store := openBackupTestStore(t, root, backupStoreTestLimits())
	target := filepath.Join(base, "target.txt")
	_ = captureManagementFixture(t, store, target, "bounded diagnostic bytes", false)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	diagnostic, err := OpenExistingForDiagnosis(DiagnosticOpenOptions{Directory: root, Limits: backupStoreTestLimits()})
	if err != nil {
		t.Fatal(err)
	}
	limited, err := diagnostic.Diagnose(context.Background(), DiagnosticOptions{Mode: AuditFull, MaxBytes: 1})
	if err != nil {
		_ = diagnostic.Close()
		t.Fatal(err)
	}
	if limited.SafeForNormalOpen || !diagnosticHasIssue(limited, AuditIssueLimit) {
		_ = diagnostic.Close()
		t.Fatalf("limited report=%#v", limited)
	}
	first, err := json.Marshal(limited)
	if err != nil {
		_ = diagnostic.Close()
		t.Fatal(err)
	}
	secondReport, err := diagnostic.Diagnose(context.Background(), DiagnosticOptions{Mode: AuditFull, MaxBytes: 1})
	if err != nil {
		_ = diagnostic.Close()
		t.Fatal(err)
	}
	second, err := json.Marshal(secondReport)
	if err != nil {
		_ = diagnostic.Close()
		t.Fatal(err)
	}
	if string(first) != string(second) {
		_ = diagnostic.Close()
		t.Fatalf("diagnostic report is nondeterministic:\n%s\n%s", first, second)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := diagnostic.Diagnose(ctx, DiagnosticOptions{}); operation.KindOf(err) != operation.KindCancelled {
		_ = diagnostic.Close()
		t.Fatalf("cancelled diagnosis error=%v, want CANCELLED", err)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
}

func diagnosticHasIssue(report DiagnosticReport, code string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func diagnosticCheckStatus(report DiagnosticReport, name string) DiagnosticCheckStatus {
	for _, check := range report.Checks {
		if check.Name == name {
			return check.Status
		}
	}
	return ""
}
