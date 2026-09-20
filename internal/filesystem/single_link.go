package filesystem

import (
	"os"

	"github.com/zoster81/scripthold/internal/operation"
)

// CaptureSingleLinkFileIdentity captures one stable regular-file identity while
// also requiring that the same opened object has exactly one hard link.
func CaptureSingleLinkFileIdentity(path string) (identity ObjectIdentity, err error) {
	defer func() {
		err = operation.WrapFilesystem("capture_single_link_file_identity", path, err)
	}()
	key, volumeKey, err := captureSingleLinkFileIdentity(path)
	if err != nil {
		return ObjectIdentity{}, err
	}
	if key == "" {
		return ObjectIdentity{}, operation.New(operation.KindUnsupported, "stable single-link file identity is unavailable on this platform")
	}
	return ObjectIdentity{key: key, volumeKey: volumeKey}, nil
}

// OpenVerifiedSingleLinkFile opens one regular file without following links and
// requires the opened object to match expected identity with exactly one hard link.
func OpenVerifiedSingleLinkFile(path string, expected ObjectIdentity) (file *os.File, err error) {
	defer func() {
		err = operation.WrapFilesystem("open_verified_single_link_file", path, err)
	}()
	if expected.key == "" || expected.isDir {
		return nil, operation.New(operation.KindInvalidInput, "expected regular-file identity is unavailable")
	}
	return openVerifiedSingleLinkFile(path, expected)
}
