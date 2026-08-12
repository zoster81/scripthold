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

func TestPlannerRejectsUnicodeNormalizedDestinationAliases(t *testing.T) {
	root := t.TempDir()
	planner := newTestPlanner(t, root)
	_, err := planner.Plan(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{
		{Type: OperationCreateFile, Path: filepath.Join(root, "e\u0301.txt"), Content: []byte("first")},
		{Type: OperationCreateFile, Path: filepath.Join(root, "é.txt"), Content: []byte("second")},
	}})
	if operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("normalized alias error = %v, want INVALID_INPUT", err)
	}
}

func TestEngineRejectsSourceParentReplacementWithSameContent(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "source-parent")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(parent, "source.txt")
	if err := os.WriteFile(source, []byte("same-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "destination.txt")
	engine := newTestEngine(t, root, testEngineLimits(), nil)
	preview, err := engine.Preview(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{
		Type: OperationCopyFile, Source: source, Destination: destination,
	}}})
	if err != nil {
		t.Fatal(err)
	}

	oldParent := filepath.Join(root, "old-source-parent")
	if err := os.Rename(parent, oldParent); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("same-content"), 0o600); err != nil {
		t.Fatal(err)
	}

	output, err := engine.Apply(context.Background(), preview.PreviewID)
	if operation.KindOf(err) != operation.KindConflict || output.PartialCommit {
		t.Fatalf("parent replacement apply = %#v / %v", output, err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination changed after parent replacement: %v", err)
	}
}

func TestEngineRejectsAuthorizationPolicyChangeAfterPreview(t *testing.T) {
	root := t.TempDir()
	set, err := security.NormalizeAllowedDirectorySet([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	allowed := true
	limits := testEngineLimits()
	planner, err := NewPlanner(limits, func(path string) (security.PathEvidence, error) {
		if !allowed {
			return security.PathEvidence{}, operation.New(operation.KindAccessDenied, "authorization policy changed")
		}
		return security.ValidatePathEvidenceWithAllowedDirectories(path, set.Requested, set.Resolved)
	}, func() []string { return append([]string(nil), set.Resolved...) })
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(planner, limits, nil)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	preview, err := engine.Preview(context.Background(), mkdirManifest(target))
	if err != nil {
		t.Fatal(err)
	}
	allowed = false
	output, err := engine.Apply(context.Background(), preview.PreviewID)
	if operation.KindOf(err) != operation.KindAccessDenied || output.PartialCommit {
		t.Fatalf("policy-change apply = %#v / %v", output, err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target changed after policy denial: %v", err)
	}
}

func TestEngineRejectsSameContentRecursiveDescendantReplacement(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "tree")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(target, "file.txt")
	if err := os.WriteFile(file, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	engine := newTestEngine(t, root, testEngineLimits(), captureCurrentFiles)
	preview, err := engine.Preview(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{
		Type: OperationDeleteDirectory, Path: target,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(root, "replacement.txt")
	if err := os.WriteFile(replacement, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, file); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}

	output, err := engine.Apply(context.Background(), preview.PreviewID)
	if operation.KindOf(err) != operation.KindConflict || output.PartialCommit {
		t.Fatalf("same-content descendant replacement apply = %#v / %v", output, err)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "same" {
		t.Fatalf("replacement descendant changed: %q / %v", got, err)
	}
}

func TestEngineCancellationDuringBackupPreventsDeletion(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	engine := newTestEngine(t, root, testEngineLimits(), func(ctx context.Context, _ []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error) {
		cancel()
		return nil, ctx.Err()
	})
	preview, err := engine.Preview(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{Type: OperationDeleteFile, Path: target}}})
	if err != nil {
		t.Fatal(err)
	}
	output, err := engine.Apply(ctx, preview.PreviewID)
	if operation.KindOf(err) != operation.KindCancelled || output.PartialCommit {
		t.Fatalf("backup cancellation apply = %#v / %v", output, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "keep" {
		t.Fatalf("target changed after backup cancellation: %q / %v", got, err)
	}
}

func TestEngineBackupCaptureFailurePreventsDeletion(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	engine := newTestEngine(t, root, testEngineLimits(), func(context.Context, []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error) {
		return nil, operation.New(operation.KindFilesystem, "injected backup capture failure")
	})
	preview, err := engine.Preview(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{Type: OperationDeleteFile, Path: target}}})
	if err != nil {
		t.Fatal(err)
	}
	output, err := engine.Apply(context.Background(), preview.PreviewID)
	if operation.KindOf(err) != operation.KindFilesystem || output.PartialCommit {
		t.Fatalf("backup failure apply = %#v / %v", output, err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("target changed after backup failure: %v", err)
	}
}

func TestEngineDestinationRaceAfterStagingNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	engine := newTestEngine(t, root, testEngineLimits(), nil)
	preview, err := engine.Preview(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{{
		Type: OperationCreateFile, Path: target, Content: []byte("prepared"),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	engine.commitHook = func(index int, phase string) error {
		if index == 0 && phase == "before" {
			return os.WriteFile(target, []byte("racer"), 0o600)
		}
		return nil
	}
	output, err := engine.Apply(context.Background(), preview.PreviewID)
	if operation.KindOf(err) != operation.KindPartialCommit || !output.PartialCommit {
		t.Fatalf("post-staging destination race = %#v / %v", output, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "racer" {
		t.Fatalf("racer destination overwritten: %q / %v", got, err)
	}
}

func TestEngineCancellationAfterFirstCommitReportsPartialState(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	engine := newTestEngine(t, root, testEngineLimits(), nil)
	preview, err := engine.Preview(context.Background(), Manifest{FormatVersion: FormatV1, Operations: []Operation{
		{Type: OperationMkdir, Path: filepath.Join(root, "first")},
		{Type: OperationMkdir, Path: filepath.Join(root, "second")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	engine.commitHook = func(index int, phase string) error {
		if index == 0 && phase == "after" {
			cancel()
		}
		return nil
	}
	output, err := engine.Apply(ctx, preview.PreviewID)
	if operation.KindOf(err) != operation.KindPartialCommit || !output.PartialCommit {
		t.Fatalf("post-commit cancellation = %#v / %v", output, err)
	}
	if output.Results[0].State != StateCommitted || output.Results[1].State != StateUnknown {
		t.Fatalf("post-commit cancellation states = %#v", output.Results)
	}
}
