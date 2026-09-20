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
