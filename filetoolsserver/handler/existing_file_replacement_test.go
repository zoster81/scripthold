package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestPreparedEditPlanRequiresOrderedTargets(t *testing.T) {
	if _, err := newPreparedEditPlan(nil); err == nil {
		t.Fatal("empty prepared edit plan was accepted")
	}
	targets := []preparedEditPlanTarget{{index: 0}, {index: 1}}
	plan, err := newPreparedEditPlan(targets)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.targets) != 2 || plan.targets[0].index != 0 || plan.targets[1].index != 1 {
		t.Fatalf("plan targets=%+v", plan.targets)
	}
	targets[0].index = 9
	if plan.targets[0].index != 0 {
		t.Fatal("prepared edit plan did not retain its own ordered target slice")
	}
	if _, err := newPreparedEditPlan([]preparedEditPlanTarget{{index: 1}}); err == nil {
		t.Fatal("out-of-order prepared edit plan target was accepted")
	}
}

func TestPreparedEditPlanBuildsBackupRequestsForChangedTargets(t *testing.T) {
	plan, err := newPreparedEditPlan([]preparedEditPlanTarget{
		{index: 0, resolvedPath: "first", prepared: preparedEdit{changed: false}},
		{index: 1, resolvedPath: "second", prepared: preparedEdit{changed: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	requests := plan.backupCaptureRequests(backupstore.SourceOperationPatchPackage, "batch", editBackupPolicyPinned)
	if len(requests) != 1 {
		t.Fatalf("requests=%+v, want one changed target", requests)
	}
	request := requests[0]
	if request.TargetPath != "second" || request.SourceOperation != backupstore.SourceOperationPatchPackage || request.Label != "batch" || !request.Pinned {
		t.Fatalf("request=%+v", request)
	}

	single, err := newSinglePreparedEditPlan(preparedEdit{resolvedPath: "single", targetFingerprint: "before", resultFingerprint: "after", changed: true})
	if err != nil {
		t.Fatal(err)
	}
	requests = single.backupCaptureRequests(backupstore.SourceOperationEdit, "", editBackupPolicyRequired)
	if len(requests) != 1 || requests[0].TargetPath != "single" || requests[0].SourceOperation != backupstore.SourceOperationEdit || requests[0].Label != "" || requests[0].Pinned {
		t.Fatalf("single requests=%+v", requests)
	}
}

func TestPreparedExistingFileReplacementWritablePolicy(t *testing.T) {
	readOnly := os.FileMode(0o444)
	writable := os.FileMode(0o644)
	replacement := preparedExistingFileReplacement{}
	if !replacement.readOnlyRequiresApproval(readOnly) {
		t.Fatal("read-only replacement without approval was accepted")
	}
	if replacement.readOnlyRequiresApproval(writable) {
		t.Fatal("writable replacement unexpectedly requires approval")
	}
	replacement.forceWritable = true
	if replacement.readOnlyRequiresApproval(readOnly) {
		t.Fatal("forceWritable approval was ignored")
	}
	if got := existingFileReplacementWriteMode(readOnly); got != writable {
		t.Fatalf("writable mode=%#o, want %#o", got, writable)
	}
	if got := existingFileReplacementWriteMode(writable); got != writable {
		t.Fatalf("existing writable mode=%#o, want %#o", got, writable)
	}
}

func TestPreparedExistingFileReplacementMatchesTargetSnapshotFingerprint(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := filesystem.CaptureRegularFileSnapshotBounded(context.Background(), path, 64)
	if err != nil {
		t.Fatal(err)
	}
	targetFingerprint, err := filesystem.FingerprintRegularFileSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	replacement := preparedExistingFileReplacement{targetFingerprint: targetFingerprint}
	actual, matches, err := replacement.snapshotMatchesTargetFingerprint(snapshot)
	if err != nil || !matches || actual != targetFingerprint {
		t.Fatalf("actual=%q matches=%v err=%v", actual, matches, err)
	}
	replacement.targetFingerprint = filesystem.FingerprintRegularFileData([]byte("other"))
	actual, matches, err = replacement.snapshotMatchesTargetFingerprint(snapshot)
	if err != nil || matches || actual != targetFingerprint {
		t.Fatalf("mismatch actual=%q matches=%v err=%v", actual, matches, err)
	}
}

func TestInspectExistingFileReplacementBinding(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	if err := os.WriteFile(first, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := filesystem.OpenFileIdentity(first)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identity.Close() })
	replacement := preparedExistingFileReplacement{requestedPath: first, resolvedPath: first, identitySlot: &identity}
	h := NewHandler([]string{root})
	validation, issue := h.inspectExistingFileReplacementBinding(replacement)
	if !validation.Ok() || issue != existingFileReplacementBindingOK {
		t.Fatalf("valid binding validation=%+v issue=%v", validation, issue)
	}

	replacement.resolvedPath = second
	_, issue = h.inspectExistingFileReplacementBinding(replacement)
	if issue != existingFileReplacementBindingPathChanged {
		t.Fatalf("path issue=%v, want path changed", issue)
	}

	replacement.requestedPath = second
	replacement.resolvedPath = second
	_, issue = h.inspectExistingFileReplacementBinding(replacement)
	if issue != existingFileReplacementBindingIdentityChanged {
		t.Fatalf("identity issue=%v, want identity changed", issue)
	}

	replacement.identitySlot = nil
	_, issue = h.inspectExistingFileReplacementBinding(replacement)
	if issue != existingFileReplacementBindingIdentityUnavailable {
		t.Fatalf("missing identity issue=%v, want identity unavailable", issue)
	}
}

func TestPreparedExistingFileReplacementChecksRetainedResultFingerprint(t *testing.T) {
	result := []byte("result")
	replacement := preparedExistingFileReplacement{
		resultData:        result,
		resultFingerprint: filesystem.FingerprintRegularFileData(result),
	}
	if !replacement.resultMatchesFingerprint() {
		t.Fatal("matching retained result fingerprint was rejected")
	}
	replacement.resultData = []byte("tampered")
	if replacement.resultMatchesFingerprint() {
		t.Fatal("tampered retained result fingerprint was accepted")
	}
}

func TestPreparedEditPlanReplacementPreservesPlanPathBinding(t *testing.T) {
	target := preparedEditPlanTarget{
		requestedPath: "planned-requested",
		resolvedPath:  "planned-resolved",
		prepared: preparedEdit{
			requestedPath:     "prepared-requested",
			resolvedPath:      "prepared-resolved",
			data:              []byte("result"),
			targetFingerprint: "before",
			resultFingerprint: "after",
			changed:           true,
		},
	}
	replacement := preparedEditPlanReplacement(&target)
	if replacement.requestedPath != target.requestedPath || replacement.resolvedPath != target.resolvedPath {
		t.Fatalf("replacement paths=%q %q, want plan paths=%q %q", replacement.requestedPath, replacement.resolvedPath, target.requestedPath, target.resolvedPath)
	}
	if string(replacement.resultData) != "result" || replacement.targetFingerprint != "before" || replacement.resultFingerprint != "after" || !replacement.changed {
		t.Fatalf("replacement lost prepared edit state: %+v", replacement)
	}
}

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
	root := canonicalHandlerTestDir(t)
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
