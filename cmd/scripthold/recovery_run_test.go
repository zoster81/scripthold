package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/config"
)

func TestRunCommandRecoveryPlanRestartApplyOffline(t *testing.T) {
	base := canonicalBackupTestTempDir(t)
	root := filepath.Join(base, "source-store")
	target := filepath.Join(base, "target.txt")
	if err := os.WriteFile(target, []byte("offline recovery cli"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := backupstore.Open(backupstore.Options{Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(context.Background(), backupstore.CaptureRequest{TargetPath: target, SourceOperation: backupstore.SourceOperationEdit}); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	before := snapshotRecoveryCLITree(t, root)

	planPath := filepath.Join(base, "reviewed-plan.json")
	destination := filepath.Join(base, "recovered-store")
	reportPath := filepath.Join(base, "recovery-report.json")
	requestedEnvironment := make(map[string]int)
	getenv := func(name string) string {
		requestedEnvironment[name]++
		if name == config.EnvBackupStoreDir {
			return filepath.Join(base, "ambient-store-must-not-be-used")
		}
		return ""
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runCommand(context.Background(), []string{
		"backup-store", "recover-plan", "--store", root, "--output", planPath, "--pretty",
	}, &stdout, &stderr, getenv)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("recover-plan code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if requestedEnvironment[config.EnvBackupStoreDir] != 0 {
		t.Fatalf("recover-plan read %s", config.EnvBackupStoreDir)
	}
	plan, err := backupstore.DecodeRecoveryPlan(stdout.Bytes())
	if err != nil || !plan.Applicable || !plan.CoverageComplete {
		t.Fatalf("stdout plan=%#v err=%v", plan, err)
	}
	persistedPlan, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	decodedPlan, err := backupstore.DecodeRecoveryPlan(persistedPlan)
	if err != nil || !reflect.DeepEqual(decodedPlan, plan) {
		t.Fatalf("persisted plan=%#v err=%v stdout=%#v", decodedPlan, err, plan)
	}
	if strings.Contains(stdout.String(), root) || strings.Contains(stdout.String(), target) {
		t.Fatalf("recover-plan output exposed paths: %q", stdout.String())
	}
	assertRecoveryCLITreeEqual(t, root, before)

	stdout.Reset()
	stderr.Reset()
	code = runCommand(context.Background(), []string{
		"backup-store", "recover-apply", "--store", root, "--plan", planPath,
		"--destination", destination, "--report", reportPath, "--pretty",
	}, &stdout, &stderr, getenv)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("recover-apply code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if requestedEnvironment[config.EnvBackupStoreDir] != 0 {
		t.Fatalf("recover-apply read %s", config.EnvBackupStoreDir)
	}
	report, err := backupstore.DecodeRecoveryReport(stdout.Bytes())
	if err != nil || report.Status != backupstore.RecoveryStatusRecovered || !report.FullAudit {
		t.Fatalf("stdout report=%#v err=%v", report, err)
	}
	persistedReport, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	decodedReport, err := backupstore.DecodeRecoveryReport(persistedReport)
	if err != nil || decodedReport != report {
		t.Fatalf("persisted report=%#v err=%v stdout=%#v", decodedReport, err, report)
	}
	if strings.Contains(stdout.String(), root) || strings.Contains(stdout.String(), destination) || strings.Contains(stdout.String(), target) {
		t.Fatalf("recover-apply output exposed paths: %q", stdout.String())
	}
	assertRecoveryCLITreeEqual(t, root, before)

	finalDiagnostic, err := backupstore.OpenExistingForDiagnosis(backupstore.DiagnosticOpenOptions{Directory: destination})
	if err != nil {
		t.Fatal(err)
	}
	finalAudit, err := finalDiagnostic.Diagnose(context.Background(), backupstore.DiagnosticOptions{Mode: backupstore.AuditFull})
	closeErr := finalDiagnostic.Close()
	if err != nil || closeErr != nil || !finalAudit.SafeForNormalOpen || len(finalAudit.Issues) != 0 {
		t.Fatalf("final destination audit=%#v err=%v close=%v", finalAudit, err, closeErr)
	}
}

func TestRunCommandRecoveryPlanLimitedReturnsTwoAndApplyRefusesIt(t *testing.T) {
	base := canonicalBackupTestTempDir(t)
	root := filepath.Join(base, "source-store")
	store, err := backupstore.Open(backupstore.Options{Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	for i, content := range [][]byte{[]byte("one"), []byte("two")} {
		target := filepath.Join(base, "target-"+string(rune('a'+i))+".txt")
		if err := os.WriteFile(target, content, 0o600); err != nil {
			_ = store.Close()
			t.Fatal(err)
		}
		if _, err := store.Capture(context.Background(), backupstore.CaptureRequest{TargetPath: target, SourceOperation: backupstore.SourceOperationEdit}); err != nil {
			_ = store.Close()
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(base, "limited-plan.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runCommand(context.Background(), []string{
		"backup-store", "recover-plan", "--store", root, "--output", planPath, "--max-manifests", "1",
	}, &stdout, &stderr, func(string) string { return "" })
	if code != 2 || stderr.Len() != 0 {
		t.Fatalf("limited plan code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	plan, err := backupstore.DecodeRecoveryPlan(stdout.Bytes())
	if err != nil || plan.Applicable || plan.CoverageComplete {
		t.Fatalf("limited plan=%#v err=%v", plan, err)
	}

	stdout.Reset()
	stderr.Reset()
	destination := filepath.Join(base, "limited-destination")
	report := filepath.Join(base, "limited-report.json")
	code = runCommand(context.Background(), []string{
		"backup-store", "recover-apply", "--store", root, "--plan", planPath,
		"--destination", destination, "--report", report,
	}, &stdout, &stderr, func(string) string { return "" })
	if code != 1 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("limited apply code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("limited plan created destination: %v", err)
	}
	if _, err := os.Lstat(report); !os.IsNotExist(err) {
		t.Fatalf("limited plan created report: %v", err)
	}
}

func TestRunCommandRecoveryPlanActiveStoreErrorIsPathFree(t *testing.T) {
	base := canonicalBackupTestTempDir(t)
	root := filepath.Join(base, "active-store")
	active, err := backupstore.Open(backupstore.Options{Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	defer active.Close()
	output := filepath.Join(base, "plan.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runCommand(context.Background(), []string{
		"backup-store", "recover-plan", "--store", root, "--output", output,
	}, &stdout, &stderr, func(string) string { return "" })
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "already in use") {
		t.Fatalf("active recover-plan code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), root) {
		t.Fatalf("active recovery error exposed path: %q", stderr.String())
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("active recovery created plan: %v", err)
	}
}

type recoveryCLITreeEntry struct {
	mode os.FileMode
	data []byte
}

func snapshotRecoveryCLITree(t *testing.T, root string) map[string]recoveryCLITreeEntry {
	t.Helper()
	result := make(map[string]recoveryCLITreeEntry)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry := recoveryCLITreeEntry{mode: info.Mode()}
		if info.Mode().IsRegular() {
			entry.data, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		}
		result[relative] = entry
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func assertRecoveryCLITreeEqual(t *testing.T, root string, want map[string]recoveryCLITreeEntry) {
	t.Helper()
	got := snapshotRecoveryCLITree(t, root)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("source recovery store changed:\ngot=%#v\nwant=%#v", got, want)
	}
}
