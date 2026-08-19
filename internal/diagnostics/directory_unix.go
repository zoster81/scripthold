//go:build linux || darwin

package diagnostics

import "os"

func validateLogDirectoryPlatform(os.FileInfo) error {
	return nil
}
