package filesystem

import "github.com/zoster81/scripthold/internal/operation"

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
