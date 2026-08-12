package filesystempackage

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

func TestPlannerAllowsOnlyEarlierMkdirParentDependency(t *testing.T) {
	root := t.TempDir()
	planner := newTestPlanner(t, root)
	manifest := Manifest{FormatVersion: FormatV1, Operations: []Operation{
		{Type: OperationMkdir, Path: filepath.Join(root, "new")},
		{Type: OperationCreateFile, Path: filepath.Join(root, "new", "file.bin"), Content: []byte{0, 1, 2}},
		{Type: OperationMkdir, Path: filepath.Join(root, "new", "nested")},
		{Type: OperationCreateFile, Path: filepath.Join(root, "new", "nested", "empty.bin"), Content: []byte{}},
	}}
	plan, err := planner.Plan(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) != 4 || plan.Operations[1].ParentProviderIndex != 0 || plan.Operations[2].ParentProviderIndex != 0 || plan.Operations[3].ParentProviderIndex != 2 {
		t.Fatalf("unexpected mkdir dependency plan: %#v", plan.Operations)
	}
	if _, err := os.Stat(filepath.Join(root, "new")); !os.IsNotExist(err) {
		t.Fatalf("planning mutated destination: %v", err)
	}
}

func TestPlannerRejectsMissingParentWithoutMkdirAndExistingDestination(t *testing.T) {
	root := t.TempDir()
	planner := newTestPlanner(t, root)
	_, err := planner.Plan(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{
		Type: OperationCreateFile, Path: filepath.Join(root, "missing", "file"), Content: []byte("x"),
	}}})
	if operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("missing parent error = %v, want INVALID_INPUT", err)
	}

	existing := filepath.Join(root, "existing.txt")
	if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = planner.Plan(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{
		Type: OperationCreateFile, Path: existing, Content: []byte("x"),
	}}})
	if operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("existing destination error = %v, want CONFLICT", err)
	}
}

func TestPlannerRejectsAliasesOverlapsAndRecursiveSelfCopy(t *testing.T) {
	root := t.TempDir()
	planner := newTestPlanner(t, root)
	sourceDir := filepath.Join(root, "source")
	if err := os.Mkdir(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sourceDir, "file.txt")
	if err := os.WriteFile(file, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := planner.Plan(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{
		Type: OperationCopyDirectory, Source: sourceDir, Destination: filepath.Join(sourceDir, "nested-copy"),
	}}})
	if operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("recursive self-copy error = %v, want INVALID_INPUT", err)
	}

	_, err = planner.Plan(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{
		{Type: OperationCopyDirectory, Source: sourceDir, Destination: filepath.Join(root, "copy")},
		{Type: OperationDeleteFile, Path: file},
	}})
	if operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("recursive overlap error = %v, want INVALID_INPUT", err)
	}

	hardlink := filepath.Join(root, "hardlink.txt")
	if err := os.Link(file, hardlink); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	_, err = planner.Plan(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{
		{Type: OperationCopyFile, Source: file, Destination: filepath.Join(root, "copy-a")},
		{Type: OperationCopyFile, Source: hardlink, Destination: filepath.Join(root, "copy-b")},
	}})
	if operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("same-object alias error = %v, want INVALID_INPUT", err)
	}
}

func TestPlannerBuildsExactBackupRequirementsAndDeterministicSummary(t *testing.T) {
	root := t.TempDir()
	planner := newTestPlanner(t, root)
	deleteDir := filepath.Join(root, "delete")
	if err := os.MkdirAll(filepath.Join(deleteDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deleteDir, "a.txt"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deleteDir, ".git", "config"), []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{FormatVersion: FormatV1, Operations: []Operation{{Type: OperationDeleteDirectory, Path: deleteDir}}}
	first, err := planner.Plan(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := planner.Plan(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.BackupRequirements) != 2 || first.Operations[0].BackupCount != 2 {
		t.Fatalf("backup requirements = %#v", first.BackupRequirements)
	}
	if !reflect.DeepEqual(first.Summary(), second.Summary()) {
		t.Fatalf("unchanged planning is nondeterministic:\nfirst=%#v\nsecond=%#v", first.Summary(), second.Summary())
	}
}

func newTestPlanner(t *testing.T, root string) *Planner {
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
	}, func(context.Context, []backupstore.CaptureRequest) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return planner
}
