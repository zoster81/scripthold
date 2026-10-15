package releasearchive

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func flatExpected(name string, body []byte) ExpectedFile {
	sum := sha256.Sum256(body)
	return ExpectedFile{Name: name, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}
}

func flatFixture(t *testing.T) (string, []ExpectedFile) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	parts := []struct{ name, body string }{
		{"checksums.txt", "opaque bytes 1, not a package"},
		{"scripthold_linux_amd64", "opaque bytes 2, not a binary"},
	}
	files := make([]ExpectedFile, 0, len(parts))
	for _, part := range parts {
		if err := os.WriteFile(filepath.Join(dir, part.name), []byte(part.body), 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, flatExpected(part.name, []byte(part.body)))
	}
	return dir, files
}

func TestPortableStagingNameRejectsWindowsAndUnixAliases(t *testing.T) {
	for _, name := range []string{
		"", ".", "..", "CON", "con.txt", "PRN", "NUL.txt",
		"AUX", "COM1.txt", "LPT9", "file.", "file ",
		"RUNNER~1", "../file", "sub/file", "sub\\file",
		"C:\\stage\\file", "data:stream", "a..b",
	} {
		if portableStagingName(name) {
			t.Errorf("accepted nonportable staging name %q", name)
		}
	}
	for _, name := range []string{
		"checksums.txt", "mcpb-checksums.txt", "scripthold_linux_amd64",
		"scripthold-v3.4.0-provenance.sigstore.json",
	} {
		if !portableStagingName(name) {
			t.Errorf("rejected valid staging basename %q", name)
		}
	}
}

func TestVerifyFlatDirectoryAcceptsOnlyExactByteAuthenticatedFiles(t *testing.T) {
	dir, expected := flatFixture(t)
	if err := verifyFlatDirectory(dir, expected); err != nil {
		t.Fatalf("valid private directory rejected: %v", err)
	}
	reordered := []ExpectedFile{expected[1], expected[0]}
	if err := verifyFlatDirectory(dir, reordered); err != nil {
		t.Fatalf("metadata order incorrectly mattered: %v", err)
	}
	for _, bad := range [][]ExpectedFile{
		nil, {}, expected[:1], {expected[0], expected[0]},
		{{Name: "../checksums.txt", Size: expected[0].Size, SHA256: expected[0].SHA256}, expected[1]},
		{{Name: "C:\\stage\\checksums.txt", Size: expected[0].Size, SHA256: expected[0].SHA256}, expected[1]},
		{{Name: "RUNNER~1", Size: expected[0].Size, SHA256: expected[0].SHA256}, expected[1]},
		{{Name: "CON.txt", Size: expected[0].Size, SHA256: expected[0].SHA256}, expected[1]},
		{{Name: "LPT1", Size: expected[0].Size, SHA256: expected[0].SHA256}, expected[1]},
		{{Name: "checksums.txt.", Size: expected[0].Size, SHA256: expected[0].SHA256}, expected[1]},
		{{Name: expected[0].Name, Size: 0, SHA256: expected[0].SHA256}, expected[1]},
		{{Name: expected[0].Name, Size: expected[0].Size, SHA256: strings.ToUpper(expected[0].SHA256)}, expected[1]},
	} {
		if err := verifyFlatDirectory(dir, bad); err == nil {
			t.Fatalf("accepted invalid release inventory: %+v", bad)
		}
	}
}

func TestVerifyFlatDirectoryRejectsMissingExtraAndWrongBytes(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		dir, expected := flatFixture(t)
		if err := os.Remove(filepath.Join(dir, expected[0].Name)); err != nil {
			t.Fatal(err)
		}
		if err := verifyFlatDirectory(dir, expected); err == nil {
			t.Fatal("missing file accepted")
		}
	})
	t.Run("extra", func(t *testing.T) {
		dir, expected := flatFixture(t)
		if err := os.WriteFile(filepath.Join(dir, "unapproved"), []byte("extra"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := verifyFlatDirectory(dir, expected); err == nil {
			t.Fatal("extra file accepted")
		}
	})
	t.Run("altered bytes same length", func(t *testing.T) {
		dir, expected := flatFixture(t)
		if err := os.WriteFile(filepath.Join(dir, expected[0].Name), make([]byte, expected[0].Size), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := verifyFlatDirectory(dir, expected); err == nil {
			t.Fatal("same-size corruption accepted")
		}
	})
	t.Run("subdirectory", func(t *testing.T) {
		dir, expected := flatFixture(t)
		if err := os.Remove(filepath.Join(dir, expected[0].Name)); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(dir, expected[0].Name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := verifyFlatDirectory(dir, expected); err == nil {
			t.Fatal("directory-as-asset accepted")
		}
	})
}

func TestVerifyFlatDirectoryRejectsRealLinksAndRootAliases(t *testing.T) {
	t.Run("hardlink", func(t *testing.T) {
		dir, expected := flatFixture(t)
		outside := filepath.Join(t.TempDir(), "linked")
		if err := os.Link(filepath.Join(dir, expected[0].Name), outside); err != nil {
			t.Skipf("this filesystem does not support creating a hard link: %v", err)
		}
		if err := verifyFlatDirectory(dir, expected); err == nil {
			t.Fatal("hard-linked staging asset accepted")
		}
	})
	t.Run("symlink leaf", func(t *testing.T) {
		dir, expected := flatFixture(t)
		target := filepath.Join(dir, expected[1].Name)
		if err := os.Remove(filepath.Join(dir, expected[0].Name)); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, expected[0].Name)); err != nil {
			t.Skipf("symlink creation unsupported on this host: %v", err)
		}
		if err := verifyFlatDirectory(dir, expected); err == nil {
			t.Fatal("symlink accepted")
		}
	})
	t.Run("linked root", func(t *testing.T) {
		dir, expected := flatFixture(t)
		alias := filepath.Join(t.TempDir(), "alias")
		if err := os.Symlink(dir, alias); err != nil {
			t.Skipf("symlink creation unsupported on this host: %v", err)
		}
		if err := verifyFlatDirectory(alias, expected); err == nil {
			t.Fatal("symbolic staging root accepted")
		}
	})
}

func TestVerifyFlatDirectoryRequiresPrivateLinuxDirectory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux owner and permission policy is checked on the Linux runner")
	}
	dir, expected := flatFixture(t)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := verifyFlatDirectory(dir, expected); err == nil {
		t.Fatal("directory accessible to another user accepted")
	}
}

func TestVerifyFlatDirectoryRequiresLinuxRunner(t *testing.T) {
	dir, files := flatFixture(t)
	err := VerifyFlatDirectory(dir, files)
	if runtime.GOOS == "linux" && err != nil {
		t.Fatalf("Linux-private staging rejected: %v", err)
	}
	if runtime.GOOS != "linux" && err == nil {
		t.Fatal("non-Linux runner received publication authority")
	}
}
