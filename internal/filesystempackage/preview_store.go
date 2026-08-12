package filesystempackage

import (
	"container/list"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

const filesystemPackagePreviewTokenBytes = 32

type filesystemPackagePreview struct {
	id            string
	createdAt     time.Time
	expiresAt     time.Time
	prepared      PreparedPackage
	retainedBytes int64
	element       *list.Element
}

type filesystemPackagePreviewStore struct {
	mu         sync.Mutex
	entries    map[string]*filesystemPackagePreview
	order      *list.List
	maxEntries int
	maxBytes   int64
	ttl        time.Duration
	totalBytes int64
	now        func() time.Time
	random     io.Reader
}

func newFilesystemPackagePreviewStore(maxEntries int, maxBytes int64, ttl time.Duration) *filesystemPackagePreviewStore {
	return &filesystemPackagePreviewStore{
		entries: make(map[string]*filesystemPackagePreview), order: list.New(),
		maxEntries: maxEntries, maxBytes: maxBytes, ttl: ttl, now: time.Now, random: rand.Reader,
	}
}

func (store *filesystemPackagePreviewStore) put(prepared PreparedPackage) (*filesystemPackagePreview, error) {
	if store == nil || store.maxEntries <= 0 || store.maxBytes <= 0 || store.ttl <= 0 {
		return nil, operation.New(operation.KindInvalidInput, "filesystem package preview cache is not configured")
	}
	retained, err := preparedPackageRetainedBytes(prepared)
	if err != nil {
		return nil, err
	}
	if retained > store.maxBytes {
		return nil, operation.New(operation.KindLimit, fmt.Sprintf("prepared filesystem package retains %d bytes; preview cache limit is %d", retained, store.maxBytes))
	}
	prepared = clonePreparedPackage(prepared)
	store.mu.Lock()
	defer store.mu.Unlock()
	now := store.now().UTC()
	store.purgeExpiredLocked(now)
	for len(store.entries) >= store.maxEntries || store.totalBytes > store.maxBytes-retained {
		oldest := store.order.Front()
		if oldest == nil {
			break
		}
		store.removeLocked(oldest.Value.(string))
	}
	id, err := store.newIDLocked()
	if err != nil {
		return nil, err
	}
	preview := &filesystemPackagePreview{
		id: id, createdAt: now, expiresAt: now.Add(store.ttl), prepared: prepared, retainedBytes: retained,
	}
	preview.element = store.order.PushBack(id)
	store.entries[id] = preview
	store.totalBytes += retained
	return &filesystemPackagePreview{
		id: preview.id, createdAt: preview.createdAt, expiresAt: preview.expiresAt,
		prepared: clonePreparedPackage(preview.prepared), retainedBytes: preview.retainedBytes,
	}, nil
}

func (store *filesystemPackagePreviewStore) claim(id string) (*filesystemPackagePreview, error) {
	if !validFilesystemPackagePreviewID(id) {
		return nil, operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")
	}
	if store == nil {
		return nil, operation.New(operation.KindConflict, "filesystem package preview is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpiredLocked(store.now().UTC())
	preview, ok := store.entries[id]
	if !ok {
		return nil, operation.New(operation.KindConflict, "filesystem package preview is unavailable, expired, evicted, or already consumed")
	}
	store.removeLocked(id)
	preview.element = nil
	return preview, nil
}

func (store *filesystemPackagePreviewStore) purgeExpiredLocked(now time.Time) {
	for element := store.order.Front(); element != nil; {
		next := element.Next()
		id := element.Value.(string)
		preview := store.entries[id]
		if preview != nil && !preview.expiresAt.After(now) {
			store.removeLocked(id)
		}
		element = next
	}
}

func (store *filesystemPackagePreviewStore) removeLocked(id string) {
	preview, ok := store.entries[id]
	if !ok {
		return
	}
	delete(store.entries, id)
	if preview.element != nil {
		store.order.Remove(preview.element)
		preview.element = nil
	}
	store.totalBytes -= preview.retainedBytes
	if store.totalBytes < 0 {
		store.totalBytes = 0
	}
}

func (store *filesystemPackagePreviewStore) newIDLocked() (string, error) {
	var raw [filesystemPackagePreviewTokenBytes]byte
	for range 4 {
		if _, err := io.ReadFull(store.random, raw[:]); err != nil {
			return "", operation.Wrap(operation.KindFilesystem, "create_filesystem_package_preview_id", "", err)
		}
		id := hex.EncodeToString(raw[:])
		if _, exists := store.entries[id]; !exists {
			return id, nil
		}
	}
	return "", operation.New(operation.KindConflict, "could not allocate a unique filesystem package preview identifier")
}

func validFilesystemPackagePreviewID(id string) bool {
	if len(id) != filesystemPackagePreviewTokenBytes*2 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func preparedPackageRetainedBytes(prepared PreparedPackage) (int64, error) {
	var total int64
	add := func(value int64) error {
		if value < 0 || value > math.MaxInt64-total {
			return operation.New(operation.KindLimit, "prepared filesystem package retained size exceeds supported range")
		}
		total += value
		return nil
	}
	if err := add(int64(len(prepared.FormatVersion))); err != nil {
		return 0, err
	}
	for _, item := range prepared.Operations {
		for _, value := range []string{
			item.Operation.Type, item.Operation.Path, item.Operation.Source, item.Operation.Destination,
			item.Path.RequestedPath, item.Path.ResolvedPath, item.Path.NearestExistingPath,
			item.Source.RequestedPath, item.Source.ResolvedPath, item.Destination.RequestedPath,
			item.Destination.ResolvedPath, item.Destination.NearestExistingPath,
			item.ImmediateParentPath, item.ExpectedResultFingerprint,
		} {
			if err := add(int64(len(value))); err != nil {
				return 0, err
			}
		}
		if err := add(int64(len(item.Operation.Content))); err != nil {
			return 0, err
		}
		if item.Tree != nil {
			for _, entry := range item.Tree.Entries {
				if err := add(int64(len(entry.Path)+len(entry.RelativePath)) + 256); err != nil {
					return 0, err
				}
			}
		}
		if err := add(512); err != nil {
			return 0, err
		}
	}
	for _, requirement := range prepared.BackupRequirements {
		if err := add(int64(len(requirement.Path)+len(requirement.ExpectedFingerprint)) + 64); err != nil {
			return 0, err
		}
	}
	return total, nil
}

func clonePreparedPackage(prepared PreparedPackage) PreparedPackage {
	clone := prepared
	clone.Operations = make([]PreparedOperation, len(prepared.Operations))
	for index := range prepared.Operations {
		clone.Operations[index] = prepared.Operations[index]
		clone.Operations[index].Operation.Content = append([]byte(nil), prepared.Operations[index].Operation.Content...)
		if prepared.Operations[index].Tree != nil {
			tree := *prepared.Operations[index].Tree
			tree.Entries = append([]filesystem.ExactTreeEntry(nil), prepared.Operations[index].Tree.Entries...)
			clone.Operations[index].Tree = &tree
		}
	}
	clone.BackupRequirements = append([]BackupRequirement(nil), prepared.BackupRequirements...)
	return clone
}
