//go:build linux || darwin

package taskstore

import (
	"os"
	"testing"
)

func TestOpenRejectsAndDoesNotRepairPermissiveUnixMode(t *testing.T) {
	store := newTestStore(t)
	if err := os.Chmod(store.root, 0o755); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(store.root, nil, store.limits)
	if opened != nil || err == nil {
		t.Fatal("Open accepted a permissive task-store mode")
	}
	info, err := os.Stat(store.root)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("Open changed mode to %04o", info.Mode().Perm())
	}
}
