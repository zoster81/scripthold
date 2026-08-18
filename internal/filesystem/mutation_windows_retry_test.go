//go:build windows

package filesystem

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestReplaceFileRetriesTransientWindowsDeleteShareContention(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected, err := CaptureSnapshotWithDigest(target)
	if err != nil {
		t.Fatal(err)
	}

	handle := openWithoutDeleteShare(t, target)
	released := make(chan struct{})
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = windows.CloseHandle(handle)
		close(released)
	}()

	err = ReplaceFile(target, []byte("after\n"), ReplaceOptions{Mode: 0o644, Expected: &expected})
	<-released
	if err != nil {
		t.Fatalf("ReplaceFile() failed after transient delete-share contention cleared: %v", err)
	}
	payload, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(payload), "after\n"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
}

func TestAtomicReplaceRetryPolicyRetriesDeterministically(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected, err := CaptureSnapshotWithDigest(target)
	if err != nil {
		t.Fatal(err)
	}

	timing := newAtomicReplaceRetryTestTiming()
	commitCalls := 0
	err = commitStagedTargetWithRetryTimed(
		target,
		filepath.Join(root, "staged.txt"),
		&expected,
		func(string, string) error {
			commitCalls++
			if commitCalls < 5 {
				return windows.ERROR_ACCESS_DENIED
			}
			return nil
		},
		nil,
		nil,
		timing.policy(),
	)
	if err != nil {
		t.Fatalf("deterministic retry policy failed: %v", err)
	}
	if commitCalls != 5 {
		t.Fatalf("commit calls = %d, want 5", commitCalls)
	}
	if len(timing.sleeps) != 4 {
		t.Fatalf("retry sleeps = %d, want 4", len(timing.sleeps))
	}
	for index, delay := range timing.sleeps {
		if delay != atomicReplaceRetryDelay {
			t.Fatalf("retry sleep[%d] = %v, want %v", index, delay, atomicReplaceRetryDelay)
		}
	}
}

func TestReplaceFilePermanentWindowsDeleteShareContentionPreservesWin32Code(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected, err := CaptureSnapshotWithDigest(target)
	if err != nil {
		t.Fatal(err)
	}
	handle := openWithoutDeleteShare(t, target)
	defer windows.CloseHandle(handle)

	err = ReplaceFile(target, []byte("after\n"), ReplaceOptions{Mode: 0o644, Expected: &expected})
	if err == nil {
		t.Fatal("ReplaceFile() unexpectedly succeeded under persistent delete-share contention")
	}
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("error = %v, want errors.Is(ERROR_ACCESS_DENIED)", err)
	}
	if !strings.Contains(err.Error(), "Win32 code 5") {
		t.Fatalf("error = %q, want raw Win32 code 5", err)
	}
	payload, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got, want := string(payload), "before\n"; got != want {
		t.Fatalf("target = %q, want unchanged %q", got, want)
	}
}

func TestReplaceFileSucceedsWhileFileIdentityIsOpen(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected, err := CaptureSnapshotWithDigest(target)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := OpenFileIdentity(target)
	if err != nil {
		t.Fatal(err)
	}
	defer identity.Close()

	replacement := []byte("after!\n")
	if err := ReplaceFile(target, replacement, ReplaceOptions{Mode: expected.Mode, ModTime: &expected.ModTime, Expected: &expected}); err != nil {
		t.Fatalf("ReplaceFile() should not be blocked by an active FileIdentity: %v", err)
	}
	matches, err := identity.Matches(target)
	if err != nil {
		t.Fatal(err)
	}
	if matches {
		t.Fatal("FileIdentity still matched the path after replacement")
	}
}

func TestReplaceFileSucceedsWhileReadSessionIsOpen(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected, err := CaptureSnapshotWithDigest(target)
	if err != nil {
		t.Fatal(err)
	}
	session, err := OpenReadSession(target)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Start(0); err != nil {
		t.Fatal(err)
	}
	payload, err := io.ReadAll(session)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(payload), "before\n"; got != want {
		t.Fatalf("session payload = %q, want %q", got, want)
	}

	replacement := []byte("after!\n")
	if err := ReplaceFile(target, replacement, ReplaceOptions{Mode: expected.Mode, ModTime: &expected.ModTime, Expected: &expected}); err != nil {
		t.Fatalf("ReplaceFile() should not be blocked by an active ReadSession: %v", err)
	}
	payload, err = os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(payload), string(replacement); got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
	if _, err := session.Finish(); !errors.Is(err, ErrConcurrentModification) {
		t.Fatalf("Finish() error = %v, want ErrConcurrentModification after path replacement", err)
	}
}
func TestAlternativeAtomicReplaceFailureDoesNotShortenClassicRetry(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected, err := CaptureSnapshotWithDigest(target)
	if err != nil {
		t.Fatal(err)
	}

	commitCalls := 0
	alternativeCalls := 0
	timing := newAtomicReplaceRetryTestTiming()
	err = commitStagedTargetWithRetryTimed(
		target,
		filepath.Join(root, "staged.txt"),
		&expected,
		func(string, string) error {
			commitCalls++
			if commitCalls == 1 {
				return windows.ERROR_ACCESS_DENIED
			}
			return nil
		},
		nil,
		func(string, string, error) (bool, error) {
			alternativeCalls++
			return true, windows.ERROR_INVALID_NAME
		},
		timing.policy(),
	)
	if err != nil {
		t.Fatalf("classic retry should recover after alternative failure: %v", err)
	}
	if alternativeCalls != 1 {
		t.Fatalf("alternative calls = %d, want 1", alternativeCalls)
	}
	if commitCalls != 2 {
		t.Fatalf("classic commit calls = %d, want 2", commitCalls)
	}
}
func TestAlternativeAtomicReplaceRunsOnlyAfterExpectedSnapshotRevalidation(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected, err := CaptureSnapshotWithDigest(target)
	if err != nil {
		t.Fatal(err)
	}

	commitCalls := 0
	alternativeCalls := 0
	timing := newAtomicReplaceRetryTestTiming()
	err = commitStagedTargetWithRetryTimed(
		target,
		filepath.Join(root, "staged.txt"),
		&expected,
		func(string, string) error {
			commitCalls++
			if commitCalls == 1 {
				if err := os.WriteFile(target, []byte("changed\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return windows.ERROR_ACCESS_DENIED
			}
			return nil
		},
		nil,
		func(string, string, error) (bool, error) {
			alternativeCalls++
			return true, nil
		},
		timing.policy(),
	)
	if err == nil || !errors.Is(err, ErrConcurrentModification) {
		t.Fatalf("error = %v, want ErrConcurrentModification", err)
	}
	if alternativeCalls != 0 {
		t.Fatalf("alternative calls = %d, want 0 before successful revalidation", alternativeCalls)
	}
}

func TestAtomicReplaceRetryRevalidatesExpectedSnapshotDeterministically(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected, err := CaptureSnapshotWithDigest(target)
	if err != nil {
		t.Fatal(err)
	}

	timing := newAtomicReplaceRetryTestTiming()
	commitCalls := 0
	err = commitStagedTargetWithRetryTimed(
		target,
		filepath.Join(root, "staged.txt"),
		&expected,
		func(string, string) error {
			commitCalls++
			if commitCalls == 1 {
				if writeErr := os.WriteFile(target, []byte("external\n"), 0o644); writeErr != nil {
					t.Fatal(writeErr)
				}
				return windows.ERROR_ACCESS_DENIED
			}
			return nil
		},
		nil,
		nil,
		timing.policy(),
	)
	if err == nil || !errors.Is(err, ErrConcurrentModification) {
		t.Fatalf("error = %v, want ErrConcurrentModification", err)
	}
	if commitCalls != 1 {
		t.Fatalf("commit calls = %d, want 1 before revalidation abort", commitCalls)
	}
	payload, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got, want := string(payload), "external\n"; got != want {
		t.Fatalf("target = %q, want concurrent content %q", got, want)
	}
}

type atomicReplaceRetryTestTiming struct {
	nowValue time.Time
	sleeps   []time.Duration
}

func newAtomicReplaceRetryTestTiming() *atomicReplaceRetryTestTiming {
	return &atomicReplaceRetryTestTiming{nowValue: time.Unix(1, 0)}
}

func (timing *atomicReplaceRetryTestTiming) policy() atomicReplaceRetryTiming {
	return atomicReplaceRetryTiming{
		now: func() time.Time { return timing.nowValue },
		sleep: func(delay time.Duration) {
			timing.sleeps = append(timing.sleeps, delay)
			timing.nowValue = timing.nowValue.Add(delay)
		},
	}
}

func openWithoutDeleteShare(t *testing.T, path string) windows.Handle {
	t.Helper()
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	return handle
}
