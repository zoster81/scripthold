package filesystem

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestObjectIdentityDetectsFileAndDirectoryReplacement(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "target.txt")
	if err := os.WriteFile(file, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	fileIdentity, err := CaptureObjectIdentity(file)
	if err != nil {
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
	if matches, err := fileIdentity.Matches(file); err != nil || matches {
		t.Fatalf("file replacement match=%v err=%v, want false nil", matches, err)
	}

	directory := filepath.Join(root, "dir")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	directoryIdentity, err := CaptureObjectIdentity(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if matches, err := directoryIdentity.Matches(directory); err != nil || matches {
		t.Fatalf("directory replacement match=%v err=%v, want false nil", matches, err)
	}
}

func TestObjectIdentityReportsStableVolume(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("stable volume identity is only required on supported R24 platforms")
	}
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	rootIdentity, err := CaptureObjectIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	fileIdentity, err := CaptureObjectIdentity(file)
	if err != nil {
		t.Fatal(err)
	}
	if rootIdentity.VolumeKey() == "" || fileIdentity.VolumeKey() == "" {
		t.Fatalf("empty volume identity: root=%q file=%q", rootIdentity.VolumeKey(), fileIdentity.VolumeKey())
	}
	if rootIdentity.VolumeKey() != fileIdentity.VolumeKey() {
		t.Fatalf("same temp filesystem reported different volumes: %q != %q", rootIdentity.VolumeKey(), fileIdentity.VolumeKey())
	}
}
