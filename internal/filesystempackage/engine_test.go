package filesystempackage

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

func TestEnginePreviewIsReadOnlyAndApplyCommitsAllSevenOperations(t *testing.T) {
	root := t.TempDir()
	copySource := filepath.Join(root, "copy-source.txt")
	copyDirSource := filepath.Join(root, "copy-dir-source")
	moveSource := filepath.Join(root, "move-source.txt")
	deleteFile := filepath.Join(root, "delete-file.txt")
	deleteDir := filepath.Join(root, "delete-dir")
	mustWritePackageTestFile(t, copySource, []byte("copy"))
	if err := os.MkdirAll(filepath.Join(copyDirSource, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWritePackageTestFile(t, filepath.Join(copyDirSource, ".hidden"), []byte("hidden"))
	mustWritePackageTestFile(t, filepath.Join(copyDirSource, ".git", "config"), []byte("git"))
	mustWritePackageTestFile(t, moveSource, []byte("move"))
	mustWritePackageTestFile(t, deleteFile, []byte("delete"))
	if err := os.MkdirAll(filepath.Join(deleteDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWritePackageTestFile(t, filepath.Join(deleteDir, "nested", "gone.txt"), []byte("gone"))

	engine := newTestEngine(t, root, testEngineLimits(), captureCurrentFiles)
	manifest := Manifest{FormatVersion: FormatV1, Operations: []Operation{
		{Type: OperationMkdir, Path: filepath.Join(root, "new-parent")},
		{Type: OperationCreateFile, Path: filepath.Join(root, "new-parent", "raw.bin"), Content: []byte{0x00, 0xff, 0x0a}},
		{Type: OperationCopyFile, Source: copySource, Destination: filepath.Join(root, "copied.txt")},
		{Type: OperationCopyDirectory, Source: copyDirSource, Destination: filepath.Join(root, "copied-dir")},
		{Type: OperationMove, Source: moveSource, Destination: filepath.Join(root, "moved.txt")},
		{Type: OperationDeleteFile, Path: deleteFile},
		{Type: OperationDeleteDirectory, Path: deleteDir},
	}}
	before := filesystemPackageTestTreeNames(t, root)
	preview, err := engine.Preview(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if preview.PreviewID == "" || preview.Plan.OperationCount != 7 || preview.Plan.BackupCount != 2 {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	if after := filesystemPackageTestTreeNames(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("preview changed workspace:\nbefore=%v\nafter=%v", before, after)
	}

	output, err := engine.Apply(context.Background(), preview.PreviewID)
	if err != nil {
		t.Fatal(err)
	}
	if !output.Applied || output.PartialCommit || len(output.BackupIDs) != 2 {
		t.Fatalf("unexpected apply output: %#v", output)
	}
	for _, result := range output.Results {
		if result.State != StateCommitted {
			t.Fatalf("operation %d state = %q", result.Index, result.State)
		}
	}
	raw, err := os.ReadFile(filepath.Join(root, "new-parent", "raw.bin"))
	if err != nil || !reflect.DeepEqual(raw, []byte{0x00, 0xff, 0x0a}) {
		t.Fatalf("raw file = %x / %v", raw, err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "copied.txt")); err != nil || string(got) != "copy" {
		t.Fatalf("copied file = %q / %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "copied-dir", ".git", "config")); err != nil {
		t.Fatalf("recursive copy is incomplete: %v", err)
	}
	if _, err := os.Stat(moveSource); !os.IsNotExist(err) {
		t.Fatalf("move source remains: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "moved.txt")); err != nil || string(got) != "move" {
		t.Fatalf("moved file = %q / %v", got, err)
	}
	if _, err := os.Stat(deleteFile); !os.IsNotExist(err) {
		t.Fatalf("deleteFile target remains: %v", err)
	}
	if _, err := os.Stat(deleteDir); !os.IsNotExist(err) {
		t.Fatalf("deleteDirectory target remains: %v", err)
	}
	if _, err := engine.Apply(context.Background(), preview.PreviewID); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("replay error = %v, want CONFLICT", err)
	}
}

func TestEngineStagesAllFeasibleContentBeforeFirstCommit(t *testing.T) {
	root := t.TempDir()
	engine := newTestEngine(t, root, testEngineLimits(), nil)
	manifest := Manifest{FormatVersion: FormatV1, Operations: []Operation{
		{Type: OperationCreateFile, Path: filepath.Join(root, "first.bin"), Content: []byte("first")},
		{Type: OperationCreateFile, Path: filepath.Join(root, "second.bin"), Content: []byte("second")},
	}}
	preview, err := engine.Preview(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	originalPublish := engine.commitOps.publishFile
	inspected := false
	engine.commitOps.publishFile = func(staged *filesystem.StagedFile, destination string) error {
		if !inspected {
			inspected = true
			entries, err := os.ReadDir(root)
			if err != nil {
				return err
			}
			stagingCount := 0
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".scripthold-r24-file-") {
					stagingCount++
				}
			}
			if stagingCount != 2 {
				return operation.New(operation.KindConflict, "all file stages were not ready before the first publish")
			}
			for _, target := range []string{"first.bin", "second.bin"} {
				if _, err := os.Stat(filepath.Join(root, target)); !os.IsNotExist(err) {
					return operation.New(operation.KindConflict, "target appeared before first publish")
				}
			}
		}
		return originalPublish(staged, destination)
	}
	output, err := engine.Apply(context.Background(), preview.PreviewID)
	if err != nil || !output.Applied {
		t.Fatalf("apply = %#v / %v", output, err)
	}
	for _, entry := range filesystemPackageTestTreeNames(t, root) {
		if strings.Contains(entry, ".scripthold-r24-file-") {
			t.Fatalf("staging residue remains after successful apply: %s", entry)
		}
	}
}

func TestEnginePrecommitConflictConsumesCapabilityWithoutMutation(t *testing.T) {
	root := t.TempDir()
	engine := newTestEngine(t, root, testEngineLimits(), nil)
	target := filepath.Join(root, "target.txt")
	preview, err := engine.Preview(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{
		Type: OperationCreateFile, Path: target, Content: []byte("prepared"),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	mustWritePackageTestFile(t, target, []byte("racer"))
	output, err := engine.Apply(context.Background(), preview.PreviewID)
	if operation.KindOf(err) != operation.KindConflict || output.PartialCommit {
		t.Fatalf("precommit apply = %#v / %v", output, err)
	}
	if got, readErr := os.ReadFile(target); readErr != nil || string(got) != "racer" {
		t.Fatalf("racer target changed: %q / %v", got, readErr)
	}
	if _, replayErr := engine.Apply(context.Background(), preview.PreviewID); operation.KindOf(replayErr) != operation.KindConflict {
		t.Fatalf("consumed conflict preview replay error = %v", replayErr)
	}
}

func TestEngineMidCommitFailureReturnsDeterministicPartialState(t *testing.T) {
	root := t.TempDir()
	engine := newTestEngine(t, root, testEngineLimits(), nil)
	preview, err := engine.Preview(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{
		{Type: OperationMkdir, Path: filepath.Join(root, "first")},
		{Type: OperationMkdir, Path: filepath.Join(root, "second")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	originalCreate := engine.commitOps.createDirectory
	createCalls := 0
	engine.commitOps.createDirectory = func(path string, mode os.FileMode) error {
		createCalls++
		if createCalls == 2 {
			return operation.New(operation.KindFilesystem, "injected second-operation failure")
		}
		return originalCreate(path, mode)
	}
	output, err := engine.Apply(context.Background(), preview.PreviewID)
	if operation.KindOf(err) != operation.KindPartialCommit || !output.PartialCommit {
		t.Fatalf("partial apply = %#v / %v", output, err)
	}
	if output.Results[0].State != StateCommitted || output.Results[1].State != StateUnchanged {
		t.Fatalf("partial states = %#v", output.Results)
	}
	if output.FailedIndex == nil || *output.FailedIndex != 1 {
		t.Fatalf("failed index = %#v", output.FailedIndex)
	}
}

func TestEnginePostCommitFailureClassifiesCommittedCurrentOperation(t *testing.T) {
	root := t.TempDir()
	engine := newTestEngine(t, root, testEngineLimits(), nil)
	preview, err := engine.Preview(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{
		Type: OperationMkdir, Path: filepath.Join(root, "created"),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	engine.commitOps.captureObjectIdentity = func(string) (filesystem.ObjectIdentity, error) {
		return filesystem.ObjectIdentity{}, operation.New(operation.KindFilesystem, "injected post-commit verification failure")
	}
	output, err := engine.Apply(context.Background(), preview.PreviewID)
	if operation.KindOf(err) != operation.KindPartialCommit || output.Results[0].State != StateCommitted {
		t.Fatalf("post-commit classification = %#v / %v", output, err)
	}
}

func TestEngineCapabilityExpiryEvictionRestartWrongKindAndConcurrentClaim(t *testing.T) {
	t.Run("expiry", func(t *testing.T) {
		root := t.TempDir()
		limits := testEngineLimits()
		limits.PreviewTTLSeconds = 1
		engine := newTestEngine(t, root, limits, nil)
		now := time.Now().UTC()
		engine.previews.now = func() time.Time { return now }
		preview, err := engine.Preview(context.Background(), mkdirManifest(filepath.Join(root, "expired")))
		if err != nil {
			t.Fatal(err)
		}
		now = now.Add(2 * time.Second)
		if _, err := engine.Apply(context.Background(), preview.PreviewID); operation.KindOf(err) != operation.KindConflict {
			t.Fatalf("expired apply error = %v", err)
		}
	})

	t.Run("eviction", func(t *testing.T) {
		root := t.TempDir()
		limits := testEngineLimits()
		limits.MaxPreviews = 1
		engine := newTestEngine(t, root, limits, nil)
		first, err := engine.Preview(context.Background(), mkdirManifest(filepath.Join(root, "first")))
		if err != nil {
			t.Fatal(err)
		}
		second, err := engine.Preview(context.Background(), mkdirManifest(filepath.Join(root, "second")))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := engine.Apply(context.Background(), first.PreviewID); operation.KindOf(err) != operation.KindConflict {
			t.Fatalf("evicted apply error = %v", err)
		}
		if output, err := engine.Apply(context.Background(), second.PreviewID); err != nil || !output.Applied {
			t.Fatalf("newest preview apply = %#v / %v", output, err)
		}
	})

	t.Run("restart and wrong kind", func(t *testing.T) {
		root := t.TempDir()
		engine := newTestEngine(t, root, testEngineLimits(), nil)
		preview, err := engine.Preview(context.Background(), mkdirManifest(filepath.Join(root, "restart")))
		if err != nil {
			t.Fatal(err)
		}
		restarted := newTestEngine(t, root, testEngineLimits(), nil)
		if _, err := restarted.Apply(context.Background(), preview.PreviewID); operation.KindOf(err) != operation.KindConflict {
			t.Fatalf("restart-invalidated apply error = %v", err)
		}
		if _, err := engine.Apply(context.Background(), strings.Repeat("a", 64)); operation.KindOf(err) != operation.KindConflict {
			t.Fatalf("wrong-kind/unowned token error = %v", err)
		}
	})

	t.Run("concurrent one shot", func(t *testing.T) {
		root := t.TempDir()
		engine := newTestEngine(t, root, testEngineLimits(), nil)
		preview, err := engine.Preview(context.Background(), mkdirManifest(filepath.Join(root, "once")))
		if err != nil {
			t.Fatal(err)
		}
		type outcome struct {
			output ApplyOutput
			err    error
		}
		start := make(chan struct{})
		outcomes := make(chan outcome, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				output, err := engine.Apply(context.Background(), preview.PreviewID)
				outcomes <- outcome{output: output, err: err}
			}()
		}
		close(start)
		wg.Wait()
		close(outcomes)
		successes, conflicts := 0, 0
		for result := range outcomes {
			switch {
			case result.err == nil && result.output.Applied:
				successes++
			case operation.KindOf(result.err) == operation.KindConflict:
				conflicts++
			default:
				t.Fatalf("unexpected concurrent outcome: %#v / %v", result.output, result.err)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatalf("concurrent successes=%d conflicts=%d", successes, conflicts)
		}
	})
}

func TestEngineRejectsWorstCaseOutputBeforeCapabilityCreation(t *testing.T) {
	root := t.TempDir()
	limits := testEngineLimits()
	// Preview itself fits, but the bounded worst-case partial-commit response,
	// including cleanup residue and error text, must not.
	limits.MaxOutputBytes = 16 * 1024
	engine := newTestEngine(t, root, limits, nil)
	_, err := engine.Preview(context.Background(), mkdirManifest(filepath.Join(root, "target")))
	if operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("output limit error = %v, want LIMIT", err)
	}
	if len(engine.previews.entries) != 0 {
		t.Fatalf("preview capability was retained after output-limit rejection: %d", len(engine.previews.entries))
	}
}

func newTestEngine(t *testing.T, root string, limits Limits, capture BackupCaptureFunc) *Engine {
	t.Helper()
	set, err := security.NormalizeAllowedDirectorySet([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := NewPlanner(limits, func(path string) (security.PathEvidence, error) {
		return security.ValidatePathEvidenceWithAllowedDirectories(path, set.Requested, set.Resolved)
	}, func() []string {
		return append([]string(nil), set.Resolved...)
	}, func(context.Context, []backupstore.CaptureRequest) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(planner, limits, capture)
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func testEngineLimits() Limits {
	limits := testManifestLimits()
	limits.MaxOperations = 32
	limits.MaxRecursiveEntries = 512
	limits.MaxRecursiveDepth = 32
	limits.MaxAggregateBytes = 1 << 20
	limits.MaxStagingBytes = 1 << 20
	limits.MaxPreviews = 8
	limits.MaxPreviewBytes = 4 << 20
	limits.PreviewTTLSeconds = 60
	limits.MaxOutputBytes = 1 << 20
	return limits
}

func captureCurrentFiles(ctx context.Context, requests []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error) {
	results := make([]backupstore.CaptureResult, len(requests))
	for index, request := range requests {
		snapshot, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, request.TargetPath, 1<<20)
		if err != nil {
			return results[:index], err
		}
		fingerprint, err := filesystem.FingerprintRegularFileSnapshot(snapshot)
		if err != nil {
			return results[:index], err
		}
		results[index] = backupstore.CaptureResult{Manifest: backupstore.Manifest{
			BackupID:   "test-backup-" + filepath.Base(request.TargetPath),
			TargetPath: request.TargetPath, SourceOperation: backupstore.SourceOperationFilesystemPackage,
			ContentFingerprint: fingerprint, ObjectBytes: snapshot.Size,
		}}
	}
	return results, nil
}

func mkdirManifest(path string) Manifest {
	return Manifest{FormatVersion: FormatV1, Operations: []Operation{{Type: OperationMkdir, Path: path}}}
}

func mustWritePackageTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func filesystemPackageTestTreeNames(t *testing.T, root string) []string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return names
}
