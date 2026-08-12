package filesystempackage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

func TestDeletePreparedFileRequiresVerifiedPersistentBackup(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	planner := newTestPlanner(t, root)
	plan, err := planner.Plan(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{
		Type: OperationDeleteFile, Path: target,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := DeletePreparedFile(context.Background(), plan.Operations[0], VerifiedBackupBatch{}); operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("delete without proof error = %v, want INVALID_INPUT", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("delete without proof changed target: %v", err)
	}
}

func TestCapturePreparedBackupsThenDeleteFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	planner := newTestPlanner(t, root)
	plan, err := planner.Plan(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{
		Type: OperationDeleteFile, Path: target,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := CapturePreparedBackups(context.Background(), plan, fakeVerifiedCapture(plan), testPreparedTreeOptions(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(proof.BackupIDs()) != 1 {
		t.Fatalf("backup ids = %v", proof.BackupIDs())
	}
	if err := DeletePreparedFile(context.Background(), plan.Operations[0], proof); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target still exists after verified delete: %v", err)
	}
}

func TestCapturePreparedBackupsRejectsMismatchedManifestEvidence(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	planner := newTestPlanner(t, root)
	plan, err := planner.Plan(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{Type: OperationDeleteFile, Path: target}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = CapturePreparedBackups(context.Background(), plan, func(_ context.Context, requests []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error) {
		return []backupstore.CaptureResult{{Manifest: backupstore.Manifest{
			BackupID: "backup-1", TargetPath: requests[0].TargetPath,
			SourceOperation:    backupstore.SourceOperationFilesystemPackage,
			ContentFingerprint: "wrong", ObjectBytes: plan.BackupRequirements[0].Bytes,
		}}}, nil
	}, testPreparedTreeOptions(root))
	if operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("mismatched backup evidence error = %v, want CONFLICT", err)
	}
	if _, statErr := os.Stat(target); statErr != nil {
		t.Fatalf("backup evidence rejection changed target: %v", statErr)
	}
}

func TestDeletePreparedDirectoryNeverSweepsNewDescendant(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(filepath.Join(target, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "nested", "prepared.txt"), []byte("prepared"), 0o600); err != nil {
		t.Fatal(err)
	}
	planner := newTestPlanner(t, root)
	plan, err := planner.Plan(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{Type: OperationDeleteDirectory, Path: target}}})
	if err != nil {
		t.Fatal(err)
	}
	options := testPreparedTreeOptions(root)
	proof, err := CapturePreparedBackups(context.Background(), plan, fakeVerifiedCapture(plan), options)
	if err != nil {
		t.Fatal(err)
	}
	intruder := filepath.Join(target, "nested", "intruder.txt")
	if err := os.WriteFile(intruder, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := DeletePreparedDirectory(context.Background(), plan.Operations[0], proof, options); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("new descendant delete error = %v, want CONFLICT", err)
	}
	if got, err := os.ReadFile(intruder); err != nil || string(got) != "new" {
		t.Fatalf("new descendant was swept or changed: %q / %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(target, "nested", "prepared.txt")); err != nil {
		t.Fatalf("prepared tree changed after pre-delete conflict: %v", err)
	}
}

func TestDeletePreparedDirectoryRemovesOnlyExactPreparedTree(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(filepath.Join(target, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "nested", "b.txt"), []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	planner := newTestPlanner(t, root)
	plan, err := planner.Plan(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{Type: OperationDeleteDirectory, Path: target}}})
	if err != nil {
		t.Fatal(err)
	}
	options := testPreparedTreeOptions(root)
	proof, err := CapturePreparedBackups(context.Background(), plan, fakeVerifiedCapture(plan), options)
	if err != nil {
		t.Fatal(err)
	}
	if err := DeletePreparedDirectory(context.Background(), plan.Operations[0], proof, options); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("directory still exists after exact delete: %v", err)
	}
}

func fakeVerifiedCapture(plan PreparedPackage) BackupCaptureFunc {
	return func(_ context.Context, requests []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error) {
		results := make([]backupstore.CaptureResult, len(requests))
		for index, request := range requests {
			requirement := plan.BackupRequirements[index]
			results[index] = backupstore.CaptureResult{Manifest: backupstore.Manifest{
				BackupID: "backup-" + filepath.Base(request.TargetPath), TargetPath: request.TargetPath,
				SourceOperation:    backupstore.SourceOperationFilesystemPackage,
				ContentFingerprint: requirement.ExpectedFingerprint, ObjectBytes: requirement.Bytes,
			}}
		}
		return results, nil
	}
}

func testPreparedTreeOptions(root string) filesystem.ExactTreeOptions {
	return filesystem.ExactTreeOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
		MaxEntries:          256, MaxDepth: 32, MaxFileBytes: 1024, MaxAggregateBytes: 1 << 20,
	}
}
