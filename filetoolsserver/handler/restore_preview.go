package handler

import (
	"container/list"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

const (
	restorePreviewTokenBytes = 32
	restorePreviewMaxEntries = 64
	restorePreviewMaxBytes   = int64(16 * 1024 * 1024)
	restoreDiffInputMaxBytes = int64(1024 * 1024)
)

type preparedRestore struct {
	source            *backupstore.ReadSource
	backupID          string
	requestedPath     string
	resolvedPath      string
	targetExisted     bool
	targetSnapshot    filesystem.FileSnapshot
	targetFingerprint string
	targetIdentity    *filesystem.FileIdentity
	resultFingerprint string
	objectBytes       int64
	restoreMode       uint32
	restoreModTime    time.Time
	diff              string
}

func (prepared *preparedRestore) close() error {
	if prepared == nil {
		return nil
	}
	var err error
	if prepared.targetIdentity != nil {
		err = prepared.targetIdentity.Close()
		prepared.targetIdentity = nil
	}
	if prepared.source != nil {
		err = errors.Join(err, prepared.source.Close())
		prepared.source = nil
	}
	return err
}

func (prepared preparedRestore) retainedBytes() (int64, error) {
	parts := []int{
		len(prepared.backupID), len(prepared.requestedPath), len(prepared.resolvedPath),
		len(prepared.targetFingerprint), len(prepared.resultFingerprint), len(prepared.diff),
	}
	var total int64
	for _, part := range parts {
		if int64(part) > math.MaxInt64-total {
			return 0, operation.New(operation.KindLimit, "restore preview size exceeds supported range")
		}
		total += int64(part)
	}
	return total, nil
}

type restorePreview struct {
	id            string
	createdAt     time.Time
	expiresAt     time.Time
	prepared      preparedRestore
	retainedBytes int64
	element       *list.Element
}

type restorePreviewStore struct {
	mu         sync.Mutex
	entries    map[string]*restorePreview
	order      *list.List
	maxEntries int
	maxBytes   int64
	ttl        time.Duration
	totalBytes int64
	now        func() time.Time
	random     io.Reader
}

func newRestorePreviewStore(maxEntries int, maxBytes int64, ttl time.Duration) *restorePreviewStore {
	return &restorePreviewStore{
		entries:    make(map[string]*restorePreview),
		order:      list.New(),
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		ttl:        ttl,
		now:        time.Now,
		random:     rand.Reader,
	}
}

func (store *restorePreviewStore) put(prepared preparedRestore) (*restorePreview, error) {
	if store == nil || store.maxEntries <= 0 || store.maxBytes <= 0 || store.ttl <= 0 {
		return nil, operation.New(operation.KindInvalidInput, "restore preview cache is not configured")
	}
	retainedBytes, err := prepared.retainedBytes()
	if err != nil {
		return nil, err
	}
	if retainedBytes > store.maxBytes {
		return nil, operation.New(operation.KindLimit, fmt.Sprintf("restore preview retains %d bytes; cache limit is %d", retainedBytes, store.maxBytes))
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	now := store.now().UTC()
	store.purgeExpiredLocked(now)
	for len(store.entries) >= store.maxEntries || store.totalBytes > store.maxBytes-retainedBytes {
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
	preview := &restorePreview{
		id:            id,
		createdAt:     now,
		expiresAt:     now.Add(store.ttl),
		prepared:      prepared,
		retainedBytes: retainedBytes,
	}
	preview.element = store.order.PushBack(id)
	store.entries[id] = preview
	store.totalBytes += retainedBytes
	return preview, nil
}

func (store *restorePreviewStore) claim(id string) (*restorePreview, error) {
	if !validRestorePreviewID(id) {
		return nil, operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")
	}
	if store == nil {
		return nil, operation.New(operation.KindConflict, "restore preview is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpiredLocked(store.now().UTC())
	preview, ok := store.entries[id]
	if !ok {
		return nil, operation.New(operation.KindConflict, "restore preview is unavailable, expired, or already consumed")
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
	return preview, nil
}

func (store *restorePreviewStore) discard(id string) {
	if store == nil {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.removeLocked(id)
}

func (store *restorePreviewStore) len() int {
	if store == nil {
		return 0
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpiredLocked(store.now().UTC())
	return len(store.entries)
}

func (store *restorePreviewStore) purgeExpiredLocked(now time.Time) {
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

func (store *restorePreviewStore) removeLocked(id string) {
	preview, ok := store.entries[id]
	if !ok {
		return
	}
	delete(store.entries, id)
	if preview.element != nil {
		store.order.Remove(preview.element)
	}
	store.totalBytes -= preview.retainedBytes
	if store.totalBytes < 0 {
		store.totalBytes = 0
	}
	_ = preview.prepared.close()
}

func (store *restorePreviewStore) newIDLocked() (string, error) {
	var raw [restorePreviewTokenBytes]byte
	for range 4 {
		if _, err := io.ReadFull(store.random, raw[:]); err != nil {
			return "", operation.Wrap(operation.KindFilesystem, "create_restore_preview_id", "", err)
		}
		id := hex.EncodeToString(raw[:])
		if _, exists := store.entries[id]; !exists {
			return id, nil
		}
	}
	return "", operation.New(operation.KindConflict, "could not allocate a unique restore preview identifier")
}

func validRestorePreviewID(id string) bool {
	if len(id) != restorePreviewTokenBytes*2 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
