package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMarkdownWorkspaceInspectScanMixedEncoding(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.md"), encodeUTF16LEWithBOM(t, "# A\r\n\r\n[B](b.md)\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.md"), []byte("# B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	result, output, err := h.HandleMarkdownWorkspace(context.Background(), nil, MarkdownWorkspaceInput{
		Action: "inspect", Root: root, Discovery: MarkdownWorkspaceDiscovery{Mode: "scan"}, Limit: 1,
	})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("inspect result=%+v output=%+v err=%v", result, output, err)
	}
	if output.TotalDocuments != 2 || len(output.Documents) != 1 || output.Documents[0] != "a.md" || !output.Truncated {
		t.Fatalf("inspect output=%+v", output)
	}
}

func TestMarkdownWorkspaceFollowReachableAndValidate(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("# A\n\n[B](b.md)\n[Missing](missing.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.md"), []byte("# B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unrelated.md"), []byte("# U\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	result, output, err := h.HandleMarkdownWorkspace(context.Background(), nil, MarkdownWorkspaceInput{
		Action: "query", Root: root, Discovery: MarkdownWorkspaceDiscovery{Mode: "follow", Entries: []string{"a.md"}},
		Query: "reachable", Document: "a.md", Limit: 10,
	})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("query result=%+v output=%+v err=%v", result, output, err)
	}
	if len(output.Documents) != 1 || output.Documents[0] != "b.md" {
		t.Fatalf("reachable=%+v", output.Documents)
	}

	result, output, err = h.HandleMarkdownWorkspace(context.Background(), nil, MarkdownWorkspaceInput{
		Action: "validate", Root: root, Discovery: MarkdownWorkspaceDiscovery{Mode: "scan"}, Limit: 10,
	})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("validate result=%+v output=%+v err=%v", result, output, err)
	}
	found := false
	for _, diagnostic := range output.Diagnostics {
		if diagnostic.Kind == "missing_document" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics=%+v, want missing_document", output.Diagnostics)
	}
}

func TestMarkdownWorkspaceValidateProjectsManagedTOCRepairPlan(t *testing.T) {
	root := t.TempDir()
	source := "# Root\n\n## Contents\n\n- [Root](#old-root)\n- [Child](#child)\n\n## Child\nbody\n"
	if err := os.WriteFile(filepath.Join(root, "doc.md"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	result, output, err := h.HandleMarkdownWorkspace(context.Background(), nil, MarkdownWorkspaceInput{
		Action: "validate", Root: root, Discovery: MarkdownWorkspaceDiscovery{Mode: "scan"}, Limit: 10,
		ManagedTOCs: []MarkdownWorkspaceManagedTOC{{Document: "doc.md", Fragment: "#contents"}},
	})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("validate result=%+v output=%+v err=%v", result, output, err)
	}
	if output.TotalRepairs != 1 || len(output.RepairDocuments) != 1 || output.RepairDocuments[0] != "doc.md" {
		t.Fatalf("repair plan=%+v", output)
	}
	found := false
	for _, diagnostic := range output.Diagnostics {
		if diagnostic.Kind == "stale_generated_index" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics=%+v, want stale_generated_index", output.Diagnostics)
	}
}

func TestMarkdownWorkspaceRepairPreviewAppliesThroughMarkdownApply(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	source := "# Root\n\n## Contents\n\n- [Root](#old-root)\n- [Child](#child)\n\n## Child\nbody\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	result, output, err := h.HandleMarkdownWorkspace(context.Background(), nil, MarkdownWorkspaceInput{
		Action: "repairPreview", Root: root, Discovery: MarkdownWorkspaceDiscovery{Mode: "scan"}, Limit: 10,
		ManagedTOCs: []MarkdownWorkspaceManagedTOC{{Document: "doc.md", Fragment: "#contents"}},
	})
	if err != nil || result == nil || result.IsError || output.RepairPreview == nil {
		t.Fatalf("repair preview result=%+v output=%+v err=%v", result, output, err)
	}
	preview := output.RepairPreview
	if len(preview.PreviewID) != 64 || len(preview.Targets) != 1 || preview.Targets[0].Document != "doc.md" ||
		!preview.Targets[0].Changed || preview.Targets[0].Diff == "" ||
		len(preview.Targets[0].TargetFingerprint) != 64 || len(preview.Targets[0].ResultFingerprint) != 64 {
		t.Fatalf("repair preview=%+v", preview)
	}
	if got, readErr := os.ReadFile(path); readErr != nil || string(got) != source {
		t.Fatalf("repair preview mutated target: %q err=%v", got, readErr)
	}
	applyResult, applied, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult == nil || applyResult.IsError || applied.Workspace == nil {
		t.Fatalf("apply result=%+v output=%+v err=%v", applyResult, applied, err)
	}
	if applied.Workspace.CommittedCount != 1 || len(applied.Workspace.Results) != 1 ||
		applied.Workspace.Results[0].ActualFingerprint != preview.Targets[0].ResultFingerprint {
		t.Fatalf("apply output=%+v", applied.Workspace)
	}
}

func TestMarkdownWorkspaceRepairPreviewReturnsNoTokenWhenNothingIsStale(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "doc.md"), []byte("# Root\n\n## Contents\n\n- [Root](#old-root)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	input := MarkdownWorkspaceInput{
		Action: "repairPreview", Root: root, Discovery: MarkdownWorkspaceDiscovery{Mode: "scan"}, Limit: 10,
		ManagedTOCs: []MarkdownWorkspaceManagedTOC{{Document: "doc.md", Fragment: "#contents"}},
	}
	firstResult, first, err := h.HandleMarkdownWorkspace(context.Background(), nil, input)
	if err != nil || firstResult == nil || firstResult.IsError || first.RepairPreview == nil {
		t.Fatalf("initial repair preview result=%+v output=%+v err=%v", firstResult, first, err)
	}
	applyResult, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: first.RepairPreview.PreviewID})
	if err != nil || applyResult == nil || applyResult.IsError {
		t.Fatalf("initial repair apply result=%+v err=%v", applyResult, err)
	}
	result, output, err := h.HandleMarkdownWorkspace(context.Background(), nil, input)
	if err != nil || result == nil || result.IsError {
		t.Fatalf("second repair preview result=%+v output=%+v err=%v", result, output, err)
	}
	if output.TotalRepairs != 0 || output.RepairPreview != nil {
		t.Fatalf("zero-repair preview=%+v", output)
	}
}

func TestMarkdownWorkspaceRepairPreviewRejectsPlanAboveLimit(t *testing.T) {
	root := t.TempDir()
	source := []byte("# Root\n\n## Contents\n\n- [Root](#old-root)\n")
	for _, name := range []string{"a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(root, name), source, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler([]string{root})
	result, _, err := h.HandleMarkdownWorkspace(context.Background(), nil, MarkdownWorkspaceInput{
		Action: "repairPreview", Root: root, Discovery: MarkdownWorkspaceDiscovery{Mode: "scan"}, Limit: 1,
		ManagedTOCs: []MarkdownWorkspaceManagedTOC{
			{Document: "a.md", Fragment: "#contents"},
			{Document: "b.md", Fragment: "#contents"},
		},
	})
	if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("repair preview limit result=%+v err=%v", result, err)
	}
}

func TestMarkdownWorkspaceReportsMarkspliceBudgetExhaustion(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("# A\n\n[B](b.md)\n[C](c.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	one := 1
	h := NewHandler([]string{root})
	result, _, err := h.HandleMarkdownWorkspace(context.Background(), nil, MarkdownWorkspaceInput{
		Action: "inspect", Root: root, Discovery: MarkdownWorkspaceDiscovery{Mode: "scan"}, Limit: 10, MaxRelationships: &one,
	})
	if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit || result.Meta[MarkdownErrorCodeMetaKey] != MarkdownErrWorkspaceBudgetExceeded {
		t.Fatalf("budget exhaustion result=%+v err=%v", result, err)
	}
}

func TestMarkdownWorkspaceRejectsDocumentBudgetAboveConfiguredCeiling(t *testing.T) {
	root := t.TempDir()
	h := NewHandler([]string{root})
	tooMany := h.maxMarkdownWorkspaceDocuments() + 1
	result, _, err := h.HandleMarkdownWorkspace(context.Background(), nil, MarkdownWorkspaceInput{
		Action: "inspect", Root: root, Discovery: MarkdownWorkspaceDiscovery{Mode: "scan"}, Limit: 1, MaxDocuments: &tooMany,
	})
	if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("budget result=%+v err=%v", result, err)
	}
}
