package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
	"golang.org/x/text/unicode/norm"
)

// ExactTreeOptions bounds complete recursive mutation enumeration. Hitting any
// bound is an error; this API never returns a truncated mutation scope.
type ExactTreeOptions struct {
	ResolvedAllowedDirs []string
	MaxEntries          int
	MaxDepth            int
	MaxFileBytes        int64
	MaxAggregateBytes   int64
}

// ExactTreeEntry is one real regular file or directory in lexical path order.
// The root is always entry zero with RelativePath ".".
type ExactTreeEntry struct {
	RelativePath string
	Path         string
	Depth        int
	IsDirectory  bool
	Mode         os.FileMode
	ModTime      time.Time
	Size         int64
	Snapshot     FileSnapshot
	Identity     ObjectIdentity
}

// ExactTree is a complete, bounded recursive source or deletion scope.
type ExactTree struct {
	Root               string
	Entries            []ExactTreeEntry
	FileCount          int
	DirectoryCount     int
	TotalBytes         int64
	ContentFingerprint string
	StateFingerprint   string
}

type identityCaptureFunc func(string) (ObjectIdentity, error)

// EnumerateExactTree returns a complete deterministic scope or an error.
func EnumerateExactTree(ctx context.Context, root string, options ExactTreeOptions) (ExactTree, error) {
	return enumerateExactTree(ctx, root, options, CaptureObjectIdentity)
}

func enumerateExactTree(ctx context.Context, root string, options ExactTreeOptions, captureIdentity identityCaptureFunc) (ExactTree, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateExactTreeOptions(options); err != nil {
		return ExactTree{}, err
	}
	if err := ctx.Err(); err != nil {
		return ExactTree{}, operation.Wrap(operation.KindCancelled, "enumerate_exact_tree", root, err)
	}
	resolvedRoot, safe := security.ResolvePathSafe(root, options.ResolvedAllowedDirs)
	if !safe {
		return ExactTree{}, operation.New(operation.KindSymlinkEscape, fmt.Sprintf("recursive root is outside allowed directories: %s", root))
	}
	rootIdentity, err := captureIdentity(resolvedRoot)
	if err != nil {
		return ExactTree{}, err
	}
	if !rootIdentity.IsDirectory() {
		return ExactTree{}, operation.New(operation.KindInvalidInput, fmt.Sprintf("recursive root is not a real directory: %s", root))
	}
	if rootIdentity.VolumeKey() == "" {
		return ExactTree{}, operation.New(operation.KindUnsupported, "recursive mutation requires stable volume identity")
	}
	rootInfo, err := os.Stat(resolvedRoot)
	if err != nil {
		return ExactTree{}, operation.WrapFilesystem("stat_exact_tree_root", resolvedRoot, err)
	}

	tree := ExactTree{Root: resolvedRoot, Entries: make([]ExactTreeEntry, 0, min(options.MaxEntries, 128))}
	addDirectory := func(relative, path string, depth int, info os.FileInfo, identity ObjectIdentity) error {
		if len(tree.Entries) >= options.MaxEntries {
			return operation.New(operation.KindLimit, fmt.Sprintf("recursive entry count exceeds limit %d", options.MaxEntries))
		}
		if depth > options.MaxDepth {
			return operation.New(operation.KindLimit, fmt.Sprintf("recursive depth exceeds limit %d", options.MaxDepth))
		}
		tree.Entries = append(tree.Entries, ExactTreeEntry{
			RelativePath: canonicalTreePath(relative), Path: path, Depth: depth,
			IsDirectory: true, Mode: info.Mode(), ModTime: info.ModTime(), Identity: identity,
		})
		tree.DirectoryCount++
		return nil
	}
	addFile := func(relative, path string, depth int, info os.FileInfo, identity ObjectIdentity) error {
		if len(tree.Entries) >= options.MaxEntries {
			return operation.New(operation.KindLimit, fmt.Sprintf("recursive entry count exceeds limit %d", options.MaxEntries))
		}
		if depth > options.MaxDepth {
			return operation.New(operation.KindLimit, fmt.Sprintf("recursive depth exceeds limit %d", options.MaxDepth))
		}
		if info.Size() < 0 || info.Size() > math.MaxInt64-tree.TotalBytes || tree.TotalBytes+info.Size() > options.MaxAggregateBytes {
			return operation.New(operation.KindLimit, fmt.Sprintf("recursive aggregate bytes exceed limit %d", options.MaxAggregateBytes))
		}
		snapshot, err := CaptureRegularFileSnapshotBounded(ctx, path, options.MaxFileBytes)
		if err != nil {
			return err
		}
		tree.Entries = append(tree.Entries, ExactTreeEntry{
			RelativePath: canonicalTreePath(relative), Path: path, Depth: depth,
			Mode: info.Mode(), ModTime: info.ModTime(), Size: info.Size(), Snapshot: snapshot, Identity: identity,
		})
		tree.FileCount++
		tree.TotalBytes += info.Size()
		return nil
	}

	if err := addDirectory(".", resolvedRoot, 0, rootInfo, rootIdentity); err != nil {
		return ExactTree{}, err
	}
	walkErr := Walk(ctx, resolvedRoot, WalkOptions{
		ResolvedAllowedDirs: options.ResolvedAllowedDirs,
		RespectGitignore:    false,
		OnUnsafe: func(path string, _ int) error {
			return operation.New(operation.KindSymlinkEscape, fmt.Sprintf("link-like or escaping recursive entry is not allowed: %s", path))
		},
		OnError: func(path string, _ int, err error) error {
			if errors.Is(err, context.Canceled) {
				return operation.Wrap(operation.KindCancelled, "enumerate_exact_tree", path, err)
			}
			if errors.Is(err, os.ErrNotExist) {
				return operation.Wrap(operation.KindConflict, "enumerate_exact_tree", path, err)
			}
			return operation.WrapFilesystem("enumerate_exact_tree", path, err)
		},
	}, func(entry Entry) (WalkAction, error) {
		if err := ctx.Err(); err != nil {
			return WalkStop, operation.Wrap(operation.KindCancelled, "enumerate_exact_tree", entry.Path, err)
		}
		if entry.Depth > options.MaxDepth {
			return WalkStop, operation.New(operation.KindLimit, fmt.Sprintf("recursive depth exceeds limit %d", options.MaxDepth))
		}
		if entry.IsLink {
			return WalkStop, operation.New(operation.KindSymlinkEscape, fmt.Sprintf("link-like recursive entry is not allowed: %s", entry.Path))
		}
		identity, err := captureIdentity(entry.ResolvedPath)
		if err != nil {
			return WalkStop, err
		}
		if identity.VolumeKey() == "" || identity.VolumeKey() != rootIdentity.VolumeKey() {
			return WalkStop, operation.New(operation.KindUnsupported, fmt.Sprintf("recursive scope crosses a filesystem volume boundary at %s", entry.Path))
		}
		info, err := os.Stat(entry.ResolvedPath)
		if err != nil {
			if os.IsNotExist(err) {
				return WalkStop, operation.Wrap(operation.KindConflict, "stat_exact_tree_entry", entry.Path, err)
			}
			return WalkStop, operation.WrapFilesystem("stat_exact_tree_entry", entry.Path, err)
		}
		if identity.IsDirectory() {
			if !info.IsDir() {
				return WalkStop, operation.New(operation.KindConflict, fmt.Sprintf("recursive entry kind changed: %s", entry.Path))
			}
			return WalkContinue, addDirectory(entry.RelativePath, entry.ResolvedPath, entry.Depth, info, identity)
		}
		if !info.Mode().IsRegular() {
			return WalkStop, operation.New(operation.KindUnsupported, fmt.Sprintf("special recursive entry is unsupported: %s", entry.Path))
		}
		return WalkContinue, addFile(entry.RelativePath, entry.ResolvedPath, entry.Depth, info, identity)
	})
	if walkErr != nil {
		return ExactTree{}, walkErr
	}
	content, state, err := exactTreeFingerprints(tree.Entries)
	if err != nil {
		return ExactTree{}, err
	}
	tree.ContentFingerprint = content
	tree.StateFingerprint = state
	return tree, nil
}

// VerifyExactTree requires the recursive scope and all retained identities and
// snapshots to remain exactly equal to the prepared tree.
func VerifyExactTree(ctx context.Context, expected ExactTree, options ExactTreeOptions) error {
	current, err := EnumerateExactTree(ctx, expected.Root, options)
	if err != nil {
		if operation.KindOf(err) == operation.KindCancelled {
			return err
		}
		return operation.Wrap(operation.KindConflict, "verify_exact_tree", expected.Root, err)
	}
	if expected.StateFingerprint != current.StateFingerprint || len(expected.Entries) != len(current.Entries) {
		return operation.New(operation.KindConflict, fmt.Sprintf("recursive scope changed: %s", expected.Root))
	}
	return nil
}

// ExactTreeContentEqual compares only the structure, metadata promised by copy,
// and regular-file contents. Filesystem object identity is intentionally ignored.
func ExactTreeContentEqual(first, second ExactTree) bool {
	return first.ContentFingerprint != "" && first.ContentFingerprint == second.ContentFingerprint &&
		first.FileCount == second.FileCount && first.DirectoryCount == second.DirectoryCount && first.TotalBytes == second.TotalBytes
}

func validateExactTreeOptions(options ExactTreeOptions) error {
	if len(options.ResolvedAllowedDirs) == 0 || options.MaxEntries <= 0 || options.MaxDepth <= 0 || options.MaxFileBytes <= 0 || options.MaxAggregateBytes <= 0 {
		return operation.New(operation.KindInvalidInput, "exact recursive tree limits and allowed directories must be positive and non-empty")
	}
	return nil
}

func exactTreeFingerprints(entries []ExactTreeEntry) (string, string, error) {
	contentHash := sha256.New()
	stateHash := sha256.New()
	_, _ = contentHash.Write([]byte("scripthold:exact-tree:content-v1\x00"))
	_, _ = stateHash.Write([]byte("scripthold:exact-tree:state-v1\x00"))
	for _, entry := range entries {
		writeTreeString(contentHash, entry.RelativePath)
		writeTreeString(stateHash, entry.RelativePath)
		kind := "file"
		if entry.IsDirectory {
			kind = "directory"
		}
		writeTreeString(contentHash, kind)
		writeTreeString(stateHash, kind)
		writeTreeUint64(contentHash, uint64(entry.Mode.Perm()))
		writeTreeUint64(stateHash, uint64(entry.Mode))
		writeTreeUint64(stateHash, uint64(entry.ModTime.UnixNano()))
		writeTreeString(stateHash, entry.Identity.key)
		writeTreeString(stateHash, entry.Identity.volumeKey)
		if entry.IsDirectory {
			continue
		}
		digest, ok := entry.Snapshot.ContentDigest()
		if !ok {
			return "", "", operation.New(operation.KindFilesystem, "exact tree file snapshot has no content digest")
		}
		writeTreeUint64(contentHash, uint64(entry.Size))
		writeTreeUint64(stateHash, uint64(entry.Size))
		writeTreeUint64(contentHash, uint64(entry.ModTime.UnixNano()))
		_, _ = contentHash.Write(digest[:])
		_, _ = stateHash.Write(digest[:])
	}
	return hex.EncodeToString(contentHash.Sum(nil)), hex.EncodeToString(stateHash.Sum(nil)), nil
}

func writeTreeString(target hash.Hash, value string) {
	writeTreeUint64(target, uint64(len(value)))
	_, _ = target.Write([]byte(value))
}

func writeTreeUint64(target hash.Hash, value uint64) {
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], value)
	_, _ = target.Write(raw[:])
}

func canonicalTreePath(path string) string {
	canonical := filepath.ToSlash(filepath.Clean(path))
	if canonical == "" {
		canonical = "."
	}
	canonical = norm.NFC.String(canonical)
	if runtime.GOOS == "windows" {
		// Preserve display spelling but reject names that collapse under the host's
		// case-insensitive namespace by comparing keys during planner analysis.
		return canonical
	}
	return strings.TrimPrefix(canonical, "./")
}
