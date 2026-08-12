package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestMovePreparedNativeNoReplaceMovesFileAndDirectory(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []bool{false, true} {
		name := "file"
		if directory {
			name = "dir"
		}
		source := filepath.Join(root, name+"-source")
		destination := filepath.Join(root, name+"-destination")
		if directory {
			if err := os.Mkdir(source, 0o755); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(source, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
		sourceIdentity, err := CaptureObjectIdentity(source)
		if err != nil {
			t.Fatal(err)
		}
		parentIdentity, err := CaptureObjectIdentity(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := MovePreparedNativeNoReplace(source, destination, sourceIdentity, parentIdentity, parentIdentity); err != nil {
			t.Fatalf("move %s: %v", name, err)
		}
		if _, err := os.Stat(source); !os.IsNotExist(err) {
			t.Fatalf("source still exists: %v", err)
		}
		moved, err := CaptureObjectIdentity(destination)
		if err != nil || !sourceIdentity.Equal(moved) {
			t.Fatalf("moved identity = %#v / %v", moved, err)
		}
	}
}

func TestMovePreparedNativeNoReplaceRejectsDestinationRaceAndReplacement(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	destination := filepath.Join(root, "destination.txt")
	if err := os.WriteFile(source, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	sourceIdentity, _ := CaptureObjectIdentity(source)
	parentIdentity, _ := CaptureObjectIdentity(root)
	if err := os.WriteFile(destination, []byte("racer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MovePreparedNativeNoReplace(source, destination, sourceIdentity, parentIdentity, parentIdentity); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("destination race error = %v, want ErrDestinationExists", err)
	}
	if err := os.Remove(destination); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(root, "replacement.txt")
	if err := os.WriteFile(replacement, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, source); err != nil {
		t.Fatal(err)
	}
	if err := MovePreparedNativeNoReplace(source, destination, sourceIdentity, parentIdentity, parentIdentity); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("replacement error = %v, want CONFLICT", err)
	}
}

func TestMovePreparedNativeNoReplaceRejectsCrossVolumeBeforeMove(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	destination := filepath.Join(root, "destination.txt")
	if err := os.WriteFile(source, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	sourceIdentity, _ := CaptureObjectIdentity(source)
	parentIdentity, _ := CaptureObjectIdentity(root)
	foreignParent := parentIdentity
	foreignParent.volumeKey += ":foreign"
	called := false
	err := movePreparedNativeNoReplace(source, destination, sourceIdentity, parentIdentity, foreignParent, func(string, string) error {
		called = true
		return nil
	})
	if operation.KindOf(err) != operation.KindUnsupported || called {
		t.Fatalf("cross-volume error = %v, moveCalled=%v", err, called)
	}
	if _, statErr := os.Stat(source); statErr != nil {
		t.Fatalf("source changed during cross-volume rejection: %v", statErr)
	}
}
