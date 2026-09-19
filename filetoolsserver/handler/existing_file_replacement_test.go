package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestExistingFileReplacementBatchStagesAndCommitsPreparedReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	original := []byte("alpha\n")
	result := []byte("omega\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := filesystem.OpenFileIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identity.Close() })
	current, err := filesystem.CaptureRegularFileSnapshotBounded(context.Background(), path, int64(len(original)+32))
	if err != nil {
		t.Fatal(err)
	}
	targetFingerprint, err := filesystem.FingerprintRegularFileSnapshot(current)
	if err != nil {
		t.Fatal(err)
	}
	replacement := preparedExistingFileReplacement{
		requestedPath:     path,
		resolvedPath:      path,
		resultData:        result,
		targetFingerprint: targetFingerprint,
		resultFingerprint: filesystem.FingerprintRegularFileData(result),
		identitySlot:      &identity,
		changed:           true,
	}
	h := NewHandler([]string{root})
	batch, err := h.stageExistingFileReplacementBatch(
		context.Background(),
		[]preparedExistingFileReplacement{replacement},
		[]os.FileMode{current.Mode.Perm()},
		existingFileReplacementOps{
			stage:   stageExistingFileReplacement,
			commit:  commitExistingFileReplacement,
			cleanup: func(staged *filesystem.StagedReplacement) error { return staged.Cleanup() },
		},
		"stage_existing_file_replacement_test",
		"cleanup_existing_file_replacement_test",
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = batch.cleanup("cleanup_existing_file_replacement_test") })
	if len(batch.staged) != 1 || batch.staged[0] == nil {
		t.Fatalf("staged=%+v", batch.staged)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("staging mutated target=%q err=%v", got, err)
	}
	if err := replacement.closeIdentity(); err != nil {
		t.Fatal(err)
	}
	actual, err := h.commitExistingFileReplacementBatchTarget(
		context.Background(),
		batch,
		0,
		current,
		"committed replacement mismatch",
	)
	if err != nil {
		t.Fatal(err)
	}
	if actual != replacement.resultFingerprint || batch.staged[0] != nil {
		t.Fatalf("actual=%q staged=%+v", actual, batch.staged)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(result) {
		t.Fatalf("committed target=%q err=%v", got, err)
	}
}

func TestClassifyExistingFileReplacementUsesNeutralObservedStates(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	source := []byte("alpha")
	result := []byte("omega")
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	replacement := preparedExistingFileReplacement{
		requestedPath:     path,
		resolvedPath:      path,
		targetFingerprint: filesystem.FingerprintRegularFileData(source),
		resultFingerprint: filesystem.FingerprintRegularFileData(result),
		changed:           true,
	}
	state, actual, applied := h.classifyExistingFileReplacement(context.Background(), replacement)
	if state != existingFileReplacementStateUnchanged || actual != replacement.targetFingerprint || applied {
		t.Fatalf("unchanged classification=%q %q %v", state, actual, applied)
	}
	if err := os.WriteFile(path, result, 0o600); err != nil {
		t.Fatal(err)
	}
	state, actual, applied = h.classifyExistingFileReplacement(context.Background(), replacement)
	if state != existingFileReplacementStateCommitted || actual != replacement.resultFingerprint || !applied {
		t.Fatalf("committed classification=%q %q %v", state, actual, applied)
	}
	if err := os.WriteFile(path, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, actual, applied = h.classifyExistingFileReplacement(context.Background(), replacement)
	if state != existingFileReplacementStateUnknown || actual == "" || applied {
		t.Fatalf("unknown classification=%q %q %v", state, actual, applied)
	}
}

func TestExistingFileReplacementBatchStageFailureCleansEarlierStages(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt")}
	replacements := make([]preparedExistingFileReplacement, len(paths))
	modes := make([]os.FileMode, len(paths))
	for index, path := range paths {
		source := []byte{byte('a' + index)}
		result := []byte{byte('x' + index)}
		if err := os.WriteFile(path, source, 0o600); err != nil {
			t.Fatal(err)
		}
		replacements[index] = preparedExistingFileReplacement{
			requestedPath:     path,
			resolvedPath:      path,
			resultData:        result,
			resultFingerprint: filesystem.FingerprintRegularFileData(result),
			changed:           true,
		}
		modes[index] = 0o600
	}
	stages := 0
	ops := existingFileReplacementOps{
		stage: func(ctx context.Context, path string, data []byte, mode os.FileMode) (*filesystem.StagedReplacement, error) {
			stages++
			if stages == 2 {
				return nil, os.ErrPermission
			}
			return stageExistingFileReplacement(ctx, path, data, mode)
		},
		commit: commitExistingFileReplacement,
		cleanup: func(staged *filesystem.StagedReplacement) error {
			return staged.Cleanup()
		},
	}
	h := NewHandler([]string{root})
	batch, err := h.stageExistingFileReplacementBatch(
		context.Background(), replacements, modes, ops,
		"stage_existing_file_replacement_test", "cleanup_existing_file_replacement_test",
	)
	if err == nil || batch != nil {
		t.Fatalf("batch=%+v err=%v, want staging failure", batch, err)
	}
	for _, path := range paths {
		matches, globErr := filepath.Glob(filepath.Join(filepath.Dir(path), ".*.tmp"))
		if globErr != nil || len(matches) != 0 {
			t.Fatalf("staging artifacts remain after failure: %v err=%v", matches, globErr)
		}
	}
}
