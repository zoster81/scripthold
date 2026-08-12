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

	evidence, err := ValidatePathEvidenceWithAllowedDirectories(requested, set.Requested, set.Resolved)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Exists {
		t.Fatal("missing path was reported as existing")
	}
	if !PathsEqual(evidence.NearestExistingPath, parent) {
		t.Fatalf("nearest existing path = %q, want %q", evidence.NearestExistingPath, parent)
	}
	if !PathsEqual(evidence.ResolvedPath, requested) {
		t.Fatalf("resolved projected path = %q, want %q", evidence.ResolvedPath, requested)
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
	evidence, err := ValidatePathEvidenceWithAllowedDirectories(path, set.Requested, set.Resolved)
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.Exists || !PathsEqual(evidence.ResolvedPath, path) || !PathsEqual(evidence.NearestExistingPath, path) {
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
