package filesystem

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zoster81/scripthold/internal/operation"
)

type nativeMoveFunc func(string, string) error

// MovePreparedNativeNoReplace performs only a native same-volume no-replace
// namespace move. It verifies the prepared source and both parent identities
// before mutation and verifies object identity transfer afterwards.
func MovePreparedNativeNoReplace(source, destination string, expectedSource, expectedSourceParent, expectedDestinationParent ObjectIdentity) error {
	return movePreparedNativeNoReplace(source, destination, expectedSource, expectedSourceParent, expectedDestinationParent, nativeMovePathNoReplace)
}

func movePreparedNativeNoReplace(source, destination string, expectedSource, expectedSourceParent, expectedDestinationParent ObjectIdentity, move nativeMoveFunc) (err error) {
	defer func() {
		err = operation.WrapFilesystem("move_prepared_native_no_replace", destination, err)
	}()
	if move == nil {
		return operation.New(operation.KindUnsupported, "native no-replace move is unavailable")
	}
	if err := verifyObjectIdentity(source, expectedSource, "move source"); err != nil {
		return err
	}
	sourceParent := filepath.Dir(source)
	destinationParent := filepath.Dir(destination)
	if err := verifyObjectIdentity(sourceParent, expectedSourceParent, "move source parent"); err != nil {
		return err
	}
	if err := verifyObjectIdentity(destinationParent, expectedDestinationParent, "move destination parent"); err != nil {
		return err
	}
	sameVolume, err := expectedSource.SameVolume(expectedDestinationParent)
	if err != nil {
		return err
	}
	if !sameVolume {
		return operation.New(operation.KindUnsupported, "cross-filesystem move is unsupported")
	}
	if _, err := os.Lstat(destination); err == nil {
		return ErrDestinationExists
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := move(source, destination); err != nil {
		if isDestinationExistsError(err) || errors.Is(err, fs.ErrExist) {
			return ErrDestinationExists
		}
		return err
	}
	for _, directory := range uniqueDirectories(sourceParent, destinationParent) {
		if err := defaultMutationOps.syncDirectory(directory); err != nil {
			return fmt.Errorf("move committed but directory %s could not be synced: %w", directory, err)
		}
	}
	if _, err := os.Lstat(source); err == nil {
		return operation.New(operation.KindConflict, "move source still exists after native move")
	} else if !os.IsNotExist(err) {
		return err
	}
	moved, err := CaptureObjectIdentity(destination)
	if err != nil {
		return err
	}
	if !expectedSource.Equal(moved) {
		return operation.New(operation.KindConflict, "moved destination does not retain prepared source identity")
	}
	return nil
}

func verifyObjectIdentity(path string, expected ObjectIdentity, label string) error {
	matches, err := expected.Matches(path)
	if err != nil {
		return err
	}
	if !matches {
		return operation.New(operation.KindConflict, fmt.Sprintf("%s identity changed: %s", label, path))
	}
	return nil
}
