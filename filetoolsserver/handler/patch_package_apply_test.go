package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestPatchPackageDryRunApplyVerifyOneShot(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	if err := os.WriteFile(first, []byte("alpha\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("beta\n"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{
		{path: first, oldText: "alpha", newText: "omega"},
		{path: second, oldText: "beta", newText: "gamma"},
	})
	h := NewHandler([]string{root})
	originalCommit := h.patchPackageCommitReplacement
	var commitOrder []int
	h.patchPackageCommitReplacement = func(index int, staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
		commitOrder = append(commitOrder, index)
		return originalCommit(index, staged, options)
	}

	dryResult, dryRun, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	if err != nil || dryResult.IsError {
		t.Fatalf("dryRun result=%+v output=%+v err=%v", dryResult, dryRun, err)
	}
	if len(dryRun.PreviewID) != 64 || dryRun.CreatedAt == "" || dryRun.ExpiresAt == "" {
		t.Fatalf("dryRun capability metadata missing: %+v", dryRun)
	}
	if dryRun.Results[0].State != patchPackageStatePrepared || dryRun.Results[1].State != patchPackageStatePrepared {
		t.Fatalf("dryRun states=%+v", dryRun.Results)
	}
	assertFileBytes(t, first, []byte("alpha\n"))
	assertFileBytes(t, second, []byte("beta\n"))

	applyResult, applied, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || applyResult.IsError {
		t.Fatalf("apply result=%+v output=%+v err=%v", applyResult, applied, err)
	}
	if !applied.Applied || applied.PartialCommit || applied.PreviewID != "" || applied.CommittedCount != 2 {
		t.Fatalf("unexpected apply output: %+v", applied)
	}
	if applied.Results[0].State != patchPackageStateCommitted || applied.Results[1].State != patchPackageStateCommitted {
		t.Fatalf("apply states=%+v", applied.Results)
	}
	if !reflect.DeepEqual(commitOrder, []int{0, 1}) {
		t.Fatalf("commit order=%v, want [0 1]", commitOrder)
	}
	assertFileBytes(t, first, []byte("omega\n"))
	assertFileBytes(t, second, []byte("gamma\n"))

	replay, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("replay result=%+v err=%v", replay, err)
	}

	verifiedManifest := manifest
	verifiedManifest.Targets = append([]PatchPackageTarget(nil), manifest.Targets...)
	for index := range verifiedManifest.Targets {
		verifiedManifest.Targets[index].ExpectedResultFingerprint = dryRun.Results[index].ResultFingerprint
	}
	verifyResult, verified, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionVerify, Manifest: verifiedManifest})
	if err != nil || verifyResult.IsError {
		t.Fatalf("verify result=%+v output=%+v err=%v", verifyResult, verified, err)
	}
	if !verified.Verified || verified.MismatchCount != 0 || verified.Results[0].State != patchPackageStateVerified {
		t.Fatalf("unexpected verify output: %+v", verified)
	}
}

func TestPatchPackageApplyStagesAllTargetsBeforeCommit(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt"), filepath.Join(root, "c.txt")}
	fixtures := make([]patchPackageApplyFixture, len(paths))
	for index, path := range paths {
		original := string(rune('a' + index))
		if err := os.WriteFile(path, []byte(original), 0644); err != nil {
			t.Fatal(err)
		}
		fixtures[index] = patchPackageApplyFixture{path: path, oldText: original, newText: original + "-new"}
	}
	h := NewHandler([]string{root})
	manifest := patchPackageManifestForApplyTest(t, fixtures)
	_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})

	originalStage := h.patchPackageStageReplacement
	var staged atomic.Int32
	h.patchPackageStageReplacement = func(ctx context.Context, path string, data []byte, mode os.FileMode) (*filesystem.StagedReplacement, error) {
		staged.Add(1)
		if staged.Load() == 3 {
			return nil, errors.New("injected staging failure")
		}
		return originalStage(ctx, path, data, mode)
	}
	result, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeIO {
		t.Fatalf("stage failure result=%+v err=%v", result, err)
	}
	for index, path := range paths {
		assertFileBytes(t, path, []byte(string(rune('a'+index))))
	}
	matches, globErr := filepath.Glob(filepath.Join(root, ".*.tmp"))
	if globErr != nil || len(matches) != 0 {
		t.Fatalf("staging artifacts remain: %v err=%v", matches, globErr)
	}
	terminal, _, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if !terminal.IsError || terminal.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("failed staging did not consume capability: %+v", terminal)
	}
}

func TestPatchPackageStagingCleanupFailureIsSurfaced(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt"), filepath.Join(root, "c.txt")}
	fixtures := make([]patchPackageApplyFixture, len(paths))
	for index, path := range paths {
		original := string(rune('a' + index))
		if err := os.WriteFile(path, []byte(original), 0644); err != nil {
			t.Fatal(err)
		}
		fixtures[index] = patchPackageApplyFixture{path: path, oldText: original, newText: original + "-new"}
	}
	h := NewHandler([]string{root})
	manifest := patchPackageManifestForApplyTest(t, fixtures)
	_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	originalStage := h.patchPackageStageReplacement
	var staged atomic.Int32
	h.patchPackageStageReplacement = func(ctx context.Context, path string, data []byte, mode os.FileMode) (*filesystem.StagedReplacement, error) {
		if staged.Add(1) == 3 {
			return nil, errors.New("injected staging failure")
		}
		return originalStage(ctx, path, data, mode)
	}
	originalCleanup := h.patchPackageCleanupReplacement
	var cleanups atomic.Int32
	h.patchPackageCleanupReplacement = func(replacement *filesystem.StagedReplacement) error {
		err := originalCleanup(replacement)
		if cleanups.Add(1) == 1 {
			return errors.Join(err, errors.New("injected cleanup failure"))
		}
		return err
	}
	result, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeIO {
		t.Fatalf("cleanup failure result=%+v err=%v", result, err)
	}
	text, _ := result.Content[0].(*mcp.TextContent)
	if text == nil || !strings.Contains(text.Text, "cleanup") {
		t.Fatalf("cleanup failure was not surfaced: %+v", result.Content)
	}
	for index, path := range paths {
		assertFileBytes(t, path, []byte(string(rune('a'+index))))
	}
	matches, globErr := filepath.Glob(filepath.Join(root, ".*.tmp"))
	if globErr != nil || len(matches) != 0 {
		t.Fatalf("staging artifacts remain: %v err=%v", matches, globErr)
	}
}

func TestPatchPackageApplyReportsPartialCommit(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt"), filepath.Join(root, "c.txt")}
	fixtures := make([]patchPackageApplyFixture, len(paths))
	for index, path := range paths {
		original := string(rune('a' + index))
		if err := os.WriteFile(path, []byte(original), 0644); err != nil {
			t.Fatal(err)
		}
		fixtures[index] = patchPackageApplyFixture{path: path, oldText: original, newText: original + "-new"}
	}
	h := NewHandler([]string{root})
	manifest := patchPackageManifestForApplyTest(t, fixtures)
	_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	originalCommit := h.patchPackageCommitReplacement
	h.patchPackageCommitReplacement = func(index int, staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
		if index == 1 {
			return false, errors.New("injected second commit failure")
		}
		return originalCommit(index, staged, options)
	}

	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodePartialCommit {
		t.Fatalf("partial result=%+v output=%+v err=%v", result, output, err)
	}
	if !output.PartialCommit || output.CommittedCount != 1 || output.UnchangedCount != 2 || output.UnknownCount != 0 {
		t.Fatalf("partial counts=%+v", output)
	}
	if output.FailedIndex == nil || *output.FailedIndex != 1 || output.FailureCode == "" {
		t.Fatalf("partial failure metadata=%+v", output)
	}
	if output.Results[0].State != patchPackageStateCommitted || output.Results[1].State != patchPackageStateUnchanged || output.Results[2].State != patchPackageStateUnchanged {
		t.Fatalf("partial states=%+v", output.Results)
	}
	assertFileBytes(t, paths[0], []byte("a-new"))
	assertFileBytes(t, paths[1], []byte("b"))
	assertFileBytes(t, paths[2], []byte("c"))
}

func TestPatchPackageFinalVerificationCatchesEarlierTargetChange(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	if err := os.WriteFile(first, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("beta"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{
		{path: first, oldText: "alpha", newText: "omega"},
		{path: second, oldText: "beta", newText: "gamma"},
	})
	_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	originalCommit := h.patchPackageCommitReplacement
	h.patchPackageCommitReplacement = func(index int, staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
		changed, err := originalCommit(index, staged, options)
		if err == nil && index == 1 {
			if writeErr := os.WriteFile(first, []byte("external"), 0644); writeErr != nil {
				return changed, writeErr
			}
		}
		return changed, err
	}
	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodePartialCommit {
		t.Fatalf("final verification result=%+v output=%+v err=%v", result, output, err)
	}
	if output.CommittedCount != 1 || output.UnknownCount != 1 || output.UnchangedCount != 0 {
		t.Fatalf("final verification counts=%+v", output)
	}
	if output.Results[0].State != patchPackageStateUnknown || output.Results[0].Applied ||
		output.Results[1].State != patchPackageStateCommitted || !output.Results[1].Applied {
		t.Fatalf("final verification states=%+v", output.Results)
	}
	assertFileBytes(t, first, []byte("external"))
	assertFileBytes(t, second, []byte("gamma"))
}

func TestPatchPackageApplyClassifiesCommitThenError(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: path, oldText: "alpha", newText: "omega"}})
	_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	originalCommit := h.patchPackageCommitReplacement
	h.patchPackageCommitReplacement = func(index int, staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
		_, commitErr := originalCommit(index, staged, options)
		if commitErr != nil {
			return false, commitErr
		}
		return false, errors.New("injected post-commit sync-style failure")
	}
	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodePartialCommit {
		t.Fatalf("post-commit result=%+v output=%+v err=%v", result, output, err)
	}
	if output.CommittedCount != 1 || output.Results[0].State != patchPackageStateCommitted {
		t.Fatalf("post-commit classification=%+v", output)
	}
	assertFileBytes(t, path, []byte("omega"))
}

func TestPatchPackageApplyConcurrentClaimHasOneWinner(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: path, oldText: "alpha", newText: "omega"}})
	_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})

	var wg sync.WaitGroup
	wg.Add(2)
	results := make(chan *patchPackageApplyAttempt, 2)
	for range 2 {
		go func() {
			defer wg.Done()
			result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
			results <- &patchPackageApplyAttempt{result: result, output: output, err: err}
		}()
	}
	wg.Wait()
	close(results)
	var success, conflict int
	for attempt := range results {
		if attempt.err != nil {
			t.Fatal(attempt.err)
		}
		if attempt.result.IsError {
			if attempt.result.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
				t.Fatalf("loser result=%+v", attempt.result)
			}
			conflict++
		} else {
			if !attempt.output.Applied {
				t.Fatalf("winner output=%+v", attempt.output)
			}
			success++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}

func TestPatchPackageVerifyMismatchReturnsStructuredConflict(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: path, oldText: "alpha", newText: "omega"}})
	manifest.Targets[0].ExpectedResultFingerprint = strings64("0")
	h := NewHandler([]string{root})
	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionVerify, Manifest: manifest})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("verify mismatch result=%+v output=%+v err=%v", result, output, err)
	}
	if output.Verified || output.MismatchCount != 1 || output.Results[0].State != patchPackageStateMismatch || output.Results[0].ActualFingerprint == "" {
		t.Fatalf("verify mismatch output=%+v", output)
	}
}

func TestBoundedPatchPackageFailureMessageIsValidUTF8(t *testing.T) {
	message := strings.Repeat("é", maxPatchPackageFailureMessageBytes) + "\xfftail"
	bounded := boundedPatchPackageFailureMessage(message)
	if len(bounded) > maxPatchPackageFailureMessageBytes || !utf8.ValidString(bounded) {
		t.Fatalf("bounded message length=%d valid=%v", len(bounded), utf8.ValidString(bounded))
	}
}

func TestPatchPackageApplyStrictInput(t *testing.T) {
	root := t.TempDir()
	h := NewHandler([]string{root})
	result, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{
		Action: patchPackageActionApply, PreviewID: strings64("a"),
		Manifest: PatchPackageManifest{FormatVersion: PatchPackageFormatV1},
	})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("strict apply result=%+v err=%v", result, err)
	}
}

func TestPatchPackageApplyFailureAtEveryCommitPosition(t *testing.T) {
	for failureIndex := 0; failureIndex < 3; failureIndex++ {
		t.Run(fmt.Sprintf("index_%d", failureIndex), func(t *testing.T) {
			root := t.TempDir()
			fixtures := make([]patchPackageApplyFixture, 3)
			for index := range fixtures {
				path := filepath.Join(root, fmt.Sprintf("%d.txt", index))
				original := fmt.Sprintf("old-%d", index)
				if err := os.WriteFile(path, []byte(original), 0644); err != nil {
					t.Fatal(err)
				}
				fixtures[index] = patchPackageApplyFixture{path: path, oldText: original, newText: fmt.Sprintf("new-%d", index)}
			}
			h := NewHandler([]string{root})
			manifest := patchPackageManifestForApplyTest(t, fixtures)
			_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
			originalCommit := h.patchPackageCommitReplacement
			h.patchPackageCommitReplacement = func(index int, staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
				if index == failureIndex {
					return false, errors.New("injected commit failure")
				}
				return originalCommit(index, staged, options)
			}
			result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
			if err != nil || !result.IsError {
				t.Fatalf("result=%+v output=%+v err=%v", result, output, err)
			}
			wantCode := ErrCodeIO
			if failureIndex > 0 {
				wantCode = ErrCodePartialCommit
			}
			if result.Meta[ErrorCodeMetaKey] != wantCode || output.CommittedCount != failureIndex || output.UnchangedCount != 3-failureIndex {
				t.Fatalf("failure %d result=%+v output=%+v", failureIndex, result, output)
			}
			for index, fixture := range fixtures {
				want := fixture.oldText
				if index < failureIndex {
					want = fixture.newText
				}
				assertFileBytes(t, fixture.path, []byte(want))
			}
		})
	}
}

func TestPatchPackageApplyForceWritable(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatal(err)
	}
	forceWritable := true
	manifest := PatchPackageManifest{
		FormatVersion: PatchPackageFormatV1, FingerprintAlgorithm: "sha256", FingerprintMode: "content-v1",
		Targets: []PatchPackageTarget{{
			Path: path, ExpectedFingerprint: fingerprintRegularFileForTest(t, path), ForceWritable: &forceWritable,
			Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}},
		}},
	}
	h := NewHandler([]string{root})
	result, dryRun, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	if err != nil || result.IsError {
		t.Fatalf("forceWritable dryRun result=%+v output=%+v err=%v", result, dryRun, err)
	}
	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || result.IsError || output.CommittedCount != 1 || !output.Results[0].ReadOnlyCleared {
		t.Fatalf("forceWritable apply result=%+v output=%+v err=%v", result, output, err)
	}
	assertFileBytes(t, path, []byte("omega"))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if isReadOnly(info.Mode().Perm()) {
		t.Fatalf("target remained read-only: %v", info.Mode())
	}
}

func TestPatchPackageExternalChangeAfterStagingReturnsUnknownPartialCommit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: path, oldText: "alpha", newText: "omega"}})
	_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	originalStage := h.patchPackageStageReplacement
	h.patchPackageStageReplacement = func(ctx context.Context, targetPath string, data []byte, mode os.FileMode) (*filesystem.StagedReplacement, error) {
		replacement, stageErr := originalStage(ctx, targetPath, data, mode)
		if stageErr != nil {
			return replacement, stageErr
		}
		if writeErr := os.WriteFile(path, []byte("external"), 0644); writeErr != nil {
			return replacement, writeErr
		}
		return replacement, nil
	}
	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodePartialCommit {
		t.Fatalf("external change result=%+v output=%+v err=%v", result, output, err)
	}
	if !output.PartialCommit || output.UnknownCount != 1 || output.CommittedCount != 0 || output.Results[0].State != patchPackageStateUnknown {
		t.Fatalf("external change classification=%+v", output)
	}
	assertFileBytes(t, path, []byte("external"))
	matches, globErr := filepath.Glob(filepath.Join(root, ".*.tmp"))
	if globErr != nil || len(matches) != 0 {
		t.Fatalf("staging artifacts remain: %v err=%v", matches, globErr)
	}
}

func TestPatchPackageCancellationDuringStagingCleansUpAndConsumesCapability(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: path, oldText: "alpha", newText: strings.Repeat("omega", 4096)}})
	_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	ctx, cancel := context.WithCancel(context.Background())
	originalStage := h.patchPackageStageReplacement
	h.patchPackageStageReplacement = func(stageCtx context.Context, path string, data []byte, mode os.FileMode) (*filesystem.StagedReplacement, error) {
		cancel()
		return originalStage(stageCtx, path, data, mode)
	}
	result, _, err := h.HandlePatchPackage(ctx, nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeCancelled {
		t.Fatalf("staging cancellation result=%+v err=%v", result, err)
	}
	assertFileBytes(t, path, []byte("alpha"))
	matches, globErr := filepath.Glob(filepath.Join(root, ".*.tmp"))
	if globErr != nil || len(matches) != 0 {
		t.Fatalf("staging artifacts remain: %v err=%v", matches, globErr)
	}
	terminal, _, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if !terminal.IsError || terminal.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("cancelled staging capability was not terminal: %+v", terminal)
	}
}

func TestPatchPackageCancellationAfterStagingLeavesTargetsUnchanged(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: path, oldText: "alpha", newText: "omega"}})
	_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	ctx, cancel := context.WithCancel(context.Background())
	originalStage := h.patchPackageStageReplacement
	h.patchPackageStageReplacement = func(stageCtx context.Context, targetPath string, data []byte, mode os.FileMode) (*filesystem.StagedReplacement, error) {
		replacement, stageErr := originalStage(stageCtx, targetPath, data, mode)
		if stageErr == nil {
			cancel()
		}
		return replacement, stageErr
	}
	result, output, err := h.HandlePatchPackage(ctx, nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeCancelled || output.CommittedCount != 0 {
		t.Fatalf("cancelled result=%+v output=%+v err=%v", result, output, err)
	}
	assertFileBytes(t, path, []byte("alpha"))
	matches, globErr := filepath.Glob(filepath.Join(root, ".*.tmp"))
	if globErr != nil || len(matches) != 0 {
		t.Fatalf("staging artifacts remain: %v err=%v", matches, globErr)
	}
}

func TestPatchPackageNoOpApplyPreservesBytesAndMetadata(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	original := []byte("alpha\r\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: path, oldText: "alpha", newText: "alpha"}})
	_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || result.IsError || !output.Applied || output.CommittedCount != 0 || output.UnchangedCount != 1 {
		t.Fatalf("no-op result=%+v output=%+v err=%v", result, output, err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	assertFileBytes(t, path, original)
	if !before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode() {
		t.Fatalf("no-op metadata changed: before=%+v after=%+v", before, after)
	}
}

func TestPatchPackageApplyPreservesUTF16BOMAndCRLF(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.data")
	original := encodeUTF16LEWithBOM(t, "alpha\r\nbeta\r\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	manifest := PatchPackageManifest{
		FormatVersion: PatchPackageFormatV1, FingerprintAlgorithm: "sha256", FingerprintMode: "content-v1",
		Targets: []PatchPackageTarget{{
			Path: path, ExpectedFingerprint: fingerprintRegularFileForTest(t, path), Encoding: "utf-16-le",
			Edits: []EditOperation{{OldText: "beta", NewText: "gamma"}},
		}},
	}
	h := NewHandler([]string{root})
	_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	result, output, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || result.IsError || output.CommittedCount != 1 {
		t.Fatalf("UTF-16 result=%+v output=%+v err=%v", result, output, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := encodeUTF16LEWithBOM(t, "alpha\r\ngamma\r\n")
	if !bytes.Equal(data, want) {
		t.Fatalf("UTF-16 bytes differ\ngot:  % X\nwant: % X", data, want)
	}
}

func TestPatchPackageApplyOutputLimitIsTerminalAndNonMutating(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: path, oldText: "alpha", newText: "omega"}})
	_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	h.config.Limits.MaxOutputBytes = 1
	result, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("output-limited result=%+v err=%v", result, err)
	}
	h.config.Limits.MaxOutputBytes = config.DefaultMaxOutputBytes
	terminal, _, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if !terminal.IsError || terminal.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("output-limited capability was not terminal: %+v", terminal)
	}
	assertFileBytes(t, path, []byte("alpha"))
}

type patchPackageApplyFixture struct {
	path    string
	oldText string
	newText string
}

func patchPackageManifestForApplyTest(t *testing.T, fixtures []patchPackageApplyFixture) PatchPackageManifest {
	t.Helper()
	manifest := PatchPackageManifest{
		FormatVersion: PatchPackageFormatV1, FingerprintAlgorithm: "sha256", FingerprintMode: "content-v1",
		Targets: make([]PatchPackageTarget, len(fixtures)),
	}
	for index, fixture := range fixtures {
		manifest.Targets[index] = PatchPackageTarget{
			Path: fixture.path, ExpectedFingerprint: fingerprintRegularFileForTest(t, fixture.path),
			Edits: []EditOperation{{OldText: fixture.oldText, NewText: fixture.newText}},
		}
	}
	return manifest
}

func strings64(value string) string {
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}

type patchPackageApplyAttempt struct {
	result *mcp.CallToolResult
	output PatchPackageOutput
	err    error
}
