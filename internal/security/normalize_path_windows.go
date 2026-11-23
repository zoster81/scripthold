//go:build windows

package security

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

const maxLongPathUTF16Units = 1 << 16

// normalizePlatformPath expands Windows 8.3 components without following
// symlinks or junctions. Missing suffixes are projected from the nearest
// existing ancestor so lexical containment remains stable for new paths.
func normalizePlatformPath(path string) string {
	expanded, err := expandWindowsLongPathAllowMissing(path)
	if err != nil {
		return path
	}
	// GetLongPathName retains the Win32 extended-length prefix. Compare only
	// canonical absolute drive/UNC spellings against already-normalized roots;
	// preserve other device namespaces instead of rewriting them as paths.
	canonical := normalizeWindowsFinalPath(expanded)
	if filepath.IsAbs(canonical) && filepath.VolumeName(canonical) != "" {
		return canonical
	}
	return expanded
}

func expandWindowsLongPathAllowMissing(path string) (string, error) {
	current := filepath.Clean(path)
	missingParts := make([]string, 0, 4)

	for {
		expanded, err := windowsLongPath(current)
		if err == nil {
			for i := len(missingParts) - 1; i >= 0; i-- {
				expanded = filepath.Join(expanded, missingParts[i])
			}
			return filepath.Clean(expanded), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missingParts = append(missingParts, filepath.Base(current))
		current = parent
	}
}

func windowsLongPath(path string) (string, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}

	bufferSize := uint32(260)
	for bufferSize <= maxLongPathUTF16Units {
		buffer := make([]uint16, bufferSize)
		length, err := windows.GetLongPathName(pathPtr, &buffer[0], bufferSize)
		if err != nil {
			return "", err
		}
		if length < bufferSize {
			return filepath.Clean(windows.UTF16ToString(buffer[:length])), nil
		}
		bufferSize = length + 1
	}
	return "", fmt.Errorf("long path exceeds %d UTF-16 code units", maxLongPathUTF16Units)
}
