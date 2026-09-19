package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMarkdownCreatePreviewApplyReplayAndNoReplaceRace(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler([]string{dir})

	input := MarkdownCreateInput{
		Path:     filepath.Join(dir, "created.md"),
		Encoding: "utf-8",
		BOM:      "never",
		Blocks: []MarkdownCreateBlock{{
			Type:  "heading",
			Level: 1,
			Content: []MarkdownCreateInline{{
				Type: "text",
				Text: "Title",
			}},
		}},
	}
	previewResult, preview, err := h.HandleMarkdownCreate(context.Background(), nil, input)
	if err != nil || previewResult.IsError {
		t.Fatalf("preview result=%+v output=%+v err=%v", previewResult, preview, err)
	}
	if preview.PreviewID == "" || preview.ResultFingerprint == "" || preview.Markdown != "# Title\n" {
		t.Fatalf("preview=%+v", preview)
	}
	if _, err := os.Stat(input.Path); !os.IsNotExist(err) {
		t.Fatalf("preview mutated target: err=%v", err)
	}

	applyResult, applied, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || applyResult.IsError {
		t.Fatalf("apply result=%+v output=%+v err=%v", applyResult, applied, err)
	}
	if !applied.Applied || !applied.Changed || applied.State != editApplyStateCommitted || applied.ActualFingerprint != preview.ResultFingerprint {
		t.Fatalf("applied=%+v", applied)
	}
	if got, err := os.ReadFile(input.Path); err != nil || string(got) != "# Title\n" {
		t.Fatalf("created bytes=%q err=%v", got, err)
	}

	replay, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || !replay.IsError {
		t.Fatalf("replay result=%+v err=%v", replay, err)
	}

	racePath := filepath.Join(dir, "race.md")
	input.Path = racePath
	previewResult, racePreview, err := h.HandleMarkdownCreate(context.Background(), nil, input)
	if err != nil || previewResult.IsError {
		t.Fatalf("race preview result=%+v output=%+v err=%v", previewResult, racePreview, err)
	}
	if err := os.WriteFile(racePath, []byte("competitor\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	raceApply, raceOutput, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: racePreview.PreviewID})
	if err != nil || !raceApply.IsError {
		t.Fatalf("race apply result=%+v output=%+v err=%v", raceApply, raceOutput, err)
	}
	if got, err := os.ReadFile(racePath); err != nil || string(got) != "competitor\n" {
		t.Fatalf("race target overwritten: %q err=%v", got, err)
	}
}

func TestMarkdownCreateRejectsExistingTargetAndPreserveBOM(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler([]string{dir})
	existing := filepath.Join(dir, "existing.md")
	if err := os.WriteFile(existing, []byte("# Existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, _, err := h.HandleMarkdownCreate(context.Background(), nil, MarkdownCreateInput{Path: existing})
	if err != nil || !result.IsError {
		t.Fatalf("existing result=%+v err=%v", result, err)
	}

	missing := filepath.Join(dir, "new.md")
	result, _, err = h.HandleMarkdownCreate(context.Background(), nil, MarkdownCreateInput{
		Path: missing,
		BOM:  "preserve",
		Blocks: []MarkdownCreateBlock{{
			Type:        "paragraph",
			RawMarkdown: "body",
		}},
	})
	if err != nil || !result.IsError {
		t.Fatalf("preserve result=%+v err=%v", result, err)
	}
	if _, statErr := os.Stat(missing); !os.IsNotExist(statErr) {
		t.Fatalf("invalid preview created target: err=%v", statErr)
	}
}

func TestMarkdownCreateMapsMarkspliceConstructionFailure(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler([]string{dir})
	result, _, err := h.HandleMarkdownCreate(context.Background(), nil, MarkdownCreateInput{
		Path: filepath.Join(dir, "invalid.md"),
		Blocks: []MarkdownCreateBlock{{
			Type:        "paragraph",
			RawMarkdown: "# becomes a heading",
		}},
	})
	if err != nil || !result.IsError {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("error code=%v, want %s", result.Meta[ErrorCodeMetaKey], ErrCodeInvalidInput)
	}
}
