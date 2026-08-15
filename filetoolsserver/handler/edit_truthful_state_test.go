package handler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestEditApplyClassifiesPostCommitFailureFromActualDiskState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: editActionPreview,
		Path:   path,
		Edits:  []EditOperation{{OldText: "alpha", NewText: "omega"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	originalReplace := h.replaceFile
	h.replaceFile = func(path string, data []byte, options filesystem.ReplaceOptions) error {
		if err := originalReplace(path, data, options); err != nil {
			return err
		}
		return errors.New("injected post-commit durability failure")
	}

	result, output, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action:    editActionApply,
		PreviewID: preview.PreviewID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("result = %#v, want tool error", result)
	}
	if result.Meta[ErrorCodeMetaKey] != ErrCodePartialCommit {
		t.Fatalf("error code = %#v, want %q", result.Meta[ErrorCodeMetaKey], ErrCodePartialCommit)
	}
	if output.State != editApplyStateCommitted || !output.Changed || output.Applied {
		t.Fatalf("output = %+v, want committed changed=true applied=false", output)
	}
	if output.ActualFingerprint != preview.ResultFingerprint {
		t.Fatalf("actual fingerprint = %q, want %q", output.ActualFingerprint, preview.ResultFingerprint)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "omega" {
		t.Fatalf("target = %q, want omega", data)
	}
}

func TestEditApplyClassifiesPreCommitWriteFailureAsUnchanged(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: editActionPreview,
		Path:   path,
		Edits:  []EditOperation{{OldText: "alpha", NewText: "omega"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.replaceFile = func(string, []byte, filesystem.ReplaceOptions) error {
		return filesystem.ErrConcurrentModification
	}

	result, output, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action:    editActionApply,
		PreviewID: preview.PreviewID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("result = %#v, want tool error", result)
	}
	if result.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("error code = %#v, want %q", result.Meta[ErrorCodeMetaKey], ErrCodeConflict)
	}
	if output.State != editApplyStateUnchanged || output.Changed || output.Applied {
		t.Fatalf("output = %+v, want unchanged changed=false applied=false", output)
	}
	if output.ActualFingerprint != preview.TargetFingerprint {
		t.Fatalf("actual fingerprint = %q, want %q", output.ActualFingerprint, preview.TargetFingerprint)
	}
}
