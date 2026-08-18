package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
	"golang.org/x/text/unicode/norm"
)

const fingerprintModeContentV1 = "content-v1"

// FingerprintOptions controls deterministic filesystem fingerprinting.
type FingerprintOptions struct {
	ResolvedAllowedDirs []string
	RespectGitignore    bool
	IncludeEntries      bool
	MaxEntries          int
	MaxEntryDetails     int
}

// FingerprintEntry is one canonical entry included in an aggregate fingerprint.
type FingerprintEntry struct {
	RootIndex int
	Path      string
	Type      string
	Size      int64
	SHA256    string
}

// FingerprintResult contains aggregate state and optional bounded entry details.
type FingerprintResult struct {
	Algorithm        string
	Mode             string
	Fingerprint      string
	RootCount        int
	FileCount        int
	DirectoryCount   int
	TotalBytes       int64
	Entries          []FingerprintEntry
	EntriesTruncated bool
}

// FingerprintRegularFileSnapshot returns the content-v1 aggregate fingerprint
// for one digest-bearing regular-file snapshot.
func FingerprintRegularFileSnapshot(snapshot FileSnapshot) (string, error) {
	if !snapshot.Exists || snapshot.Size < 0 {
		return "", operation.New(operation.KindInvalidInput, "fingerprint requires an existing regular-file snapshot")
	}
	digest, ok := snapshot.ContentDigest()
	if !ok {
		return "", operation.New(operation.KindInvalidInput, "fingerprint requires a content digest")
	}
	return fingerprintRegularFileAggregate(snapshot.Size, digest), nil
}

// FingerprintRegularFileData returns the content-v1 aggregate fingerprint for
// one regular file whose complete bytes are already available.
func FingerprintRegularFileData(data []byte) string {
	return fingerprintRegularFileAggregate(int64(len(data)), sha256.Sum256(data))
}

// CaptureRegularFileSnapshotBounded streams exactly the regular file size
// observed when the read session opens. It rejects oversized input before
// reading and reports practical concurrent changes as conflicts.
func CaptureRegularFileSnapshotBounded(ctx context.Context, path string, maxBytes int64) (snapshot FileSnapshot, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if maxBytes <= 0 {
		return FileSnapshot{}, operation.New(operation.KindInvalidInput, "maximum fingerprint file bytes must be positive")
	}
	if err := ctx.Err(); err != nil {
		return FileSnapshot{}, operation.Wrap(operation.KindCancelled, "fingerprint_regular_file", path, err)
	}
	session, err := OpenReadSession(path)
	if err != nil {
		return FileSnapshot{}, err
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil {
			err = errors.Join(err, operation.WrapFilesystem("close_fingerprint_file", path, closeErr))
		}
	}()
	if session.Size() > maxBytes {
		return FileSnapshot{}, operation.New(operation.KindLimit, fmt.Sprintf("file size %d exceeds limit %d", session.Size(), maxBytes))
	}
	if err := session.Start(0); err != nil {
		return FileSnapshot{}, err
	}

	buffer := make([]byte, 128*1024)
	remaining := session.Size()
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return FileSnapshot{}, operation.Wrap(operation.KindCancelled, "fingerprint_regular_file", path, err)
		}
		readSize := int64(len(buffer))
		if remaining < readSize {
			readSize = remaining
		}
		read, readErr := session.Read(buffer[:int(readSize)])
		remaining -= int64(read)
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return FileSnapshot{}, operation.Wrap(operation.KindConflict, "fingerprint_regular_file", path, ErrIncompleteRead)
			}
			return FileSnapshot{}, readErr
		}
		if read == 0 {
			return FileSnapshot{}, operation.Wrap(operation.KindConflict, "fingerprint_regular_file", path, ErrIncompleteRead)
		}
	}
	snapshot, err = session.Finish()
	if err != nil {
		if errors.Is(err, ErrConcurrentModification) || errors.Is(err, ErrIncompleteRead) {
			return FileSnapshot{}, operation.Wrap(operation.KindConflict, "fingerprint_regular_file", path, err)
		}
		return FileSnapshot{}, err
	}
	return snapshot, nil
}

// FingerprintRegularFilePathBounded returns the content-v1 fingerprint of one
// bounded, digest-bearing regular-file snapshot.
func FingerprintRegularFilePathBounded(ctx context.Context, path string, maxBytes int64) (string, error) {
	snapshot, err := CaptureRegularFileSnapshotBounded(ctx, path, maxBytes)
	if err != nil {
		return "", err
	}
	return FingerprintRegularFileSnapshot(snapshot)
}

// FingerprintRegularFileContentDigest converts a regular file's byte length and
// raw SHA-256 content digest into the same content-v1 fingerprint used elsewhere.
func FingerprintRegularFileContentDigest(size int64, contentSHA256 string) (string, error) {
	if size < 0 {
		return "", operation.New(operation.KindInvalidInput, "fingerprint size must not be negative")
	}
	decoded, err := hex.DecodeString(contentSHA256)
	if err != nil || len(decoded) != sha256.Size {
		return "", operation.New(operation.KindInvalidInput, "content digest must be 64 hexadecimal characters")
	}
	var digest [sha256.Size]byte
	copy(digest[:], decoded)
	return fingerprintRegularFileAggregate(size, digest), nil
}

func fingerprintRegularFileAggregate(size int64, digest [sha256.Size]byte) string {
	aggregate := sha256.New()
	writeFingerprintHeader(aggregate, 1, true)
	writeFingerprintRecord(aggregate, 0, ".", "file", size, digest)
	return hex.EncodeToString(aggregate.Sum(nil))
}

// FingerprintPaths hashes explicit regular files and directory roots using
// deterministic canonical records. A second complete pass must reproduce the
// same aggregate before success, detecting practical file and directory changes.
func FingerprintPaths(ctx context.Context, paths []string, options FingerprintOptions) (FingerprintResult, error) {
	first, err := fingerprintPathsOnce(ctx, paths, options)
	if err != nil {
		return FingerprintResult{}, err
	}
	return verifyFingerprintPaths(ctx, paths, options, first)
}

func verifyFingerprintPaths(ctx context.Context, paths []string, options FingerprintOptions, first FingerprintResult) (FingerprintResult, error) {
	verifyOptions := options
	verifyOptions.IncludeEntries = false
	verifyOptions.MaxEntryDetails = 0
	second, err := fingerprintPathsOnce(ctx, paths, verifyOptions)
	if err != nil {
		switch operation.KindOf(err) {
		case operation.KindCancelled, operation.KindSymlinkEscape:
			return FingerprintResult{}, err
		default:
			return FingerprintResult{}, operation.Wrap(operation.KindConflict, "fingerprint_verify", "", err)
		}
	}
	if !fingerprintResultsEquivalent(first, second) {
		return FingerprintResult{}, operation.New(operation.KindConflict, "fingerprint inputs changed during inspection")
	}
	return first, nil
}

func fingerprintPathsOnce(ctx context.Context, paths []string, options FingerprintOptions) (FingerprintResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return FingerprintResult{}, operation.Wrap(operation.KindCancelled, "fingerprint_paths", "", err)
	}
	if len(paths) == 0 {
		return FingerprintResult{}, operation.New(operation.KindInvalidInput, "at least one path is required")
	}
	if len(options.ResolvedAllowedDirs) == 0 {
		return FingerprintResult{}, operation.New(operation.KindInvalidInput, "resolved allowed directories are required")
	}
	if options.MaxEntries <= 0 {
		return FingerprintResult{}, operation.New(operation.KindInvalidInput, "max fingerprint entries must be positive")
	}
	if options.IncludeEntries && options.MaxEntryDetails <= 0 {
		return FingerprintResult{}, operation.New(operation.KindInvalidInput, "max fingerprint entry details must be positive when entries are requested")
	}

	result := FingerprintResult{
		Algorithm: "sha256",
		Mode:      fingerprintModeContentV1,
		RootCount: len(paths),
	}
	if options.IncludeEntries {
		result.Entries = make([]FingerprintEntry, 0, min(options.MaxEntryDetails, 64))
	}

	aggregate := sha256.New()
	writeFingerprintHeader(aggregate, len(paths), options.RespectGitignore)
	buffer := make([]byte, 128*1024)
	seenRoots := make(map[string]struct{}, len(paths))
	type entryKey struct {
		rootIndex int
		path      string
	}
	seenEntries := make(map[entryKey]struct{})
	entryCount := 0

	addEntry := func(rootIndex int, relativePath, entryType string, size int64, digest [sha256.Size]byte) error {
		canonicalPath := canonicalFingerprintPath(relativePath)
		key := entryKey{rootIndex: rootIndex, path: canonicalPath}
		if _, duplicate := seenEntries[key]; duplicate {
			return operation.New(operation.KindInvalidInput, fmt.Sprintf("multiple entries normalize to the same fingerprint path: %s", canonicalPath))
		}
		seenEntries[key] = struct{}{}
		entryCount++
		if entryCount > options.MaxEntries {
			return operation.New(operation.KindLimit, fmt.Sprintf("fingerprint entry count exceeds limit %d", options.MaxEntries))
		}
		if size < 0 || size > math.MaxInt64-result.TotalBytes {
			return operation.New(operation.KindLimit, "fingerprint byte count exceeds supported range")
		}
		writeFingerprintRecord(aggregate, rootIndex, canonicalPath, entryType, size, digest)
		switch entryType {
		case "file":
			result.FileCount++
			result.TotalBytes += size
		case "directory":
			result.DirectoryCount++
		}
		if options.IncludeEntries {
			if len(result.Entries) < options.MaxEntryDetails {
				detail := FingerprintEntry{RootIndex: rootIndex, Path: canonicalPath, Type: entryType, Size: size}
				if entryType == "file" {
					detail.SHA256 = hex.EncodeToString(digest[:])
				}
				result.Entries = append(result.Entries, detail)
			} else {
				result.EntriesTruncated = true
			}
		}
		return nil
	}

	for rootIndex, root := range paths {
		if err := ctx.Err(); err != nil {
			return FingerprintResult{}, operation.Wrap(operation.KindCancelled, "fingerprint_paths", root, err)
		}
		cleanRoot := filepath.Clean(root)
		resolvedRoot, safe := security.ResolvePathSafe(cleanRoot, options.ResolvedAllowedDirs)
		if !safe {
			if _, statErr := os.Lstat(cleanRoot); statErr != nil {
				return FingerprintResult{}, operation.WrapFilesystem("fingerprint_root", cleanRoot, statErr)
			}
			return FingerprintResult{}, operation.New(operation.KindSymlinkEscape, fmt.Sprintf("path resolves outside allowed directories: %s", root))
		}
		key := fingerprintPathKey(resolvedRoot)
		if _, duplicate := seenRoots[key]; duplicate {
			return FingerprintResult{}, operation.New(operation.KindInvalidInput, fmt.Sprintf("duplicate fingerprint root: %s", root))
		}
		seenRoots[key] = struct{}{}

		info, err := os.Stat(resolvedRoot)
		if err != nil {
			return FingerprintResult{}, operation.WrapFilesystem("fingerprint_root", cleanRoot, err)
		}
		if info.Mode().IsRegular() {
			digest, size, hashErr := fingerprintRegularFile(ctx, resolvedRoot, buffer)
			if hashErr != nil {
				return FingerprintResult{}, hashErr
			}
			if err := addEntry(rootIndex, ".", "file", size, digest); err != nil {
				return FingerprintResult{}, err
			}
			continue
		}
		if !info.IsDir() {
			return FingerprintResult{}, operation.New(operation.KindInvalidInput, fmt.Sprintf("path is not a regular file or directory: %s", root))
		}
		if strings.EqualFold(filepath.Base(resolvedRoot), ".git") {
			return FingerprintResult{}, operation.New(operation.KindInvalidInput, "fingerprinting a .git directory is not supported")
		}
		if err := addEntry(rootIndex, ".", "directory", 0, [sha256.Size]byte{}); err != nil {
			return FingerprintResult{}, err
		}

		walkErr := Walk(ctx, resolvedRoot, WalkOptions{
			ResolvedAllowedDirs: options.ResolvedAllowedDirs,
			RespectGitignore:    options.RespectGitignore,
			Exclude: func(entry Entry) bool {
				return entry.DirEntry.IsDir() && strings.EqualFold(entry.Name, ".git")
			},
			OnUnsafe: func(path string, _ int) error {
				if _, statErr := os.Lstat(path); statErr != nil {
					if errors.Is(statErr, os.ErrNotExist) {
						return operation.Wrap(operation.KindConflict, "fingerprint_walk", path, statErr)
					}
					return operation.WrapFilesystem("fingerprint_walk", path, statErr)
				}
				return operation.New(operation.KindSymlinkEscape, fmt.Sprintf("path resolves outside allowed directories: %s", path))
			},
			OnError: func(path string, _ int, err error) error {
				if errors.Is(err, os.ErrNotExist) {
					return operation.Wrap(operation.KindConflict, "fingerprint_walk", path, err)
				}
				return operation.WrapFilesystem("fingerprint_walk", path, err)
			},
		}, func(entry Entry) (WalkAction, error) {
			if err := ctx.Err(); err != nil {
				return WalkStop, operation.Wrap(operation.KindCancelled, "fingerprint_walk", entry.Path, err)
			}
			if entry.IsLink {
				return WalkContinue, nil
			}
			relative := entry.RelativePath
			if entry.DirEntry.IsDir() {
				return WalkContinue, addEntry(rootIndex, relative, "directory", 0, [sha256.Size]byte{})
			}
			entryInfo, statErr := os.Stat(entry.ResolvedPath)
			if statErr != nil {
				if errors.Is(statErr, os.ErrNotExist) {
					return WalkStop, operation.Wrap(operation.KindConflict, "fingerprint_stat", entry.Path, statErr)
				}
				return WalkStop, operation.WrapFilesystem("fingerprint_stat", entry.Path, statErr)
			}
			if !entryInfo.Mode().IsRegular() {
				return WalkContinue, nil
			}
			digest, size, hashErr := fingerprintRegularFile(ctx, entry.ResolvedPath, buffer)
			if hashErr != nil {
				return WalkStop, hashErr
			}
			return WalkContinue, addEntry(rootIndex, relative, "file", size, digest)
		})
		if walkErr != nil {
			if operation.KindOf(walkErr) == operation.KindUnknown && errors.Is(walkErr, context.Canceled) {
				walkErr = operation.Wrap(operation.KindCancelled, "fingerprint_walk", resolvedRoot, walkErr)
			}
			return FingerprintResult{}, walkErr
		}
	}

	result.Fingerprint = hex.EncodeToString(aggregate.Sum(nil))
	return result, nil
}

func fingerprintRegularFile(ctx context.Context, path string, buffer []byte) (digest [sha256.Size]byte, size int64, err error) {
	session, err := OpenReadSession(path)
	if err != nil {
		return digest, 0, err
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	if err := session.Start(0); err != nil {
		return digest, 0, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return digest, 0, operation.Wrap(operation.KindCancelled, "fingerprint_file", path, err)
		}
		_, readErr := session.Read(buffer)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return digest, 0, readErr
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	snapshot, err := session.Finish()
	if err != nil {
		return digest, 0, err
	}
	digest, ok := snapshot.ContentDigest()
	if !ok {
		return digest, 0, operation.New(operation.KindFilesystem, "fingerprint digest is unavailable")
	}
	return digest, snapshot.Size, nil
}

func fingerprintResultsEquivalent(first, second FingerprintResult) bool {
	return first.Algorithm == second.Algorithm &&
		first.Mode == second.Mode &&
		first.Fingerprint == second.Fingerprint &&
		first.RootCount == second.RootCount &&
		first.FileCount == second.FileCount &&
		first.DirectoryCount == second.DirectoryCount &&
		first.TotalBytes == second.TotalBytes
}

func writeFingerprintHeader(target hash.Hash, rootCount int, respectGitignore bool) {
	_, _ = target.Write([]byte("mcp-file-tools:fingerprint:content-v1\x00"))
	writeFingerprintUint64(target, uint64(rootCount))
	if respectGitignore {
		_, _ = target.Write([]byte{1})
	} else {
		_, _ = target.Write([]byte{0})
	}
}

func writeFingerprintRecord(target hash.Hash, rootIndex int, relativePath, entryType string, size int64, digest [sha256.Size]byte) {
	writeFingerprintUint64(target, uint64(rootIndex))
	writeFingerprintString(target, relativePath)
	writeFingerprintString(target, entryType)
	writeFingerprintUint64(target, uint64(size))
	if entryType == "file" {
		_, _ = target.Write(digest[:])
	}
}

func writeFingerprintString(target hash.Hash, value string) {
	writeFingerprintUint64(target, uint64(len(value)))
	_, _ = target.Write([]byte(value))
}

func writeFingerprintUint64(target hash.Hash, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = target.Write(encoded[:])
}

func canonicalFingerprintPath(path string) string {
	canonical := filepath.ToSlash(filepath.Clean(path))
	if canonical == "" {
		canonical = "."
	}
	return norm.NFC.String(canonical)
}

func fingerprintPathKey(path string) string {
	clean := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}
	return clean
}
