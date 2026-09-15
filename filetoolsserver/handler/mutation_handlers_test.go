package handler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleConvertEncoding_ReplacesExistingBackupWithOriginal(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler([]string{dir})
	path := filepath.Join(dir, "convert.txt")
	backupPath := path + ".bak"
	original := []byte("Привет")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("stale backup"), 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path:   path,
		From:   "utf-8",
		To:     "cp1251",
		Backup: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %v", result.Content)
	}
	if !sameExistingTestFile(t, output.BackupPath, backupPath) {
		t.Fatalf("backup path = %q, want same file as %q", output.BackupPath, backupPath)
	}
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backup, original) {
		t.Fatalf("backup = %q, want original", backup)
	}
}

func TestHandleManageBom_CancelledLeavesFileUnchanged(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler([]string{dir})
	path := filepath.Join(dir, "target.txt")
	original := append([]byte{0xEF, 0xBB, 0xBF}, []byte("target")...)
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, _, err := h.HandleManageBom(ctx, nil, ManageBomInput{Path: path, Action: "strip"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected cancellation error")
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, original) {
		t.Fatal("cancelled BOM operation changed file")
	}
}
