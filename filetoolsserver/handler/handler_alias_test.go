package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHandlerAllowsConfiguredDirectoryAliasButRejectsExternalAlias(t *testing.T) {
	parent := t.TempDir()
	realRoot := filepath.Join(parent, "real-root")
	if err := os.Mkdir(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	configuredAlias := filepath.Join(parent, "configured-alias")
	externalAlias := filepath.Join(parent, "external-alias")
	createDirectoryLinkForTest(t, realRoot, configuredAlias)
	createDirectoryLinkForTest(t, realRoot, externalAlias)

	h := NewHandler([]string{configuredAlias})
	allowedPath := filepath.Join(configuredAlias, "allowed.txt")
	result, _, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path:    allowedPath,
		Content: "allowed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("write through configured directory alias failed: %v", result.Content)
	}

	allowedDirs := h.GetAllowedDirectories()
	if len(allowedDirs) != 1 {
		t.Fatalf("allowed directories = %v, want one resolved root", allowedDirs)
	}
	canonicalPath := filepath.Join(allowedDirs[0], "canonical.txt")
	result, _, err = h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path:    canonicalPath,
		Content: "canonical",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("write through resolved directory path failed: %v", result.Content)
	}

	deniedPath := filepath.Join(externalAlias, "denied.txt")
	result, _, err = h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path:    deniedPath,
		Content: "denied",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("write through an external alias to the allowed target succeeded")
	}
	if _, err := os.Stat(deniedPath); !os.IsNotExist(err) {
		t.Fatalf("denied path was created or returned an unexpected error: %v", err)
	}
}
