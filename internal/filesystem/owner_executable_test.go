package filesystem

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteOwnerOnlyExecutableNoReplace(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "helper")
	snapshot, err := WriteOwnerOnlyExecutableNoReplace(context.Background(), target, bytes.NewBufferString("binary"), 6)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Exists || snapshot.Size != 6 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if err := ValidateOwnerOnlyExecutable(target); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("mode = %04o, want 0700", info.Mode().Perm())
		}
	}
	if _, err := WriteOwnerOnlyExecutableNoReplace(context.Background(), target, bytes.NewBufferString("other!"), 6); err == nil {
		t.Fatal("existing destination must fail no-replace")
	}
}
