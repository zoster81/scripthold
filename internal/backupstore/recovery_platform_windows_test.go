//go:build windows

package backupstore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestRecoveryWindowsCaseAliasCannotBypassSourceSeparation(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("Windows case alias recovery"))
	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	plan, err := diagnostic.CreateRecoveryPlan(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(root)
	planPath := filepath.Join(parent, "windows-case-plan.json")
	report := filepath.Join(parent, "windows-case-report.json")
	if err := diagnostic.WriteRecoveryPlan(context.Background(), planPath, plan, false); err != nil {
		t.Fatal(err)
	}
	caseAlias := strings.ToUpper(root)
	if _, err := os.Stat(caseAlias); err != nil {
		t.Skipf("case-insensitive path alias unavailable: %v", err)
	}
	if _, err := diagnostic.AuthorizeRecoveryApplyPaths(
		planPath,
		filepath.Join(caseAlias, "nested-recovery-destination"),
		report,
		plan,
	); err == nil {
		t.Fatal("case-aliased destination inside source store bypassed recovery separation")
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func TestRecoveryWindowsShortPathAliasCannotBypassSourceSeparation(t *testing.T) {
	base := canonicalTempDir(t)
	longParent := filepath.Join(base, "recovery parent with a deliberately long component name")
	if err := os.Mkdir(longParent, 0o700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(longParent, "source backup store with a long component name")
	store, err := Open(Options{Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "windows-short-source.txt")
	if err := os.WriteFile(target, []byte("Windows 8.3 recovery alias"), 0o600); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if _, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	shortRoot := recoveryWindowsShortPathForTest(t, root)
	if strings.EqualFold(filepath.Clean(shortRoot), filepath.Clean(root)) {
		t.Skip("8.3 short names are unavailable on this filesystem")
	}
	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	plan, err := diagnostic.CreateRecoveryPlan(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(longParent, "windows-short-plan.json")
	report := filepath.Join(longParent, "windows-short-report.json")
	if err := diagnostic.WriteRecoveryPlan(context.Background(), planPath, plan, false); err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.AuthorizeRecoveryApplyPaths(
		planPath,
		filepath.Join(shortRoot, "nested-recovery-destination"),
		report,
		plan,
	); err == nil {
		t.Fatal("8.3-aliased destination inside source store bypassed recovery separation")
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func recoveryWindowsShortPathForTest(t *testing.T, path string) string {
	t.Helper()
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	bufferSize := uint32(260)
	for bufferSize <= 1<<15 {
		buffer := make([]uint16, bufferSize)
		length, err := windows.GetShortPathName(pathPtr, &buffer[0], bufferSize)
		if err != nil {
			t.Skipf("8.3 short names are unavailable: %v", err)
		}
		if length < bufferSize {
			return filepath.Clean(windows.UTF16ToString(buffer[:length]))
		}
		bufferSize = length + 1
	}
	t.Fatal("short path exceeded supported test buffer")
	return ""
}

func TestRecoveryWindowsObjectAlgorithmJunctionIsNeverTraversed(t *testing.T) {
	root, manifests := createRecoveryScanStore(t, []byte("Windows junction source object"))
	algorithmRoot := filepath.Join(root, "objects", ObjectAlgorithm)
	external := filepath.Join(filepath.Dir(root), "external-object-junction-target")
	if err := os.Rename(algorithmRoot, external); err != nil {
		t.Fatal(err)
	}
	createRecoveryWindowsJunctionForTest(t, external, algorithmRoot)
	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.CoverageComplete || len(evidence.TrustedRecords) != 0 || evidence.RejectedRecordCount != len(manifests) ||
		!containsRecoveryWarning(evidence.WarningCodes, RecoveryWarningLayoutInvalid) {
		t.Fatalf("object junction became recovery authority: %#v", evidence)
	}
	if containsRecoveryWarning(evidence.WarningCodes, RecoveryWarningScanLimited) {
		t.Fatalf("object junction was mislabeled as a resource limit: %#v", evidence.WarningCodes)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func TestRecoveryWindowsDestinationParentJunctionIsRejected(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("Windows junction destination"))
	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	plan, err := diagnostic.CreateRecoveryPlan(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Dir(root)
	planPath := filepath.Join(base, "windows-junction-plan.json")
	report := filepath.Join(base, "windows-junction-report.json")
	if err := diagnostic.WriteRecoveryPlan(context.Background(), planPath, plan, false); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(base, "source-store-junction")
	createRecoveryWindowsJunctionForTest(t, root, junction)
	if _, err := diagnostic.AuthorizeRecoveryApplyPaths(
		planPath,
		filepath.Join(junction, "nested-destination"),
		report,
		plan,
	); err == nil {
		t.Fatal("junction-backed destination parent was accepted")
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func createRecoveryWindowsJunctionForTest(t *testing.T, target, link string) {
	t.Helper()
	output, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		t.Skipf("directory junction creation is not supported in this environment: %v (%s)", err, output)
	}
}
