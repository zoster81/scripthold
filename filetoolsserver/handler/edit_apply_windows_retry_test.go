//go:build windows

package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestHandleEditFileApplyRetriesTransientWindowsDeleteShareContention(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandler([]string{root})
	previewResult, preview, err := h.HandleEditFilePreview(context.Background(), nil, EditFilePreviewInput{
		Action: editActionPreview,
		Path:   target,
		Edits:  []EditOperation{{OldText: "alpha", NewText: "omega"}},
	})
	if err != nil || previewResult.IsError || len(preview.PreviewID) != 64 {
		t.Fatalf("preview result=%+v output=%+v err=%v", previewResult, preview, err)
	}

	handle := openEditTargetWithoutDeleteShare(t, target)
	released := make(chan struct{})
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = windows.CloseHandle(handle)
		close(released)
	}()

	applyResult, applied, err := h.HandleEditFileApply(context.Background(), nil, PreviewApplyInput{PreviewID: preview.PreviewID})
	<-released
	if err != nil || applyResult.IsError || !applied.Applied || !applied.Changed {
		t.Fatalf("apply result=%+v output=%+v err=%v", applyResult, applied, err)
	}
	payload, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(payload), "omega\n"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
}

func openEditTargetWithoutDeleteShare(t *testing.T, path string) windows.Handle {
	t.Helper()
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	return handle
}
