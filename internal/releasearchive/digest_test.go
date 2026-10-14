package releasearchive

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"testing"
)

func TestVerifyWithDigestChecksOriginalZIPAndContents(t *testing.T) {
	files := []struct {
		name, content string
		mode          os.FileMode
	}{
		{"checksums.txt", "synthetic checksum manifest", 0o644},
		{"scripthold_linux_amd64", "synthetic binary bytes", 0o755},
	}
	archive := archiveFixture(t, files)
	sum := sha256.Sum256(archive)
	originalDigest := hex.EncodeToString(sum[:])
	expected := []ExpectedFile{fileExpect(files[0].name, files[0].content),
		fileExpect(files[1].name, files[1].content)}
	if err := VerifyWithDigest(bytes.NewReader(archive), int64(len(archive)),
		originalDigest, expected); err != nil {
		t.Fatalf("rejected matching original ZIP archive: %v", err)
	}
	for _, tc := range []struct {
		name, digest string
		size         int64
		data         []byte
		entries      []ExpectedFile
	}{
		{"wrong original ZIP digest", strings.Repeat("a", 64), int64(len(archive)), archive, expected},
		{"missing digest", "", int64(len(archive)), archive, expected},
		{"uppercase digest", strings.ToUpper(originalDigest), int64(len(archive)), archive, expected},
		{"short digest", originalDigest[:63], int64(len(archive)), archive, expected},
		{"truncated original ZIP", originalDigest, int64(len(archive) - 1), archive[:len(archive)-1], expected},
		{"excess ZIP payload", originalDigest, int64(len(archive) + 1), append(bytes.Clone(archive), 0), expected},
		{"wrong original asset digest", originalDigest, int64(len(archive)), archive,
			[]ExpectedFile{fileExpect(files[0].name, "not the content"), expected[1]}},
		{"archive length above cap", originalDigest, maxArchiveBytes + 1, archive, expected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := VerifyWithDigest(bytes.NewReader(tc.data), tc.size, tc.digest, tc.entries); err == nil {
				t.Fatal("accepted an invalid original archive or original file digest")
			}
		})
	}
	if err := VerifyWithDigest(nil, int64(len(archive)), originalDigest, expected); err == nil {
		t.Fatal("nil source accepted")
	}
}

type truncatedReaderAt struct{ data []byte }

func (r truncatedReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(r.data)-1) {
		return 0, io.EOF
	}
	available := r.data[off : len(r.data)-1]
	n := copy(p, available)
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func TestVerifyWithDigestRejectsIncompleteReaderAt(t *testing.T) {
	original := archiveFixture(t, []struct {
		name, content string
		mode          os.FileMode
	}{
		{"checksums.txt", "synthetic", 0o644},
	})
	sum := sha256.Sum256(original)
	if err := VerifyWithDigest(truncatedReaderAt{original}, int64(len(original)),
		hex.EncodeToString(sum[:]), []ExpectedFile{fileExpect("checksums.txt", "synthetic")}); err == nil {
		t.Fatal("accepted a source that cannot provide the declared original ZIP size")
	}
}
