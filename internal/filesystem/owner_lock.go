package filesystem

import "errors"

// FileLockMode selects shared reader or exclusive writer ownership.
type FileLockMode uint8

const (
	LockShared FileLockMode = iota + 1
	LockExclusive
)

// ErrFileLockBusy reports that another process currently holds an incompatible lock.
var ErrFileLockBusy = errors.New("filesystem lock is busy")

// RestrictOwnerOnlyPath changes an existing regular file or directory to owner-only access.
func RestrictOwnerOnlyPath(path string, directory bool) error {
	return restrictOwnerOnlyPath(path, directory)
}

// ValidateOwnerOnlyPath verifies exact owner-only access for an existing path.
func ValidateOwnerOnlyPath(path string, directory bool) error {
	return validateOwnerOnlyPath(path, directory)
}

// ReadOwnerOnlyFileBounded reads one existing owner-only regular single-link file
// without following links and rejects data above maxBytes.
func ReadOwnerOnlyFileBounded(path string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, errors.New("owner-only read limit must be positive")
	}
	return readOwnerOnlyFileBounded(path, maxBytes)
}

// RestrictOwnerOnlyExecutable changes an existing regular file to owner-only executable access.
func RestrictOwnerOnlyExecutable(path string) error {
	return restrictOwnerOnlyExecutable(path)
}

// ValidateOwnerOnlyExecutable verifies owner-only executable access for an existing regular file.
func ValidateOwnerOnlyExecutable(path string) error {
	return validateOwnerOnlyExecutable(path)
}
