package updater

import (
	"path/filepath"
	"testing"
)

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatalf("resolve temporary directory: %v", err)
	}
	return filepath.Clean(resolved)
}
