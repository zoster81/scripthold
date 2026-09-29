package handler

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
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

func TestMarkdownCreateFixedWidthUnicodeEncodings(t *testing.T) {
	for _, encodingName := range []string{"utf-16-le", "utf-16-be", "utf-32-le", "utf-32-be"} {
		encodingName := encodingName
		t.Run(encodingName, func(t *testing.T) {
			dir := t.TempDir()
			h := NewHandler([]string{dir})
			path := filepath.Join(dir, "created.md")
			result, preview, err := h.HandleMarkdownCreate(context.Background(), nil, MarkdownCreateInput{
				Path: path, Encoding: encodingName, BOM: "always",
				Blocks: []MarkdownCreateBlock{{
					Type: "heading", Level: 1,
					Content: []MarkdownCreateInline{{Type: "text", Text: "Città 中文"}},
				}},
			})
			if err != nil || result.IsError {
				t.Fatalf("preview result=%+v output=%+v err=%v", result, preview, err)
			}
			if preview.Encoding != encodingName || !preview.HasBOM || preview.BOMType != encodingName || preview.Markdown != "# Città 中文\n" {
				t.Fatalf("preview=%+v", preview)
			}
			applyResult, applied, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
			if err != nil || applyResult.IsError || !applied.Applied {
				t.Fatalf("apply result=%+v output=%+v err=%v", applyResult, applied, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			bom := fileEncoding.BOMBytesFor(encodingName)
			if !bytes.HasPrefix(data, bom) {
				t.Fatalf("missing %s BOM: %x", encodingName, data[:min(len(data), len(bom))])
			}
			enc, ok := fileEncoding.Get(encodingName)
			if !ok {
				t.Fatalf("encoding %s unavailable", encodingName)
			}
			decoded, err := enc.NewDecoder().Bytes(data[len(bom):])
			if err != nil {
				t.Fatal(err)
			}
			if string(decoded) != "# Città 中文\n" {
				t.Fatalf("decoded=%q", decoded)
			}
		})
	}
}

func TestMarkdownCreateRejectsParentReplacement(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	path := filepath.Join(parent, "new.md")
	result, preview, err := h.HandleMarkdownCreate(context.Background(), nil, MarkdownCreateInput{
		Path:   path,
		Blocks: []MarkdownCreateBlock{{Type: "paragraph", RawMarkdown: "body"}},
	})
	if err != nil || result.IsError {
		t.Fatalf("preview result=%+v output=%+v err=%v", result, preview, err)
	}

	oldParent := filepath.Join(root, "parent-old")
	if err := os.Rename(parent, oldParent); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || !applyResult.IsError {
		t.Fatalf("apply result=%+v output=%+v err=%v", applyResult, output, err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("replacement parent received target: err=%v", statErr)
	}
}

func TestMarkdownCreateLimitsConsumeNoMutation(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler([]string{dir})
	path := filepath.Join(dir, "limited.md")
	h.config.Limits.MaxFileBytes = 4
	result, _, err := h.HandleMarkdownCreate(context.Background(), nil, MarkdownCreateInput{
		Path:   path,
		Blocks: []MarkdownCreateBlock{{Type: "heading", Level: 1, Content: []MarkdownCreateInline{{Type: "text", Text: "Title"}}}},
	})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("file limit result=%+v err=%v", result, err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("file-limit preview mutated target: err=%v", statErr)
	}

	h.config.Limits.MaxFileBytes = 1 << 20
	result, preview, err := h.HandleMarkdownCreate(context.Background(), nil, MarkdownCreateInput{
		Path:   path,
		Blocks: []MarkdownCreateBlock{{Type: "paragraph", RawMarkdown: "body"}},
	})
	if err != nil || result.IsError {
		t.Fatalf("preview result=%+v output=%+v err=%v", result, preview, err)
	}
	h.config.Limits.MaxOutputBytes = 1
	applyResult, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || !applyResult.IsError || applyResult.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("output-limit apply result=%+v err=%v", applyResult, err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("output-limited apply mutated target: err=%v", statErr)
	}
	h.config.Limits.MaxOutputBytes = 1 << 20
	replay, _, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("output-limit replay result=%+v err=%v", replay, err)
	}
}

func TestMarkdownCreateTruthfulStateAfterCommittedReplaceError(t *testing.T) {
	dir := canonicalHandlerTestDir(t)
	h := NewHandler([]string{dir})
	path := filepath.Join(dir, "partial.md")
	result, preview, err := h.HandleMarkdownCreate(context.Background(), nil, MarkdownCreateInput{
		Path:   path,
		Blocks: []MarkdownCreateBlock{{Type: "paragraph", RawMarkdown: "body"}},
	})
	if err != nil || result.IsError {
		t.Fatalf("preview result=%+v output=%+v err=%v", result, preview, err)
	}

	h.replaceFile = func(path string, data []byte, options filesystem.ReplaceOptions) error {
		if err := filesystem.ReplaceFile(path, data, options); err != nil {
			return err
		}
		return errors.New("injected post-commit failure")
	}
	applyResult, output, err := h.HandleMarkdownApply(context.Background(), nil, MarkdownApplyInput{PreviewID: preview.PreviewID})
	if err != nil || !applyResult.IsError || applyResult.Meta[ErrorCodeMetaKey] != ErrCodePartialCommit {
		t.Fatalf("apply result=%+v output=%+v err=%v", applyResult, output, err)
	}
	if output.State != editApplyStateCommitted || !output.Changed || output.Applied || output.ActualFingerprint != preview.ResultFingerprint {
		t.Fatalf("truthful output=%+v", output)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "body\n" {
		t.Fatalf("committed bytes=%q err=%v", got, err)
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
