package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOwnerOnlyPathRoundTrip(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RestrictOwnerOnlyPath(directory, true); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOwnerOnlyPath(directory, true); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "state.json")
	if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RestrictOwnerOnlyPath(file, false); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOwnerOnlyPath(file, false); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(directory)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("directory permissions = %04o", info.Mode().Perm())
		}
	}
}

func TestOwnerOnlyFileLockSharedExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "use.lock")
	first, err := TryAcquireOwnerOnlyFileLock(path, LockShared, true)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	second, err := TryAcquireOwnerOnlyFileLock(path, LockShared, false)
	if err != nil {
		t.Fatalf("second shared lock failed: %v", err)
	}
	defer second.Close()

	if _, err := TryAcquireOwnerOnlyFileLock(path, LockExclusive, false); !errors.Is(err, ErrFileLockBusy) {
		t.Fatalf("exclusive while shared = %v, want ErrFileLockBusy", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	exclusive, err := TryAcquireOwnerOnlyFileLock(path, LockExclusive, false)
	if err != nil {
		t.Fatal(err)
	}
	defer exclusive.Close()
	if _, err := TryAcquireOwnerOnlyFileLock(path, LockShared, false); !errors.Is(err, ErrFileLockBusy) {
		t.Fatalf("shared while exclusive = %v, want ErrFileLockBusy", err)
	}
}

func TestOwnerOnlyFileLockRejectsHardLink(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "control.lock")
	lock, err := TryAcquireOwnerOnlyFileLock(path, LockExclusive, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, filepath.Join(directory, "alias")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if _, err := TryAcquireOwnerOnlyFileLock(path, LockExclusive, false); err == nil {
		t.Fatal("hard-linked lock must fail closed")
	}
}

func TestOwnerOnlyFileLockValidateRejectsPathReplacement(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "control.lock")
	lock, err := TryAcquireOwnerOnlyFileLock(path, LockExclusive, true)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()

	retained := filepath.Join(directory, "retained.lock")
	if err := os.Rename(path, retained); err != nil {
		t.Skipf("locked-file rename unavailable: %v", err)
	}
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := RestrictOwnerOnlyPath(path, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := lock.Validate(path); err == nil {
		t.Fatal("replaced lock path must fail identity validation")
	}
}
