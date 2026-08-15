//go:build !windows

package filesystem

func isRetryableAtomicReplaceError(error) bool {
	return false
}
