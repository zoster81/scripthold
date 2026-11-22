package filesystem

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zoster81/scripthold/internal/security"
)

// WalkAction controls traversal after an entry is visited.
type WalkAction uint8

const (
	WalkContinue WalkAction = iota
	WalkSkipDir
	WalkStop
)

// Entry describes one filesystem entry relative to the traversal root.
type Entry struct {
	Path         string
	ResolvedPath string
	RelativePath string
	Name         string
	Depth        int
	DirEntry     fs.DirEntry
	IsLink       bool
}

// WalkOptions defines shared traversal policy. Exclude prunes matching
// directories and skips matching files. MaxDepth is unlimited when <= 0.
type WalkOptions struct {
	ResolvedAllowedDirs []string
	MaxDepth            int
	Exclude             func(Entry) bool
	OnUnsafe            func(path string, depth int) error
	OnError             func(path string, depth int, err error) error
	RespectGitignore    bool
}

// Visitor processes a safe entry and selects the next traversal action.
type Visitor func(Entry) (WalkAction, error)

var errWalkStopped = errors.New("filesystem walk stopped")

// Walk traverses root in deterministic lexical order without following
// directory symlinks or reparse points. Every entry is resolved and checked
// against the allowed directories before it is exposed to the visitor.
func Walk(ctx context.Context, root string, options WalkOptions, visitor Visitor) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if visitor == nil {
		return errors.New("filesystem visitor is required")
	}
	if len(options.ResolvedAllowedDirs) == 0 {
		return errors.New("resolved allowed directories are required")
	}

	matcher := security.NewAllowedDirectoryMatcher(options.ResolvedAllowedDirs)
	resolvedRoot, safe := matcher.ResolvePathSafe(root)
	if !safe {
		if _, err := os.Stat(root); err != nil {
			return handleWalkError(options, root, 0, err)
		}
		return handleWalkError(options, root, 0, fmt.Errorf("root path resolves outside allowed directories: %s", root))
	}

	err := walkDirectory(ctx, resolvedRoot, "", 0, options, matcher, nil, visitor)
	if errors.Is(err, errWalkStopped) {
		return nil
	}
	return err
}

func walkDirectory(ctx context.Context, dirPath, relativeDir string, depth int, options WalkOptions, matcher security.AllowedDirectoryMatcher, scopes []ignoreScope, visitor Visitor) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	resolvedDir, safe := matcher.ResolvePathSafe(dirPath)
	if !safe {
		if _, err := os.Stat(dirPath); err != nil {
			return handleWalkError(options, dirPath, depth, err)
		}
		return handleWalkError(options, dirPath, depth, fmt.Errorf("path is no longer safe: %s", dirPath))
	}

	currentScopes := scopes
	if options.RespectGitignore {
		scope, scopeErr := loadIgnoreScope(resolvedDir, relativeDir, options.ResolvedAllowedDirs)
		if scopeErr != nil {
			if errors.Is(scopeErr, ErrInvalidGitignore) {
				return fmt.Errorf("%s: %w", filepath.Join(resolvedDir, ".gitignore"), scopeErr)
			}
			if handled := handleWalkError(options, filepath.Join(resolvedDir, ".gitignore"), depth, scopeErr); handled != nil {
				return handled
			}
		} else if len(scope.rules) > 0 {
			currentScopes = append(append([]ignoreScope(nil), scopes...), scope)
		}
	}

	entries, err := os.ReadDir(resolvedDir)
	if err != nil {
		return handleWalkError(options, resolvedDir, depth, err)
	}

	for _, dirEntry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}

		childPath := filepath.Join(resolvedDir, dirEntry.Name())
		resolvedChild, safe := matcher.ResolvePathSafe(childPath)
		if !safe {
			if options.OnUnsafe != nil {
				if unsafeErr := options.OnUnsafe(childPath, depth+1); unsafeErr != nil {
					return unsafeErr
				}
			}
			continue
		}

		relativePath := dirEntry.Name()
		if relativeDir != "" {
			relativePath = filepath.Join(relativeDir, dirEntry.Name())
		}
		entry := Entry{
			Path:         childPath,
			ResolvedPath: resolvedChild,
			RelativePath: relativePath,
			Name:         dirEntry.Name(),
			Depth:        depth + 1,
			DirEntry:     dirEntry,
			IsLink:       dirEntry.Type()&os.ModeSymlink != 0 || !sameTraversalPath(childPath, resolvedChild),
		}
		if options.RespectGitignore {
			if dirEntry.IsDir() && strings.EqualFold(dirEntry.Name(), ".git") {
				continue
			}
			if ignoredByScopes(currentScopes, relativePath, dirEntry.IsDir()) {
				continue
			}
		}
		if options.Exclude != nil && options.Exclude(entry) {
			continue
		}

		action, err := visitor(entry)
		if err != nil {
			return err
		}
		if action == WalkStop {
			return errWalkStopped
		}
		if entry.IsLink || !dirEntry.IsDir() || action == WalkSkipDir {
			continue
		}
		if options.MaxDepth > 0 && entry.Depth >= options.MaxDepth {
			continue
		}
		if err := walkDirectory(ctx, resolvedChild, relativePath, entry.Depth, options, matcher, currentScopes, visitor); err != nil {
			return err
		}
	}
	return nil
}

func sameTraversalPath(first, second string) bool {
	first = filepath.Clean(first)
	second = filepath.Clean(second)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(first, second)
	}
	return first == second
}

func handleWalkError(options WalkOptions, path string, depth int, err error) error {
	if options.OnError == nil {
		if depth == 0 {
			return err
		}
		return nil
	}
	return options.OnError(path, depth, err)
}
