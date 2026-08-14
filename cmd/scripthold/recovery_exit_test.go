package main

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/backupstore"
)

func TestBackupRecoveryExitCodeMapping(t *testing.T) {
	if got := backupRecoveryPlanExitCode(backupstore.RecoveryPlan{Applicable: true, CoverageComplete: true}, nil); got != 0 {
		t.Fatalf("applicable plan exit=%d", got)
	}
	if got := backupRecoveryPlanExitCode(backupstore.RecoveryPlan{CoverageComplete: true}, nil); got != 2 {
		t.Fatalf("non-applicable plan exit=%d", got)
	}
	if got := backupRecoveryPlanExitCode(backupstore.RecoveryPlan{Applicable: false, CoverageComplete: false}, nil); got != 2 {
		t.Fatalf("limited plan exit=%d", got)
	}
	if got := backupRecoveryPlanExitCode(backupstore.RecoveryPlan{}, errors.New("failure")); got != 1 {
		t.Fatalf("failed plan exit=%d", got)
	}
	if got := backupRecoveryApplyExitCode(nil); got != 0 {
		t.Fatalf("successful apply exit=%d", got)
	}
	if got := backupRecoveryApplyExitCode(errors.New("failure")); got != 1 {
		t.Fatalf("failed apply exit=%d", got)
	}
}

func TestParseBackupRecoveryPlanRejectsHardLimitOverflow(t *testing.T) {
	root := canonicalBackupTestTempDir(t)
	store := filepath.Join(root, "source")
	output := filepath.Join(root, "plan.json")
	for _, option := range []string{
		"--max-manifests=1000001",
		"--max-objects=1000001",
		"--max-bytes=1099511627777",
	} {
		if _, matched, err := parseBackupRecoveryCommand([]string{
			"backup-store", "recover-plan", "--store", store, "--output", output, option,
		}); !matched || err == nil {
			t.Fatalf("overflow option %q matched=%v err=%v", option, matched, err)
		}
	}
}
