package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

func TestEnumerateExactTreeIsDeterministicCompleteAndIncludesGit(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, ".git"))
	mustMkdirAll(t, filepath.Join(root, "a", "nested"))
	mustWriteFile(t, filepath.Join(root, ".git", "config"))
	mustWriteFile(t, filepath.Join(root, ".hidden"))
	mustWriteFile(t, filepath.Join(root, "a", "nested", "z.txt"))
	mustWriteFile(t, filepath.Join(root, "b.txt"))

	tree, err := EnumerateExactTree(context.Background(), root, ExactTreeOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
		MaxEntries:          32,
		MaxDepth:            8,
		MaxFileBytes:        1024,
		MaxAggregateBytes:   4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, entry := range tree.Entries {
		paths = append(paths, filepath.ToSlash(entry.RelativePath))
	}
	want := []string{".", ".git", ".git/config", ".hidden", "a", "a/nested", "a/nested/z.txt", "b.txt"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	if tree.FileCount != 4 || tree.DirectoryCount != 4 || tree.TotalBytes != 16 {
		t.Fatalf("unexpected counts: %#v", tree)
	}
	if tree.ContentFingerprint == "" || tree.StateFingerprint == "" {
		t.Fatalf("fingerprints are missing: %#v", tree)
	}
}

func TestEnumerateExactTreeRejectsLinksAndLimitTruncation(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "dir", "deep"))
	mustWriteFile(t, filepath.Join(root, "dir", "deep", "file.txt"))

	options := ExactTreeOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
		MaxEntries:          2,
		MaxDepth:            8,
		MaxFileBytes:        1024,
		MaxAggregateBytes:   4096,
	}
	if _, err := EnumerateExactTree(context.Background(), root, options); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("entry limit error = %v, want LIMIT", err)
	}
	options.MaxEntries = 16
	options.MaxDepth = 1
	if _, err := EnumerateExactTree(context.Background(), root, options); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("depth limit error = %v, want LIMIT", err)
	}
	options.MaxDepth = 8
	options.MaxAggregateBytes = 3
	if _, err := EnumerateExactTree(context.Background(), root, options); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("aggregate limit error = %v, want LIMIT", err)
	}

	options.MaxAggregateBytes = 4096
	link := filepath.Join(root, "alias")
	if err := os.Symlink(filepath.Join(root, "dir"), link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := EnumerateExactTree(context.Background(), root, options); operation.KindOf(err) != operation.KindSymlinkEscape {
		t.Fatalf("link error = %v, want SYMLINK_ESCAPE", err)
	}
}

func TestExactTreeVerificationDetectsAddedEntryAndReplacement(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := ExactTreeOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
		MaxEntries:          16,
		MaxDepth:            4,
		MaxFileBytes:        1024,
		MaxAggregateBytes:   4096,
	}
	expected, err := EnumerateExactTree(context.Background(), root, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyExactTree(context.Background(), expected, options); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("added entry verification error = %v, want CONFLICT", err)
	}
	if err := os.Remove(filepath.Join(root, "new.txt")); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(root, "replacement.txt")
	if err := os.WriteFile(replacement, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, file); err != nil {
		t.Fatal(err)
	}
	if err := VerifyExactTree(context.Background(), expected, options); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("replacement verification error = %v, want CONFLICT", err)
	}
}

func TestEnumerateExactTreeRejectsNestedVolumeBoundary(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := ExactTreeOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
		MaxEntries:          16,
		MaxDepth:            4,
		MaxFileBytes:        1024,
		MaxAggregateBytes:   4096,
	}
	_, err := enumerateExactTree(context.Background(), root, options, func(path string) (ObjectIdentity, error) {
		identity, captureErr := CaptureObjectIdentity(path)
		if captureErr == nil && filepath.Base(path) == "file.txt" {
			identity.volumeKey += ":other"
		}
		return identity, captureErr
	})
	if operation.KindOf(err) != operation.KindUnsupported {
		t.Fatalf("nested volume error = %v, want UNSUPPORTED", err)
	}
}
