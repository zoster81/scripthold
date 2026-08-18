package backupstore

import (
	"context"
	"crypto/sha256"
	"os"
)

func overrideAfterCaptureStage(store *Store, callback func() error) func() {
	original := store.ops.stageTarget
	store.ops.stageTarget = func(current *Store, ctx context.Context, target string, expectedSize int64) (string, [sha256.Size]byte, int64, error) {
		path, digest, size, err := original(current, ctx, target, expectedSize)
		if err != nil {
			return path, digest, size, err
		}
		if callbackErr := callback(); callbackErr != nil {
			return path, digest, size, callbackErr
		}
		return path, digest, size, nil
	}
	return func() { store.ops.stageTarget = original }
}

func overrideBeforeManifestCommit(store *Store, callback func() error) func() {
	original := store.ops.commitManifest
	store.ops.commitManifest = func(current *Store, manifest Manifest) (Manifest, bool, error) {
		if err := callback(); err != nil {
			return Manifest{}, false, err
		}
		return original(current, manifest)
	}
	return func() { store.ops.commitManifest = original }
}

func overrideBeforeIndexPersist(store *Store, callback func() error) func() {
	original := store.ops.persistIndex
	store.ops.persistIndex = func(root string, index Index) error {
		if err := callback(); err != nil {
			return err
		}
		return original(root, index)
	}
	return func() { store.ops.persistIndex = original }
}

func overrideAfterGCMove(store *Store, description string, callback func() error) func() {
	original := store.ops.moveGCEntry
	store.ops.moveGCEntry = func(source, destination string, expected os.FileInfo, actualDescription string) (bool, error) {
		moved, err := original(source, destination, expected, actualDescription)
		if err != nil || !moved || actualDescription != description {
			return moved, err
		}
		if callbackErr := callback(); callbackErr != nil {
			return moved, callbackErr
		}
		return moved, nil
	}
	return func() { store.ops.moveGCEntry = original }
}

func overrideGCTrashRemoval(store *Store, callback func(string) error) func() {
	original := store.ops.removeGCTrashEntry
	store.ops.removeGCTrashEntry = func(path string) (bool, error) {
		if err := callback(path); err != nil {
			return false, err
		}
		return original(path)
	}
	return func() { store.ops.removeGCTrashEntry = original }
}
