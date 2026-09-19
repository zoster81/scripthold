package handler

import (
	"container/list"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/zoster81/scripthold/internal/operation"
)

const patchPackagePreviewTokenBytes = 32

type preparedPatchPackage struct {
	formatVersion              string
	label                      string
	fingerprintAlgorithm       string
	fingerprintMode            string
	backupPolicy               string
	aggregateMode              string
	aggregateBeforeFingerprint string
	aggregateAfterFingerprint  string
	targets                    []preparedEditPlanTarget
}

type patchPackagePreview struct {
	id            string
	createdAt     time.Time
	expiresAt     time.Time
	prepared      preparedPatchPackage
	retainedBytes int64
	element       *list.Element
}

type patchPackagePreviewStore struct {
	mu         sync.Mutex
	entries    map[string]*patchPackagePreview
	order      *list.List
	maxEntries int
	maxBytes   int64
	ttl        time.Duration
	totalBytes int64
	now        func() time.Time
	random     io.Reader
}

func newPatchPackagePreviewStore(maxEntries int, maxBytes int64, ttl time.Duration) *patchPackagePreviewStore {
	return &patchPackagePreviewStore{
		entries:    make(map[string]*patchPackagePreview),
		order:      list.New(),
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		ttl:        ttl,
		now:        time.Now,
		random:     rand.Reader,
	}
}

func (store *patchPackagePreviewStore) put(prepared preparedPatchPackage) (*patchPackagePreview, error) {
	if store == nil || store.maxEntries <= 0 || store.maxBytes <= 0 || store.ttl <= 0 {
		return nil, operation.New(operation.KindInvalidInput, "patch package preview cache is not configured")
	}
	retainedBytes, err := prepared.retainedBytes()
	if err != nil {
		return nil, err
	}
	if retainedBytes > store.maxBytes {
		return nil, operation.New(operation.KindLimit, fmt.Sprintf("prepared patch package retains %d bytes; cache limit is %d", retainedBytes, store.maxBytes))
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
	preview := &patchPackagePreview{
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

func (store *patchPackagePreviewStore) claim(id string) (*patchPackagePreview, error) {
	if !validPatchPackagePreviewID(id) {
		return nil, operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")
	}
	if store == nil {
		return nil, operation.New(operation.KindConflict, "patch package preview is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpiredLocked(store.now().UTC())
	preview, ok := store.entries[id]
	if !ok {
		return nil, operation.New(operation.KindConflict, "patch package preview is unavailable, expired, or already consumed")
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

func (store *patchPackagePreviewStore) discard(id string) {
	if store == nil {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.removeLocked(id)
}

func (store *patchPackagePreviewStore) newIDLocked() (string, error) {
	var raw [patchPackagePreviewTokenBytes]byte
	for range 4 {
		if _, err := io.ReadFull(store.random, raw[:]); err != nil {
			return "", operation.Wrap(operation.KindFilesystem, "create_patch_package_preview_id", "", err)
		}
		id := hex.EncodeToString(raw[:])
		if _, exists := store.entries[id]; !exists {
			return id, nil
		}
	}
	return "", operation.New(operation.KindConflict, "could not allocate a unique patch package preview identifier")
}

func (store *patchPackagePreviewStore) purgeExpiredLocked(now time.Time) {
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

func (store *patchPackagePreviewStore) removeLocked(id string) {
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
	preview.prepared.close()
}

func validPatchPackagePreviewID(id string) bool {
	if len(id) != patchPackagePreviewTokenBytes*2 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func (prepared *preparedPatchPackage) close() {
	if prepared == nil {
		return
	}
	for index := range prepared.targets {
		identity := prepared.targets[index].prepared.identityFile
		if identity != nil {
			_ = identity.Close()
			prepared.targets[index].prepared.identityFile = nil
		}
	}
}

func (prepared preparedPatchPackage) retainedBytes() (int64, error) {
	parts := []int{
		len(prepared.formatVersion), len(prepared.label), len(prepared.fingerprintAlgorithm),
		len(prepared.fingerprintMode), len(prepared.backupPolicy), len(prepared.aggregateMode),
		len(prepared.aggregateBeforeFingerprint), len(prepared.aggregateAfterFingerprint),
	}
	var total int64
	for _, part := range parts {
		if int64(part) > math.MaxInt64-total {
			return 0, operation.New(operation.KindLimit, "prepared patch package size exceeds supported range")
		}
		total += int64(part)
	}
	for _, target := range prepared.targets {
		retained, err := target.prepared.retainedBytes()
		if err != nil {
			return 0, err
		}
		for _, part := range []int{
			len(target.canonicalManifestPath), len(target.expectedFingerprint), len(target.expectedResultFingerprint),
		} {
			if int64(part) > math.MaxInt64-total {
				return 0, operation.New(operation.KindLimit, "prepared patch package size exceeds supported range")
			}
			total += int64(part)
		}
		if retained > math.MaxInt64-total {
			return 0, operation.New(operation.KindLimit, "prepared patch package size exceeds supported range")
		}
		total += retained
	}
	return total, nil
}
