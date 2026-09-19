package handler

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/marksplice/workspacefs"
)

func TestMarkdownWorkspaceFSDecodesMarkdownAndWalksDirectories(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "docs")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(nested, "guide.md")
	content := "## Café\r\n"
	if err := os.WriteFile(path, encodeUTF16LEWithBOM(t, content), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandler([]string{root})
	workspace, err := newMarkdownWorkspaceFS(context.Background(), h, root, "")
	if err != nil {
		t.Fatal(err)
	}
	file, err := workspace.Open("docs/guide.md")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("decoded Markdown = %q, want %q", data, content)
	}
	info, err := fs.Stat(workspace, "docs/guide.md")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(len([]byte(content))) {
		t.Fatalf("decoded size = %d, want %d", info.Size(), len([]byte(content)))
	}

	var names []string
	if err := fs.WalkDir(workspace, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		names = append(names, name)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 || names[0] != "." || names[1] != "docs" || names[2] != "docs/guide.md" {
		t.Fatalf("walk names = %#v", names)
	}
}

func TestMarkdownWorkspaceFSSupportsMarkspliceScan(t *testing.T) {
	root := t.TempDir()
	first := "# A\r\n\r\n[B](b.md)\r\n"
	if err := os.WriteFile(filepath.Join(root, "a.md"), encodeUTF16LEWithBOM(t, first), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.md"), []byte("# B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ignored.txt"), []byte("not Markdown\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandler([]string{root})
	adapter, err := newMarkdownWorkspaceFS(context.Background(), h, root, "")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := workspacefs.Scan(adapter, ".", workspacefs.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	documents := workspace.Documents()
	if len(documents) != 2 || documents[0].Key != "a.md" || documents[1].Key != "b.md" {
		t.Fatalf("workspace documents = %#v", documents)
	}
	if _, err := adapter.Open("ignored.txt"); err == nil || !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("non-Markdown open error = %v, want fs.ErrInvalid", err)
	}
	graph, err := workspace.BuildGraph()
	if err != nil {
		t.Fatal(err)
	}
	if keys := graph.DocumentKeys(); len(keys) != 2 || keys[0] != "a.md" || keys[1] != "b.md" {
		t.Fatalf("workspace graph keys = %#v", keys)
	}
}

func TestMarkdownWorkspaceFSRejectsWorkspaceRootEscape(t *testing.T) {
	allowed := t.TempDir()
	root := filepath.Join(allowed, "workspace")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(allowed, "outside.md")
	if err := os.WriteFile(outside, []byte("# Outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.md")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	h := NewHandler([]string{allowed})
	workspace, err := newMarkdownWorkspaceFS(context.Background(), h, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Open("escape.md"); err == nil || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("escape open error = %v, want fs.ErrPermission", err)
	}
	if _, err := workspace.Open("../outside.md"); err == nil || !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("invalid path open error = %v, want fs.ErrInvalid", err)
	}
}
