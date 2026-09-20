package filesystem

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCaptureSingleLinkFileIdentityAcceptsRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := CaptureSingleLinkFileIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if identity.IsDirectory() {
		t.Fatal("regular file identity reported as directory")
	}
	matches, err := identity.Matches(path)
	if err != nil || !matches {
		t.Fatalf("identity match = %v, %v", matches, err)
	}
}

func TestCaptureSingleLinkFileIdentityRejectsHardLink(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "candidate")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, filepath.Join(directory, "alias")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if _, err := CaptureSingleLinkFileIdentity(path); err == nil {
		t.Fatal("hard-linked file must fail closed")
	}
}

func TestCaptureSingleLinkFileIdentityRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	if err := os.WriteFile(target, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "candidate")
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if _, err := CaptureSingleLinkFileIdentity(link); err == nil {
		t.Fatal("symlink must fail closed")
	}
}

func TestCaptureSingleLinkFileIdentityDetectsLaterReplacement(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "candidate")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := CaptureSingleLinkFileIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(directory, "retained")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	matches, err := identity.Matches(path)
	if err != nil {
		t.Fatal(err)
	}
	if matches {
		t.Fatal("replacement still matched captured identity")
	}
}
