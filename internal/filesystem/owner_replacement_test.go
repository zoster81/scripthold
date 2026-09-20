package filesystem

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestStagedReplacementRestrictOwnerOnlyBeforePublish(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "state.json")
	staged, err := StageReplacement(target, bytes.NewBufferString("{}"), 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Cleanup()
	if err := staged.RestrictOwnerOnly(); err != nil {
		t.Fatal(err)
	}
	missing := FileSnapshot{}
	if _, err := staged.Commit(ReplaceOptions{Mode: 0o600, Expected: &missing}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOwnerOnlyPath(target, false); err != nil {
		t.Fatal(err)
	}
}
