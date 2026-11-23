//go:build windows

package security

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestValidatePath_RejectsJunctionEscape(t *testing.T) {
	allowedDir := t.TempDir()
	outsideDir := t.TempDir()
	junction := filepath.Join(allowedDir, "escape")
	createJunctionForTest(t, outsideDir, junction)

	_, err := ValidatePath(junction, []string{allowedDir})
	if !errors.Is(err, ErrSymlinkDenied) {
		t.Fatalf("error = %v, want ErrSymlinkDenied", err)
	}

	for _, path := range []string{
		filepath.Join(junction, "new.txt"),
		filepath.Join(junction, "missing", "nested", "new.txt"),
	} {
		_, err := ValidatePath(path, []string{allowedDir})
		if !errors.Is(err, ErrParentDirDenied) {
			t.Fatalf("ValidatePath(%q) error = %v, want ErrParentDirDenied", path, err)
		}
	}

	resolved, safe := ResolvePathSafe(junction, ResolveAllowedDirs([]string{allowedDir}))
	if safe {
		t.Fatalf("junction escape resolved as safe: %s", resolved)
	}
	// The extended spelling must not turn an escaping reparse point into an
	// authorized path, including a missing descendant.
	extendedJunction := `\\?\` + junction
	if _, err := ValidatePath(extendedJunction, []string{allowedDir}); !errors.Is(err, ErrSymlinkDenied) {
		t.Errorf("extended escaping junction error = %v, want ErrSymlinkDenied", err)
	}
	if _, err := ValidatePath(filepath.Join(extendedJunction, "missing", "new.txt"), []string{allowedDir}); !errors.Is(err, ErrParentDirDenied) {
		t.Errorf("extended escaping junction child error = %v, want ErrParentDirDenied", err)
	}
	if _, safe := ResolvePathSafe(extendedJunction, ResolveAllowedDirs([]string{allowedDir})); safe {
		t.Error("extended escaping junction resolved as safe")
	}
}

func TestValidatePath_AllowsMissingPathThroughSafeJunction(t *testing.T) {
	allowedDir := t.TempDir()
	target := filepath.Join(allowedDir, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(allowedDir, "safe-link")
	createJunctionForTest(t, target, junction)
	requested := filepath.Join(junction, "missing", "nested", "new.txt")

	validated, err := ValidatePath(requested, []string{allowedDir})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(validated) != filepath.Clean(requested) {
		t.Fatalf("validated path = %q, want %q", validated, requested)
	}
}

func TestResolveExistingPath_ResolvesJunctionTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(root, "junction")
	createJunctionForTest(t, target, junction)

	resolved, err := resolveExistingPath(junction)
	if err != nil {
		t.Fatal(err)
	}
	resolvedInfo, err := os.Stat(resolved)
	if err != nil {
		t.Fatal(err)
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(resolvedInfo, targetInfo) {
		t.Fatalf("resolved path = %q, want same directory as %q", resolved, target)
	}
}

func TestNormalizeWindowsFinalPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: `\\?\C:\work\file.txt`, want: `C:\work\file.txt`},
		{input: `\\?\UNC\server\share\file.txt`, want: `\\server\share\file.txt`},
		{input: `\??\C:\work\file.txt`, want: `C:\work\file.txt`},
		{input: `\??\UNC\server\share\file.txt`, want: `\\server\share\file.txt`},
	}
	for _, tt := range tests {
		if got := normalizeWindowsFinalPath(tt.input); got != tt.want {
			t.Errorf("normalizeWindowsFinalPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestWindowsShortNameAliasesPreserveAllowedBoundaries exercises real NTFS 8.3
// aliases where the volume exposes them. An unavailable alias is not evidence
// that the long-path and short-path forms have equivalent security behavior.
func TestWindowsShortNameAliasesPreserveAllowedBoundaries(t *testing.T) {
	parent := t.TempDir()
	allowed := filepath.Join(parent, "LongAllowedDirectoryName")
	outside := filepath.Join(parent, "LongAllowedDirectoryName-sibling")
	for _, dir := range []string{allowed, outside} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	allowedFile := filepath.Join(allowed, "allowed.txt")
	outsideFile := filepath.Join(outside, "outside.txt")
	for _, path := range []string{allowedFile, outsideFile} {
		if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	shortAllowed := windowsShortPathForTest(t, allowed)
	if strings.EqualFold(filepath.Base(shortAllowed), filepath.Base(allowed)) {
		t.Skip("8.3 alias generation is not enabled for the test directory")
	}
	shortOutside := windowsShortPathForTest(t, outside)
	shortAllowedFile := filepath.Join(shortAllowed, filepath.Base(allowedFile))
	shortOutsideFile := filepath.Join(shortOutside, filepath.Base(outsideFile))
	extendedShortAllowedFile := `\\?\` + shortAllowedFile
	extendedShortOutsideFile := `\\?\` + shortOutsideFile
	shortMissing := filepath.Join(shortAllowed, "missing", "new.txt")

	for _, configuredRoot := range []string{allowed, shortAllowed} {
		t.Run(filepath.Base(configuredRoot), func(t *testing.T) {
			set, err := NormalizeAllowedDirectorySet([]string{configuredRoot})
			if err != nil {
				t.Fatal(err)
			}
			if !IsPathWithinAllowedDirectories(shortAllowedFile, set.Requested) {
				t.Fatal("8.3 child spelling was rejected at the requested-root boundary")
			}
			if IsPathWithinAllowedDirectories(shortOutsideFile, set.Requested) {
				t.Fatal("8.3 sibling spelling escaped the requested-root boundary")
			}

			for _, candidate := range []string{allowedFile, shortAllowedFile, extendedShortAllowedFile, shortMissing} {
				if _, err := ValidatePathWithAllowedDirectories(candidate, set.Requested, set.Resolved); err != nil {
					t.Errorf("valid candidate %q: %v", candidate, err)
				}
			}
			for _, candidate := range []string{outsideFile, shortOutsideFile, extendedShortOutsideFile} {
				if _, err := ValidatePathWithAllowedDirectories(candidate, set.Requested, set.Resolved); !errors.Is(err, ErrPathDenied) {
					t.Errorf("sibling %q: error = %v, want ErrPathDenied", candidate, err)
				}
			}
			matcher := NewAllowedDirectoryMatcher(set.Resolved)
			if _, ok := matcher.ResolvePathSafe(shortAllowedFile); !ok {
				t.Error("resolved 8.3 child spelling was rejected by matcher")
			}
			if _, ok := matcher.ResolvePathSafe(shortOutsideFile); ok {
				t.Error("resolved 8.3 sibling spelling escaped matcher")
			}
		})
	}
}

// TestWindowsExtendedPathPrefixesPreserveAllowedBoundaries verifies that an
// existing extended-length Win32 spelling has the same access boundary as its
// ordinary absolute spelling, including a not-yet-created child.
func TestWindowsExtendedPathPrefixesPreserveAllowedBoundaries(t *testing.T) {
	parent := t.TempDir()
	allowed := filepath.Join(parent, "LongAllowedDirectoryName")
	outside := filepath.Join(parent, "LongAllowedDirectoryName-sibling")
	for _, dir := range []string{allowed, outside} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	insideFile := filepath.Join(allowed, "inside.txt")
	outsideFile := filepath.Join(outside, "outside.txt")
	for _, path := range []string{insideFile, outsideFile} {
		if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	extended := func(path string) string { return `\\?\` + path }
	for _, root := range []struct {
		name string
		path string
	}{
		{name: "ordinary-root", path: allowed},
		{name: "extended-root", path: extended(allowed)},
	} {
		t.Run(root.name, func(t *testing.T) {
			set, err := NormalizeAllowedDirectorySet([]string{root.path})
			if err != nil {
				t.Fatalf("root normalization failed: %v", err)
			}
			for _, candidate := range []string{
				insideFile,
				extended(insideFile),
				filepath.Join(extended(allowed), "missing", "new.txt"),
			} {
				if _, err := ValidatePathWithAllowedDirectories(candidate, set.Requested, set.Resolved); err != nil {
					t.Errorf("valid candidate %q: %v", candidate, err)
				}
			}
			for _, candidate := range []string{
				outsideFile,
				extended(outsideFile),
				extended(allowed) + `\..\LongAllowedDirectoryName-sibling\outside.txt`,
			} {
				if _, err := ValidatePathWithAllowedDirectories(candidate, set.Requested, set.Resolved); !errors.Is(err, ErrPathDenied) {
					t.Errorf("escaping candidate %q: error = %v, want ErrPathDenied", candidate, err)
				}
			}
			matcher := NewAllowedDirectoryMatcher(set.Resolved)
			if _, ok := matcher.ResolvePathSafe(extended(insideFile)); !ok {
				t.Error("authorized extended-length file was rejected by resolved matcher")
			}
			if _, ok := matcher.ResolvePathSafe(extended(outsideFile)); ok {
				t.Error("extended-length outside file escaped resolved matcher")
			}
		})
	}
}

func windowsShortPathForTest(t *testing.T, path string) string {
	t.Helper()
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]uint16, 512)
	length, err := windows.GetShortPathName(ptr, &buffer[0], uint32(len(buffer)))
	if err != nil {
		t.Skipf("Windows short-path lookup is unavailable: %v", err)
	}
	if length >= uint32(len(buffer)) {
		if length > maxLongPathUTF16Units {
			t.Fatalf("short-path buffer requirement exceeds limit: %d", length)
		}
		buffer = make([]uint16, length+1)
		length, err = windows.GetShortPathName(ptr, &buffer[0], uint32(len(buffer)))
		if err != nil {
			t.Fatalf("short-path retry failed: %v", err)
		}
	}
	if length == 0 || length >= uint32(len(buffer)) {
		t.Fatalf("invalid Windows short-path length: %d", length)
	}
	return windows.UTF16ToString(buffer[:length])
}

func createJunctionForTest(t *testing.T, target, link string) {
	t.Helper()
	output, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		t.Skipf("directory junction creation is not supported in this environment: %v (%s)", err, output)
	}
}
