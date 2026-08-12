package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePathEvidenceProjectsMissingPathFromNearestExistingAncestor(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	requested := filepath.Join(parent, "missing", "target.txt")
	set, err := NormalizeAllowedDirectorySet([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		t.Fatal(err)
	}
	resolvedRequested := filepath.Join(resolvedParent, "missing", "target.txt")

	evidence, err := ValidatePathEvidenceWithAllowedDirectories(requested, set.Requested, set.Resolved)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Exists {
		t.Fatal("missing path was reported as existing")
	}
	if !PathsEqual(evidence.NearestExistingPath, resolvedParent) {
		t.Fatalf("nearest existing path = %q, want %q", evidence.NearestExistingPath, resolvedParent)
	}
	if !PathsEqual(evidence.ResolvedPath, resolvedRequested) {
		t.Fatalf("resolved projected path = %q, want %q", evidence.ResolvedPath, resolvedRequested)
	}
	if !PathsEqual(evidence.RequestedPath, requested) {
		t.Fatalf("requested path = %q, want %q", evidence.RequestedPath, requested)
	}
}

func TestValidatePathEvidenceExistingPathIsCanonical(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	set, err := NormalizeAllowedDirectorySet([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := ValidatePathEvidenceWithAllowedDirectories(path, set.Requested, set.Resolved)
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.Exists || !PathsEqual(evidence.ResolvedPath, resolvedPath) || !PathsEqual(evidence.NearestExistingPath, resolvedPath) || !PathsEqual(evidence.RequestedPath, path) {
		t.Fatalf("unexpected existing evidence: %#v", evidence)
	}
}

func TestValidatePathEvidenceRejectsMissingChildBelowFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	set, err := NormalizeAllowedDirectorySet([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ValidatePathEvidenceWithAllowedDirectories(filepath.Join(file, "child"), set.Requested, set.Resolved)
	if err == nil {
		t.Fatal("missing child below regular file was accepted")
	}
}
