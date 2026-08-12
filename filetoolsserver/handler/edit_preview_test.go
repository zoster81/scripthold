package handler

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/config"
	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestHandleEditFilePreviewApplyIsExactAndOneShot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("Hello World"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})

	previewResult, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: "preview",
		Path:   path,
		Edits:  []EditOperation{{OldText: "World", NewText: "Go"}},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("preview result=%+v output=%+v err=%v", previewResult, preview, err)
	}
	if preview.Action != "preview" || !preview.Changed || preview.Applied {
		t.Fatalf("unexpected preview state: %+v", preview)
	}
	if len(preview.PreviewID) != 64 {
		t.Fatalf("preview ID length = %d, want 64", len(preview.PreviewID))
	}
	if _, err := hex.DecodeString(preview.PreviewID); err != nil {
		t.Fatalf("preview ID is not hexadecimal: %v", err)
	}
	if len(preview.TargetFingerprint) != 64 || len(preview.ResultFingerprint) != 64 || preview.TargetFingerprint == preview.ResultFingerprint {
		t.Fatalf("unexpected fingerprints: %+v", preview)
	}
	if _, err := time.Parse(time.RFC3339Nano, preview.CreatedAt); err != nil {
		t.Fatalf("invalid createdAt: %v", err)
	}
	if _, err := time.Parse(time.RFC3339Nano, preview.ExpiresAt); err != nil {
		t.Fatalf("invalid expiresAt: %v", err)
	}
	if preview.Encoding != "utf-8" || preview.LineEndingStyle != LineEndingNone {
		t.Fatalf("unexpected text metadata: %+v", preview)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "Hello World" {
		t.Fatalf("preview changed target: %q err=%v", data, err)
	}

	applyResult, applied, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action:    "apply",
		PreviewID: preview.PreviewID,
	})
	if err != nil || applyResult.IsError {
		t.Fatalf("apply result=%+v output=%+v text=%q err=%v", applyResult, applied, extractTextFromResultRead(applyResult.Content), err)
	}
	if applied.Action != "apply" || !applied.Changed || !applied.Applied {
		t.Fatalf("unexpected apply state: %+v", applied)
	}
	if applied.TargetFingerprint != preview.TargetFingerprint || applied.ResultFingerprint != preview.ResultFingerprint {
		t.Fatalf("apply fingerprints diverged: preview=%+v apply=%+v", preview, applied)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "Hello Go" {
		t.Fatalf("apply content = %q err=%v", data, err)
	}

	replay, _, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: preview.PreviewID})
	if err != nil || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("replay result=%+v err=%v", replay, err)
	}
}

func TestHandleEditFilePreviewRejectsStaleTargetAndConsumesToken(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})

	result, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: "preview",
		Path:   path,
		Edits:  []EditOperation{{OldText: "alpha", NewText: "omega"}},
	})
	if err != nil || result.IsError {
		t.Fatalf("preview result=%+v output=%+v err=%v", result, preview, err)
	}
	if err := os.WriteFile(path, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}

	stale, _, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: preview.PreviewID})
	if err != nil || stale.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale apply result=%+v err=%v", stale, err)
	}
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	consumed, _, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: preview.PreviewID})
	if err != nil || consumed.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("consumed stale token result=%+v err=%v", consumed, err)
	}
}

func TestHandleEditFilePreviewPreservesUTF16BOMAndCRLF(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.data")
	original := encodeUTF16LEWithBOM(t, "alpha\r\nbeta\r\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})

	result, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: "preview",
		Path:   path,
		Edits:  []EditOperation{{OldText: "beta", NewText: "gamma"}},
	})
	if err != nil || result.IsError {
		t.Fatalf("preview result=%+v output=%+v err=%v", result, preview, err)
	}
	if preview.Encoding != "utf-16-le" || !preview.HasBOM || preview.BOMType != "utf-16-le" || preview.LineEndingStyle != LineEndingCRLF {
		t.Fatalf("preview metadata=%+v", preview)
	}
	result, applied, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: preview.PreviewID})
	if err != nil || result.IsError || !applied.Applied {
		t.Fatalf("apply result=%+v output=%+v err=%v", result, applied, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bom := fileEncoding.BOMBytesFor("utf-16-le")
	if !bytes.HasPrefix(data, bom) {
		t.Fatal("UTF-16 LE BOM was not preserved")
	}
	enc, _ := fileEncoding.Get("utf-16-le")
	decoded, err := enc.NewDecoder().Bytes(data[len(bom):])
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(decoded), "alpha\r\ngamma\r\n"; got != want {
		t.Fatalf("decoded content=%q, want %q", got, want)
	}
}

func TestHandleEditFilePreviewExpiryEvictionAndRestartInvalidation(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("value"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{DefaultEncoding: "utf-8", Limits: config.Limits{
		MaxFileBytes:          config.DefaultMaxFileBytes,
		MaxOutputBytes:        config.DefaultMaxOutputBytes,
		MaxEditPreviews:       1,
		MaxEditPreviewBytes:   1 << 20,
		EditPreviewTTLSeconds: 60,
	}}
	now := time.Date(2026, 8, 3, 20, 0, 0, 0, time.UTC)
	h := NewHandler([]string{root}, WithConfig(cfg))
	h.editPreviews.now = func() time.Time { return now }

	_, firstPreview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: "preview", Path: first, Edits: []EditOperation{{OldText: "value", NewText: "first"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, secondPreview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: "preview", Path: second, Edits: []EditOperation{{OldText: "value", NewText: "second"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	evicted, _, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: firstPreview.PreviewID})
	if err != nil || evicted.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("evicted result=%+v err=%v", evicted, err)
	}

	now = now.Add(2 * time.Minute)
	expired, _, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: secondPreview.PreviewID})
	if err != nil || expired.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("expired result=%+v err=%v", expired, err)
	}

	h2 := NewHandler([]string{root}, WithConfig(cfg))
	unknownAfterRestart, _, err := h2.HandleEditFile(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: secondPreview.PreviewID})
	if err != nil || unknownAfterRestart.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("restart invalidation result=%+v err=%v", unknownAfterRestart, err)
	}
}

func TestHandleEditFilePreviewConcurrentApplyHasOneWinner(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: "preview", Path: path, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	type outcome struct {
		resultCode string
		success    bool
		err        error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, _, callErr := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: preview.PreviewID})
			code, _ := result.Meta[ErrorCodeMetaKey].(string)
			outcomes <- outcome{resultCode: code, success: !result.IsError, err: callErr}
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
		} else if got.resultCode == ErrCodeConflict {
			conflicts++
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}
}

func TestHandleEditFilePreviewApplyFailureIsTerminalAndInputIsStrict(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: "preview", Path: path, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.replaceFile = func(string, []byte, filesystem.ReplaceOptions) error { return errors.New("injected write failure") }
	failed, _, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: preview.PreviewID})
	if err != nil || !failed.IsError {
		t.Fatalf("write failure result=%+v err=%v", failed, err)
	}
	terminal, _, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: preview.PreviewID})
	if err != nil || terminal.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("terminal result=%+v err=%v", terminal, err)
	}

	for _, input := range []EditFileInput{
		{Action: "unknown", Path: path, Edits: []EditOperation{{OldText: "a", NewText: "b"}}},
		{Action: "preview", Path: path, DryRun: true, Edits: []EditOperation{{OldText: "a", NewText: "b"}}},
		{Action: "apply", PreviewID: "not-a-token"},
		{Action: "apply", PreviewID: preview.PreviewID, Path: path},
	} {
		result, _, err := h.HandleEditFile(context.Background(), nil, input)
		if err != nil || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("strict input %+v result=%+v err=%v", input, result, err)
		}
	}
}
func TestHandleEditFilePreviewRejectsSameContentFileReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: "preview", Path: path, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	replacement := filepath.Join(root, "replacement.txt")
	if err := os.WriteFile(replacement, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}

	result, _, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: preview.PreviewID})
	if err != nil || result.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("replacement result=%+v err=%v", result, err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "alpha" {
		t.Fatalf("replacement content=%q err=%v", data, err)
	}
}

func TestHandleEditFileNoOpPreviewApplyLeavesBytesAndMetadataUnchanged(t *testing.T) {
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

	result, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: "preview", Path: path, Edits: []EditOperation{{OldText: "alpha", NewText: "alpha"}},
	})
	if err != nil || result.IsError {
		t.Fatalf("preview result=%+v output=%+v err=%v", result, preview, err)
	}
	if preview.Changed || preview.TargetFingerprint != preview.ResultFingerprint || preview.Diff != "" {
		t.Fatalf("unexpected no-op preview: %+v", preview)
	}
	result, applied, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: preview.PreviewID})
	if err != nil || result.IsError || applied.Applied || applied.Changed {
		t.Fatalf("no-op apply result=%+v output=%+v err=%v", result, applied, err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, original) || !before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode() {
		t.Fatalf("no-op changed target: data=%q before=%+v after=%+v", data, before, after)
	}
}

func TestHandleEditFilePreviewEnforcesRetainedByteLimit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root}, WithConfig(&config.Config{
		DefaultEncoding: "utf-8",
		Limits: config.Limits{
			MaxFileBytes:          config.DefaultMaxFileBytes,
			MaxOutputBytes:        config.DefaultMaxOutputBytes,
			MaxEditPreviews:       4,
			MaxEditPreviewBytes:   1,
			EditPreviewTTLSeconds: 60,
		},
	}))
	result, _, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: "preview", Path: path, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}},
	})
	if err != nil || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("byte-limit result=%+v err=%v", result, err)
	}
}

func TestHandleEditFileCancelledApplyConsumesPreview(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: "preview", Path: path, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled, _, err := h.HandleEditFile(ctx, nil, EditFileInput{Action: "apply", PreviewID: preview.PreviewID})
	if err != nil || cancelled.Meta[ErrorCodeMetaKey] != ErrCodeCancelled {
		t.Fatalf("cancelled result=%+v err=%v", cancelled, err)
	}
	terminal, _, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: preview.PreviewID})
	if err != nil || terminal.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("terminal result=%+v err=%v", terminal, err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "alpha" {
		t.Fatalf("cancelled apply changed content=%q err=%v", data, err)
	}
}

func TestEditPreviewTokenIsNotLogged(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := NewHandler([]string{root})
	wrapped := Wrap(logger, "edit_file", h.HandleEditFile)
	result, preview, err := wrapped(context.Background(), nil, EditFileInput{
		Action: "preview", Path: path, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}},
	})
	if err != nil || result.IsError {
		t.Fatalf("preview result=%+v output=%+v err=%v", result, preview, err)
	}
	if strings.Contains(logs.String(), preview.PreviewID) {
		t.Fatalf("preview token entered logs: %s", logs.String())
	}
	result, _, err = wrapped(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: preview.PreviewID})
	if err != nil || result.IsError {
		t.Fatalf("apply result=%+v text=%q err=%v", result, extractTextFromResultRead(result.Content), err)
	}
	if strings.Contains(logs.String(), preview.PreviewID) {
		t.Fatalf("preview token entered logs after apply: %s", logs.String())
	}
}
func TestHandleEditFilePreviewOutputLimitReleasesCachedState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root}, WithConfig(&config.Config{
		DefaultEncoding: "utf-8",
		Limits: config.Limits{
			MaxFileBytes:          config.DefaultMaxFileBytes,
			MaxOutputBytes:        1,
			MaxEditPreviews:       4,
			MaxEditPreviewBytes:   1 << 20,
			EditPreviewTTLSeconds: 60,
		},
	}))
	result, _, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: "preview", Path: path, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}},
	})
	if err != nil || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("preview output-limit result=%+v err=%v", result, err)
	}
	h.editPreviews.mu.Lock()
	entries, retained := len(h.editPreviews.entries), h.editPreviews.totalBytes
	h.editPreviews.mu.Unlock()
	if entries != 0 || retained != 0 {
		t.Fatalf("preview state leaked after output limit: entries=%d retained=%d", entries, retained)
	}
}

func TestHandleEditFileApplyOutputLimitIsTerminalAndNonMutating(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: "preview", Path: path, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.config.Limits.MaxOutputBytes = 1
	limited, _, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: preview.PreviewID})
	if err != nil || limited.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("apply output-limit result=%+v err=%v", limited, err)
	}
	h.config.Limits.MaxOutputBytes = config.DefaultMaxOutputBytes
	terminal, _, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: "apply", PreviewID: preview.PreviewID})
	if err != nil || terminal.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("terminal result=%+v err=%v", terminal, err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "alpha" {
		t.Fatalf("output-limited apply changed content=%q err=%v", data, err)
	}
}
