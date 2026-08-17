//go:build !windows

package filesystem

func isRetryableAtomicReplaceError(error) bool {
	return false
}

func reportAtomicReplaceRetry(string, string, atomicReplaceRetryReport) {}
