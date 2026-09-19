package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zoster81/marksplice"
	"github.com/zoster81/marksplice/workspacefs"
)

func TestPrepareMarkdownWorkspaceRepairPreviewPreservesPhysicalEncodingAndOwnership(t *testing.T) {
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
	if len(prepared.targets) != 1 {
		t.Fatalf("targets=%d, want 1", len(prepared.targets))
	}
	target := &prepared.targets[0]
	t.Cleanup(func() { target.close() })
	if target.documentKey != "doc.md" || target.encoding != "utf-16-le" || !target.hasBOM || !target.changed {
		t.Fatalf("target=%+v", target)
	}
	if target.identityFile == nil {
		t.Fatal("prepared repair did not retain file identity")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("preview mutated source: bytes=%x err=%v", got, err)
	}

	preview, err := h.markdownPreviews.putWorkspaceRepair(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if preview.kind != markdownPreviewWorkspaceRepair || preview.workspaceRepair == nil || len(preview.workspaceRepair.targets) != 1 {
		t.Fatalf("preview=%+v", preview)
	}
	identity := target.identityFile
	h.markdownPreviews.discard(preview.id)
	if _, err := identity.Matches(path); err == nil {
		t.Fatal("discarded workspace repair preview retained target identity")
	}
}

func TestPrepareMarkdownWorkspaceRepairPreviewRetainsMultipleTargetsInPlanOrder(t *testing.T) {
	root := t.TempDir()
	source := "# Root\n\n## Contents\n\n- [Root](#old-root)\n"
	for _, name := range []string{"a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler([]string{root})
	adapter, err := newMarkdownWorkspaceFS(context.Background(), h, root, "")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := workspacefs.Scan(adapter, ".", workspacefs.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	documents := workspace.Documents()
	managed := make([]marksplice.ManagedTOC, 0, len(documents))
	for _, item := range documents {
		target, ok := item.Document.ResolveFragment("#contents")
		if !ok {
			t.Fatalf("%s managed TOC did not resolve", item.Key)
		}
		managed = append(managed, marksplice.ManagedTOC{Document: item.Key, HeadingID: target.NodeID()})
	}
	report, err := workspace.Validate(marksplice.WorkspaceValidationOptions{ManagedTOCs: managed})
	if err != nil {
		t.Fatal(err)
	}
	repairs := report.RepairPlan().Repairs()
	prepared, failure := h.prepareMarkdownWorkspaceRepairPreview(context.Background(), root, repairs, "")
	if failure != nil {
		t.Fatalf("prepare failure=%+v", failure)
	}
	defer prepared.close()
	if len(prepared.targets) != 2 || prepared.targets[0].documentKey != "a.md" || prepared.targets[1].documentKey != "b.md" {
		t.Fatalf("prepared targets=%+v", prepared.targets)
	}
	for index := range prepared.targets {
		if !prepared.targets[index].changed || prepared.targets[index].identityFile == nil {
			t.Fatalf("target %d=%+v", index, prepared.targets[index])
		}
	}
}

func TestPrepareMarkdownWorkspaceRepairPreviewRejectsChangedWorkspaceSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	source := "# Root\n\n## Contents\n\n- [Root](#old-root)\n\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	repairs := markdownWorkspaceRepairFixture(t, h, root)
	if err := os.WriteFile(path, []byte(source+"external\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	prepared, failure := h.prepareMarkdownWorkspaceRepairPreview(context.Background(), root, repairs, "")
	if failure == nil || !failure.IsError {
		for index := range prepared.targets {
			prepared.targets[index].close()
		}
		t.Fatalf("changed source unexpectedly prepared: %+v", prepared)
	}
}

func markdownWorkspaceRepairFixture(t *testing.T, h *Handler, root string) []marksplice.WorkspaceRepair {
	t.Helper()
	adapter, err := newMarkdownWorkspaceFS(context.Background(), h, root, "")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := workspacefs.Scan(adapter, ".", workspacefs.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	documents := workspace.Documents()
	if len(documents) != 1 {
		t.Fatalf("documents=%d, want 1", len(documents))
	}
	target, ok := documents[0].Document.ResolveFragment("#contents")
	if !ok || target.Kind() != marksplice.FragmentTargetHeading {
		t.Fatal("managed TOC heading did not resolve")
	}
	report, err := workspace.Validate(marksplice.WorkspaceValidationOptions{
		ManagedTOCs: []marksplice.ManagedTOC{{Document: documents[0].Key, HeadingID: target.NodeID()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	repairs := report.RepairPlan().Repairs()
	if len(repairs) != 1 {
		t.Fatalf("repairs=%d, want 1", len(repairs))
	}
	return repairs
}

func TestMarkdownWorkspaceRepairPreviewStoreExpiresOwnedIdentities(t *testing.T) {
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
	identity := prepared.targets[0].identityFile
	store := newMarkdownPreviewStore(2, 1<<20, time.Second)
	now := time.Date(2026, 9, 19, 20, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	preview, err := store.putWorkspaceRepair(prepared)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	if _, err := store.claim(preview.id); err == nil {
		t.Fatal("expired workspace repair preview remained claimable")
	}
	if _, err := identity.Matches(path); err == nil {
		t.Fatal("expired workspace repair preview retained target identity")
	}
}
