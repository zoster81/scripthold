package handler

import (
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
