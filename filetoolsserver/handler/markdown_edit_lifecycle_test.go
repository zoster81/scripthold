package handler

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/markdownintelligence"
)

func TestMarkdownEditPreviewApplyLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Old\r\n\r\nBody.\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandler([]string{dir})
	if h.markdownEditPreviews == nil {
		t.Fatal("markdown edit preview store is not initialized")
	}
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"heading"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 1 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}

	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
		Path:       path,
		Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: read.Nodes[0].TargetID, Text: "New"}},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if !validMarkdownEditPreviewID(preview.PreviewID) || !preview.Changed || preview.TargetFingerprint == preview.ResultFingerprint {
		t.Fatalf("preview=%+v", preview)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("preview mutated target: %q err=%v", got, err)
	}

	applyResult, applied, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError {
		t.Fatalf("apply=%+v result=%+v err=%v", applied, applyResult, err)
	}
	if !applied.Applied || applied.State != editApplyStateCommitted || applied.ActualFingerprint != preview.ResultFingerprint {
		t.Fatalf("apply=%+v", applied)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "# New\r\n\r\nBody.\n" {
		t.Fatalf("applied bytes=%q err=%v", got, err)
	}

	replayResult, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || replayResult == nil || !replayResult.IsError {
		t.Fatalf("replay result=%+v err=%v", replayResult, err)
	}
}

func TestMarkdownEditComposesIndependentHeadingRenames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# One\r\n\r\n## Two\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"heading"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 2 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operations := []MarkdownEditOperation{
		{Action: "rename", Subject: "heading", TargetID: read.Nodes[0].TargetID, Text: "First"},
		{Action: "rename", Subject: "heading", TargetID: read.Nodes[1].TargetID, Text: "Second"},
	}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: operations})
	if err != nil || previewResult.IsError || !preview.Changed || len(preview.Operations) != 2 {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("multi-edit preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "# First\r\n\r\n## Second\n" {
		t.Fatalf("multi-edit target=%q err=%v", got, err)
	}
}

func TestMarkdownEditComposesRenameAndHeadingLevelChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# One\r\n\r\n## Two\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"heading"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 2 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operations := []MarkdownEditOperation{
		{Action: "rename", Subject: "heading", TargetID: read.Nodes[0].TargetID, Text: "First"},
		{Action: "set", Subject: "heading", TargetID: read.Nodes[1].TargetID, Level: 3},
	}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: operations})
	if err != nil || previewResult.IsError || !preview.Changed || len(preview.Operations) != 2 {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("mixed preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "# First\r\n\r\n### Two\n" {
		t.Fatalf("mixed target=%q err=%v", got, err)
	}
}

func TestMarkdownEditReplacesListItemContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("1. parent old\n   - child\n2. tail\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"list_item"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 3 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operation := MarkdownEditOperation{Action: "replace", Subject: "list_item", TargetID: read.Nodes[0].TargetID, Markdown: "parent **new**"}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("list item preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	want := "1. parent **new**\n   - child\n2. tail\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("list item target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditReplacesListItemSubtree(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("- outer\n  - target\n    - old child\n  - tail\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"list_item"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 4 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operation := MarkdownEditOperation{Action: "replace", Subject: "list_item", TargetID: read.Nodes[1].TargetID, Part: "subtree", Markdown: "  - replaced\n    + new child\n"}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("list item subtree preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	want := "- outer\n  - replaced\n    + new child\n  - tail\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("list item subtree target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditRejectsInvalidListItemReplaceShapeBeforeFilesystemWork(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	base := MarkdownEditOperation{Action: "replace", Subject: "list_item", TargetID: strings.Repeat("a", 64), Markdown: "new content"}
	cases := []MarkdownEditOperation{
		func() MarkdownEditOperation { op := base; op.Text = "extra"; return op }(),
		func() MarkdownEditOperation { op := base; op.Position = "after"; return op }(),
		func() MarkdownEditOperation { op := base; op.Part = "body"; return op }(),
		func() MarkdownEditOperation { op := base; op.AnchorTargetID = strings.Repeat("b", 64); return op }(),
		func() MarkdownEditOperation { op := base; op.Part = "subtree"; op.Text = "extra"; return op }(),
	}
	for _, op := range cases {
		result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: filepath.Join(t.TempDir(), "missing.md"), Operations: []MarkdownEditOperation{op}})
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid list item replace shape result=%+v err=%v op=%+v", result, err, op)
		}
	}
}

func TestMarkdownEditRemovesListItemSubtree(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("- root\n  - remove\n    - child\n  - keep\n- tail\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"list_item"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 5 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operation := MarkdownEditOperation{Action: "remove", Subject: "list_item", TargetID: read.Nodes[1].TargetID}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("list item remove preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	want := "- root\n  - keep\n- tail\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("list item remove target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditRejectsInvalidListItemRemoveShapeBeforeFilesystemWork(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	base := MarkdownEditOperation{Action: "remove", Subject: "list_item", TargetID: strings.Repeat("a", 64)}
	cases := []MarkdownEditOperation{
		func() MarkdownEditOperation { op := base; op.Text = "extra"; return op }(),
		func() MarkdownEditOperation { op := base; op.Markdown = "extra"; return op }(),
		func() MarkdownEditOperation { op := base; op.Position = "after"; return op }(),
		func() MarkdownEditOperation { op := base; op.Part = "subtree"; return op }(),
		func() MarkdownEditOperation { op := base; op.Level = 2; return op }(),
	}
	for _, op := range cases {
		result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: filepath.Join(t.TempDir(), "missing.md"), Operations: []MarkdownEditOperation{op}})
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid list item remove shape result=%+v err=%v op=%+v", result, err, op)
		}
	}
}

func TestMarkdownEditInsertsListItemSibling(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("- keep\n- tail\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"list_item"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 2 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operation := MarkdownEditOperation{Action: "insert", Subject: "list_item", TargetID: read.Nodes[1].TargetID, Position: "before", Markdown: "- [ ] inserted task\n"}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("list item insert preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	want := "- keep\n- [ ] inserted task\n- tail\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("list item insert target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditRejectsInvalidListItemInsertShapeBeforeFilesystemWork(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	base := MarkdownEditOperation{Action: "insert", Subject: "list_item", TargetID: strings.Repeat("a", 64), Position: "before", Markdown: "- inserted\n"}
	cases := []MarkdownEditOperation{
		func() MarkdownEditOperation { op := base; op.Position = "middle"; return op }(),
		func() MarkdownEditOperation { op := base; op.Text = "extra"; return op }(),
		func() MarkdownEditOperation { op := base; op.Part = "subtree"; return op }(),
		func() MarkdownEditOperation { op := base; op.Level = 2; return op }(),
		func() MarkdownEditOperation { op := base; op.AnchorTargetID = strings.Repeat("b", 64); return op }(),
	}
	for _, op := range cases {
		result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: filepath.Join(t.TempDir(), "missing.md"), Operations: []MarkdownEditOperation{op}})
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid list item insert shape result=%+v err=%v op=%+v", result, err, op)
		}
	}
}

func TestMarkdownEditSetsTaskCheckedState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("- [ ] todo\n- [X] done\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"task"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 2 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	checked := true
	operation := MarkdownEditOperation{Action: "set", Subject: "task", TargetID: read.Nodes[0].TargetID, Checked: &checked}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("task preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	want := "- [x] todo\n- [X] done\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("task target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditRejectsInvalidTaskSetShapeBeforeFilesystemWork(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	checked := false
	base := MarkdownEditOperation{Action: "set", Subject: "task", TargetID: strings.Repeat("a", 64), Checked: &checked}
	cases := []MarkdownEditOperation{
		func() MarkdownEditOperation { op := base; op.Checked = nil; return op }(),
		func() MarkdownEditOperation { op := base; op.Text = "extra"; return op }(),
		func() MarkdownEditOperation { op := base; op.Level = 1; return op }(),
		func() MarkdownEditOperation { op := base; op.Markdown = "extra"; return op }(),
		func() MarkdownEditOperation { op := base; op.Position = "after"; return op }(),
		func() MarkdownEditOperation { op := base; op.Part = "subtree"; return op }(),
		func() MarkdownEditOperation { op := base; op.AnchorTargetID = strings.Repeat("b", 64); return op }(),
	}
	for _, op := range cases {
		result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: filepath.Join(t.TempDir(), "missing.md"), Operations: []MarkdownEditOperation{op}})
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid task set shape result=%+v err=%v op=%+v", result, err, op)
		}
	}
}

func TestMarkdownEditReplacesCodeSpanContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("Use ``old`code`` here.\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"code_span"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 1 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operation := MarkdownEditOperation{Action: "replace", Subject: "code_span", TargetID: read.Nodes[0].TargetID, Text: "new`code"}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("code-span preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	want := "Use ``new`code`` here.\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("code-span target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditReplacesSimpleInlineContent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		kind   string
		text   string
		want   string
	}{
		{name: "strikethrough", source: "before ~~old~~ after\n", kind: "strikethrough", text: "new", want: "before ~~new~~ after\n"},
		{name: "code span", source: "before `old` after\n", kind: "code_span", text: "new", want: "before `new` after\n"},
		{name: "emphasis", source: "before _old_ after\n", kind: "emphasis", text: "new", want: "before _new_ after\n"},
		{name: "strong", source: "before **old** after\n", kind: "strong", text: "new", want: "before **new** after\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "doc.md")
			if err := os.WriteFile(path, []byte(tt.source), 0o644); err != nil {
				t.Fatal(err)
			}
			h := NewHandler([]string{dir})
			result, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{tt.kind}, Limit: 8})
			if err != nil || result.IsError || len(read.Nodes) != 1 {
				t.Fatalf("read=%+v result=%+v err=%v", read, result, err)
			}
			operation := MarkdownEditOperation{Action: "replace", Subject: tt.kind, TargetID: read.Nodes[0].TargetID, Text: tt.text}
			previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
			if err != nil || previewResult.IsError || !preview.Changed {
				t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
			}
			applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
			if err != nil || applyResult.IsError || !output.Applied {
				t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != tt.want {
				t.Fatalf("target=%q want=%q err=%v", got, tt.want, err)
			}
		})
	}
}

func TestMarkdownEditReplacesDirectLinkFamily(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		subject string
		part    string
		text    string
		want    string
	}{
		{name: "inline link destination", source: "before [label](<old/path> \"title\") after\n", subject: "inline_link", part: "destination", text: "new/path", want: "before [label](<new/path> \"title\") after\n"},
		{name: "inline link label", source: "before [old](path) after\n", subject: "inline_link", part: "label", text: "new", want: "before [new](path) after\n"},
		{name: "image destination", source: "before ![alt](<old path> 'title') after\n", subject: "image", part: "destination", text: "new path", want: "before ![alt](<new path> 'title') after\n"},
		{name: "image alt", source: "before ![old](path) after\n", subject: "image", part: "alt", text: "new", want: "before ![new](path) after\n"},
		{name: "autolink", source: "before <https://old.example/path> after\n", subject: "autolink", text: "https://new.example/path", want: "before <https://new.example/path> after\n"},
		{name: "html comment", source: "before <!--  old comment  --> after\r\n", subject: "html_comment", text: "new comment", want: "before <!--  new comment  --> after\r\n"},
		{name: "html anchor", source: "before <A class='x' ID=\"old-anchor\">text</A> after\n", subject: "html_anchor", text: "new-anchor", want: "before <A class='x' ID=\"new-anchor\">text</A> after\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "doc.md")
			if err := os.WriteFile(path, []byte(tt.source), 0o644); err != nil {
				t.Fatal(err)
			}
			h := NewHandler([]string{dir})
			result, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{tt.subject}, Limit: 4})
			if err != nil || result.IsError || len(read.Nodes) != 1 {
				t.Fatalf("read=%+v result=%+v err=%v", read, result, err)
			}
			operation := MarkdownEditOperation{Action: "replace", Subject: tt.subject, TargetID: read.Nodes[0].TargetID, Part: tt.part, Text: tt.text}
			previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
			if err != nil || previewResult.IsError || !preview.Changed {
				t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
			}
			applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
			if err != nil || applyResult.IsError || !output.Applied {
				t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != tt.want {
				t.Fatalf("target=%q want=%q err=%v", got, tt.want, err)
			}
		})
	}
}

func TestMarkdownEditFencedCodeMutations(t *testing.T) {
	tests := []struct {
		name   string
		source string
		action string
		part   string
		text   string
		want   string
	}{
		{name: "replace body", source: "```` go extra\nline one\nline two\n  `````  \n", action: "replace", part: "body", text: "new one\nnew two", want: "```` go extra\nnew one\nnew two\n  `````  \n"},
		{name: "populate empty body", source: "```math\n```\n", action: "replace", part: "body", text: "x + y", want: "```math\nx + y\n```\n"},
		{name: "set info", source: "  ~~~~  go old  \nbody\n ~~~~~~   \n", action: "set", part: "info", text: "typescript module", want: "  ~~~~  typescript module  \nbody\n ~~~~~~   \n"},
		{name: "clear info", source: "```  go extra  \r\nbody\r\n```\r\n", action: "set", part: "info", text: "", want: "```    \r\nbody\r\n```\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "doc.md")
			if err := os.WriteFile(path, []byte(tt.source), 0o644); err != nil {
				t.Fatal(err)
			}
			h := NewHandler([]string{dir})
			result, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "inspect", Path: path, Limit: 4})
			if err != nil || result.IsError || read.Inspect == nil || len(read.Inspect.FencedBlocks) != 1 {
				t.Fatalf("read=%+v result=%+v err=%v", read, result, err)
			}
			operation := MarkdownEditOperation{Action: tt.action, Subject: "fenced_code", TargetID: read.Inspect.FencedBlocks[0].TargetID, Part: tt.part, Text: tt.text}
			previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
			if err != nil || previewResult.IsError || !preview.Changed {
				t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
			}
			applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
			if err != nil || applyResult.IsError || !output.Applied {
				t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != tt.want {
				t.Fatalf("target=%q want=%q err=%v", got, tt.want, err)
			}
		})
	}
}

func TestMarkdownEditRejectsInvalidFencedCodeShapesBeforeFilesystemWork(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	targetID := strings.Repeat("a", 64)
	cases := []MarkdownEditOperation{
		{Action: "replace", Subject: "fenced_code", TargetID: targetID, Part: "body"},
		{Action: "replace", Subject: "fenced_code", TargetID: targetID, Part: "info", Text: "body"},
		{Action: "set", Subject: "fenced_code", TargetID: targetID, Part: "body", Text: "go"},
		{Action: "set", Subject: "fenced_code", TargetID: targetID, Part: "info", Markdown: "extra"},
	}
	for _, operation := range cases {
		result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
			Path:       filepath.Join(t.TempDir(), "missing.md"),
			Operations: []MarkdownEditOperation{operation},
		})
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid fenced-code shape result=%+v err=%v operation=%+v", result, err, operation)
		}
	}
}

func TestMarkdownEditReplacesFrontMatterField(t *testing.T) {
	source := []byte("---\r\ntitle: \"Old\"\r\nkeep: yes\r\n---\r\n\r\nBody.\r\n")
	want := "---\r\ntitle: \"New\"\r\nkeep: yes\r\n---\r\n\r\nBody.\r\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, source, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	result, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{
		Action: "query", Path: path, Query: "nodes", Kinds: []string{"front_matter_field"}, Limit: 4,
	})
	if err != nil || result.IsError || len(read.Nodes) != 2 {
		t.Fatalf("read=%+v result=%+v err=%v", read, result, err)
	}
	var targetID string
	for _, node := range read.Nodes {
		if node.Attributes["key"] == "title" {
			targetID = node.TargetID
			break
		}
	}
	if targetID == "" {
		t.Fatalf("title front matter field not found: %+v", read.Nodes)
	}
	operation := MarkdownEditOperation{Action: "replace", Subject: "front_matter_field", TargetID: targetID, Text: "New"}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, source) {
		t.Fatalf("preview mutated target=%q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditRejectsInvalidFrontMatterFieldReplaceShapesBeforeFilesystemWork(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	targetID := strings.Repeat("a", 64)
	cases := []MarkdownEditOperation{
		{Action: "replace", Subject: "front_matter_field", TargetID: targetID},
		{Action: "replace", Subject: "front_matter_field", TargetID: targetID, Text: "new", Part: "value"},
		{Action: "replace", Subject: "front_matter_field", TargetID: targetID, Text: "new", Markdown: "extra"},
	}
	for _, operation := range cases {
		result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
			Path: filepath.Join(t.TempDir(), "missing.md"), Operations: []MarkdownEditOperation{operation},
		})
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid front matter field replace result=%+v err=%v operation=%+v", result, err, operation)
		}
	}
}

func TestMarkdownEditRenameReferenceDefinition(t *testing.T) {
	source := "[one]: <dest> \"Title\"\r\n\r\n[visible][one] [one][] [one] ![alt][one]\r\n"
	want := "[renamed]: <dest> \"Title\"\r\n\r\n[visible][renamed] [one][renamed] [one][renamed] ![alt][renamed]\r\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	result, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{
		Action: "query", Path: path, Query: "nodes", Kinds: []string{"reference_definition"}, Limit: 4,
	})
	if err != nil || result.IsError || len(read.Nodes) != 1 {
		t.Fatalf("read=%+v result=%+v err=%v", read, result, err)
	}
	operation := MarkdownEditOperation{Action: "rename", Subject: "reference_definition", TargetID: read.Nodes[0].TargetID, Text: "renamed"}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditRejectsInvalidReferenceDefinitionRenameShapesBeforeFilesystemWork(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	targetID := strings.Repeat("a", 64)
	cases := []MarkdownEditOperation{
		{Action: "rename", Subject: "reference_definition", TargetID: targetID},
		{Action: "rename", Subject: "reference_definition", TargetID: targetID, Text: "renamed", Part: "label"},
		{Action: "rename", Subject: "reference_definition", TargetID: targetID, Text: "renamed", Markdown: "extra"},
	}
	for _, operation := range cases {
		result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
			Path:       filepath.Join(t.TempDir(), "missing.md"),
			Operations: []MarkdownEditOperation{operation},
		})
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid reference-definition rename result=%+v err=%v operation=%+v", result, err, operation)
		}
	}
}

func TestMarkdownEditRenameFrontMatterField(t *testing.T) {
	source := "---\r\ntitle: \"Old\"\r\nkeep: yes\r\n---\r\n\r\nBody.\r\n"
	want := "---\r\nname: \"Old\"\r\nkeep: yes\r\n---\r\n\r\nBody.\r\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	result, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{
		Action: "query", Path: path, Query: "nodes", Kinds: []string{"front_matter_field"}, Limit: 8,
	})
	if err != nil || result.IsError {
		t.Fatalf("read=%+v result=%+v err=%v", read, result, err)
	}
	var titleTarget string
	for _, node := range read.Nodes {
		if node.Attributes["key"] == "title" {
			titleTarget = node.TargetID
			break
		}
	}
	if titleTarget == "" {
		t.Fatalf("title field not found: %+v", read.Nodes)
	}
	operation := MarkdownEditOperation{Action: "rename", Subject: "front_matter_field", TargetID: titleTarget, Text: "name"}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != source {
		t.Fatalf("preview mutated target=%q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditRejectsInvalidFrontMatterFieldRenameShapesBeforeFilesystemWork(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	targetID := strings.Repeat("a", 64)
	cases := []MarkdownEditOperation{
		{Action: "rename", Subject: "front_matter_field", TargetID: targetID},
		{Action: "rename", Subject: "front_matter_field", TargetID: targetID, Text: "name", Part: "key"},
		{Action: "rename", Subject: "front_matter_field", TargetID: targetID, Text: "name", Markdown: "extra"},
	}
	for _, operation := range cases {
		result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
			Path:       filepath.Join(t.TempDir(), "missing.md"),
			Operations: []MarkdownEditOperation{operation},
		})
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid front-matter rename result=%+v err=%v operation=%+v", result, err, operation)
		}
	}
}

func TestMarkdownEditRemoveFrontMatterField(t *testing.T) {
	source := "---\r\ntitle: \"Old\"\r\nauthor: \"Ada\"\r\n---\r\n\r\nBody.\r\n"
	want := "---\r\nauthor: \"Ada\"\r\n---\r\n\r\nBody.\r\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	result, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{
		Action: "query", Path: path, Query: "nodes", Kinds: []string{"front_matter_field"}, Limit: 8,
	})
	if err != nil || result.IsError {
		t.Fatalf("read=%+v result=%+v err=%v", read, result, err)
	}
	var titleTarget string
	for _, node := range read.Nodes {
		if node.Attributes["key"] == "title" {
			titleTarget = node.TargetID
			break
		}
	}
	if titleTarget == "" {
		t.Fatalf("title field not found: %+v", read.Nodes)
	}

	operation := MarkdownEditOperation{Action: "remove", Subject: "front_matter_field", TargetID: titleTarget}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
		Path: path, Operations: []MarkdownEditOperation{operation},
	})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != source {
		t.Fatalf("preview mutated target=%q err=%v", got, err)
	}

	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditRejectsInvalidFrontMatterFieldRemoveShapesBeforeFilesystemWork(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	targetID := strings.Repeat("a", 64)
	cases := []MarkdownEditOperation{
		{Action: "remove", Subject: "front_matter_field", TargetID: targetID, Text: "unexpected"},
		{Action: "remove", Subject: "front_matter_field", TargetID: targetID, Part: "field"},
		{Action: "remove", Subject: "front_matter_field", TargetID: targetID, Markdown: "extra"},
	}
	for _, operation := range cases {
		result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
			Path:       filepath.Join(t.TempDir(), "missing.md"),
			Operations: []MarkdownEditOperation{operation},
		})
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid front-matter remove result=%+v err=%v operation=%+v", result, err, operation)
		}
	}
}

func TestMarkdownEditRemoveReferenceDefinition(t *testing.T) {
	source := "before\r\n\r\n  [unused]: <target> \"Title\"   \r\n\r\nafter\r\n"
	want := "before\r\n\r\n\r\nafter\r\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	result, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{
		Action: "query", Path: path, Query: "nodes", Kinds: []string{"reference_definition"}, Limit: 4,
	})
	if err != nil || result.IsError || len(read.Nodes) != 1 {
		t.Fatalf("read=%+v result=%+v err=%v", read, result, err)
	}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
		Path: path,
		Operations: []MarkdownEditOperation{{
			Action: "remove", Subject: "reference_definition", TargetID: read.Nodes[0].TargetID,
		}},
	})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditRejectsUsedReferenceDefinitionRemoval(t *testing.T) {
	source := "[docs]: <target>\n\n[visible][docs]\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	result, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{
		Action: "query", Path: path, Query: "nodes", Kinds: []string{"reference_definition"}, Limit: 4,
	})
	if err != nil || result.IsError || len(read.Nodes) != 1 {
		t.Fatalf("read=%+v result=%+v err=%v", read, result, err)
	}
	result, _, err = h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
		Path: path,
		Operations: []MarkdownEditOperation{{
			Action: "remove", Subject: "reference_definition", TargetID: read.Nodes[0].TargetID,
		}},
	})
	if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput || result.Meta[MarkdownErrorCodeMetaKey] != MarkdownErrInvalidStructure {
		t.Fatalf("used reference-definition removal result=%+v err=%v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != source {
		t.Fatalf("used reference-definition removal mutated target: %q err=%v", got, err)
	}
}

func TestMarkdownEditRejectsInvalidReferenceDefinitionRemoveShapesBeforeFilesystemWork(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	targetID := strings.Repeat("a", 64)
	cases := []MarkdownEditOperation{
		{Action: "remove", Subject: "reference_definition", TargetID: targetID, Text: "extra"},
		{Action: "remove", Subject: "reference_definition", TargetID: targetID, Part: "destination"},
		{Action: "remove", Subject: "reference_definition", TargetID: targetID, Markdown: "extra"},
	}
	for _, operation := range cases {
		result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
			Path:       filepath.Join(t.TempDir(), "missing.md"),
			Operations: []MarkdownEditOperation{operation},
		})
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid reference-definition remove result=%+v err=%v operation=%+v", result, err, operation)
		}
	}
}

func TestMarkdownEditReferenceDefinitionParts(t *testing.T) {
	tests := []struct {
		name   string
		source string
		action string
		part   string
		text   string
		want   string
	}{
		{name: "replace destination", source: "  [docs]: <old/path>\t'Old title'   \r\n", action: "replace", part: "destination", text: "new/path", want: "  [docs]: <new/path>\t'Old title'   \r\n"},
		{name: "replace title", source: "  [docs]: <old/path>\t'Old title'   \r\n", action: "replace", part: "title", text: "New title", want: "  [docs]: <old/path>\t'New title'   \r\n"},
		{name: "add title", source: "[docs]: <target>\n", action: "add", part: "title", text: "Title", want: "[docs]: <target> \"Title\"\n"},
		{name: "remove title", source: "[docs]: <target> \"Title\"\n", action: "remove", part: "title", want: "[docs]: <target>\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "doc.md")
			if err := os.WriteFile(path, []byte(tt.source), 0o644); err != nil {
				t.Fatal(err)
			}
			h := NewHandler([]string{dir})
			result, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{
				Action: "query", Path: path, Query: "nodes", Kinds: []string{"reference_definition"}, Limit: 4,
			})
			if err != nil || result.IsError || len(read.Nodes) != 1 {
				t.Fatalf("read=%+v result=%+v err=%v", read, result, err)
			}
			operation := MarkdownEditOperation{Action: tt.action, Subject: "reference_definition", TargetID: read.Nodes[0].TargetID, Part: tt.part, Text: tt.text}
			previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
			if err != nil || previewResult.IsError || !preview.Changed {
				t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
			}
			applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
			if err != nil || applyResult.IsError || !output.Applied {
				t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != tt.want {
				t.Fatalf("target=%q want=%q err=%v", got, tt.want, err)
			}
		})
	}
}

func TestMarkdownEditRejectsInvalidReferenceDefinitionShapesBeforeFilesystemWork(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	targetID := strings.Repeat("a", 64)
	cases := []MarkdownEditOperation{
		{Action: "replace", Subject: "reference_definition", TargetID: targetID, Part: "destination"},
		{Action: "replace", Subject: "reference_definition", TargetID: targetID, Part: "label", Text: "new"},
		{Action: "add", Subject: "reference_definition", TargetID: targetID, Part: "title"},
		{Action: "remove", Subject: "reference_definition", TargetID: targetID, Part: "title", Text: "extra"},
		{Action: "remove", Subject: "reference_definition", TargetID: targetID, Part: "destination"},
	}
	for _, operation := range cases {
		result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
			Path:       filepath.Join(t.TempDir(), "missing.md"),
			Operations: []MarkdownEditOperation{operation},
		})
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid reference-definition shape result=%+v err=%v operation=%+v", result, err, operation)
		}
	}
}

func TestMarkdownEditDirectTitleLifecycle(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		subject string
		action  string
		text    string
		want    string
	}{
		{name: "replace inline link title", source: "[label](dest   \"old title\")\n", subject: "inline_link", action: "replace", text: "new", want: "[label](dest   \"new\")\n"},
		{name: "add inline link title", source: "[label](<dest path>   )\n", subject: "inline_link", action: "add", text: "new title", want: "[label](<dest path>    \"new title\")\n"},
		{name: "remove inline link title", source: "[label](dest   'old')\n", subject: "inline_link", action: "remove", want: "[label](dest   )\n"},
		{name: "replace image title", source: "![alt](dest  (old title))\r\n", subject: "image", action: "replace", text: "a longer title", want: "![alt](dest  (a longer title))\r\n"},
		{name: "add image title", source: "![alt](image.png)\r\n", subject: "image", action: "add", text: "caption p", want: "![alt](image.png \"caption p\")\r\n"},
		{name: "remove image title", source: "![alt](dest 'old')\n", subject: "image", action: "remove", want: "![alt](dest )\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "doc.md")
			if err := os.WriteFile(path, []byte(tt.source), 0o644); err != nil {
				t.Fatal(err)
			}
			h := NewHandler([]string{dir})
			result, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{tt.subject}, Limit: 4})
			if err != nil || result.IsError || len(read.Nodes) != 1 {
				t.Fatalf("read=%+v result=%+v err=%v", read, result, err)
			}
			operation := MarkdownEditOperation{Action: tt.action, Subject: tt.subject, TargetID: read.Nodes[0].TargetID, Part: "title", Text: tt.text}
			previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
			if err != nil || previewResult.IsError || !preview.Changed {
				t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
			}
			applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
			if err != nil || applyResult.IsError || !output.Applied {
				t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != tt.want {
				t.Fatalf("target=%q want=%q err=%v", got, tt.want, err)
			}
		})
	}
}

func TestMarkdownEditRejectsInvalidTitleLifecycleShapesBeforeFilesystemWork(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	targetID := strings.Repeat("a", 64)
	cases := []MarkdownEditOperation{
		{Action: "replace", Subject: "inline_link", TargetID: targetID, Part: "title"},
		{Action: "add", Subject: "inline_link", TargetID: targetID, Part: "title"},
		{Action: "add", Subject: "image", TargetID: targetID, Part: "alt", Text: "new"},
		{Action: "add", Subject: "autolink", TargetID: targetID, Part: "title", Text: "new"},
		{Action: "remove", Subject: "inline_link", TargetID: targetID, Part: "title", Text: "extra"},
		{Action: "remove", Subject: "image", TargetID: targetID, Part: "alt"},
	}
	for _, operation := range cases {
		result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
			Path:       filepath.Join(t.TempDir(), "missing.md"),
			Operations: []MarkdownEditOperation{operation},
		})
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid title lifecycle shape result=%+v err=%v operation=%+v", result, err, operation)
		}
	}
}

func TestMarkdownEditRejectsInvalidDirectLinkShapesBeforeFilesystemWork(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	targetID := strings.Repeat("a", 64)
	cases := []MarkdownEditOperation{
		{Action: "replace", Subject: "inline_link", TargetID: targetID, Text: "new"},
		{Action: "replace", Subject: "inline_link", TargetID: targetID, Part: "destination"},
		{Action: "replace", Subject: "inline_link", TargetID: targetID, Part: "alt", Text: "new"},
		{Action: "replace", Subject: "inline_link", TargetID: targetID, Part: "label", Text: "new", Markdown: "extra"},
		{Action: "replace", Subject: "image", TargetID: targetID, Part: "label", Text: "new"},
		{Action: "replace", Subject: "image", TargetID: targetID, Part: "alt"},
		{Action: "replace", Subject: "image", TargetID: targetID, Part: "alt", Text: "new", Position: "after"},
		{Action: "replace", Subject: "autolink", TargetID: targetID},
		{Action: "replace", Subject: "autolink", TargetID: targetID, Part: "destination", Text: "https://example.test"},
		{Action: "replace", Subject: "html_comment", TargetID: targetID},
		{Action: "replace", Subject: "html_comment", TargetID: targetID, Text: "new", Part: "content"},
		{Action: "replace", Subject: "html_anchor", TargetID: targetID},
		{Action: "replace", Subject: "html_anchor", TargetID: targetID, Text: "new-anchor", Markdown: "extra"},
	}
	for _, operation := range cases {
		result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
			Path:       filepath.Join(t.TempDir(), "missing.md"),
			Operations: []MarkdownEditOperation{operation},
		})
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid direct-link shape result=%+v err=%v operation=%+v", result, err, operation)
		}
	}
}

func TestMarkdownEditRejectsInteractingSimpleInlineComposition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("~~strike~~ `code` _em_ **strong**\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	cases := []struct {
		kind string
		text string
	}{
		{kind: "strikethrough", text: "gone"},
		{kind: "code_span", text: "cmd"},
		{kind: "emphasis", text: "italic"},
		{kind: "strong", text: "bold"},
	}
	operations := make([]MarkdownEditOperation, 0, len(cases))
	for _, tt := range cases {
		result, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{tt.kind}, Limit: 8})
		if err != nil || result.IsError || len(read.Nodes) != 1 {
			t.Fatalf("kind=%s read=%+v result=%+v err=%v", tt.kind, read, result, err)
		}
		operations = append(operations, MarkdownEditOperation{Action: "replace", Subject: tt.kind, TargetID: read.Nodes[0].TargetID, Text: tt.text})
	}
	result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: operations})
	if err != nil || result == nil || !result.IsError {
		t.Fatalf("interacting simple-inline composition result=%+v err=%v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("interacting simple-inline composition mutated target: %q err=%v", got, err)
	}
}

func TestMarkdownEditRejectsInvalidCodeSpanReplaceShapeBeforeFilesystemWork(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	base := MarkdownEditOperation{Action: "replace", Subject: "code_span", TargetID: strings.Repeat("a", 64), Text: "new"}
	cases := []MarkdownEditOperation{
		func() MarkdownEditOperation { op := base; op.Level = 1; return op }(),
		func() MarkdownEditOperation { op := base; op.Markdown = "extra"; return op }(),
		func() MarkdownEditOperation { op := base; op.Position = "after"; return op }(),
		func() MarkdownEditOperation { op := base; op.Part = "subtree"; return op }(),
		func() MarkdownEditOperation { op := base; op.AnchorTargetID = strings.Repeat("b", 64); return op }(),
	}
	for _, op := range cases {
		result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: filepath.Join(t.TempDir(), "missing.md"), Operations: []MarkdownEditOperation{op}})
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid code-span replace shape result=%+v err=%v op=%+v", result, err, op)
		}
	}
}

func TestMarkdownEditReplacesParagraphMarkdown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Title\r\n\r\nOld paragraph.\r\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"paragraph"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 1 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operation := MarkdownEditOperation{Action: "replace", Subject: "paragraph", TargetID: read.Nodes[0].TargetID, Markdown: "New **bold** paragraph."}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("paragraph preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "# Title\r\n\r\nNew **bold** paragraph.\r\n" {
		t.Fatalf("paragraph target=%q err=%v", got, err)
	}
}

func TestMarkdownEditRemovesParagraph(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("before\n\nremove me\n\nafter\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"paragraph"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 3 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operation := MarkdownEditOperation{Action: "remove", Subject: "paragraph", TargetID: read.Nodes[1].TargetID}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("paragraph removal preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "before\n\nafter\n" {
		t.Fatalf("paragraph removal target=%q err=%v", got, err)
	}
}

func TestMarkdownEditRemovesSectionSubtree(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Root\r\n\r\nIntro.\r\n\r\n## Remove\r\n\r\nRemove body.\r\n\r\n### Child\r\n\r\nChild body.\r\n\r\n## Keep\r\n\r\nKeep body.\r\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "sections", Levels: []int{2}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Sections) != 2 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operation := MarkdownEditOperation{Action: "remove", Subject: "section", TargetID: read.Sections[0].TargetID}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("section removal preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	want := "# Root\r\n\r\nIntro.\r\n\r\n## Keep\r\n\r\nKeep body.\r\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("section removal target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditRejectsInvalidSectionRemoveShapeBeforeFilesystemWork(t *testing.T) {
	operation := MarkdownEditOperation{Action: "remove", Subject: "section", TargetID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Markdown: "unexpected"}
	result := validateMarkdownEditInput(MarkdownEditInput{Path: "unused.md", Operations: []MarkdownEditOperation{operation}})
	if result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("invalid section remove result=%+v", result)
	}
}

func TestMarkdownEditInsertsSectionBeforeAndAfter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Root\r\n\r\n## Alpha\r\n\r\nAlpha.\r\n\r\n### Child\r\n\r\nChild.\r\n\r\n## Beta\r\n\r\nBeta.\r\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "sections", Levels: []int{2}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Sections) != 2 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operations := []MarkdownEditOperation{
		{Action: "insert", Subject: "section", TargetID: read.Sections[0].TargetID, Position: "after", Markdown: "## After Alpha\r\n\r\nBody.\r\n"},
		{Action: "insert", Subject: "section", TargetID: read.Sections[0].TargetID, Position: "before", Markdown: "## Before Alpha\r\n\r\nBody.\r\n"},
	}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: operations})
	if err != nil || previewResult.IsError || !preview.Changed || len(preview.Operations) != 2 {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("section insert preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	want := "# Root\r\n\r\n## Before Alpha\r\n\r\nBody.\r\n## Alpha\r\n\r\nAlpha.\r\n\r\n### Child\r\n\r\nChild.\r\n\r\n## After Alpha\r\n\r\nBody.\r\n## Beta\r\n\r\nBeta.\r\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("section insert target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditRejectsConflictingSectionInsertionsWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Root\n\n## Alpha\n\nAlpha.\n\n## Beta\n\nBeta.\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "sections", Levels: []int{2}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Sections) != 2 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{
		{Action: "insert", Subject: "section", TargetID: read.Sections[0].TargetID, Position: "after", Markdown: "## After Alpha\n"},
		{Action: "insert", Subject: "section", TargetID: read.Sections[1].TargetID, Position: "before", Markdown: "## Before Beta\n"},
	}})
	if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput || result.Meta[MarkdownErrorCodeMetaKey] != MarkdownErrInvalidStructure {
		t.Fatalf("conflicting section insert result=%+v err=%v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("conflicting section insert mutated target: %q err=%v", got, err)
	}
}

func TestMarkdownEditAppendsSectionChild(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Root\n\n## Parent\n\nBody.\n\n### Existing\n\nExisting.\n\n## Sibling\n\nSibling.\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "sections", Levels: []int{2}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Sections) != 2 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{{
		Action: "insert", Subject: "section", TargetID: read.Sections[0].TargetID, Position: "child", Markdown: "### Added\n\nAdded.\n",
	}}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("child preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	want := "# Root\n\n## Parent\n\nBody.\n\n### Existing\n\nExisting.\n\n### Added\n\nAdded.\n## Sibling\n\nSibling.\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("child target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditRejectsInvalidSectionChildWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Root\n\n## Parent\n\nBody.\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "sections", Levels: []int{2}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Sections) != 1 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{{
		Action: "insert", Subject: "section", TargetID: read.Sections[0].TargetID, Position: "child", Markdown: "## Wrong level\n",
	}}})
	if err != nil || result == nil || !result.IsError || result.Meta[MarkdownErrorCodeMetaKey] != MarkdownErrInvalidStructure {
		t.Fatalf("invalid child result=%+v err=%v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("invalid child mutated target: %q err=%v", got, err)
	}
}

func TestMarkdownEditMovesSectionAcrossParents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# One\n\n## Move\n\nMove.\n\n### Child\n\nChild.\n\n# Two\n\n## Anchor\n\nAnchor.\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "sections", Levels: []int{2}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Sections) != 2 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{{
		Action: "move", Subject: "section", TargetID: read.Sections[0].TargetID, AnchorTargetID: read.Sections[1].TargetID, Position: "after",
	}}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("move preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	want := "# One\n\n# Two\n\n## Anchor\n\nAnchor.\n## Move\n\nMove.\n\n### Child\n\nChild.\n\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("move target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditAppendsListItemChild(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("- parent\n- tail\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"list_item"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 2 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operation := MarkdownEditOperation{Action: "insert", Subject: "list_item", TargetID: read.Nodes[0].TargetID, Position: "child", Markdown: "  - child\n"}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("child preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	want := "- parent\n  - child\n- tail\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("child target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditMovesListItemAcrossParents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("1. first\n   - move\n     - child\n2. second\n   - anchor\n3. tail\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"list_item"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 6 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{{
		Action: "move", Subject: "list_item", TargetID: read.Nodes[1].TargetID, AnchorTargetID: read.Nodes[4].TargetID, Position: "after",
	}}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("move preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	want := "1. first\n2. second\n   - anchor\n   - move\n     - child\n3. tail\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("move target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditRejectsInvalidSectionMoveShapeBeforeFilesystemWork(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	base := MarkdownEditOperation{Action: "move", Subject: "section", TargetID: strings.Repeat("a", 64), AnchorTargetID: strings.Repeat("b", 64), Position: "before"}
	cases := []MarkdownEditOperation{
		func() MarkdownEditOperation { op := base; op.AnchorTargetID = "bad"; return op }(),
		func() MarkdownEditOperation { op := base; op.Position = "child"; return op }(),
		func() MarkdownEditOperation { op := base; op.Markdown = "## extra\n"; return op }(),
	}
	for _, op := range cases {
		result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: filepath.Join(t.TempDir(), "missing.md"), Operations: []MarkdownEditOperation{op}})
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid move shape result=%+v err=%v op=%+v", result, err, op)
		}
	}
	legacy := MarkdownEditOperation{Action: "remove", Subject: "section", TargetID: strings.Repeat("a", 64), AnchorTargetID: strings.Repeat("b", 64)}
	result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: filepath.Join(t.TempDir(), "missing.md"), Operations: []MarkdownEditOperation{legacy}})
	if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("legacy operation accepted anchorTargetId: result=%+v err=%v", result, err)
	}
}

func TestMarkdownEditRejectsInvalidSectionInsertShapeBeforeFilesystemWork(t *testing.T) {
	base := MarkdownEditOperation{Action: "insert", Subject: "section", TargetID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Position: "before", Markdown: "## Section\n"}
	cases := []MarkdownEditOperation{
		{Action: base.Action, Subject: base.Subject, TargetID: base.TargetID, Position: "inside", Markdown: base.Markdown},
		{Action: base.Action, Subject: base.Subject, TargetID: base.TargetID, Position: base.Position, Markdown: base.Markdown, Part: "body"},
	}
	for _, operation := range cases {
		result := validateMarkdownEditInput(MarkdownEditInput{Path: "unused.md", Operations: []MarkdownEditOperation{operation}})
		if result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid section insert operation=%+v result=%+v", operation, result)
		}
	}
}

func TestMarkdownEditReplacesSectionBodyAndPreservesChildren(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Root\r\n\r\n## Target\r\n\r\nOld body.\r\n\r\n### Child\r\n\r\nChild body.\r\n\r\n## Keep\r\n\r\nKeep body.\r\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "sections", Levels: []int{2}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Sections) != 2 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operation := MarkdownEditOperation{Action: "replace", Subject: "section", TargetID: read.Sections[0].TargetID, Part: "body", Markdown: "New **body**.\r\n\r\nSecond paragraph.\r\n\r\n"}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("section body preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "Old body.") || !strings.Contains(string(got), "New **body**.") || !strings.Contains(string(got), "### Child\r\n\r\nChild body.\r\n") || !strings.Contains(string(got), "## Keep\r\n\r\nKeep body.\r\n") {
		t.Fatalf("section body replacement target=%q", got)
	}
}

func TestMarkdownEditReplacesCompleteSectionSubtree(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Root\r\n\r\n## Target\r\n\r\nOld body.\r\n\r\n### Old Child\r\n\r\nOld child body.\r\n\r\n## Keep\r\n\r\nKeep body.\r\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "sections", Levels: []int{2}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Sections) != 2 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operation := MarkdownEditOperation{Action: "replace", Subject: "section", TargetID: read.Sections[0].TargetID, Part: "subtree", Markdown: "## Replaced\r\n\r\nNew body.\r\n\r\n### New Child\r\n\r\nNew child body.\r\n"}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{operation}})
	if err != nil || previewResult.IsError || !preview.Changed {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("section subtree preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "## Target") || strings.Contains(string(got), "### Old Child") || !strings.Contains(string(got), "## Replaced\r\n\r\nNew body.\r\n\r\n### New Child\r\n\r\nNew child body.\r\n") || !strings.Contains(string(got), "## Keep\r\n\r\nKeep body.\r\n") {
		t.Fatalf("section subtree replacement target=%q", got)
	}
}

func TestMarkdownEditRejectsInvalidSectionReplaceShapeBeforeFilesystemWork(t *testing.T) {
	base := MarkdownEditOperation{Action: "replace", Subject: "section", TargetID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Part: "body", Markdown: "Body."}
	cases := []MarkdownEditOperation{
		{Action: base.Action, Subject: base.Subject, TargetID: base.TargetID, Part: "unknown", Markdown: base.Markdown},
		{Action: base.Action, Subject: base.Subject, TargetID: base.TargetID, Part: base.Part, Markdown: base.Markdown, Position: "after"},
	}
	for _, operation := range cases {
		result := validateMarkdownEditInput(MarkdownEditInput{Path: "unused.md", Operations: []MarkdownEditOperation{operation}})
		if result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid section body operation=%+v result=%+v", operation, result)
		}
	}
}

func TestMarkdownEditInsertsParagraphBeforeAndAfter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("first\r\n\r\nmiddle\r\n\r\nlast\r\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"paragraph"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 3 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	operations := []MarkdownEditOperation{
		{Action: "insert", Subject: "paragraph", TargetID: read.Nodes[0].TargetID, Position: "after", Markdown: "after first *insert*"},
		{Action: "insert", Subject: "paragraph", TargetID: read.Nodes[2].TargetID, Position: "before", Markdown: "before last **insert**"},
	}
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: operations})
	if err != nil || previewResult.IsError || !preview.Changed || len(preview.Operations) != 2 {
		t.Fatalf("preview=%+v result=%+v err=%v", preview, previewResult, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("paragraph insert preview mutated target: %q err=%v", got, err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !output.Applied || output.State != editApplyStateCommitted {
		t.Fatalf("apply=%+v result=%+v err=%v", output, applyResult, err)
	}
	want := "first\r\n\r\nafter first *insert*\r\n\r\nmiddle\r\n\r\nbefore last **insert**\r\n\r\nlast\r\n"
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("paragraph insert target=%q want=%q err=%v", got, want, err)
	}
}

func TestMarkdownEditRejectsInvalidParagraphInsertShapeBeforeFilesystemWork(t *testing.T) {
	base := MarkdownEditOperation{Action: "insert", Subject: "paragraph", TargetID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Position: "before", Markdown: "Paragraph."}
	cases := []MarkdownEditOperation{
		{Action: base.Action, Subject: base.Subject, TargetID: base.TargetID, Position: "inside", Markdown: base.Markdown},
		{Action: base.Action, Subject: base.Subject, TargetID: base.TargetID, Position: base.Position, Markdown: base.Markdown, Text: "unexpected"},
	}
	for _, operation := range cases {
		result := validateMarkdownEditInput(MarkdownEditInput{Path: "unused.md", Operations: []MarkdownEditOperation{operation}})
		if result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid insert operation=%+v result=%+v", operation, result)
		}
	}
}

func TestMarkdownEditRejectsReplaceAndRemoveSameParagraphWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("old paragraph\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"paragraph"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 1 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	targetID := read.Nodes[0].TargetID
	result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{
		{Action: "replace", Subject: "paragraph", TargetID: targetID, Markdown: "new paragraph"},
		{Action: "remove", Subject: "paragraph", TargetID: targetID},
	}})
	if err != nil || result == nil || !result.IsError {
		t.Fatalf("overlap result=%+v err=%v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("overlap mutated target: %q err=%v", got, err)
	}
}
func TestMarkdownEditRejectsMultiParagraphReplacementWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("Old paragraph.\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"paragraph"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 1 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{{
		Action: "replace", Subject: "paragraph", TargetID: read.Nodes[0].TargetID, Markdown: "First.\n\nSecond.",
	}}})
	if err != nil || result == nil || !result.IsError {
		t.Fatalf("invalid paragraph result=%+v err=%v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("invalid paragraph mutated target: %q err=%v", got, err)
	}
}

func TestMarkdownEditRejectsInvalidHeadingLevelBeforeFilesystemWork(t *testing.T) {
	result := validateMarkdownEditInput(MarkdownEditInput{Path: "unused.md", Operations: []MarkdownEditOperation{{
		Action: "set", Subject: "heading", TargetID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Level: 7,
	}}})
	if result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("invalid level result=%+v", result)
	}
}

func TestMarkdownEditRejectsOverlappingHeadingRenamesWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Old\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	targetID := markdownHeadingTargetID(t, h, path)
	result, _, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{
		{Action: "rename", Subject: "heading", TargetID: targetID, Text: "First"},
		{Action: "rename", Subject: "heading", TargetID: targetID, Text: "Second"},
	}})
	if err != nil || result == nil || !result.IsError {
		t.Fatalf("overlap result=%+v err=%v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("overlap target=%q err=%v", got, err)
	}
}

func TestMarkdownEditRejectsOperationCountAboveFixedLimit(t *testing.T) {
	operations := make([]MarkdownEditOperation, markdownintelligence.MaxEditOperations+1)
	for index := range operations {
		operations[index] = MarkdownEditOperation{
			Action: "rename", Subject: "heading",
			TargetID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Text: "New",
		}
	}
	result, _, err := NewHandler(nil).HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: "unused.md", Operations: operations})
	if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("over-limit result=%+v err=%v", result, err)
	}
}

func TestMarkdownApplyConsumesPreviewBeforeCancellation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("# Old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	targetID := markdownHeadingTargetID(t, h, path)
	_, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "New"}}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, _, err := h.HandleMarkdownApply(ctx, nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || result == nil || !result.IsError {
		t.Fatalf("cancelled apply result=%+v err=%v", result, err)
	}
	replay, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || replay == nil || !replay.IsError {
		t.Fatalf("replay result=%+v err=%v", replay, err)
	}
}

func TestMarkdownApplyRejectsReadOnlyTargetAfterPreviewAndConsumesPreview(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Old\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	targetID := markdownHeadingTargetID(t, h, path)
	_, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "New"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	replaceCalled := false
	h.replaceFile = func(string, []byte, filesystem.ReplaceOptions) error {
		replaceCalled = true
		return nil
	}

	result, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodePermission {
		t.Fatalf("read-only apply result=%+v err=%v", result, err)
	}
	if replaceCalled {
		t.Fatal("read-only apply reached durable replacement without explicit writable approval")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("read-only target=%q err=%v", got, err)
	}
	replay, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || replay == nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("read-only replay result=%+v err=%v", replay, err)
	}
}

func TestMarkdownApplyRejectsStaleSourceAndConsumesPreview(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("# Old\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	targetID := markdownHeadingTargetID(t, h, path)
	_, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "New"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Old\n\nExternal.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale apply result=%+v err=%v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "# Old\n\nExternal.\n" {
		t.Fatalf("stale target=%q err=%v", got, err)
	}
	replay, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || replay == nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale replay result=%+v err=%v", replay, err)
	}
}

func TestMarkdownApplyRequiredBackupCapturesApprovedPreState(t *testing.T) {
	h, store, path := newEditBackupFixture(t, backupstore.Limits{})
	original := []byte("# Old\r\n\nBody.\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	targetID := markdownHeadingTargetID(t, h, path)
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
		Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "New"}}, BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil || previewResult.IsError || store.Index().ManifestCount != 0 {
		t.Fatalf("preview result=%+v output=%+v manifests=%d err=%v", previewResult, preview, store.Index().ManifestCount, err)
	}

	applyResult, applied, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || !applied.Applied || len(applied.BackupID) != 64 || applied.BackupPolicy != editBackupPolicyRequired {
		t.Fatalf("apply result=%+v output=%+v err=%v", applyResult, applied, err)
	}
	inspected, err := store.Inspect(context.Background(), applied.BackupID, backupstore.InspectOptions{})
	if err != nil || !inspected.ObjectVerified || inspected.Manifest.TargetPath != path || inspected.Manifest.SourceOperation != backupstore.SourceOperationEdit || inspected.Manifest.ContentFingerprint != filesystem.FingerprintRegularFileData(original) {
		t.Fatalf("backup=%+v err=%v", inspected, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "# New\r\n\nBody.\n" {
		t.Fatalf("applied target=%q err=%v", got, err)
	}
}

func TestMarkdownApplyWriteFailureIsTerminalAndPreservesBackup(t *testing.T) {
	h, store, path := newEditBackupFixture(t, backupstore.Limits{})
	original := []byte("# Old\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	targetID := markdownHeadingTargetID(t, h, path)
	_, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
		Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "New"}}, BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.replaceFile = func(string, []byte, filesystem.ReplaceOptions) error { return errors.New("injected write failure") }

	result, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || result == nil || !result.IsError || output.Applied || output.State != editApplyStateUnchanged || len(output.BackupID) != 64 {
		t.Fatalf("failed apply result=%+v output=%+v err=%v", result, output, err)
	}
	if store.Index().ManifestCount != 1 {
		t.Fatalf("manifest count=%d, want 1", store.Index().ManifestCount)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("failed target=%q err=%v", got, err)
	}
	replay, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || replay == nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("failed replay result=%+v err=%v", replay, err)
	}
}

func TestMarkdownApplyNoOpIsTerminalWithoutBackupOrWrite(t *testing.T) {
	h, store, path := newEditBackupFixture(t, backupstore.Limits{})
	original := []byte("# Old\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	targetID := markdownHeadingTargetID(t, h, path)
	previewResult, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{
		Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "Old"}}, BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil || previewResult.IsError || preview.Changed || store.Index().ManifestCount != 0 {
		t.Fatalf("no-op preview result=%+v output=%+v manifests=%d err=%v", previewResult, preview, store.Index().ManifestCount, err)
	}
	replaceCalled := false
	h.replaceFile = func(string, []byte, filesystem.ReplaceOptions) error {
		replaceCalled = true
		return errors.New("unexpected durable replace")
	}

	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError || output.Applied || output.Changed || output.State != editApplyStateUnchanged || output.ActualFingerprint != preview.TargetFingerprint {
		t.Fatalf("no-op apply result=%+v output=%+v err=%v", applyResult, output, err)
	}
	if replaceCalled || store.Index().ManifestCount != 0 {
		t.Fatalf("no-op apply replaceCalled=%v manifests=%d", replaceCalled, store.Index().ManifestCount)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("no-op target=%q err=%v", got, err)
	}
	replay, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || replay == nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("no-op replay result=%+v err=%v", replay, err)
	}
}

func TestMarkdownApplyConcurrentClaimHasOneWinner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("# Old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	targetID := markdownHeadingTargetID(t, h, path)
	_, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "New"}}})
	if err != nil {
		t.Fatal(err)
	}

	type outcome struct {
		code    string
		success bool
		err     error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, _, callErr := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
			if result == nil {
				outcomes <- outcome{err: errors.New("markdown_apply returned nil result")}
				return
			}
			code, _ := result.Meta[ErrorCodeMetaKey].(string)
			outcomes <- outcome{code: code, success: !result.IsError, err: callErr}
		}()
	}
	close(start)
	wg.Wait()
	close(outcomes)

	successes, conflicts := 0, 0
	for got := range outcomes {
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.success {
			successes++
		} else if got.code == ErrCodeConflict {
			conflicts++
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "# New\n" {
		t.Fatalf("concurrent apply target=%q err=%v", got, err)
	}
}

func TestMarkdownApplyOutputLimitIsTerminalAndNonMutating(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	original := []byte("# Old\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{dir})
	targetID := markdownHeadingTargetID(t, h, path)
	_, preview, err := h.HandleMarkdownEdit(context.Background(), nil, MarkdownEditInput{Path: path, Operations: []MarkdownEditOperation{{Action: "rename", Subject: "heading", TargetID: targetID, Text: "New"}}})
	if err != nil {
		t.Fatal(err)
	}
	replaceCalled := false
	h.replaceFile = func(string, []byte, filesystem.ReplaceOptions) error {
		replaceCalled = true
		return errors.New("unexpected durable replace")
	}
	h.config.Limits.MaxOutputBytes = 1

	limited, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || limited == nil || !limited.IsError || limited.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("output-limited apply result=%+v err=%v", limited, err)
	}
	if replaceCalled {
		t.Fatal("output-limited apply reached durable replacement")
	}
	h.config.Limits.MaxOutputBytes = config.DefaultMaxOutputBytes
	replay, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || replay == nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("output-limited replay result=%+v err=%v", replay, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("output-limited target=%q err=%v", got, err)
	}
}

func markdownHeadingTargetID(t *testing.T, h *Handler, path string) string {
	t.Helper()
	readResult, read, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "query", Path: path, Query: "nodes", Kinds: []string{"heading"}, Limit: 8})
	if err != nil || readResult.IsError || len(read.Nodes) != 1 {
		t.Fatalf("read=%+v result=%+v err=%v", read, readResult, err)
	}
	return read.Nodes[0].TargetID
}
