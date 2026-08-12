//go:build !windows && !linux && !darwin

package filesystem

import "github.com/zoster81/scripthold/internal/operation"

func captureObjectIdentity(path string) (key, volumeKey string, isDir bool, err error) {
	return "", "", false, operation.New(operation.KindUnsupported, "stable filesystem object identity is unavailable on this platform")
}
