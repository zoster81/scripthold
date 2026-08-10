package security

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// AllowedDirectorySet keeps both the requested spelling of each configured
// root and its fully resolved destination. Validation requires containment in
// both views so legitimate aliases remain usable without allowing an external
// symlink or junction to become an alternate entry point.
type AllowedDirectorySet struct {
	Requested []string
	Resolved  []string
}

// PathsEqual reports whether two absolute paths are equal under platform path
// normalization and case semantics.
func PathsEqual(first, second string) bool {
	if first == "" || second == "" || strings.Contains(first, "\x00") || strings.Contains(second, "\x00") {
		return false
	}
	first = filepath.Clean(first)
	second = filepath.Clean(second)
	if !filepath.IsAbs(first) || !filepath.IsAbs(second) {
		return false
	}
	return pathsEqual(normalizePath(first), normalizePath(second))
}

// PathsOverlap reports whether two absolute paths are equal or one contains the
// other using platform path comparison and real component boundaries.
func PathsOverlap(first, second string) bool {
	if first == "" || second == "" || strings.Contains(first, "\x00") || strings.Contains(second, "\x00") ||
		!filepath.IsAbs(filepath.Clean(first)) || !filepath.IsAbs(filepath.Clean(second)) {
		return false
	}
	first = normalizePath(first)
	second = normalizePath(second)
	return pathsEqual(first, second) || pathContains(first, second) || pathContains(second, first)
}

func pathContains(parent, child string) bool {
	separator := string(filepath.Separator)
	prefix := parent
	if !strings.HasSuffix(prefix, separator) {
		prefix += separator
	}
	return pathHasPrefix(child, prefix)
}

func IsPathWithinAllowedDirectories(absolutePath string, allowedDirs []string) bool {
	if absolutePath == "" || len(allowedDirs) == 0 {
		return false
	}

	if strings.Contains(absolutePath, "\x00") {
		return false
	}

	normalized := filepath.Clean(absolutePath)
	if !filepath.IsAbs(normalized) {
		return false
	}

	normalized = normalizePath(normalized)

	for _, allowedDir := range allowedDirs {
		cleanAllowed := normalizePath(filepath.Clean(allowedDir))

		if pathsEqual(normalized, cleanAllowed) {
			return true
		}

		separator := string(filepath.Separator)
		allowedPrefix := cleanAllowed
		if !strings.HasSuffix(allowedPrefix, separator) {
			allowedPrefix += separator
		}
		if pathHasPrefix(normalized, allowedPrefix) {
			return true
		}
	}

	return false
}

// ValidatePath resolves a path and ensures it's within allowed directories.
func ValidatePath(requestedPath string, allowedDirs []string) (string, error) {
	return ValidatePathWithAllowedDirectories(requestedPath, allowedDirs, ResolveAllowedDirs(allowedDirs))
}

// ValidatePathWithAllowedDirectories validates the requested spelling against
// requestedAllowedDirs and the resolved destination against resolvedAllowedDirs.
func ValidatePathWithAllowedDirectories(requestedPath string, requestedAllowedDirs, resolvedAllowedDirs []string) (validated string, err error) {
	defer func() {
		err = operation.WrapFilesystem("validate_path", requestedPath, err)
	}()

	if len(requestedAllowedDirs) == 0 {
		return "", ErrNoAllowedDirs
	}

	expanded := ExpandHome(requestedPath)

	var absolute string
	if filepath.IsAbs(expanded) {
		absolute = filepath.Clean(expanded)
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
		absolute = filepath.Clean(filepath.Join(cwd, expanded))
	}

	normalized := normalizePath(absolute)

	if !IsPathWithinAllowedDirectories(normalized, requestedAllowedDirs) {
		return "", fmt.Errorf("%w: %s", ErrPathDenied, absolute)
	}

	resolvedPath, exists, err := resolvePathAllowMissing(absolute)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", ErrParentNotExists, filepath.Dir(absolute))
		}
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	normalizedResolved := normalizePath(resolvedPath)
	if !IsPathWithinAllowedDirectories(normalizedResolved, resolvedAllowedDirs) {
		if exists {
			return "", fmt.Errorf("%w: %s", ErrSymlinkDenied, resolvedPath)
		}
		return "", fmt.Errorf("%w: %s", ErrParentDirDenied, filepath.Dir(resolvedPath))
	}
	if !exists {
		return absolute, nil
	}
	return resolvedPath, nil
}

// resolvePathAllowMissing resolves the nearest existing ancestor and projects
// any missing suffix onto that resolved path. Existing but unresolvable links
// fail closed instead of being treated as missing paths.
func resolvePathAllowMissing(path string) (resolved string, exists bool, err error) {
	return resolvePathAllowMissingWith(path, resolveExistingPath, os.Lstat)
}

func resolvePathAllowMissingWith(
	path string,
	resolve func(string) (string, error),
	lstat func(string) (os.FileInfo, error),
) (resolved string, exists bool, err error) {
	current := filepath.Clean(path)
	missingParts := make([]string, 0, 4)
	projectResolved := func(resolvedCurrent string) (string, bool, error) {
		resolvedCurrent = filepath.Clean(resolvedCurrent)
		for i := len(missingParts) - 1; i >= 0; i-- {
			resolvedCurrent = filepath.Join(resolvedCurrent, missingParts[i])
		}
		return filepath.Clean(resolvedCurrent), len(missingParts) == 0, nil
	}

	for {
		resolvedCurrent, resolveErr := resolve(current)
		if resolveErr == nil {
			return projectResolved(resolvedCurrent)
		}
		if !os.IsNotExist(resolveErr) {
			return "", false, resolveErr
		}

		if _, lstatErr := lstat(current); lstatErr == nil {
			// The path can legitimately appear between the failed resolve above and
			// this metadata check. Re-resolve it once before classifying an existing
			// entry as an unresolvable link/reparse point.
			resolvedCurrent, retryErr := resolve(current)
			if retryErr == nil {
				return projectResolved(resolvedCurrent)
			}
			if !os.IsNotExist(retryErr) {
				return "", false, retryErr
			}
			if _, retryLstatErr := lstat(current); retryLstatErr == nil {
				return "", false, fmt.Errorf("existing path cannot be resolved: %s: %w", current, retryErr)
			} else if !os.IsNotExist(retryLstatErr) {
				return "", false, retryLstatErr
			}
		} else if !os.IsNotExist(lstatErr) {
			return "", false, lstatErr
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", false, resolveErr
		}
		missingParts = append(missingParts, filepath.Base(current))
		current = parent
	}
}

func normalizePath(p string) string {
	p = strings.Trim(p, "\"' \t\n")
	p = filepath.Clean(p)
	p = normalizePlatformPath(p)
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" && len(p) >= 2 && p[1] == ':' {
		p = strings.ToUpper(p[:1]) + p[1:]
	}

	return p
}

func pathsEqual(first, second string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(first, second)
	}
	return first == second
}

func pathHasPrefix(path, prefix string) bool {
	if runtime.GOOS == "windows" {
		return len(path) >= len(prefix) && strings.EqualFold(path[:len(prefix)], prefix)
	}
	return strings.HasPrefix(path, prefix)
}

// ResolveAllowedDirs resolves allowed directories once. Missing directories
// are projected from their nearest existing ancestor; unresolvable existing
// links are omitted so callers fail closed.
func ResolveAllowedDirs(allowedDirs []string) []string {
	resolved := make([]string, 0, len(allowedDirs))
	for _, dir := range allowedDirs {
		resolvedDir, _, err := resolvePathAllowMissing(dir)
		if err != nil {
			continue
		}
		resolved = append(resolved, normalizePath(resolvedDir))
	}
	return resolved
}

// ResolvePathSafe resolves a path and returns the resolved path only when it remains
// within the pre-resolved allowed directories.
func ResolvePathSafe(path string, resolvedAllowedDirs []string) (string, bool) {
	if path == "" || len(resolvedAllowedDirs) == 0 {
		return "", false
	}

	resolved, err := resolveExistingPath(path)
	if err != nil {
		return "", false
	}
	resolved = filepath.Clean(resolved)
	if !IsPathWithinAllowedDirectories(resolved, resolvedAllowedDirs) {
		return "", false
	}
	return resolved, true
}

// IsPathSafeResolved checks if a path (after resolving symlinks) is within pre-resolved allowed dirs.
func IsPathSafeResolved(path string, resolvedAllowedDirs []string) bool {
	_, safe := ResolvePathSafe(path, resolvedAllowedDirs)
	return safe
}

func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func NormalizeAllowedDirs(dirs []string) ([]string, error) {
	set, err := NormalizeAllowedDirectorySet(dirs)
	if err != nil {
		return nil, err
	}
	return set.Resolved, nil
}

// NormalizeAllowedDirectorySet validates configured roots while retaining both
// their normalized requested spelling and fully resolved destination.
func NormalizeAllowedDirectorySet(dirs []string) (set AllowedDirectorySet, err error) {
	defer func() {
		err = operation.WrapFilesystem("normalize_allowed_directories", "", err)
	}()

	set.Requested = make([]string, 0, len(dirs))
	set.Resolved = make([]string, 0, len(dirs))
	for _, dir := range dirs {
		expanded := ExpandHome(dir)

		absolute, err := filepath.Abs(expanded)
		if err != nil {
			return AllowedDirectorySet{}, fmt.Errorf("invalid directory %s: %w", dir, err)
		}

		resolved, exists, err := resolvePathAllowMissing(absolute)
		if err != nil {
			return AllowedDirectorySet{}, fmt.Errorf("cannot resolve directory %s: %w", dir, err)
		}
		if exists {
			info, err := os.Stat(resolved)
			if err != nil {
				return AllowedDirectorySet{}, fmt.Errorf("cannot stat directory %s: %w", resolved, err)
			}
			if !info.IsDir() {
				return AllowedDirectorySet{}, fmt.Errorf("%w: %s", ErrNotDirectory, resolved)
			}
		}

		set.Requested = append(set.Requested, normalizePath(absolute))
		set.Resolved = append(set.Resolved, normalizePath(resolved))
	}
	return set, nil
}
