package filesystem

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrepareExecutableReplacementCandidatePreservesBytesAndIdentity(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	candidate := filepath.Join(dir, "candidate")
	if err := os.WriteFile(target, []byte("target-bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidate, []byte("candidate-bytes"), 0o700); err != nil {
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
	got, err := os.ReadFile(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "candidate-bytes" {
		t.Fatalf("candidate bytes = %q", got)
	}
	matches, err := candidateID.Matches(candidate)
	if err != nil || !matches {
		t.Fatalf("candidate identity changed: matches=%v err=%v", matches, err)
	}
	targetGot, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(targetGot) != "target-bytes" {
		t.Fatalf("target bytes changed: %q", targetGot)
	}
}

func TestPrepareExecutableReplacementCandidateRejectsWrongTargetIdentity(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	other := filepath.Join(dir, "other")
	candidate := filepath.Join(dir, "candidate")
	for path, data := range map[string]string{target: "target", other: "other", candidate: "candidate"} {
		if err := os.WriteFile(path, []byte(data), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if runtime.GOOS == "windows" {
		if err := RestrictOwnerOnlyExecutable(candidate); err != nil {
			t.Fatal(err)
		}
	}
	wrongID, err := CaptureSingleLinkFileIdentity(other)
	if err != nil {
		t.Fatal(err)
	}
	candidateID, err := CaptureSingleLinkFileIdentity(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := PrepareExecutableReplacementCandidate(target, wrongID, candidate, candidateID); err == nil {
		t.Fatal("wrong target identity must fail closed")
	}
}
