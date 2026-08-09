//go:build windows

package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/security"
	"golang.org/x/sys/windows"
)

func TestHandlerCanonicalizesShortAllowedDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "directory with a long component name")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	shortRoot := windowsShortPathForTest(t, root)
	if strings.EqualFold(filepath.Clean(shortRoot), filepath.Clean(root)) {
		t.Skip("8.3 short names are unavailable on this filesystem")
	}

	canonical, err := security.NormalizeAllowedDirs([]string{root})
	if err != nil || len(canonical) != 1 {
		t.Fatalf("normalize canonical root: %v", err)
	}
	h := NewHandler([]string{shortRoot})
	if got := h.GetAllowedDirectories(); len(got) != 1 || got[0] != canonical[0] {
		t.Fatalf("active allowed directories = %v, want canonical %q", got, canonical[0])
	}

	updated := NewHandler(nil)
	updated.UpdateAllowedDirectories([]string{shortRoot})
	if got := updated.GetAllowedDirectories(); len(got) != 1 || got[0] != canonical[0] {
		t.Fatalf("updated allowed directories = %v, want canonical %q", got, canonical[0])
	}

	merged := NewHandler(nil).MergeAllowedDirectories([]string{shortRoot})
	if len(merged) != 1 || merged[0] != canonical[0] {
		t.Fatalf("merged allowed directories = %v, want canonical %q", merged, canonical[0])
	}

	convertPath := filepath.Join(root, "convert.txt")
	convertRequestPath := filepath.Join(shortRoot, "convert.txt")
	original := []byte("Привет")
	if err := os.WriteFile(convertPath, original, 0644); err != nil {
		t.Fatal(err)
	}
	result, converted, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: convertRequestPath, From: "utf-8", To: "cp1251", Backup: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("conversion through canonical path failed: %v", result.Content)
	}
	if converted.BackupPath == "" {
		t.Fatal("expected backup path after byte-changing conversion")
	}
	backup, err := os.ReadFile(converted.BackupPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != string(original) {
		t.Fatalf("backup = %q, want %q", backup, original)
	}

	grepPath := filepath.Join(root, "grep.txt")
	grepRequestPath := filepath.Join(shortRoot, "grep.txt")
	if err := os.WriteFile(grepPath, []byte("line one\nline two with pattern\n"), 0644); err != nil {
		t.Fatal(err)
	}
	grepResult, output, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern: "pattern", Paths: []string{grepRequestPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	if grepResult.IsError {
		t.Fatalf("grep through canonical path failed: %v", grepResult.Content)
	}
	if output.TotalMatches != 1 || len(output.Matches) != 1 || output.Matches[0].Line != 2 {
		t.Fatalf("unexpected grep output: %+v", output)
	}
}

func TestRestoreTargetComparisonAcceptsEquivalentShortPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "restore target with a long component")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	shortRoot := windowsShortPathForTest(t, root)
	if strings.EqualFold(filepath.Clean(shortRoot), filepath.Clean(root)) {
		t.Skip("8.3 short names are unavailable on this filesystem")
	}

	longExisting := filepath.Join(root, "existing.txt")
	if err := os.WriteFile(longExisting, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !sameResolvedRestoreTarget(filepath.Join(shortRoot, "existing.txt"), longExisting) {
		t.Fatal("existing short and canonical restore targets were not treated as equivalent")
	}
	if !sameResolvedRestoreTarget(filepath.Join(shortRoot, "missing.txt"), filepath.Join(root, "missing.txt")) {
		t.Fatal("missing short and canonical restore targets were not treated as equivalent")
	}
}

func windowsShortPathForTest(t *testing.T, path string) string {
	t.Helper()
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}

	bufferSize := uint32(260)
	for bufferSize <= 1<<15 {
		buffer := make([]uint16, bufferSize)
		length, err := windows.GetShortPathName(pathPtr, &buffer[0], bufferSize)
		if err != nil {
			t.Skipf("8.3 short names are unavailable: %v", err)
		}
		if length < bufferSize {
			return filepath.Clean(windows.UTF16ToString(buffer[:length]))
		}
		bufferSize = length + 1
	}
	t.Fatal("short path exceeded supported test buffer")
	return ""
}
