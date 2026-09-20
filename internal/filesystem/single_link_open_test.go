package filesystem

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenVerifiedSingleLinkFileAcceptsExpectedIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binary")
	if err := os.WriteFile(path, []byte("content"), 0o700); err != nil {
		t.Fatal(err)
	}
	identity, err := CaptureSingleLinkFileIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := OpenVerifiedSingleLinkFile(path, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "content" {
		t.Fatalf("content = %q", data)
	}
}

func TestOpenVerifiedSingleLinkFileRejectsWrongIdentity(t *testing.T) {
	directory := t.TempDir()
	first := filepath.Join(directory, "first")
	second := filepath.Join(directory, "second")
	if err := os.WriteFile(first, []byte("one"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("two"), 0o700); err != nil {
		t.Fatal(err)
	}
	identity, err := CaptureSingleLinkFileIdentity(first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenVerifiedSingleLinkFile(second, identity); err == nil {
		t.Fatal("wrong identity must fail closed")
	}
}
