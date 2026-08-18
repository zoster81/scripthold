package backupstore

import (
	"context"
	"crypto/sha256"
	"os"
)

// storeOperations defines the durable I/O boundaries used by capture and GC.
// Store instances created by Open use the real filesystem-backed operations.
type storeOperations struct {
	stageTarget        func(*Store, context.Context, string, int64) (string, [sha256.Size]byte, int64, error)
	commitManifest     func(*Store, Manifest) (Manifest, bool, error)
	persistIndex       func(string, Index) error
	moveGCEntry        func(string, string, os.FileInfo, string) (bool, error)
	removeGCTrashEntry func(string) (bool, error)
}

func defaultStoreOperations() storeOperations {
	return storeOperations{
		stageTarget: func(store *Store, ctx context.Context, target string, expectedSize int64) (string, [sha256.Size]byte, int64, error) {
			return store.stageTarget(ctx, target, expectedSize)
		},
		commitManifest: func(store *Store, manifest Manifest) (Manifest, bool, error) {
			return store.commitManifest(manifest)
		},
		persistIndex:       persistIndex,
		moveGCEntry:        moveGCEntry,
		removeGCTrashEntry: removeGCTrashEntry,
	}
}
