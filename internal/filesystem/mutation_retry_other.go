//go:build !windows

package filesystem

func isRetryableAtomicReplaceError(error) bool {
	return false
}

func tryAlternativeAtomicReplace(string, string, error) (bool, error) {
	return false, nil
}

func reportAtomicReplaceRetry(string, string, atomicReplaceRetryReport) {}
