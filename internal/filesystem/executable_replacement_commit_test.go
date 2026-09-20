package filesystem

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCommitExecutableReplacementCandidateConsumesCandidate(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	candidate := filepath.Join(dir, "candidate")
	if err := os.WriteFile(target, []byte("old-bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidate, []byte("new-bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := RestrictOwnerOnlyExecutable(candidate); err != nil {
			t.Fatal(err)
		}
	}
	targetID, err := CaptureSingleLinkFileIdentity(target)
	if err != nil {
		t.Fatal(err)
	}
	candidateID, err := CaptureSingleLinkFileIdentity(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := PrepareExecutableReplacementCandidate(target, targetID, candidate, candidateID); err != nil {
		t.Fatal(err)
	}
	installedID, err := CommitExecutableReplacementCandidate(target, targetID, candidate, candidateID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(candidate); !os.IsNotExist(err) {
		t.Fatalf("candidate still exists after commit: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-bytes" {
		t.Fatalf("installed bytes = %q, want new-bytes", got)
	}
	if installedID.StableKey() != candidateID.StableKey() ||
		installedID.VolumeKey() != candidateID.VolumeKey() {
		t.Fatalf("installed identity = %q/%q, candidate = %q/%q",
			installedID.StableKey(), installedID.VolumeKey(),
			candidateID.StableKey(), candidateID.VolumeKey())
	}
	if matches, err := targetID.Matches(target); err != nil {
		t.Fatal(err)
	} else if matches {
		t.Fatal("old target identity still matches after replacement")
	}
}

func TestCommitExecutableReplacementCandidateRejectsChangedCandidate(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	candidate := filepath.Join(dir, "candidate")
	if err := os.WriteFile(target, []byte("old"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidate, []byte("new"), 0o700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := RestrictOwnerOnlyExecutable(candidate); err != nil {
			t.Fatal(err)
		}
	}
	targetID, err := CaptureSingleLinkFileIdentity(target)
	if err != nil {
		t.Fatal(err)
	}
	candidateID, err := CaptureSingleLinkFileIdentity(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(candidate); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidate, []byte("replacement"), 0o700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := RestrictOwnerOnlyExecutable(candidate); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := CommitExecutableReplacementCandidate(target, targetID, candidate, candidateID); err == nil {
		t.Fatal("changed candidate identity must fail closed")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("target changed on rejected commit: %q", got)
	}
}
