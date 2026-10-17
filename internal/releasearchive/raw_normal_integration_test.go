package releasearchive

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// This exercises the original-byte handoff into a future release job. Its ZIP
// fixture is an ordinary synthetic archive, never an MCPB or release output.
func syntheticNormalZIP(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	file, err := writer.Create("original.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("preserve original compressed bytes")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestRawNormalFifteenOriginalsStageAndVerifyUnchanged(t *testing.T) {
	sourceDir := t.TempDir()
	stageDir, err := createPrivateRawStagingDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	zipOriginal := syntheticNormalZIP(t)
	names := []string{
		"scripthold_windows_amd64.exe", "scripthold_windows_arm64.exe",
		"scripthold_linux_amd64", "scripthold_linux_arm64",
		"scripthold_darwin_amd64", "scripthold_darwin_arm64",
		"scripthold_windows_amd64.zip", "scripthold_windows_arm64.zip",
		"scripthold_linux_amd64.tar.gz", "scripthold_linux_arm64.tar.gz",
		"scripthold_darwin_amd64.tar.gz", "scripthold_darwin_arm64.tar.gz",
		"checksums.txt", "scripthold-v3.4.0-provenance.intoto.jsonl",
		"scripthold-v3.4.0-provenance.sigstore.json",
	}
	if len(names) != 15 {
		t.Fatal("fixture must cover exactly 15 normal originals")
	}
	staged := make([]ExpectedFile, 0, len(names))
	for index, name := range names {
		original := []byte(fmt.Sprintf("independent synthetic original %02d: %s", index+1, name))
		if index == 6 || index == 7 {
			original = zipOriginal
		}
		if err := os.WriteFile(filepath.Join(sourceDir, name), original, 0o600); err != nil {
			t.Fatal(err)
		}
		originalDigest := sha256.Sum256(original)
		expected := ExpectedFile{Name: name, Size: int64(len(original)), SHA256: hex.EncodeToString(originalDigest[:])}
		physicalName := fmt.Sprintf("raw-normal-9001-2-%02d-%s", index+1, name)
		if err := stageVerifiedSourceFile(sourceDir, name, stageDir, physicalName, expected); err != nil {
			t.Fatalf("stage original %q: %v", name, err)
		}
		got, err := os.ReadFile(filepath.Join(stageDir, physicalName))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, original) {
			t.Fatalf("original bytes changed for %q", name)
		}
		staged = append(staged, ExpectedFile{Name: physicalName, Size: expected.Size, SHA256: expected.SHA256})
	}
	if err := verifyFlatDirectory(stageDir, staged); err != nil {
		t.Fatalf("complete normal handoff rejected: %v", err)
	}
	// A raw .zip is one preserved physical file, not an extracted directory.
	zipIndex := 6
	zipPath := filepath.Join(stageDir, staged[zipIndex].Name)
	zipBytes, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil || len(archive.File) != 1 || archive.File[0].Name != "original.txt" {
		t.Fatalf("raw ZIP changed during staging: %v", err)
	}
	reader, err := archive.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(reader)
	if closeErr := reader.Close(); err != nil || closeErr != nil {
		t.Fatalf("ZIP member unreadable: read=%v close=%v", err, closeErr)
	}
	t.Run("rerun cannot replace a previously verified original", func(t *testing.T) {
		first := staged[0]
		expected := ExpectedFile{Name: names[0], Size: first.Size, SHA256: first.SHA256}
		if err := stageVerifiedSourceFile(sourceDir, names[0], stageDir, first.Name, expected); err == nil {
			t.Fatal("accepted replacement of an existing raw original")
		}
		if err := verifyFlatDirectory(stageDir, staged); err != nil {
			t.Fatalf("failed duplicate staging modified the original set: %v", err)
		}
	})
	t.Run("later producer edit does not change verified handoff copies", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(sourceDir, names[0]), []byte("producer changed"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := verifyFlatDirectory(stageDir, staged); err != nil {
			t.Fatalf("staged snapshot unexpectedly depends on producer's later pathname: %v", err)
		}
	})
	// The next workflow step cannot treat a removed, extra or corrupted source
	// as a valid 15-file handoff even if all earlier copies succeeded.
	t.Run("missing staged original", func(t *testing.T) {
		if err := os.Remove(zipPath); err != nil {
			t.Fatal(err)
		}
		if err := verifyFlatDirectory(stageDir, staged); err == nil {
			t.Fatal("accepted missing original ZIP")
		}
		if err := os.WriteFile(zipPath, zipBytes, 0o600); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("extra staged file", func(t *testing.T) {
		extra := filepath.Join(stageDir, "untrusted")
		if err := os.WriteFile(extra, []byte("extra"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := verifyFlatDirectory(stageDir, staged); err == nil {
			t.Fatal("accepted an undeclared transfer file")
		}
		if err := os.Remove(extra); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("corruption after source verification", func(t *testing.T) {
		broken := append([]byte(nil), zipBytes...)
		broken[0] ^= 0xff
		if err := os.WriteFile(zipPath, broken, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := verifyFlatDirectory(stageDir, staged); err == nil {
			t.Fatal("accepted altered original ZIP after staging")
		}
	})
}
