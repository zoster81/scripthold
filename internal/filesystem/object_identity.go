package filesystem

import (
	"fmt"
	"os"

	"github.com/zoster81/scripthold/internal/operation"
)

// ObjectIdentity is a stable filesystem object identifier plus its containing
// volume identity. It is intentionally opaque to callers.
type ObjectIdentity struct {
	key       string
	volumeKey string
	isDir     bool
}

// CaptureObjectIdentity records a stable identity for one real regular file or
// directory. Link-like entries are rejected by platform implementations.
func CaptureObjectIdentity(path string) (identity ObjectIdentity, err error) {
	defer func() {
		err = operation.WrapFilesystem("capture_object_identity", path, err)
	}()
	key, volumeKey, isDir, err := captureObjectIdentity(path)
	if err != nil {
		return ObjectIdentity{}, err
	}
	if key == "" {
		return ObjectIdentity{}, operation.New(operation.KindUnsupported, "stable filesystem object identity is unavailable on this platform")
	}
	return ObjectIdentity{key: key, volumeKey: volumeKey, isDir: isDir}, nil
}

// Matches reports whether path still names the same filesystem object and kind.
func (identity ObjectIdentity) Matches(path string) (bool, error) {
	current, err := CaptureObjectIdentity(path)
	if err != nil {
		if os.IsNotExist(err) || operation.KindOf(err) == operation.KindNotFound {
			return false, nil
		}
		return false, err
	}
	return identity.key == current.key && identity.isDir == current.isDir, nil
}

// Equal reports whether two captured identities name the same object.
func (identity ObjectIdentity) Equal(other ObjectIdentity) bool {
	return identity.key != "" && identity.key == other.key && identity.isDir == other.isDir
}

// IsDirectory reports the captured object kind.
func (identity ObjectIdentity) IsDirectory() bool { return identity.isDir }

// StableKey returns the opaque stable object identifier for internal conflict analysis.
func (identity ObjectIdentity) StableKey() string { return identity.key }

// VolumeKey returns the opaque stable volume identifier.
func (identity ObjectIdentity) VolumeKey() string { return identity.volumeKey }

// SameVolume reports whether two identities are on the same known volume.
func (identity ObjectIdentity) SameVolume(other ObjectIdentity) (bool, error) {
	if identity.volumeKey == "" || other.volumeKey == "" {
		return false, operation.New(operation.KindUnsupported, "stable filesystem volume identity is unavailable")
	}
	return identity.volumeKey == other.volumeKey, nil
}

func validateIdentityKind(path string, info os.FileInfo) (bool, error) {
	if info.Mode()&os.ModeSymlink != 0 {
		return false, operation.New(operation.KindSymlinkEscape, fmt.Sprintf("link-like filesystem object is not allowed: %s", path))
	}
	if info.IsDir() {
		return true, nil
	}
	if !info.Mode().IsRegular() {
		return false, operation.New(operation.KindUnsupported, fmt.Sprintf("special filesystem object is unsupported: %s", path))
	}
	return false, nil
}
