// Package releasearchive verifies GitHub Actions staging ZIP contents without
// extracting files, executing binaries, or trusting archive paths.
package releasearchive

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	maxArchiveBytes = int64(4) << 30
	maxAssetBytes   = int64(2) << 30
	maxFiles        = 22
)

// ExpectedFile is an independently authenticated uncompressed release asset.
// Callers must supply exact names, sizes and digests from the validated stage manifest.
type ExpectedFile struct {
	Name   string
	Size   int64
	SHA256 string
}

// Verify checks the exact set of ZIP entries, their ordinary-file types,
// lengths, CRC-32 and authenticated uncompressed SHA-256 values. It neither
// writes to disk nor interprets executable permissions as publication authority.
// The caller must separately authenticate the compressed ZIP bytes and keep its
// ReaderAt stable throughout this operation.
func Verify(source io.ReaderAt, archiveSize int64, files []ExpectedFile) error {
	if source == nil || archiveSize <= 0 || archiveSize > maxArchiveBytes {
		return errors.New("invalid staging archive source or size")
	}
	if len(files) == 0 || len(files) > maxFiles {
		return errors.New("invalid expected ZIP entry count")
	}
	expected := make(map[string]ExpectedFile, len(files))
	var total int64
	for _, file := range files {
		if !safeFlatName(file.Name) || file.Size <= 0 || file.Size > maxAssetBytes ||
			!validDigest(file.SHA256) {
			return fmt.Errorf("invalid expected release asset %q", file.Name)
		}
		if _, exists := expected[file.Name]; exists {
			return fmt.Errorf("duplicate expected release asset %q", file.Name)
		}
		total += file.Size
		if total > maxArchiveBytes {
			return errors.New("uncompressed staging total exceeds limit")
		}
		expected[file.Name] = file
	}

	reader, err := zip.NewReader(source, archiveSize)
	if err != nil {
		return fmt.Errorf("open staging ZIP: %w", err)
	}
	if len(reader.File) != len(expected) {
		return errors.New("staging ZIP entry count mismatch")
	}
	seen := make(map[string]bool, len(files))
	for _, member := range reader.File {
		file, ok := expected[member.Name]
		if !ok || !safeFlatName(member.Name) {
			return fmt.Errorf("unexpected staging ZIP entry %q", member.Name)
		}
		if seen[member.Name] {
			return fmt.Errorf("duplicate staging ZIP entry %q", member.Name)
		}
		seen[member.Name] = true
		mode := member.Mode()
		if !mode.IsRegular() || mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 ||
			member.Flags&1 != 0 {
			return fmt.Errorf("unsafe ZIP entry type, permissions or encryption: %q", member.Name)
		}
		if member.Method != zip.Store && member.Method != zip.Deflate {
			return fmt.Errorf("unsupported ZIP compression for %q", member.Name)
		}
		if member.UncompressedSize64 != uint64(file.Size) {
			return fmt.Errorf("staging entry size mismatch: %q", member.Name)
		}
		body, err := member.Open()
		if err != nil {
			return fmt.Errorf("open ZIP entry %q: %w", member.Name, err)
		}
		sum := sha256.New()
		// Reading one byte past the expected length reaches EOF so that
		// archive/zip verifies the CRC-32 before the comparison below.
		n, readErr := io.Copy(sum, io.LimitReader(body, file.Size+1))
		closeErr := body.Close()
		if readErr != nil || closeErr != nil {
			return fmt.Errorf("read or close ZIP entry %q: %w", member.Name, errors.Join(readErr, closeErr))
		}
		if n != file.Size {
			return fmt.Errorf("ZIP entry data length mismatch: %q", member.Name)
		}
		if hex.EncodeToString(sum.Sum(nil)) != file.SHA256 {
			return fmt.Errorf("staging entry digest mismatch: %q", member.Name)
		}
	}
	return nil
}

func safeFlatName(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") || strings.Contains(name, "..") {
		return false
	}
	for _, ch := range name {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '.' || ch == '_' || ch == '-') {
			return false
		}
	}
	return true
}

func validDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, ch := range s {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}
