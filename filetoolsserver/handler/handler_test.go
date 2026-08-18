package handler

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/operation"
)

func TestNewHandler(t *testing.T) {
	dirs := []string{"/tmp", "/home"}
	h := NewHandler(dirs)

	if h == nil {
		t.Fatal("expected handler, got nil")
	}

	got := h.GetAllowedDirectories()
	if len(got) != len(dirs) {
		t.Errorf("expected %d dirs, got %d", len(dirs), len(got))
	}
}

func TestWithConfig(t *testing.T) {
	cfg := &config.Config{
		DefaultEncoding: "utf-8",
	}

	h := NewHandler([]string{"/tmp"}, WithConfig(cfg))

	if h.config != cfg {
		t.Error("expected config to be set via WithConfig option")
	}
}

func TestWithConfig_Nil(t *testing.T) {
	h := NewHandler([]string{"/tmp"}, WithConfig(nil))

	if h.config == nil {
		t.Error("config should not be nil when WithConfig(nil) is passed")
	}
}

func TestGetAllowedDirectories_ReturnsCopy(t *testing.T) {
	dirs := []string{"/tmp", "/home"}
	h := NewHandler(dirs)

	got := h.GetAllowedDirectories()
	got[0] = "/modified"

	// Original should be unchanged
	original := h.GetAllowedDirectories()
	if original[0] == "/modified" {
		t.Error("GetAllowedDirectories should return a copy, not the original slice")
	}
}

func TestUpdateAllowedDirectories(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})

	newDirs := []string{t.TempDir(), t.TempDir(), t.TempDir()}
	h.UpdateAllowedDirectories(newDirs)
	_, want := normalizeAllowedDirectorySets(newDirs)

	got := h.GetAllowedDirectories()
	if len(got) != len(want) {
		t.Fatalf("expected %d dirs, got %d", len(want), len(got))
	}

	for i, d := range want {
		if got[i] != d {
			t.Errorf("dir[%d] = %q, want canonical %q", i, got[i], d)
		}
	}
}

func TestUpdateAllowedDirectories_Empty(t *testing.T) {
	h := NewHandler([]string{"/tmp", "/home"})

	h.UpdateAllowedDirectories([]string{})

	got := h.GetAllowedDirectories()
	if len(got) != 0 {
		t.Errorf("expected 0 dirs, got %d", len(got))
	}
}

func TestHasConfiguredDirectoriesIgnoresDynamicRoots(t *testing.T) {
	dynamic := NewHandler(nil)
	if dynamic.HasConfiguredDirectories() {
		t.Fatal("empty process unexpectedly reported configured directories")
	}
	dynamic.MergeAllowedDirectories([]string{t.TempDir()})
	if dynamic.HasConfiguredDirectories() {
		t.Fatal("dynamic roots became an authoritative process baseline")
	}

	configured := NewHandler([]string{t.TempDir()})
	if !configured.HasConfiguredDirectories() {
		t.Fatal("configured process did not report its authoritative baseline")
	}
}

func TestProtectedDirectoriesRemainInaccessible(t *testing.T) {
	publicRoot := t.TempDir()
	protected := filepath.Join(publicRoot, "internal-backups")
	h := NewHandler([]string{publicRoot}, WithProtectedDirectories([]string{protected}))

	_, err := h.validatePath(filepath.Join(protected, "store.json"))
	if err == nil {
		t.Fatal("protected path unexpectedly validated")
	}
	if operation.KindOf(err) != operation.KindAccessDenied {
		t.Fatalf("error kind = %s, want access_denied: %v", operation.KindOf(err), err)
	}
	if errors.Is(err, ErrPathRequired) {
		t.Fatalf("protected-path error was mapped as an input error: %v", err)
	}
}

func TestDynamicRootsCannotOverlapProtectedDirectories(t *testing.T) {
	protected := t.TempDir()
	h := NewHandler(nil, WithProtectedDirectories([]string{protected}))

	if got := h.MergeAllowedDirectories([]string{protected}); len(got) != 0 {
		t.Fatalf("protected dynamic root was retained: %v", got)
	}
	if got := h.MergeAllowedDirectories([]string{filepath.Dir(protected)}); len(got) != 0 {
		t.Fatalf("ancestor of protected root was retained: %v", got)
	}
}
