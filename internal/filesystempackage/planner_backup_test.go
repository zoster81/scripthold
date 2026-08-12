package filesystempackage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

func TestPlannerRequiresBackupPreflightForDestructiveRegularFiles(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	planner := newTestPlannerWithoutBackup(t, root)
	_, err := planner.Plan(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{
		Type: OperationDeleteFile, Path: target,
	}}})
	if operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("delete without backup preflight error = %v, want INVALID_INPUT", err)
	}
}

func TestPlannerBackupPreflightReceivesExactFilesystemPackageSet(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "delete")
	if err := os.MkdirAll(filepath.Join(directory, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(directory, "a.txt")
	second := filepath.Join(directory, "nested", "b.txt")
	for path, contents := range map[string]string{first: "alpha", second: "beta"} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var got []backupstore.CaptureRequest
	planner := newTestPlannerWithBackup(t, root, func(_ context.Context, requests []backupstore.CaptureRequest) error {
		got = append([]backupstore.CaptureRequest(nil), requests...)
		return nil
	})
	plan, err := planner.Plan(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{
		Type: OperationDeleteDirectory, Path: directory,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(plan.BackupRequirements) != 2 {
		t.Fatalf("preflight count = %d, requirements = %d; want 2", len(got), len(plan.BackupRequirements))
	}
	for index, request := range got {
		if request.SourceOperation != backupstore.SourceOperationFilesystemPackage {
			t.Fatalf("request %d source operation = %q", index, request.SourceOperation)
		}
		if request.TargetPath != plan.BackupRequirements[index].Path {
			t.Fatalf("request %d path = %q, want %q", index, request.TargetPath, plan.BackupRequirements[index].Path)
		}
	}
}

func TestPlannerPropagatesBackupQuotaFailure(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	planner := newTestPlannerWithBackup(t, root, func(context.Context, []backupstore.CaptureRequest) error {
		return operation.New(operation.KindLimit, "backup quota exceeded")
	})
	_, err := planner.Plan(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{
		Type: OperationDeleteFile, Path: target,
	}}})
	if operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("quota error = %v, want LIMIT", err)
	}
}

func TestPlannerDetectsBackupSourceMutationDuringPreflight(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	planner := newTestPlannerWithBackup(t, root, func(context.Context, []backupstore.CaptureRequest) error {
		return os.WriteFile(target, []byte("changed"), 0o600)
	})
	_, err := planner.Plan(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{
		Type: OperationDeleteFile, Path: target,
	}}})
	if operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("mutation during backup preflight error = %v, want CONFLICT", err)
	}
}

func TestPlannerDoesNotRequireBackupForEmptyDirectoryOrNoReplaceOperations(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}

	planner := newTestPlannerWithoutBackup(t, root)
	tests := []Manifest{
		{FormatVersion: FormatV1, Operations: []Operation{{Type: OperationDeleteDirectory, Path: empty}}},
		{FormatVersion: FormatV1, Operations: []Operation{{Type: OperationMkdir, Path: filepath.Join(root, "new-dir")}}},
		{FormatVersion: FormatV1, Operations: []Operation{{Type: OperationCreateFile, Path: filepath.Join(root, "new.bin"), Content: []byte("x")}}},
		{FormatVersion: FormatV1, Operations: []Operation{{Type: OperationCopyFile, Source: source, Destination: filepath.Join(root, "copy.txt")}}},
		{FormatVersion: FormatV1, Operations: []Operation{{Type: OperationMove, Source: source, Destination: filepath.Join(root, "moved.txt")}}},
	}
	for index, manifest := range tests {
		if _, err := planner.Plan(context.Background(), manifest); err != nil {
			t.Fatalf("non-backup plan %d failed: %v", index, err)
		}
	}
}

func newTestPlannerWithoutBackup(t *testing.T, root string) *Planner {
	t.Helper()
	return newTestPlannerConfigured(t, root)
}

func newTestPlannerWithBackup(t *testing.T, root string, preflight BackupPreflightFunc) *Planner {
	t.Helper()
	return newTestPlannerConfigured(t, root, preflight)
}

func newTestPlannerConfigured(t *testing.T, root string, preflight ...BackupPreflightFunc) *Planner {
	t.Helper()
	set, err := security.NormalizeAllowedDirectorySet([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	limits := testManifestLimits()
	limits.MaxRecursiveEntries = 256
	limits.MaxRecursiveDepth = 32
	limits.MaxAggregateBytes = 1 << 20
	limits.MaxStagingBytes = 1 << 20
	planner, err := NewPlanner(limits, func(path string) (security.PathEvidence, error) {
		return security.ValidatePathEvidenceWithAllowedDirectories(path, set.Requested, set.Resolved)
	}, func() []string {
		return append([]string(nil), set.Resolved...)
	}, preflight...)
	if err != nil {
		t.Fatal(err)
	}
	return planner
}
