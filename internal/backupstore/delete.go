package backupstore

import (
	"context"
	"errors"
	"os"

	"github.com/zoster81/scripthold/internal/operation"
)

// DeleteResult reports durable progress from one explicitly requested backup
// deletion. A removed manifest is authoritative even when later object/trash
// cleanup reports an error.
type DeleteResult struct {
	BackupID        string
	Pinned          bool
	ManifestRemoved bool
	ObjectRemoved   bool
	BytesReclaimed  int64
	Generation      string
}

// DeleteBackup explicitly removes one selected manifest and, when no live or
// active restore reference remains, its now-unreferenced object. Automatic
// retention never calls this path for pinned manifests.
func (store *Store) DeleteBackup(ctx context.Context, backupID string) (result DeleteResult, err error) {
	if store == nil {
		return DeleteResult{}, operation.New(operation.KindInvalidInput, "backup store is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !validHexIdentifier(backupID) {
		return DeleteResult{}, operation.New(operation.KindInvalidInput, "backup identifier is invalid")
	}
	if err := ctx.Err(); err != nil {
		return DeleteResult{}, operation.Wrap(operation.KindCancelled, "delete_backup", "", err)
	}
	store.transactionMu.Lock()
	defer store.transactionMu.Unlock()
	if store.isClosed() {
		return DeleteResult{}, operation.New(operation.KindConflict, "backup store is closed")
	}
	if err := store.validateIdentityAndLayout(); err != nil {
		return DeleteResult{}, err
	}

	index, activeManifests, activeObjects, err := store.gcAuthoritativeSnapshot(ctx)
	if err != nil {
		return DeleteResult{}, err
	}
	if activeManifests[backupID] > 0 {
		return DeleteResult{}, operation.New(operation.KindConflict, "backup is retained by an active restore")
	}

	var candidate *ManifestSummary
	for i := range index.Manifests {
		if index.Manifests[i].BackupID == backupID {
			item := index.Manifests[i]
			candidate = &item
			break
		}
	}
	if candidate == nil {
		return DeleteResult{}, operation.New(operation.KindNotFound, "backup does not exist")
	}

	result.BackupID = candidate.BackupID
	result.Pinned = candidate.Pinned
	result.Generation = index.Generation
	durableStateChanged := false
	indexRefreshed := false
	defer func() {
		if durableStateChanged && !indexRefreshed {
			refreshErr := store.refreshDerivedIndex(context.Background())
			err = errors.Join(err, refreshErr)
			result.Generation = store.Index().Generation
		}
	}()

	source := manifestPath(store.root, candidate.BackupID)
	info, statErr := os.Lstat(source)
	if statErr != nil {
		return result, operation.New(operation.KindConflict, "backup manifest changed before explicit deletion")
	}
	manifest, readErr := readManifest(source, info, store.descriptor)
	if readErr != nil || manifest.BackupID != candidate.BackupID ||
		manifest.ObjectDigest != candidate.ObjectDigest ||
		manifest.ManifestChecksum != candidate.ManifestChecksum ||
		manifest.Pinned != candidate.Pinned {
		return result, operation.New(operation.KindConflict, "backup manifest changed before explicit deletion")
	}

	manifestTrash := gcManifestTrashPath(store.root, candidate.BackupID)
	moved, moveErr := store.ops.moveGCEntry(source, manifestTrash, info, "explicit backup manifest deletion")
	if moved {
		durableStateChanged = true
		result.ManifestRemoved = true
	}
	if moveErr != nil {
		return result, moveErr
	}

	trash := []gcTrashEntry{{kind: "manifest", id: candidate.BackupID, path: manifestTrash}}
	postIndex, _, postActiveObjects, scanErr := store.gcAuthoritativeSnapshot(ctx)
	if scanErr != nil {
		return result, scanErr
	}
	references := 0
	for _, object := range postIndex.Objects {
		if object.Digest == candidate.ObjectDigest {
			references = object.References
			break
		}
	}
	removedObjects := make(map[string]struct{}, 1)
	if references == 0 && activeObjects[candidate.ObjectDigest] == 0 && postActiveObjects[candidate.ObjectDigest] == 0 {
		objectSource := objectPath(store.root, candidate.ObjectDigest)
		objectInfo, objectStatErr := os.Lstat(objectSource)
		if objectStatErr != nil && !os.IsNotExist(objectStatErr) {
			return result, sanitizedFilesystemError("backup object cannot be inspected for explicit deletion", objectStatErr)
		}
		if objectStatErr == nil {
			if verifyErr := verifyExistingObject(ctx, objectSource, objectInfo, candidate.ObjectDigest, candidate.ObjectBytes); verifyErr != nil {
				return result, verifyErr
			}
			objectTrash := gcObjectTrashPath(store.root, candidate.ObjectDigest)
			objectMoved, objectMoveErr := store.ops.moveGCEntry(objectSource, objectTrash, objectInfo, "explicit backup object deletion")
			if objectMoved {
				durableStateChanged = true
				result.ObjectRemoved = true
				result.BytesReclaimed = candidate.ObjectBytes
				removedObjects[candidate.ObjectDigest] = struct{}{}
				trash = append(trash, gcTrashEntry{kind: "object", id: candidate.ObjectDigest, path: objectTrash, bytes: candidate.ObjectBytes})
			}
			if objectMoveErr != nil {
				return result, objectMoveErr
			}
		}
	}

	var cleanupErr error
	for _, entry := range trash {
		_, removeErr := store.ops.removeGCTrashEntry(entry.path)
		cleanupErr = errors.Join(cleanupErr, removeErr)
	}
	finalIndex := indexWithoutObjects(postIndex, removedObjects)
	if refreshErr := store.publishDerivedIndex(finalIndex); refreshErr != nil {
		return result, errors.Join(cleanupErr, refreshErr)
	}
	indexRefreshed = true
	result.Generation = store.Index().Generation
	return result, cleanupErr
}
