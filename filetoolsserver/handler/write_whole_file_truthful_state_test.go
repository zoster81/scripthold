package handler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestWriteWholeFileDoesNotImposeFingerprintLimitOnExistingTarget(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root}, WithConfig(&config.Config{
		DefaultEncoding: "utf-8",
		Limits: config.Limits{
			MaxFileBytes:   8,
			MaxOutputBytes: config.DefaultMaxOutputBytes,
		},
	}))

	result, output, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path: path, Content: "x", Encoding: "utf-8", BOM: "never",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.IsError {
		t.Fatalf("result = %#v, want successful replacement", result)
	}
	if output.TargetFingerprint != "" {
		t.Fatalf("target fingerprint = %q, want omitted for oversized pre-state", output.TargetFingerprint)
	}
	if output.State != writeWholeFileStateCommitted || !output.Changed || !output.Applied {
		t.Fatalf("output = %+v, want committed changed=true applied=true", output)
	}
	if output.ResultFingerprint == "" || output.ActualFingerprint != output.ResultFingerprint {
		t.Fatalf("fingerprints = %+v, want actual=result", output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "x" {
		t.Fatalf("target = %q, want x", data)
	}
}

func TestWriteWholeFileClassifiesPostCommitFailureFromActualDiskState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	originalReplace := h.replaceFile
	h.replaceFile = func(path string, data []byte, options filesystem.ReplaceOptions) error {
		if err := originalReplace(path, data, options); err != nil {
			return err
		}
		return errors.New("injected post-commit durability failure")
	}

	result, output, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path: path, Content: "omega", Encoding: "utf-8", BOM: "never",
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
	if output.State != writeWholeFileStateCommitted || !output.Changed || output.Applied {
		t.Fatalf("output = %+v, want committed changed=true applied=false", output)
	}
	if output.ActualFingerprint != output.ResultFingerprint || output.ActualFingerprint == "" {
		t.Fatalf("fingerprints = %+v, want actual=result", output)
	}
}

func TestWriteWholeFileClassifiesPreCommitFailureAsUnchanged(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	h.replaceFile = func(string, []byte, filesystem.ReplaceOptions) error {
		return filesystem.ErrConcurrentModification
	}

	result, output, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path: path, Content: "omega", Encoding: "utf-8", BOM: "never",
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
	if output.State != writeWholeFileStateUnchanged || output.Changed || output.Applied {
		t.Fatalf("output = %+v, want unchanged changed=false applied=false", output)
	}
	if output.ActualFingerprint != output.TargetFingerprint || output.ActualFingerprint == "" {
		t.Fatalf("fingerprints = %+v, want actual=target", output)
	}
}
