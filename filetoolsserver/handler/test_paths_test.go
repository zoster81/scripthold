package handler

import (
	"path/filepath"
	"testing"
)

// canonicalHandlerTestDir keeps strict private-store fixtures off platform
// aliases such as macOS /var -> /private/var and Windows 8.3 paths.
func canonicalHandlerTestDir(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	resolved, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatalf("resolve handler test directory %q: %v", base, err)
	}
	return filepath.Clean(resolved)
}
