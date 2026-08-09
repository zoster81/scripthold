package filesystem

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/security"
)

func TestWalkRespectGitignoreNestedNegation(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		".gitignore":         "*.log\nignored/\n!keep.log\n",
		"drop.log":           "drop",
		"keep.log":           "keep",
		"visible.txt":        "visible",
		"ignored/hidden.txt": "hidden",
		"nested/.gitignore":  "*.tmp\n!keep.tmp\n",
		"nested/drop.tmp":    "drop",
		"nested/keep.tmp":    "keep",
		"nested/visible.txt": "visible",
		".git/config":        "private metadata",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var got []string
	err := Walk(context.Background(), root, WalkOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
		RespectGitignore:    true,
	}, func(entry Entry) (WalkAction, error) {
		if !entry.DirEntry.IsDir() {
			got = append(got, filepath.ToSlash(entry.RelativePath))
		}
		return WalkContinue, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{".gitignore", "keep.log", "nested/.gitignore", "nested/keep.tmp", "nested/visible.txt", "visible.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("walked files = %#v, want %#v", got, want)
	}
}

func TestWalkRespectGitignoreFalsePreservesTraversal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "visible.tmp"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	found := false
	err := Walk(context.Background(), root, WalkOptions{ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root})}, func(entry Entry) (WalkAction, error) {
		found = found || entry.Name == "visible.tmp"
		return WalkContinue, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("respectGitignore=false unexpectedly filtered visible.tmp")
	}
}

func TestMatchIgnoreSegmentsGlobstar(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/generated/**", "a/generated/b/file.txt", true},
		{"src/**/test/*.go", "src/a/b/test/file.go", true},
		{"src/**/test/*.go", "src/test/file.go", true},
		{"src/**/test/*.go", "src/a/test/file.txt", false},
		{"a/**/b/**/c", "a/x/y/b/z/c", true},
		{"a/**/b/**/c", "a/x/y/c", false},
	}
	for _, test := range tests {
		got := matchIgnoreSegments(strings.Split(test.pattern, "/"), strings.Split(test.path, "/"))
		if got != test.want {
			t.Errorf("matchIgnoreSegments(%q, %q) = %v, want %v", test.pattern, test.path, got, test.want)
		}
	}
}

func TestWalkRejectsMalformedGitignore(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("[invalid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Walk(context.Background(), root, WalkOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
		RespectGitignore:    true,
	}, func(entry Entry) (WalkAction, error) {
		return WalkContinue, nil
	})
	if !errors.Is(err, ErrInvalidGitignore) || !strings.Contains(err.Error(), "invalid glob pattern") {
		t.Fatalf("malformed .gitignore error = %v", err)
	}
}

func FuzzParseIgnoreRules(f *testing.F) {
	for _, seed := range []string{"*.tmp\n!important.tmp\n", "src/**/generated/\n", "[invalid\n", strings.Repeat("a", 4097)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, content string) {
		if len(content) > maxGitignoreBytes {
			t.Skip()
		}
		_, _ = parseIgnoreRules(content)
	})
}

func TestWalkRejectsLinkedGitignore(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideIgnore := filepath.Join(outside, "outside.ignore")
	if err := os.WriteFile(outsideIgnore, []byte("*.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideIgnore, filepath.Join(root, ".gitignore")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "visible.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Walk(context.Background(), root, WalkOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
		RespectGitignore:    true,
	}, func(entry Entry) (WalkAction, error) {
		return WalkContinue, nil
	})
	if !errors.Is(err, ErrInvalidGitignore) || !strings.Contains(err.Error(), "must be a regular file") {
		t.Fatalf("linked .gitignore error = %v", err)
	}
}
