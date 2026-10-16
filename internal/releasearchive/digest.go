package releasearchive

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// VerifyWithDigest authenticates the original ZIP's compressed bytes against
// an independently trusted digest, then checks its structure and uncompressed
// files. It hashes again after verification to detect ordinary intervening
// changes. Its caller MUST hold a stable, authorized, immutable ReaderAt:
// separate passes cannot protect against an adversary able to replace bytes
// between reads. This operation does not open, create or extract filesystem files.
func VerifyWithDigest(source io.ReaderAt, archiveSize int64, originalSHA256 string, files []ExpectedFile) error {
	if source == nil || archiveSize <= 0 || archiveSize > maxArchiveBytes {
		return errors.New("invalid original staging archive source or size")
	}
	if !validDigest(originalSHA256) {
		return errors.New("invalid original staging archive digest")
	}
	if err := verifyOriginalDigest(source, archiveSize, originalSHA256); err != nil {
		return fmt.Errorf("authenticate original staging ZIP: %w", err)
	}
	if err := Verify(source, archiveSize, files); err != nil {
		return err
	}
	if err := verifyOriginalDigest(source, archiveSize, originalSHA256); err != nil {
		return fmt.Errorf("recheck original staging ZIP: %w", err)
	}
	return nil
}

func verifyOriginalDigest(source io.ReaderAt, archiveSize int64, expected string) error {
	digest := sha256.New()
	size, err := io.Copy(digest, io.NewSectionReader(source, 0, archiveSize))
	if err != nil {
		return fmt.Errorf("read original staging ZIP: %w", err)
	}
	if size != archiveSize {
		return errors.New("original staging ZIP truncated")
	}
	if hex.EncodeToString(digest.Sum(nil)) != expected {
		return errors.New("original staging ZIP digest mismatch")
	}
	return nil
}
