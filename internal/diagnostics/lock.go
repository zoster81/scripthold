package diagnostics

import "os"

type fileLock struct {
	file *os.File
}

func (lock *fileLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	return lock.file.Close()
}
