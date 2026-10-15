package releasearchive

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

func TestCleanupCreatedRawStagingFileRemovesOnlyOriginalObject(t *testing.T) {
	t.Run("original", func(t *testing.T) {
		dir := t.TempDir()
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		file, err := root.OpenFile("raw-good", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		info, err := file.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if err := removeCreatedStagingFile(root, "raw-good", info); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "raw-good")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owned failed staging file remained: %v", err)
		}
	})
	t.Run("replaced path", func(t *testing.T) {
		dir := t.TempDir()
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		file, err := root.OpenFile("raw-good", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		created, err := file.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(dir, "raw-good"), filepath.Join(dir, "moved")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "raw-good"), []byte("replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := removeCreatedStagingFile(root, "raw-good", created); err == nil {
			t.Fatal("cleanup silently deleted a replacement object")
		}
		got, err := os.ReadFile(filepath.Join(dir, "raw-good"))
		if err != nil || string(got) != "replacement" {
			t.Fatalf("cleanup damaged replacement: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "moved")); err != nil {
			t.Fatalf("cleanup deleted original after rename: %v", err)
		}
	})
}

func TestCleanupCreatedRawStagingDirectoryPreservesReplacement(t *testing.T) {
	parent := t.TempDir()
	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	stage, err := createPrivateRawStagingDirectory(parent)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(stage)
	created, err := root.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(stage, filepath.Join(parent, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := removeCreatedStagingDirectory(root, name, created); err == nil {
		t.Fatal("cleanup deleted a replacement staging directory")
	}
	if info, err := os.Lstat(stage); err != nil || !info.IsDir() {
		t.Fatalf("replacement directory was removed: %v", err)
	}
	if info, err := os.Lstat(filepath.Join(parent, "moved")); err != nil || !info.IsDir() {
		t.Fatalf("original directory disappeared: %v", err)
	}
	if err := root.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := root.Remove("moved"); err != nil {
		t.Fatal(err)
	}
}

func TestCreatePrivateRawStagingDirectoryAnchorsAndSeparatesEachRun(t *testing.T) {
	parent := t.TempDir()
	first, err := createPrivateRawStagingDirectory(parent)
	if err != nil {
		t.Fatal(err)
	}
	second, err := createPrivateRawStagingDirectory(parent)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || filepath.Dir(first) != parent || filepath.Dir(second) != parent {
		t.Fatalf("unexpected staging paths: %q, %q", first, second)
	}
	for _, path := range []string{first, second} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("staging root is not a real directory: %q: %v", path, err)
		}
		entries, err := os.ReadDir(path)
		if err != nil || len(entries) != 0 {
			t.Fatalf("staging must start empty: %q: %v", path, err)
		}
		if runtime.GOOS == "linux" {
			if err := verifyPrivateStagingRoot(info); err != nil {
				t.Fatalf("new staging directory lacks owner-only mode: %v", err)
			}
		}
	}
}

func TestCreatePrivateRawStagingDirectoryRejectsInvalidParents(t *testing.T) {
	parent := t.TempDir()
	ordinary := filepath.Join(parent, "file")
	if err := os.WriteFile(ordinary, []byte("not a folder"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", ".", "relative", ordinary, filepath.Join(parent, "missing")} {
		if _, err := createPrivateRawStagingDirectory(path); err == nil {
			t.Fatalf("untrusted or unusable staging parent %q accepted", path)
		}
	}
}

func TestCreatePrivateRawStagingDirectoryPermitsTrustedParentAlias(t *testing.T) {
	parent := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(parent, alias); err != nil {
		t.Skipf("system symlink alias creation unavailable: %v", err)
	}
	stage, err := createPrivateRawStagingDirectory(alias)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(stage); err != nil || !info.IsDir() {
		t.Fatalf("fresh stage under trusted alias missing: %v", err)
	}
}

func TestCreatePrivateRawStagingDirectoryRequiresLinuxForPublication(t *testing.T) {
	path, err := CreatePrivateRawStagingDirectory(t.TempDir())
	if runtime.GOOS == "linux" {
		if err != nil || path == "" {
			t.Fatalf("Linux publication staging unexpectedly refused: %v", err)
		}
	} else if err == nil || path != "" {
		t.Fatal("non-Linux platform was allowed to create a publication staging root")
	}
}

func TestStageVerifiedSourceFileCreatesOnlyExclusiveDigestMatchedRawCopy(t *testing.T) {
	parent := t.TempDir()
	stage, err := createPrivateRawStagingDirectory(parent)
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	contents := []byte("synthetic source bytes, not a binary or package")
	if err := os.WriteFile(filepath.Join(source, "scripthold"), contents, 0o600); err != nil {
		t.Fatal(err)
	}
	name := "scripthold-raw-normal-9001-2-scripthold_linux_amd64"
	expected := flatExpected("scripthold_linux_amd64", contents)
	if err := stageVerifiedSourceFile(source, "scripthold", stage, name, expected); err != nil {
		t.Fatal(err)
	}
	staged, err := os.ReadFile(filepath.Join(stage, name))
	if err != nil || !bytes.Equal(staged, contents) {
		t.Fatalf("staged bytes differed: %v", err)
	}
	if runtime.GOOS == "linux" {
		if info, err := os.Lstat(filepath.Join(stage, name)); err != nil ||
			info.Mode().Perm() != 0o600 {
			t.Fatalf("staged file not mode 0600: %v", err)
		}
		if err := VerifyFlatDirectory(stage, []ExpectedFile{flatExpected(name, contents)}); err != nil {
			t.Fatalf("final file verifier rejected newly staged content: %v", err)
		}
	}
	if err := stageVerifiedSourceFile(source, "scripthold", stage, name, expected); err == nil {
		t.Fatal("existing staged file overwritten")
	}
	still, err := os.ReadFile(filepath.Join(stage, name))
	if err != nil || !bytes.Equal(still, contents) {
		t.Fatalf("existing output was modified: %v", err)
	}
}

func TestStageVerifiedSourceFileFailsClosedAndLeavesNoOutput(t *testing.T) {
	cases := []struct {
		name, src, dest string
		body, expected  string
	}{
		{"digest mismatch", "scripthold", "raw-good", "wrong content", "source content"},
		{"wrong length", "scripthold", "raw-good", "too large", "short"},
		{"source alias", "RUNNER~1", "raw-good", "source content", "source content"},
		{"destination traversal", "scripthold", "../escape", "source content", "source content"},
		{"destination short alias", "scripthold", "RUNNER~1", "source content", "source content"},
		{"destination windows drive", "scripthold", "C:\\evil", "source content", "source content"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := t.TempDir()
			stage, err := createPrivateRawStagingDirectory(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if tc.src == "scripthold" {
				if err := os.WriteFile(filepath.Join(source, tc.src), []byte(tc.body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			expected := flatExpected("scripthold_linux_amd64", []byte(tc.expected))
			if err := stageVerifiedSourceFile(source, tc.src, stage, tc.dest, expected); err == nil {
				t.Fatal("invalid stage succeeded")
			}
			entries, err := os.ReadDir(stage)
			if err != nil || len(entries) != 0 {
				t.Fatalf("stage failed without removing its failed output: %v", err)
			}
		})
	}
}

func TestStageVerifiedSourceFileRejectsLinkedSourceAndExistingDest(t *testing.T) {
	t.Run("source hardlink", func(t *testing.T) {
		source := t.TempDir()
		target := filepath.Join(source, "scripthold")
		content := []byte("safe synthetic payload")
		if err := os.WriteFile(target, content, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(target, filepath.Join(t.TempDir(), "alias")); err != nil {
			t.Skipf("hardlink unsupported: %v", err)
		}
		stage, err := createPrivateRawStagingDirectory(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if err := stageVerifiedSourceFile(source, "scripthold", stage, "raw-good",
			flatExpected("scripthold_linux_amd64", content)); err == nil {
			t.Fatal("hardlinked source accepted")
		}
	})
	t.Run("preexisting destination", func(t *testing.T) {
		source := t.TempDir()
		stage, err := createPrivateRawStagingDirectory(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		content := []byte("trusted source bytes")
		if err := os.WriteFile(filepath.Join(source, "scripthold"), content, 0o600); err != nil {
			t.Fatal(err)
		}
		const name = "raw-good"
		if err := os.WriteFile(filepath.Join(stage, name), []byte("prior"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := stageVerifiedSourceFile(source, "scripthold", stage, name,
			flatExpected("scripthold_linux_amd64", content)); err == nil {
			t.Fatal("existing destination overwritten")
		}
		previous, err := os.ReadFile(filepath.Join(stage, name))
		if err != nil || string(previous) != "prior" {
			t.Fatalf("existing destination was modified: %v", err)
		}
	})
}

func TestCreatePrivateRawStagingDirectoryConcurrentCallersNeverReuseName(t *testing.T) {
	const workers = 16
	parent := t.TempDir()
	results := make(chan string, workers)
	failures := make(chan error, workers)
	for range workers {
		go func() {
			path, err := createPrivateRawStagingDirectory(parent)
			if err != nil {
				failures <- err
				return
			}
			results <- path
		}()
	}
	seen := make(map[string]bool, workers)
	for range workers {
		select {
		case path := <-results:
			if seen[path] {
				t.Errorf("concurrent callers reused private staging path: %s", path)
			}
			seen[path] = true
		case err := <-failures:
			t.Error(err)
		}
	}
	if len(seen) != workers {
		t.Fatalf("expected %d distinct private stages; got %d", workers, len(seen))
	}
}

func TestStageVerifiedSourceFileRejectsRealSourceAndDestinationSymlinks(t *testing.T) {
	t.Run("source symlink", func(t *testing.T) {
		source := t.TempDir()
		stage, err := createPrivateRawStagingDirectory(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		body := []byte("synthetic source alias bytes")
		target := filepath.Join(t.TempDir(), "outside")
		if err := os.WriteFile(target, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(source, "scripthold")); err != nil {
			t.Skipf("symlink creation unsupported on host: %v", err)
		}
		if err := stageVerifiedSourceFile(source, "scripthold", stage, "raw-good",
			flatExpected("scripthold_linux_amd64", body)); err == nil {
			t.Fatal("symbolic source leaf was accepted")
		}
		if entries, err := os.ReadDir(stage); err != nil || len(entries) != 0 {
			t.Fatalf("source symlink failure created staged content: %v", err)
		}
	})
	t.Run("existing destination symlink", func(t *testing.T) {
		source := t.TempDir()
		stage, err := createPrivateRawStagingDirectory(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		body := []byte("synthetic source bytes")
		if err := os.WriteFile(filepath.Join(source, "scripthold"), body, 0o600); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(t.TempDir(), "outside")
		if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(stage, "raw-good")); err != nil {
			t.Skipf("symlink creation unsupported on host: %v", err)
		}
		if err := stageVerifiedSourceFile(source, "scripthold", stage, "raw-good",
			flatExpected("scripthold_linux_amd64", body)); err == nil {
			t.Fatal("existing symlink was overwritten")
		}
		got, err := os.ReadFile(target)
		if err != nil || string(got) != "untouched" {
			t.Fatalf("outside symlink target changed: %v", err)
		}
	})
}

func TestStageVerifiedSourceFileRequiresLinuxForPublication(t *testing.T) {
	source := t.TempDir()
	stage := t.TempDir()
	body := []byte("synthetic only")
	if err := os.WriteFile(filepath.Join(source, "scripthold"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	err := StageVerifiedSourceFile(source, "scripthold", stage, "raw-good",
		flatExpected("scripthold_linux_amd64", body))
	if runtime.GOOS == "linux" && err != nil {
		t.Fatalf("Linux stage refused: %v", err)
	}
	if runtime.GOOS != "linux" && err == nil {
		t.Fatal("non-Linux source staging was authorized")
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
