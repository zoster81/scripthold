//go:build windows

package filesystem

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestAtomicReplaceRetryReportsRecoveredEpisodeWithoutPathDisclosure(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target-sensitive-name.txt")
	staged := filepath.Join(root, ".target-sensitive-name.txt.stage.tmp")
	if err := os.WriteFile(target, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := CaptureSnapshotWithDigest(target)
	if err != nil {
		t.Fatal(err)
	}

	attempt := 0
	var reports []atomicReplaceRetryReport
	err = commitStagedTargetWithRetryObserved(target, staged, &expected, func(string, string) error {
		attempt++
		if attempt == 1 {
			return windows.ERROR_ACCESS_DENIED
		}
		return nil
	}, func(targetPath, stagedPath string, report atomicReplaceRetryReport) {
		if targetPath != target || stagedPath != staged {
			t.Fatalf("report paths target=%q staged=%q", targetPath, stagedPath)
		}
		reports = append(reports, report)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("reports=%d want=1", len(reports))
	}
	report := reports[0]
	if report.Outcome != atomicReplaceRetryRecovered || len(report.Attempts) != 1 || report.Attempts[0].Phase != atomicReplaceAttemptCommit || !errors.Is(report.Attempts[0].Err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("report=%+v", report)
	}

	diagnostic := buildWindowsAtomicReplaceDiagnostic(target, staged, report, false)
	if diagnostic.TargetPathHash == "" || diagnostic.StagedPathHash == "" || diagnostic.TargetPathHash == diagnostic.StagedPathHash {
		t.Fatalf("diagnostic path hashes=%+v", diagnostic)
	}
	if diagnostic.TargetExtension != ".txt" || len(diagnostic.Attempts) != 1 || diagnostic.Attempts[0].Win32Code != uint32(windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("diagnostic=%+v", diagnostic)
	}
	encoded, err := json.Marshal(diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), target) || strings.Contains(string(encoded), staged) {
		t.Fatalf("diagnostic disclosed filesystem path: %s", encoded)
	}
}

func TestWindowsAtomicReplaceFailureProbeSeesDeleteShareContention(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handle := openWithoutDeleteShare(t, target)
	defer windows.CloseHandle(handle)

	probe := probeWindowsAtomicReplacePath(target, true)
	if !probe.Exists || probe.ReadAttributesErrorCode != 0 {
		t.Fatalf("read-attributes probe=%+v", probe)
	}
	if probe.DeleteAccessGranted || (probe.DeleteAccessErrorCode != uint32(windows.ERROR_SHARING_VIOLATION) && probe.DeleteAccessErrorCode != uint32(windows.ERROR_ACCESS_DENIED)) {
		t.Fatalf("delete-access probe=%+v", probe)
	}
}

func TestWindowsAtomicReplaceDeepDiagnosticIsBoundedAndPrivacySafe(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "private-target.txt")
	staged := filepath.Join(root, ".private-target.txt.stage")
	if err := os.WriteFile(target, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handle := openWithoutDeleteShare(t, target)
	defer windows.CloseHandle(handle)

	diagnostic := buildWindowsAtomicReplaceDiagnostic(target, staged, atomicReplaceRetryReport{
		Outcome: atomicReplaceRetryExhausted, CommitAttempts: 2,
		Attempts: []atomicReplaceRetryAttempt{{Phase: atomicReplaceAttemptCommit, Err: windows.ERROR_ACCESS_DENIED}},
	}, true)
	if diagnostic.TargetProbe == nil || diagnostic.StagedProbe == nil || diagnostic.ParentProbe == nil || diagnostic.RestartManager == nil {
		t.Fatalf("deep diagnostic incomplete: %+v", diagnostic)
	}
	if diagnostic.TargetProbe.DeleteAccessGranted || diagnostic.TargetProbe.DeleteAccessErrorCode == 0 {
		t.Fatalf("target delete-share contention not observed: %+v", diagnostic.TargetProbe)
	}
	if !diagnostic.StagedProbe.DeleteAccessGranted || diagnostic.StagedProbe.DeleteAccessErrorCode != 0 {
		t.Fatalf("staged delete-access probe=%+v", diagnostic.StagedProbe)
	}
	if !diagnostic.ParentProbe.AddFileGranted || diagnostic.ParentProbe.AddFileErrorCode != 0 {
		t.Fatalf("parent add-file access probe=%+v", diagnostic.ParentProbe)
	}
	if diagnostic.ParentProbe.DeleteChildGranted != (diagnostic.ParentProbe.DeleteChildErrorCode == 0) {
		t.Fatalf("parent delete-child access probe is internally inconsistent: %+v", diagnostic.ParentProbe)
	}
	if len(diagnostic.RestartManager.Processes) > maxRestartManagerProcesses {
		t.Fatalf("Restart Manager process list exceeded bound: %d", len(diagnostic.RestartManager.Processes))
	}
	encoded, err := json.Marshal(diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(encoded) || strings.Contains(string(encoded), target) || strings.Contains(string(encoded), staged) {
		t.Fatalf("deep diagnostic is invalid or disclosed a path: %s", encoded)
	}
}

func TestWindowsAtomicReplaceExhaustedWarningIsMachineReadableAndPathSafe(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "warning-private-target.txt")
	staged := filepath.Join(root, ".warning-private-target.txt.stage")
	if err := os.WriteFile(target, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(previous)

	reportAtomicReplaceRetry(target, staged, atomicReplaceRetryReport{
		Outcome: atomicReplaceRetryExhausted, CommitAttempts: 2,
		Attempts: []atomicReplaceRetryAttempt{{Phase: atomicReplaceAttemptCommit, Err: windows.ERROR_ACCESS_DENIED}},
	})
	logged := output.String()
	if !strings.Contains(logged, "windows atomic replace retry exhausted") || !strings.Contains(logged, atomicReplaceDiagnosticFormat) {
		t.Fatalf("warning did not contain versioned diagnostic: %s", logged)
	}
	if strings.Contains(logged, target) || strings.Contains(logged, staged) {
		t.Fatalf("warning disclosed a filesystem path: %s", logged)
	}
}

func TestWindowsRestartManagerIdentifiesCurrentLockerWhenAvailable(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "restart-manager-target.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handle := openWithoutDeleteShare(t, target)
	defer windows.CloseHandle(handle)

	staged := filepath.Join(root, "restart-manager-staged.txt")
	if err := os.WriteFile(staged, []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stagedHandle := openWithoutDeleteShare(t, staged)
	defer windows.CloseHandle(stagedHandle)

	diagnostic := queryWindowsRestartManager(target, staged)
	if diagnostic.StartCode != 0 || diagnostic.RegisterCode != 0 || diagnostic.GetListCode != 0 {
		t.Skipf("Restart Manager unavailable for this test environment: %+v", diagnostic)
	}
	for _, process := range diagnostic.Processes {
		if process.PID == uint32(os.Getpid()) && process.CurrentProcess {
			return
		}
	}
	t.Fatalf("Restart Manager did not report the current process holding the target: %+v", diagnostic)
}
