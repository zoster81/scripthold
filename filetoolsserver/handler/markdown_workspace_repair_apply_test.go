package handler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareMarkdownWorkspaceRepairApplyTargetsRevalidatesSemanticAndPhysicalState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	source := "# Root\r\n\r\n## Contents\r\n\r\n- [Root](#old-root)\r\n- [Child](#child)\r\n\r\n## Child\r\nbody\r\n"
	original := encodeUTF16LEWithBOM(t, source)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	h := NewHandler([]string{root})
	repairs := markdownWorkspaceRepairFixture(t, h, root)
	prepared, failure := h.prepareMarkdownWorkspaceRepairPreview(context.Background(), root, repairs, "")
	if failure != nil {
		t.Fatalf("prepare failure=%+v", failure)
	}
	defer prepared.close()

	plans, failure := h.prepareMarkdownWorkspaceRepairApplyTargets(context.Background(), &prepared)
	if failure != nil {
		t.Fatalf("apply revalidation failure=%+v", failure)
	}
	if len(plans) != 1 {
		t.Fatalf("plans=%d, want 1", len(plans))
	}
	plan := plans[0]
	target := &prepared.targets[0]
	if plan.document.Charset != target.encoding || plan.document.BOM.HasBOM != target.hasBOM || plan.document.BOM.Type != target.bomType {
		t.Fatalf("physical metadata changed: document=%+v target=%+v", plan.document, target)
	}
	if !bytes.Equal(plan.replacement.resultData, target.resultData) ||
		plan.replacement.targetFingerprint != target.targetFingerprint ||
		plan.replacement.resultFingerprint != target.resultFingerprint ||
		plan.replacement.identity() != target.identityFile {
		t.Fatalf("replacement binding diverged: %+v", plan.replacement)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("apply revalidation mutated source: bytes=%x err=%v", got, err)
	}
}

func TestPrepareMarkdownWorkspaceRepairApplyTargetsRejectsStaleSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	source := "# Root\n\n## Contents\n\n- [Root](#old-root)\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	repairs := markdownWorkspaceRepairFixture(t, h, root)
	prepared, failure := h.prepareMarkdownWorkspaceRepairPreview(context.Background(), root, repairs, "")
	if failure != nil {
		t.Fatalf("prepare failure=%+v", failure)
	}
	defer prepared.close()

	external := []byte(source + "\nexternal\n")
	if err := os.WriteFile(path, external, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, failure := h.prepareMarkdownWorkspaceRepairApplyTargets(context.Background(), &prepared); failure == nil || !failure.IsError {
		t.Fatalf("stale source unexpectedly revalidated: %+v", failure)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, external) {
		t.Fatalf("stale-source check mutated file: bytes=%q err=%v", got, err)
	}
}

func TestPrepareMarkdownWorkspaceRepairApplyTargetsRejectsTamperedPreparedResult(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	if err := os.WriteFile(path, []byte("# Root\n\n## Contents\n\n- [Root](#old-root)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	repairs := markdownWorkspaceRepairFixture(t, h, root)
	prepared, failure := h.prepareMarkdownWorkspaceRepairPreview(context.Background(), root, repairs, "")
	if failure != nil {
		t.Fatalf("prepare failure=%+v", failure)
	}
	defer prepared.close()

	prepared.targets[0].resultData = append(prepared.targets[0].resultData, []byte("tampered")...)
	if _, failure := h.prepareMarkdownWorkspaceRepairApplyTargets(context.Background(), &prepared); failure == nil || !failure.IsError {
		t.Fatalf("tampered result unexpectedly revalidated: %+v", failure)
	}
}
