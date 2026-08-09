package filetoolsserver

import (
	"path/filepath"
	"testing"
)

func canonicalServerTestDir(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	resolved, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatalf("resolve server test directory %q: %v", base, err)
	}
	return filepath.Clean(resolved)
}
