package filesystempackage

import (
	"io/fs"

	"github.com/zoster81/scripthold/internal/filesystem"
)

// commitOperations groups the concrete filesystem mutation boundaries used by
// package commit orchestration. NewEngine installs the real implementations.
type commitOperations struct {
	createDirectory       func(string, fs.FileMode) error
	captureObjectIdentity func(string) (filesystem.ObjectIdentity, error)
	publishFile           func(*filesystem.StagedFile, string) error
	publishDirectory      func(*filesystem.StagedDirectory, string) error
	movePrepared          func(string, string, filesystem.ObjectIdentity, filesystem.ObjectIdentity, filesystem.ObjectIdentity) error
}

func defaultCommitOperations() commitOperations {
	return commitOperations{
		createDirectory:       filesystem.CreateDirectoryExactNoReplace,
		captureObjectIdentity: filesystem.CaptureObjectIdentity,
		publishFile: func(staged *filesystem.StagedFile, destination string) error {
			return staged.PublishNoReplace(destination)
		},
		publishDirectory: func(staged *filesystem.StagedDirectory, destination string) error {
			return staged.PublishNoReplace(destination)
		},
		movePrepared: filesystem.MovePreparedNativeNoReplace,
	}
}
